package graph

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricLabels    = []string{"method", "status_code", "endpoint"}
	bandwidthLabels = []string{"method", "endpoint"}

	requestCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "graph_total_requests",
		Help: "The total number of requests served",
	}, metricLabels)

	requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "graph_request_duration",
		Help:    "The duration of requests in milliseconds",
		Buckets: []float64{1, 2, 5, 10, 25, 50, 100, 250, 500, 1000, 5000},
	}, metricLabels)

	cacheHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "graph_cache_hits_total",
		Help: "The total number of cache hits",
	})

	cacheMisses = promauto.NewCounter(prometheus.CounterOpts{
		Name: "graph_cache_misses_total",
		Help: "The total number of cache misses",
	})

	requestBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "graph_request_bytes_total",
		Help: "The total number of bytes received in request bodies",
	}, bandwidthLabels)

	responseBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "graph_response_bytes_total",
		Help: "The total number of bytes sent in response bodies",
	}, bandwidthLabels)
)

func registerMetric(e, m string, s int, i int64) {
	c := fmt.Sprintf("%d", s)
	requestCount.WithLabelValues(m, c, e).Inc()
	requestDuration.WithLabelValues(m, c, e).Observe(float64(time.Now().UnixMilli() - i))
}
