package graph

import (
	"net/http"
	"strings"
)

type bandwidthResponseWriter struct {
	http.ResponseWriter
	bytes int
}

func (w *bandwidthResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func normalizeEndpoint(r *http.Request) string {
	switch {
	case r.URL.Path == "/":
		return "root"
	case r.URL.Path == "/healthz", r.URL.Path == "/metrics":
		return r.URL.Path
	case strings.HasPrefix(r.URL.Path, "/relacoes/"):
		return "relations"
	case strings.HasPrefix(r.URL.Path, "/conexao/"):
		return "connection"
	default:
		return r.URL.Path
	}
}

func bandwidthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := &bandwidthResponseWriter{ResponseWriter: w}
		next.ServeHTTP(b, r)
		registerBandwidth(normalizeEndpoint(r), r.Method, int(r.ContentLength), b.bytes)
	})
}

func registerBandwidth(e, m string, reqBytes, respBytes int) {
	if reqBytes > 0 {
		requestBytes.WithLabelValues(m, e).Add(float64(reqBytes))
	}
	if respBytes > 0 {
		responseBytes.WithLabelValues(m, e).Add(float64(respBytes))
	}
}
