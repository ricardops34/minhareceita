package api

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"codeberg.org/cuducos/minha-receita/db"
	"github.com/klauspost/compress/gzhttp"
	"golang.org/x/sync/errgroup"
	"tangled.org/cuducos.me/go-cnpj"
)

const (
	maxDistance = 16
	workers     = 32
)

type pathNode struct {
	id     string
	parent *pathNode
	edge   *db.GraphEdge
}

func (app *graphAPI) findShortestPath(ctx context.Context, src, dst string) ([]db.GraphEdge, error) {
	if src == dst {
		return []db.GraphEdge{}, nil
	}

	pSrc := map[string]*pathNode{src: {id: src}}
	pDst := map[string]*pathNode{dst: {id: dst}}

	qSrc := []*pathNode{pSrc[src]}
	qDst := []*pathNode{pDst[dst]}

	fetch := func(ctx context.Context, g *errgroup.Group, cancel context.CancelFunc, q []*pathNode, other map[string]*pathNode, res [][]db.GraphEdge) {
		for i, n := range q {
			g.Go(func() error {
				r, err := app.db.GetRelated(ctx, n.id)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return nil
					}
					return err
				}
				res[i] = r
				for _, edge := range r {
					id := edge.PartnerID
					if id == n.id {
						id = edge.CompanyID
					}
					if _, ok := other[id]; ok {
						cancel()
						break
					}
				}
				return nil
			})
		}
	}

	process := func(q []*pathNode, res [][]db.GraphEdge, v, other map[string]*pathNode, isSrc bool) ([]*pathNode, []db.GraphEdge) {
		var nq []*pathNode
		for i, ns := range res {
			p := q[i]
			for _, edge := range ns {
				id := edge.PartnerID
				if id == p.id {
					id = edge.CompanyID
				}
				if _, ok := v[id]; ok {
					continue
				}
				e := edge
				n := &pathNode{id: id, parent: p, edge: &e}
				v[id] = n
				if m, ok := other[id]; ok {
					if isSrc {
						return nil, reconstructPath(n, m)
					}
					return nil, reconstructPath(m, n)
				}
				nq = append(nq, n)
			}
		}
		return nq, nil
	}

	for i := 0; i < maxDistance/2 && len(qSrc) > 0 && len(qDst) > 0; i++ {
		g, ctx := errgroup.WithContext(ctx)
		g.SetLimit(workers)
		ctx, cancel := context.WithCancel(ctx)

		resSrc := make([][]db.GraphEdge, len(qSrc))
		resDst := make([][]db.GraphEdge, len(qDst))

		var wg sync.WaitGroup
		wg.Go(func() { fetch(ctx, g, cancel, qSrc, pDst, resSrc) })
		wg.Go(func() { fetch(ctx, g, cancel, qDst, pSrc, resDst) })
		wg.Wait()

		if err := g.Wait(); err != nil {
			cancel()
			return nil, err
		}

		var pth []db.GraphEdge
		qSrc, pth = process(qSrc, resSrc, pSrc, pDst, true)
		if pth != nil {
			cancel()
			return pth, nil
		}

		qDst, pth = process(qDst, resDst, pDst, pSrc, false)
		if pth != nil {
			cancel()
			return pth, nil
		}

		cancel()
	}

	return nil, fmt.Errorf("nenhuma conexão encontrada entre %s e %s", src, dst)
}

func reconstructPath(src, dst *pathNode) []db.GraphEdge {
	var pth []db.GraphEdge
	curr := src
	for curr.edge != nil {
		pth = append([]db.GraphEdge{*curr.edge}, pth...)
		curr = curr.parent
	}
	curr = dst
	for curr.edge != nil {
		pth = append(pth, *curr.edge)
		curr = curr.parent
	}
	return pth
}

type graphDatabase interface {
	GetRelated(context.Context, string) ([]db.GraphEdge, error)
}

type graphAPI struct {
	db graphDatabase
}

