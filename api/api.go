// Package api provides the HTTP server with wrappers for JSON responses. It
// validates data before passing it to the `db.Database`, which handles the
// query and serialization.
package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"codeberg.org/cuducos/minha-receita/bloom"
	"codeberg.org/cuducos/minha-receita/db"
	"codeberg.org/cuducos/minha-receita/metrics"
	"github.com/klauspost/compress/gzhttp"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"tangled.org/cuducos.me/go-cnpj"
)

const (
	cacheMaxAge = time.Hour * 24
	timeout     = time.Second * 90
)

var cacheControl = fmt.Sprintf("max-age=%d", int(cacheMaxAge.Seconds()))

type database interface {
	GetCompany(context.Context, string) ([]byte, error)
	Search(context.Context, *db.Query) ([]byte, error)
	MetaRead(string) (string, error)
	MetaSave(string, string) error
	AllCompanies(context.Context, *string, uint32) ([]string, *string, error)
	CompanyCount(context.Context) (int64, error)
}

//go:embed admin.html login.html swagger.html openapi.json
var adminFiles embed.FS

func safeCNPJ(n string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9',
			r >= 'A' && r <= 'Z',
			r >= 'a' && r <= 'z',
			r == '.', r == '/', r == '-':
			return r
		default:
			return -1
		}
	}, n)
	if safe == "" {
		return "informado"
	}
	return cnpj.Mask(safe)
}

type api struct {
	db           database
	host         string
	token        string
	authRequired atomic.Bool
	sessions     *adminSessionStore
	jobs         *adminJobs
	cache        *cache
	check        *bloom.Filter
}

type adminStatus struct {
	Healthy       bool   `json:"healthy"`
	Companies     int64  `json:"companies"`
	UpdatedAt     string `json:"updated_at"`
	APIToken      string `json:"api_token"`
	TokenRequired bool   `json:"token_required"`
	DiskTotal     uint64 `json:"disk_total"`
	DiskFree      uint64 `json:"disk_free"`
}

type regionalSearchResult struct {
	Data   []string `json:"data"`
	Cursor *string  `json:"cursor"`
}

type companySearchPage struct {
	Data []struct {
		CNPJ string `json:"cnpj"`
	} `json:"data"`
	Cursor *string `json:"cursor"`
}

var validStates = map[string]bool{
	"AC": true, "AL": true, "AP": true, "AM": true, "BA": true, "CE": true, "DF": true,
	"ES": true, "GO": true, "MA": true, "MT": true, "MS": true, "MG": true, "PA": true,
	"PB": true, "PR": true, "PE": true, "PI": true, "RJ": true, "RN": true, "RS": true,
	"RO": true, "RR": true, "SC": true, "SP": true, "SE": true, "TO": true,
}

