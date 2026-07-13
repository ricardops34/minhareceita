package graph

import (
	"context"
	"net/http"

	"codeberg.org/cuducos/minha-receita/metrics"
)

type graphRequestKey struct{}

type bandwidthResponseWriter struct {
	http.ResponseWriter
	bytes int
}

func (w *bandwidthResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func normalizeEndpoint(path string, req *graphRequest) string {
	if path == "/" {
		return "root"
	}

	switch req.kind {
	case singleID:
		return "relations"
	case connection:
		return "connection"
	}

	return path
}

func bandwidthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := &bandwidthResponseWriter{ResponseWriter: w}
		p := parseRequest(r)
		ctx := context.WithValue(r.Context(), graphRequestKey{}, p)
		next.ServeHTTP(b, r.WithContext(ctx))
		registerBandwidth("graph", normalizeEndpoint(r.URL.Path, p), r.Method, int(r.ContentLength), b.bytes)
	})
}

func registerBandwidth(a, e, m string, reqBytes, respBytes int) {
	if reqBytes > 0 {
		metrics.RequestBytes.WithLabelValues(a, m, e).Add(float64(reqBytes))
	}
	if respBytes > 0 {
		metrics.ResponseBytes.WithLabelValues(a, m, e).Add(float64(respBytes))
	}
}
