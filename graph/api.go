package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"codeberg.org/cuducos/minha-receita/db"
	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/klauspost/compress/gzhttp"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"tangled.org/cuducos.me/go-cnpj"
)

const (
	cacheControl = "public, max-age=86400"
	cacheTTL     = 5 * time.Minute

	DefaultCacheSize = 64 // MB
)

type Server struct {
	kv    *badger.DB
	cache *ristretto.Cache[string, []string]
}

func NewServer(pth string, cacheSize int) (*Server, error) {
	slog.Info("Starting graph server warm-up...")

	slog.Info("Opening key/value storage...", "path", pth)
	opt := badger.DefaultOptions(pth).
		WithLoggingLevel(badger.WARNING).
		WithReadOnly(true)
	kv, err := badger.Open(opt)
	if err != nil {
		return nil, fmt.Errorf("could not open badger: %w", err)
	}

	var cache *ristretto.Cache[string, []string]
	if cacheSize > 0 {
		c := int64(cacheSize) << 20
		cache, err = ristretto.NewCache(&ristretto.Config[string, []string]{
			MaxCost:     c,
			NumCounters: c / 64 * 10,
			BufferItems: 64,
		})
		if err != nil {
			return nil, fmt.Errorf("could not create adjacency cache: %w", err)
		}
	}

	slog.Info("Warm-up complete. Standalone graph server is ready!")
	return &Server{kv, cache}, nil
}

func (s *Server) Close() {
	if s.cache != nil {
		s.cache.Close()
	}
	if s.kv != nil {
		if err := s.kv.Close(); err != nil {
			slog.Error("could not close badger", "error", err)
		}
	}
}

func (s *Server) getRelations(id string) ([]db.Relationship, error) {
	var out []db.Relationship

	err := s.kv.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		var ps []string
		var companies []string

		p1 := make([]byte, 6+len(id))
		copy(p1[0:4], "rel:")
		copy(p1[4:4+len(id)], id)
		copy(p1[4+len(id):], "->")
		for it.Seek(p1); it.ValidForPrefix(p1); it.Next() {
			k := string(it.Item().Key())
			parts := strings.Split(k, "->")
			if len(parts) == 2 {
				ps = append(ps, parts[1])
			}
		}

		p2 := make([]byte, 6+len(id))
		copy(p2[0:4], "rel:")
		copy(p2[4:4+len(id)], id)
		copy(p2[4+len(id):], "<-")
		for it.Seek(p2); it.ValidForPrefix(p2); it.Next() {
			k := string(it.Item().Key())
			parts := strings.Split(k, "<-")
			if len(parts) == 2 {
				companies = append(companies, parts[1])
			}
		}

		if len(ps) == 0 && len(companies) == 0 {
			return nil
		}

		k := append([]byte("meta:"), id...)
		item, err := txn.Get(k)
		if err != nil {
			return fmt.Errorf("failed to lookup metadata for entity %s: %w", id, err)
		}
		var m db.Relationship
		err = item.Value(func(val []byte) error {
			return m.Decode(val)
		})
		if err != nil {
			return fmt.Errorf("failed to decode metadata for entity %s: %w", id, err)
		}

		n := len(ps) + len(companies)
		res := make([]db.Relationship, n)

		for i, nid := range ps {
			k := append([]byte("meta:"), nid...)
			item, err := txn.Get(k)
			if err != nil {
				return fmt.Errorf("failed to lookup metadata for partner %s: %w", nid, err)
			}

			err = item.Value(func(val []byte) error {
				var pm db.Relationship
				if err := pm.Decode(val); err != nil {
					return fmt.Errorf("failed to decode metadata for partner %s: %w", nid, err)
				}

				res[i] = db.Relationship{
					CompanyID:   id,
					CompanyName: m.CompanyName,
					PartnerID:   nid,
					PartnerName: pm.PartnerName,
					PartnerCPF:  pm.PartnerCPF,
					PartnerType: pm.PartnerType,
				}
				return nil
			})
			if err != nil {
				return err
			}
		}

		nxt := len(ps)
		for i, nid := range companies {
			k := append([]byte("meta:"), nid...)
			item, err := txn.Get(k)
			if err != nil {
				return fmt.Errorf("failed to lookup metadata for company %s: %w", nid, err)
			}

			err = item.Value(func(val []byte) error {
				var cm db.Relationship
				if err := cm.Decode(val); err != nil {
					return fmt.Errorf("failed to decode metadata for company %s: %w", nid, err)
				}

				res[nxt+i] = db.Relationship{
					CompanyID:   nid,
					CompanyName: cm.CompanyName,
					PartnerID:   id,
					PartnerName: m.PartnerName,
					PartnerCPF:  m.PartnerCPF,
					PartnerType: m.PartnerType,
				}
				return nil
			})
			if err != nil {
				return err
			}
		}

		out = res
		return nil
	})

	if err != nil {
		return nil, err
	}
	return out, nil
}