func (app *api) regionalCNPJHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		app.messageResponse(w, http.StatusMethodNotAllowed, "Essa URL aceita apenas o método GET.")
		return
	}
	values := r.URL.Query()
	cnae := strings.TrimSpace(values.Get("cnae"))
	state := strings.ToUpper(strings.TrimSpace(values.Get("estado")))
	municipality := strings.TrimSpace(values.Get("municipio"))
	if len(cnae) != 7 || !onlyDigits(cnae) {
		app.messageResponse(w, http.StatusBadRequest, "O parâmetro cnae é obrigatório e deve conter 7 dígitos.")
		return
	}
	if !validStates[state] {
		app.messageResponse(w, http.StatusBadRequest, "O parâmetro estado é obrigatório e deve ser uma UF válida.")
		return
	}
	if municipality == "" || len(municipality) > 100 {
		app.messageResponse(w, http.StatusBadRequest, "O parâmetro municipio é obrigatório.")
		return
	}
	if cursor := values.Get("cursor"); cursor != "" && !onlyDigits(cursor) {
		app.messageResponse(w, http.StatusBadRequest, "O parâmetro cursor é inválido.")
		return
	}
	if limit := values.Get("limit"); limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n < 1 || n > 1000 {
			app.messageResponse(w, http.StatusBadRequest, "O parâmetro limit deve ser um número entre 1 e 1000.")
			return
		}
	}
	values.Set("uf", state)
	values.Set("cnae", cnae)
	if onlyDigits(municipality) {
		values.Set("municipio", municipality)
	} else {
		values.Del("municipio")
		values.Set("municipio_nome", municipality)
	}
	if values.Get("limit") == "" {
		values.Set("limit", "100")
	}
	q := db.NewQuery(values)
	if q == nil {
		app.messageResponse(w, http.StatusBadRequest, "Parâmetros de busca inválidos.")
		return
	}
	q.ActiveOnly = true
	result, err := app.db.Search(r.Context(), q)
	if err != nil {
		slog.Error("regional CNPJ search error", "error", err)
		app.messageResponse(w, http.StatusServiceUnavailable, "Não foi possível realizar a busca.")
		return
	}
	var page companySearchPage
	if err := json.Unmarshal(result, &page); err != nil {
		slog.Error("could not decode regional CNPJ search", "error", err)
		app.messageResponse(w, http.StatusInternalServerError, "Não foi possível processar o resultado da busca.")
		return
	}
	response := regionalSearchResult{Data: make([]string, 0, len(page.Data)), Cursor: page.Cursor}
	for _, company := range page.Data {
		response.Data = append(response.Data, company.CNPJ)
	}
	b, err := json.Marshal(response)
	if err != nil {
		app.messageResponse(w, http.StatusInternalServerError, "Não foi possível processar o resultado da busca.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func onlyDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func (app *api) adminAuthWrapper(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("minha_receita_admin")
		if err != nil || !app.sessions.valid(cookie.Value) {
			if r.URL.Path == "/admin/" {
				http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
				return
			}
			app.messageResponse(w, http.StatusUnauthorized, "Sessão administrativa expirada.")
			return
		}
		h(w, r)
	}
}

func (app *api) apiTokenWrapper(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !app.authRequired.Load() {
			h(w, r)
			return
		}
		if r.Method == http.MethodOptions {
			h(w, r)
			return
		}
		provided := r.Header.Get("X-API-Key")
		if authorization := r.Header.Get("Authorization"); strings.HasPrefix(authorization, "Bearer ") {
			provided = strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
		}
		valid := app.token != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(app.token)) == 1
		if !valid {
			w.Header().Set("WWW-Authenticate", "Bearer")
			app.messageResponse(w, http.StatusUnauthorized, "Token de acesso ausente ou inválido.")
			return
		}
		h(w, r)
	}
}

