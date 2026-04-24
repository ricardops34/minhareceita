package api

import "net/http"

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
func bandwidthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := &bandwidthResponseWriter{ResponseWriter: w}
		next.ServeHTTP(b, r)
		registerBandwidth(r.URL.Path, r.Method, int(r.ContentLength), b.bytes)
	})
}

func registerBandwidth(e, m string, reqBytes, respBytes int) {
	requestBytes.WithLabelValues(m, e).Add(float64(reqBytes))
	responseBytes.WithLabelValues(m, e).Add(float64(respBytes))
}