var errNoConnection = errors.New("no connection found")

func (s *Server) findShortestPath(ctx context.Context, src, dst string) ([]db.Relationship, error) {
	if src == dst {
		return []db.Relationship{}, nil
	}

	type node struct {
		id     string
		parent *node
	}

	var path []db.Relationship

	err := s.kv.View(func(txn *badger.Txn) error {
		srcs := map[string]*node{src: {id: src}}
		dsts := map[string]*node{dst: {id: dst}}

		qsrc := []*node{srcs[src]}
		qdst := []*node{dsts[dst]}

		reconstruct := func(n1, n2 *node) ([]db.Relationship, error) {
			var left []*node
			curr := n1
			for curr.parent != nil {
				left = append(left, curr)
				curr = curr.parent
			}

			var right []*node
			curr = n2
			for curr.parent != nil {
				right = append(right, curr)
				curr = curr.parent
			}

			uniq := make(map[string]bool)
			uniq[src] = true
			uniq[dst] = true
			for _, n := range left {
				uniq[n.id] = true
			}
			for _, n := range right {
				uniq[n.id] = true
			}

			ns := make(map[string]db.Relationship)

			for id := range uniq {
				k := append([]byte("meta:"), id...)
				i, err := txn.Get(k)
				if err != nil {
					return nil, fmt.Errorf("failed to lookup name for path node %s: %w", id, err)
				}
				err = i.Value(func(val []byte) error {
					var meta db.Relationship
					if err := meta.Decode(val); err != nil {
						return err
					}
					ns[id] = meta
					return nil
				})
				if err != nil {
					return nil, err
				}
			}

			build := func(pid, nid string) (db.Relationship, error) {
				k := make([]byte, 6+len(pid)+len(nid))
				copy(k[0:4], "rel:")
				copy(k[4:4+len(pid)], pid)
				copy(k[4+len(pid):6+len(pid)], "->")
				copy(k[6+len(pid):], nid)
				_, err := txn.Get(k)
				ok := err == nil

				p := ns[pid]
				n := ns[nid]

				if ok {
					return db.Relationship{
						CompanyID:   pid,
						CompanyName: p.CompanyName,
						PartnerID:   nid,
						PartnerName: n.PartnerName,
						PartnerCPF:  n.PartnerCPF,
						PartnerType: n.PartnerType,
					}, nil
				}
				return db.Relationship{
					CompanyID:   nid,
					CompanyName: n.CompanyName,
					PartnerID:   pid,
					PartnerName: p.PartnerName,
					PartnerCPF:  p.PartnerCPF,
					PartnerType: p.PartnerType,
				}, nil
			}

			var pth []db.Relationship
			for i := len(left) - 1; i >= 0; i-- {
				n := left[i]
				pid := n.parent.id
				rel, err := build(pid, n.id)
				if err != nil {
					return nil, err
				}
				pth = append(pth, rel)
			}

			for i := len(right) - 1; i >= 0; i-- {
				n := right[i]
				pid := n.parent.id
				rel, err := build(pid, n.id)
				if err != nil {
					return nil, err
				}
				pth = append(pth, rel)
			}

			return pth, nil
		}

		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		adj := func(id string) ([]string, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if s.cache != nil {
				if v, ok := s.cache.Get(id); ok {
					cacheHits.Inc()
					return v, nil
				}
				cacheMisses.Inc()
			}

			var res []string

			k := make([]byte, 4+len(id)+2)
			copy(k[0:4], "rel:")
			copy(k[4:4+len(id)], id)
			copy(k[4+len(id):], "->")
			for it.Seek(k); it.ValidForPrefix(k); it.Next() {
				res = append(res, string(it.Item().Key()[len(k):]))
			}

			copy(k[4+len(id):], "<-")
			for it.Seek(k); it.ValidForPrefix(k); it.Next() {
				res = append(res, string(it.Item().Key()[len(k):]))
			}

			if s.cache != nil {
				var cost int64
				for _, v := range res {
					cost += int64(len(v))
				}
				s.cache.SetWithTTL(id, res, cost, cacheTTL)
			}
			return res, nil
		}

		for len(qsrc) > 0 && len(qdst) > 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
			var nqs []*node
			for _, n := range qsrc {
				ids, err := adj(n.id)
				if err != nil {
					return err
				}
				for _, nid := range ids {
					if _, ok := srcs[nid]; ok {
						continue
					}
					cn := &node{id: nid, parent: n}
					srcs[nid] = cn
					if m, ok := dsts[nid]; ok {
						var err error
						path, err = reconstruct(cn, m)
						return err
					}
					nqs = append(nqs, cn)
				}
			}
			qsrc = nqs

			if err := ctx.Err(); err != nil {
				return err
			}
			var nqd []*node
			for _, n := range qdst {
				ids, err := adj(n.id)
				if err != nil {
					return err
				}
				for _, nid := range ids {
					if _, ok := dsts[nid]; ok {
						continue
					}
					cn := &node{id: nid, parent: n}
					dsts[nid] = cn
					if m, ok := srcs[nid]; ok {
						var err error
						path, err = reconstruct(m, cn)
						return err
					}
					nqd = append(nqd, cn)
				}
			}
			qdst = nqd
		}
		return errNoConnection
	})

	if err != nil {
		return nil, err
	}
	return path, nil
}

