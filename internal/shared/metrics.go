package shared

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

var (
	MetricsRegistry = prometheus.NewRegistry()

	CoreHTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "loyalty_core_http_requests_total",
			Help: "Total HTTP requests handled by the core service",
		},
		[]string{"method", "route", "status_class", "status_code"},
	)

	CoreBusinessTransactionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "loyalty_core_business_transactions_total",
			Help: "Business transactions processed by the core service",
		},
		[]string{"flow", "outcome"},
	)

	CoreHTTPRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "loyalty_core_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds for the core service",
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.2, 0.35, 0.5, 0.75, 1, 1.5, 2, 3, 5},
		},
		[]string{"method", "route", "status_class", "status_code"},
	)
)

func init() {
	MetricsRegistry.MustRegister(
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewGoCollector(),
		CoreHTTPRequestsTotal,
		CoreBusinessTransactionsTotal,
		CoreHTTPRequestDurationSeconds,
	)
}
