package metrics

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricLabels    = []string{"api", "method", "status_code", "endpoint"}
	bandwidthLabels = []string{"api", "method", "endpoint"}

	RequestCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "requests_total",
		Help: "The total number of requests served",
	}, metricLabels)

	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "request_duration_seconds",
		Help:    "The duration of requests in seconds",
		Buckets: []float64{0.001, 0.002, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 5},
	}, metricLabels)

	CacheHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cache_hits_total",
		Help: "The total number of cache hits",
	})

	CacheMisses = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cache_misses_total",
		Help: "The total number of cache misses",
	})

	BloomFilterReady = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "bloom_filter_ready",
		Help: "1 when the CNPJ bloom filter is fully built, 0 otherwise",
	})

	BloomFilterEarlyExits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "bloom_filter_early_exits_total",
		Help: "Total 404s short-circuited by the bloom filter (never hit the DB)",
	})

	BloomFilterBuildDuration = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "bloom_filter_build_duration_seconds",
		Help: "How long it took to build the bloom filter (seconds)",
	})

	RequestBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "request_bytes_total",
		Help: "The total number of bytes received in request bodies",
	}, bandwidthLabels)

	ResponseBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "response_bytes_total",
		Help: "The total number of bytes sent in response bodies",
	}, bandwidthLabels)
)

func RegisterMetric(a, e, m string, s int, i time.Time) {
	c := fmt.Sprintf("%d", s)
	RequestCount.WithLabelValues(a, m, c, e).Inc()
	RequestDuration.WithLabelValues(a, m, c, e).Observe(time.Since(i).Seconds())
}