func (s *Server) headersWrapper(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		i := time.Now().UnixMilli()
		n := "relations"
		if strings.HasPrefix(r.URL.Path, "/conexao/") {
			n = "connection"
		}

		w.Header().Set("Cache-Control", cacheControl)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding")
		w.Header().Set("Content-type", "application/json")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			registerMetric(n, r.Method, http.StatusOK, i)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			if _, err := io.WriteString(w, `{"message":"Essa URL aceita apenas o método GET."}`); err != nil {
				slog.Error("failed to write method not allowed response", "error", err)
			}
			registerMetric(n, r.Method, http.StatusMethodNotAllowed, i)
			return
		}
		h(w, r)
	}
}

func (s *Server) RelationsHandler(w http.ResponseWriter, r *http.Request) {
	i := time.Now().UnixMilli()
	id := strings.TrimPrefix(r.URL.Path, "/relacoes/")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		if _, err := io.WriteString(w, `{"message":"Uso: /relacoes/<ID>"}`); err != nil {
			slog.Error("failed to write relations bad request response", "error", err)
		}
		registerMetric("relations", r.Method, http.StatusBadRequest, i)
		return
	}
	if cnpj.IsValid(id) {
		id = cnpj.Unmask(id)
	}

	rels, err := s.getRelations(id)
	if err != nil || len(rels) == 0 {
		w.WriteHeader(http.StatusNotFound)
		if _, err := fmt.Fprintf(w, `{"message":"Identificador %s não encontrado ou sem conexões."}`, id); err != nil {
			slog.Error("failed to write relations not found response", "error", err)
		}
		registerMetric("relations", r.Method, http.StatusNotFound, i)
		return
	}

	for idx := range rels {
		if rels[idx].PartnerType != 2 {
			rels[idx].PartnerCPF = ""
		}
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(rels); err != nil {
		slog.Error("failed to encode relations response", "error", err)
	}
	registerMetric("relations", r.Method, http.StatusOK, i)
}

