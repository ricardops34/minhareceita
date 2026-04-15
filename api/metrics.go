package api

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricLabels = []string{"method", "status_code", "endpoint"}
	requestCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "total_requests",
		Help: "The total number of requests served",
	}, metricLabels)
	requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "request_duration",
		Help:    "The duration of requests in milliseconds",
		Buckets: []float64{1, 2, 5, 10, 25, 50, 100, 250, 500, 1000, 5000},
	}, metricLabels)
	cacheHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cache_hits_total",
		Help: "The total number of cache hits",
	})
	cacheMisses = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cache_misses_total",
		Help: "The total number of cache misses",
	})
	bloomFilterReady = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "bloom_filter_ready",
		Help: "1 when the CNPJ bloom filter is fully built, 0 otherwise",
	})
	bloomFilterEarlyExits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "bloom_filter_early_exits_total",
		Help: "Total 404s short-circuited by the bloom filter (never hit the DB)",
	})
	bloomFilterBuildDuration = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "bloom_filter_build_duration_seconds",
		Help: "How long it took to build the bloom filter (seconds)",
	})
)

func registerMetric(e, m string, s int, i int64) {
	c := fmt.Sprintf("%d", s)
	requestCount.WithLabelValues(m, c, e).Inc()
	requestDuration.WithLabelValues(m, c, e).Observe(float64(time.Now().UnixMilli() - i))
}