func (app *api) adminHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/admin/" || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	b, err := adminFiles.ReadFile("admin.html")
	if err != nil {
		http.Error(w, "Não foi possível carregar o painel.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}

func swaggerHandler(w http.ResponseWriter, r *http.Request) {
	name := "swagger.html"
	contentType := "text/html; charset=utf-8"
	if r.URL.Path == "/docs/openapi.json" {
		name, contentType = "openapi.json", "application/json"
	} else if r.URL.Path != "/docs/" {
		http.NotFound(w, r)
		return
	}
	b, err := adminFiles.ReadFile(name)
	if err != nil {
		http.Error(w, "Não foi possível carregar a documentação.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}

func (app *api) adminStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	count, err := app.db.CompanyCount(r.Context())
	if err != nil {
		http.Error(w, "Não foi possível consultar o banco de dados.", http.StatusServiceUnavailable)
		return
	}
	updated, _ := app.db.MetaRead("updated-at")
	total, free, _ := diskStats("/mnt/data")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	b, err := json.Marshal(adminStatus{Healthy: true, Companies: count, UpdatedAt: updated, APIToken: app.token, TokenRequired: app.authRequired.Load(), DiskTotal: total, DiskFree: free})
	if err != nil {
		slog.Error("could not encode admin status", "error", err)
		return
	}
	_, _ = w.Write(b)
}

func (app *api) adminAuthModeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		Required bool `json:"required"`
	}
	body, err := io.ReadAll(r.Body)
	if err != nil || json.Unmarshal(body, &request) != nil {
		app.messageResponse(w, http.StatusBadRequest, "Configuração inválida.")
		return
	}
	value := "free"
	if request.Required {
		value = "required"
	}
	if err := app.db.MetaSave("api-auth", value); err != nil {
		slog.Error("could not persist API auth mode", "error", err)
		app.messageResponse(w, http.StatusInternalServerError, "Não foi possível salvar a configuração.")
		return
	}
	app.authRequired.Store(request.Required)
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, fmt.Sprintf(`{"required":%t}`, request.Required))
}

// messageResponse takes a text message and a HTTP status, wraps the message into a
// JSON output and writes it together with the proper headers to a response.
func (app *api) messageResponse(w http.ResponseWriter, s int, m string) {
	if m != "" {
		w.Header().Set("Content-type", "application/json")
	}
	w.WriteHeader(s)
	if m != "" {
		if _, err := io.WriteString(w, fmt.Sprintf(`{"message":"%s"}`, m)); err != nil {
			slog.Error("could not write response message for", "status code", s, "message", m, "error", err)
		}
	}
	if s == http.StatusInternalServerError {
		slog.Error("Internal server error", "message", m)
	}
}

func (app *api) singleCompany(pth string, w http.ResponseWriter, r *http.Request, i time.Time) {
	w.Header().Set("Content-type", "application/json")
	if !cnpj.IsValid(pth) {
		app.messageResponse(w, http.StatusBadRequest, fmt.Sprintf("CNPJ %s inválido.", safeCNPJ(pth[1:])))
		metrics.RegisterMetric("main", "singleCompany", r.Method, http.StatusBadRequest, i)
		return
	}
	id := cnpj.Unmask(pth)
	if app.cache != nil {
		if s, ok := app.cache.get(id); ok {
			metrics.CacheHits.Inc()
			if len(s) == 0 {
				app.messageResponse(w, http.StatusNotFound, fmt.Sprintf("CNPJ %s não encontrado.", cnpj.Mask(pth)))
				metrics.RegisterMetric("main", "singleCompany", r.Method, http.StatusNotFound, i)
				return
			}
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write(s); err != nil {
				slog.Error("error responding to cached single company request", "request", r, "error", err)
			}
			metrics.RegisterMetric("main", "singleCompany", r.Method, http.StatusOK, i)
			return
		}
		metrics.CacheMisses.Inc()
	}
	if app.check != nil && app.check.Ready() {
		ok, err := app.check.Exists(id)
		if err != nil {
			slog.Error("could not check bloom filter", "cnpj", id, "error", err)
		} else if !ok {
			metrics.BloomFilterEarlyExits.Inc()
			app.messageResponse(w, http.StatusNotFound, fmt.Sprintf("CNPJ %s não encontrado.", cnpj.Mask(pth)))
			metrics.RegisterMetric("main", "singleCompany", r.Method, http.StatusNotFound, i)
			return
		}
	}
	s, err := getCompany(r.Context(), app.db, pth)
	if err != nil {
		if errors.Is(err, db.ErrCompanyNotFound) {
			if app.cache != nil {
				app.cache.set(id, nil)
			}
			app.messageResponse(w, http.StatusNotFound, fmt.Sprintf("CNPJ %s não encontrado.", cnpj.Mask(pth)))
			metrics.RegisterMetric("main", "singleCompany", r.Method, http.StatusNotFound, i)
			return
		}
		slog.Error("error retrieving company", "cnpj", pth, "error", err)
		w.Header().Set("Cache-Control", "no-store")
		app.messageResponse(w, http.StatusServiceUnavailable, "Serviço temporariamente indisponível, tente novamente.")
		metrics.RegisterMetric("main", "singleCompany", r.Method, http.StatusServiceUnavailable, i)
		return
	}
	if app.cache != nil {
		app.cache.set(id, s)
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(s); err != nil {
		slog.Error("error responding to successful single company request", "request", r, "error", err)
	}
	metrics.RegisterMetric("main", "singleCompany", r.Method, http.StatusOK, i)
}

func (app *api) paginatedSearch(q *db.Query, w http.ResponseWriter, r *http.Request, i time.Time) {
	w.Header().Set("Content-type", "application/json")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	s, err := app.db.Search(ctx, q)
	if errors.Is(err, context.DeadlineExceeded) {
		slog.Error("paginated search timed out", "query", q)
		var b bytes.Buffer
		b.WriteString("Tempo de requisição esgotou (Timeout)")
		if q.Limit/2 > 1 {
			_, err := fmt.Fprintf(&b,
				". Essa busca solicitou %d CNPJs, experimente um número menor utilizando o parâmetro limit=%d, por exemplo.",
				q.Limit,
				q.Limit/2,
			)
			if err != nil {
				slog.Warn("could not append tip to error response", "error", err)
			}
		}
		app.messageResponse(w, http.StatusRequestTimeout, b.String())
		metrics.RegisterMetric("main", "paginatedSearch", r.Method, http.StatusRequestTimeout, i)
		return
	}
	if err != nil {
		slog.Error("paginated search error", "error", err, "query", q)
		app.messageResponse(w, http.StatusNotFound, "Erro inesperado na busca.")
		metrics.RegisterMetric("main", "paginatedSearch", r.Method, http.StatusNotFound, i)
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(s); err != nil {
		slog.Error("error responding to successful paginated search request", "query", q, "request", r, "error", err)
	}
	metrics.RegisterMetric("main", "paginatedSearch", r.Method, http.StatusOK, i)
}

func (app *api) companyHandler(w http.ResponseWriter, r *http.Request) {
	i := time.Now()
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding")

	switch r.Method {
	case http.MethodGet:
		break
	case http.MethodOptions:
		w.WriteHeader(http.StatusOK)
		metrics.RegisterMetric("main", "earlyReturn", r.Method, http.StatusOK, i)
		return
	default:
		app.messageResponse(w, http.StatusMethodNotAllowed, "Essa URL aceita apenas o método GET.")
		metrics.RegisterMetric("main", "earlyReturn", r.Method, http.StatusMethodNotAllowed, i)
		return
	}
	pth := r.URL.Path
	if pth == "/" {
		q := db.NewQuery(r.URL.Query())
		if q == nil {
			http.Redirect(w, r, "https://docs.minhareceita.org", http.StatusFound)
			metrics.RegisterMetric("main", "redirectedToDocs", r.Method, http.StatusFound, i)
			return
		}
		app.paginatedSearch(q, w, r, i)
		return
	}
	app.singleCompany(pth, w, r, i)
}

func (app *api) updatedHandler(w http.ResponseWriter, r *http.Request) {
	i := time.Now()
	if r.Method != http.MethodGet {
		app.messageResponse(w, http.StatusMethodNotAllowed, "Essa URL aceita apenas o método GET.")
		metrics.RegisterMetric("main", "updated", r.Method, http.StatusMethodNotAllowed, i)
		return
	}
	s, err := app.db.MetaRead("updated-at")
	if err != nil || s == "" {
		app.messageResponse(w, http.StatusInternalServerError, "Erro buscando data de atualização.")
		metrics.RegisterMetric("main", "updated", r.Method, http.StatusInternalServerError, i)
		return
	}
	w.Header().Set("Cache-Control", cacheControl)
	app.messageResponse(w, http.StatusOK, s)
	metrics.RegisterMetric("main", "updated", r.Method, http.StatusOK, i)
}

func (app *api) healthHandler(w http.ResponseWriter, r *http.Request) {
	i := time.Now()
	if r.Method != http.MethodHead && r.Method != http.MethodGet {
		app.messageResponse(w, http.StatusMethodNotAllowed, "Essa URL aceita apenas os métodos GET e HEAD.")
		metrics.RegisterMetric("main", "health", r.Method, http.StatusMethodNotAllowed, i)
		return
	}
	w.WriteHeader(http.StatusOK)
	metrics.RegisterMetric("main", "health", r.Method, http.StatusOK, i)
}

func (app *api) allowedHostWrapper(h func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	if app.host == "" {
		return h
	}
	w := func(w http.ResponseWriter, r *http.Request) {
		if v := r.Host; v != app.host {
			slog.Error("Host not allowed", "host", v)
			w.WriteHeader(http.StatusTeapot)
			return
		}
		h(w, r)
	}
	return w
}

// Serve spins up the HTTP server.
func Serve(db database, p string, cacheSize, bloomSize int) error {
	if !strings.HasPrefix(p, ":") {
		p = ":" + p
	}
	c, err := newCache(cacheSize)
	if err != nil {
		return fmt.Errorf("api could not initialize cache: %w", err)
	}
	app := api{db: db, host: os.Getenv("ALLOWED_HOST"), token: os.Getenv("API_TOKEN"), cache: c, sessions: newAdminSessionStore(), jobs: &adminJobs{}}
	required := true
	if configured, err := db.MetaRead("api-auth"); err == nil {
		required = configured != "free"
	} else if value := os.Getenv("API_AUTH_REQUIRED"); value != "" {
		if parsed, parseErr := strconv.ParseBool(value); parseErr == nil {
			required = parsed
		}
	}
	app.authRequired.Store(required)

	if bloomSize > 0 {
		app.check = bloom.New(db, bloomSize)
		go func() {
			ini := time.Now()
			if err := app.check.Initialize(context.Background()); err != nil {
				slog.Error("could not Initialize bloom filter", "error", err)
			}
			if app.check.Ready() {
				metrics.BloomFilterBuildDuration.Set(time.Since(ini).Seconds())
				metrics.BloomFilterReady.Set(1)
			}
		}()
	}

	http.HandleFunc("/admin/login", app.allowedHostWrapper(app.adminLoginHandler))
	http.HandleFunc("/docs/", app.allowedHostWrapper(swaggerHandler))
	http.HandleFunc("/admin/", app.allowedHostWrapper(app.adminAuthWrapper(app.adminHandler)))
	http.HandleFunc("/admin/logout", app.allowedHostWrapper(app.adminAuthWrapper(app.adminLogoutHandler)))
	http.HandleFunc("/admin/api/status", app.allowedHostWrapper(app.adminAuthWrapper(app.adminStatusHandler)))
	http.HandleFunc("/admin/api/auth", app.allowedHostWrapper(app.adminAuthWrapper(app.adminAuthModeHandler)))
	http.HandleFunc("/admin/api/jobs", app.allowedHostWrapper(app.adminAuthWrapper(app.adminJobsHandler)))
	http.HandleFunc("/admin/api/jobs/download", app.allowedHostWrapper(app.adminAuthWrapper(app.adminDownloadHandler)))
	http.HandleFunc("/admin/api/jobs/transform", app.allowedHostWrapper(app.adminAuthWrapper(app.adminTransformHandler)))
	http.HandleFunc("/admin/api/jobs/cancel", app.allowedHostWrapper(app.adminAuthWrapper(app.adminCancelJobHandler)))
	http.HandleFunc("/admin/api/restart", app.allowedHostWrapper(app.adminAuthWrapper(app.adminRestartHandler)))
	http.HandleFunc("/cnae", app.allowedHostWrapper(app.apiTokenWrapper(app.regionalCNPJHandler)))

	for _, r := range []struct {
		path    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"/", app.companyHandler},
		{"/updated", app.updatedHandler},
		{"/healthz", app.healthHandler},
		{"/metrics", promhttp.Handler().ServeHTTP},
	} {
		http.HandleFunc(r.path, app.allowedHostWrapper(app.apiTokenWrapper(r.handler)))
	}

	s := &http.Server{
		Addr:         p,
		ReadTimeout:  timeout * 2,
		WriteTimeout: timeout * 2,
		Handler:      bandwidthMiddleware(gzhttp.GzipHandler(http.DefaultServeMux)),
	}
	slog.Info(fmt.Sprintf("Serving at http://0.0.0.0%s", p))
	return s.ListenAndServe()
}
