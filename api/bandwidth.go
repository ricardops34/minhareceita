package api

import (
	"net/http"

	"codeberg.org/cuducos/minha-receita/metrics"
)

// bandwidthResponseWriter wraps http.ResponseWriter to count bytes written
// to the response body for bandwidth tracking.
type bandwidthResponseWriter struct {
	http.ResponseWriter
	bytes int
}

func (w *bandwidthResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// bandwidthMiddleware wraps an http.Handler to track inbound and outbound
// bandwidth.
func normalizeEndpoint(r *http.Request) string {
	switch r.URL.Path {
	case "/":
		if r.URL.RawQuery != "" {
			return "paginatedSearch"
		}
		return "root"
	case "/updated", "/healthz", "/metrics":
		return r.URL.Path
	default:
		return "singleCompany"
	}
}

func bandwidthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := &bandwidthResponseWriter{ResponseWriter: w}
		next.ServeHTTP(b, r)
		registerBandwidth("main", normalizeEndpoint(r), r.Method, int(r.ContentLength), b.bytes)
	})
}

func registerBandwidth(a, e, m string, reqBytes, respBytes int) {
	metrics.RequestBytes.WithLabelValues(a, m, e).Add(float64(reqBytes))
	metrics.ResponseBytes.WithLabelValues(a, m, e).Add(float64(respBytes))
}
