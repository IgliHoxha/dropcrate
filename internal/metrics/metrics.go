// Package metrics defines dropcrate's Prometheus instrumentation: request
// counters and latency histograms for both transports, plus the /metrics
// exposition handler. Collectors register with the default registry, so the
// standard Go runtime and process metrics are exported alongside these.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dropcrate_http_requests_total",
		Help: "Total HTTP requests by method, route, and status code.",
	}, []string{"method", "route", "status"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "dropcrate_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds by method and route.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})

	grpcRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dropcrate_grpc_requests_total",
		Help: "Total gRPC requests by method and status code.",
	}, []string{"method", "code"})

	grpcDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "dropcrate_grpc_request_duration_seconds",
		Help:    "gRPC request duration in seconds by method.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method"})
)

// ObserveHTTP records one completed HTTP request.
func ObserveHTTP(method, route, status string, seconds float64) {
	httpRequests.WithLabelValues(method, route, status).Inc()
	httpDuration.WithLabelValues(method, route).Observe(seconds)
}

// ObserveGRPC records one completed gRPC call.
func ObserveGRPC(method, code string, seconds float64) {
	grpcRequests.WithLabelValues(method, code).Inc()
	grpcDuration.WithLabelValues(method).Observe(seconds)
}

// Handler serves the Prometheus exposition endpoint.
func Handler() http.Handler { return promhttp.Handler() }
