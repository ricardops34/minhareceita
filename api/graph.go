package api

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/klauspost/compress/gzhttp"
	"tangled.org/cuducos.me/go-cnpj"
)

type graphDatabase interface {
	GetCompanyPartners(context.Context, string) (string, error)
	GetPartnerCompanies(context.Context, string) (string, error)
}

type graphAPI struct {
	db graphDatabase
}

func (app *graphAPI) handler(w http.ResponseWriter, r *http.Request) {
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

	pth := strings.TrimPrefix(r.URL.Path, "/")
	if pth == "" {
		http.Redirect(w, r, "https://docs.minhareceita.org", http.StatusFound)
		return
	}

	if id, ok := strings.CutPrefix(pth, "qsa/"); ok {
		if !cnpj.IsValid(id) {
			w.WriteHeader(http.StatusBadRequest)
			if _, err := fmt.Fprintf(w, `{"message":"CNPJ %s inválido."}`, id); err != nil {
				slog.Error("could not write invalid CNPJ response", "error", err)
			}
			return
		}
		id = cnpj.Unmask(id)
		s, err := app.db.GetCompanyPartners(r.Context(), id)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			if _, err := fmt.Fprintf(w, `{"message":"CNPJ %s não encontrado."}`, cnpj.Mask(id)); err != nil {
				slog.Error("could not write not found response", "error", err)
			}
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := io.WriteString(w, s); err != nil {
			slog.Error("could not write company partners response", "error", err)
		}
		return
	}

	if id, ok := strings.CutPrefix(pth, "cnpjs/"); ok {
		if cnpj.IsValid(id) {
			id = cnpj.Unmask(id)
		}
		s, err := app.db.GetPartnerCompanies(r.Context(), id)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			if _, err := fmt.Fprintf(w, `{"message":"Identificador %s não encontrado."}`, id); err != nil {
				slog.Error("could not write not found response", "error", err)
			}
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := io.WriteString(w, s); err != nil {
			slog.Error("could not write partner companies response", "error", err)
		}
		return
	}

	w.WriteHeader(http.StatusNotFound)
	if _, err := fmt.Fprintf(w, `{"message":"Endpoint %s não encontrado. Use /qsa/<CNPJ> ou /cnpjs/<ID>."}`, r.URL.Path); err != nil {
		slog.Error("could not write not found response", "error", err)
	}
}

// ServeGraph spins up the HTTP server for the graph API.
func ServeGraph(db graphDatabase, p string) error {
	if !strings.HasPrefix(p, ":") {
		p = ":" + p
	}
	app := graphAPI{db: db}
	mux := http.NewServeMux()
	mux.HandleFunc("/", app.handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	s := &http.Server{
		Addr:         p,
		ReadTimeout:  timeout * 2,
		WriteTimeout: timeout * 2,
		Handler:      gzhttp.GzipHandler(mux),
	}
	slog.Info(fmt.Sprintf("Serving graph at http://0.0.0.0%s", p))
	return s.ListenAndServe()
}