func (s *Server) ConnectionHandler(w http.ResponseWriter, r *http.Request) {
	i := time.Now().UnixMilli()
	pth := strings.TrimPrefix(r.URL.Path, "/conexao/")
	p := strings.Split(pth, "/")
	if len(p) != 2 {
		w.WriteHeader(http.StatusBadRequest)
		if _, err := io.WriteString(w, `{"message":"Endpoint /conexao/ exige dois identificadores, ex: /conexao/id1/id2"}`); err != nil {
			slog.Error("failed to write connection bad request response", "error", err)
		}
		registerMetric("connection", r.Method, http.StatusBadRequest, i)
		return
	}
	src, dst := p[0], p[1]
	if cnpj.IsValid(src) {
		src = cnpj.Unmask(src)
	}
	if cnpj.IsValid(dst) {
		dst = cnpj.Unmask(dst)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	var out []db.Relationship
	ch := make(chan error, 1)
	go func() {
		var err error
		out, err = s.findShortestPath(ctx, src, dst)
		ch <- err
	}()

	select {
	case <-ctx.Done():
		w.WriteHeader(http.StatusGatewayTimeout)
		if _, err := fmt.Fprintf(w, `{"message":"Não foi possível calcular a conexão entre %s e %s em 90s."}`, src, dst); err != nil {
			slog.Error("failed to write connection timeout response", "error", err)
		}
		registerMetric("connection", r.Method, http.StatusGatewayTimeout, i)
		return
	case err := <-ch:
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			w.WriteHeader(http.StatusGatewayTimeout)
			if _, err := fmt.Fprintf(w, `{"message":"Não foi possível calcular a conexão entre %s e %s em 90s."}`, src, dst); err != nil {
				slog.Error("failed to write connection timeout response", "error", err)
			}
			registerMetric("connection", r.Method, http.StatusGatewayTimeout, i)
			return
		}
		if errors.Is(err, errNoConnection) {
			w.WriteHeader(http.StatusNotFound)
			if _, err := fmt.Fprintf(w, `{"message":"Nenhuma conexão encontrada entre %s e %s."}`, src, dst); err != nil {
				slog.Error("failed to write connection not found response", "error", err)
			}
			registerMetric("connection", r.Method, http.StatusNotFound, i)
			return
		}
		if err != nil {
			slog.Error("failed to find shortest path", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := fmt.Fprintf(w, `{"message":"Erro ao calcular a conexão entre %s e %s."}`, src, dst); err != nil {
				slog.Error("failed to write connection error response", "error", err)
			}
			registerMetric("connection", r.Method, http.StatusInternalServerError, i)
			return
		}
	}

	for idx := range out {
		if out[idx].PartnerType != 2 {
			out[idx].PartnerCPF = ""
		}
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(out); err != nil {
		slog.Error("failed to encode connection response", "error", err)
	}
	registerMetric("connection", r.Method, http.StatusOK, i)
}

func Serve(srv *Server, port string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/relacoes/", srv.headersWrapper(srv.RelationsHandler))
	mux.HandleFunc("/conexao/", srv.headersWrapper(srv.ConnectionHandler))
	mux.HandleFunc("/metrics", promhttp.Handler().ServeHTTP)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		i := time.Now().UnixMilli()
		w.WriteHeader(http.StatusOK)
		registerMetric("/healthz", r.Method, http.StatusOK, i)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		i := time.Now().UnixMilli()
		if r.URL.Path != "/" {
			w.Header().Set("Content-type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			if _, err := fmt.Fprintf(w, `{"message":"Endpoint %s não encontrado. Use /relacoes/<ID> ou /conexao/<ID>/<ID>."}`, r.URL.Path); err != nil {
				slog.Error("failed to write not found response", "error", err)
			}
			registerMetric("root", r.Method, http.StatusNotFound, i)
			return
		}
		http.Redirect(w, r, "https://docs.minhareceita.org", http.StatusFound)
		registerMetric("root", r.Method, http.StatusFound, i)
	})

	slog.Info(fmt.Sprintf("Serving standalone graph API at http://0.0.0.0:%s", port))
	server := &http.Server{
		Addr:         ":" + port,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		Handler:      bandwidthMiddleware(gzhttp.GzipHandler(mux)),
	}
	return server.ListenAndServe()
}