func (app *graphAPI) headersWrapper(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", cacheControl)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding")
		w.Header().Set("Content-type", "application/json")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			if _, err := io.WriteString(w, `{"message":"Essa URL aceita apenas o método GET."}`); err != nil {
				slog.Error("could not write response for method not allowed", "error", err)
			}
			return
		}
		h(w, r)
	}
}

func (app *graphAPI) relationsHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/relacoes/")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		if _, err := io.WriteString(w, `{"message":"Uso: /relacoes/<ID>"}`); err != nil {
			slog.Error("could not write relacoes usage response", "error", err)
		}
		return
	}
	if cnpj.IsValid(id) {
		id = cnpj.Unmask(id)
	}
	ns, err := app.db.GetRelated(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		if _, err := fmt.Fprintf(w, `{"message":"Identificador %s não encontrado."}`, id); err != nil {
			slog.Error("could not write not found response", "error", err)
		}
		return
	}
	if len(ns) == 0 {
		w.WriteHeader(http.StatusNotFound)
		if _, err := fmt.Fprintf(w, `{"message":"Identificador %s não encontrado ou sem conexões."}`, id); err != nil {
			slog.Error("could not write not found response", "error", err)
		}
		return
	}
	for i := range ns {
		if ns[i].PartnerType != 2 {
			ns[i].PartnerCPF = ""
		}
	}
	w.WriteHeader(http.StatusOK)
	if err := json.MarshalWrite(w, ns); err != nil {
		slog.Error("could not encode relations response", "error", err)
	}
}

func (app *graphAPI) connectionHandler(w http.ResponseWriter, r *http.Request) {
	pth := strings.TrimPrefix(r.URL.Path, "/conexao/")
	p := strings.Split(pth, "/")
	if len(p) != 2 {
		w.WriteHeader(http.StatusBadRequest)
		if _, err := io.WriteString(w, `{"message":"Endpoint /conexao/ exige dois identificadores, ex: /conexao/id1/id2"}`); err != nil {
			slog.Error("could not write bad request response", "error", err)
		}
		return
	}
	src, dst := p[0], p[1]
	if cnpj.IsValid(src) {
		src = cnpj.Unmask(src)
	}
	if cnpj.IsValid(dst) {
		dst = cnpj.Unmask(dst)
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	path, err := app.findShortestPath(ctx, src, dst)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		if _, err := fmt.Fprintf(w, `{"message":"Nenhuma conexão encontrada entre %s e %s."}`, src, dst); err != nil {
			slog.Error("could not write not found response", "error", err)
		}
		return
	}
	for i := range path {
		if path[i].PartnerType != 2 {
			path[i].PartnerCPF = ""
		}
	}
	w.WriteHeader(http.StatusOK)
	if err := json.MarshalWrite(w, path); err != nil {
		slog.Error("could not encode connection path response", "error", err)
	}
}

func (app *graphAPI) mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/relacoes/", app.headersWrapper(app.relationsHandler))
	mux.HandleFunc("/conexao/", app.headersWrapper(app.connectionHandler))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.Header().Set("Content-type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			if _, err := fmt.Fprintf(w, `{"message":"Endpoint %s não encontrado. Use /relacoes/<ID> ou /conexao/<ID>/<ID>."}`, r.URL.Path); err != nil {
				slog.Error("could not write not found response", "error", err)
			}
			return
		}
		http.Redirect(w, r, "https://docs.minhareceita.org", http.StatusFound)
	})
	return mux
}

// ServeGraph spins up the HTTP server for the graph API.
func ServeGraph(db graphDatabase, p string) error {
	if !strings.HasPrefix(p, ":") {
		p = ":" + p
	}
	app := graphAPI{db: db}
	s := &http.Server{
		Addr:         p,
		ReadTimeout:  timeout * 2,
		WriteTimeout: timeout * 2,
		Handler:      gzhttp.GzipHandler(app.mux()),
	}
	slog.Info(fmt.Sprintf("Serving graph at http://0.0.0.0%s", p))
	return s.ListenAndServe()
}
