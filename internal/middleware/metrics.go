package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Operational endpoints served by the gateway itself rather than proxied.
const (
	pathMetrics = "/metrics"
	pathHealth  = "/health"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"method", "path"},
	)

	httpRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gateway_http_requests_in_flight",
			Help: "Number of HTTP requests currently being processed",
		},
	)

	httpResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_http_response_size_bytes",
			Help:    "HTTP response size in bytes",
			Buckets: []float64{100, 500, 1000, 5000, 10000, 50000, 100000},
		},
		[]string{"method", "path"},
	)

	circuitBreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gateway_circuit_breaker_state",
			Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
		},
		[]string{"target"},
	)

	cacheHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gateway_cache_hits_total",
			Help: "Total number of cache hits",
		},
	)

	cacheMisses = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gateway_cache_misses_total",
			Help: "Total number of cache misses",
		},
	)

	rateLimitRejections = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gateway_rate_limit_rejections_total",
			Help: "Total number of rate limit rejections",
		},
	)
)

type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode  int
	bytesWritten int
}

func (w *metricsResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *metricsResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += n
	return n, err
}

func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't track metrics endpoint itself
		if r.URL.Path == pathMetrics {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		httpRequestsInFlight.Inc()

		recorder := &metricsResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(recorder, r)

		httpRequestsInFlight.Dec()
		duration := time.Since(start).Seconds()

		// Normalize path to avoid high cardinality (group by prefix)
		path := normalizePath(r.URL.Path)

		httpRequestsTotal.WithLabelValues(r.Method, path, strconv.Itoa(recorder.statusCode)).Inc()
		httpRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
		httpResponseSize.WithLabelValues(r.Method, path).Observe(float64(recorder.bytesWritten))
	})
}

// RecordCacheHit increments the cache hit counter
func RecordCacheHit() {
	cacheHits.Inc()
}

// RecordCacheMiss increments the cache miss counter
func RecordCacheMiss() {
	cacheMisses.Inc()
}

// RecordRateLimitRejection increments the rate limit rejection counter
func RecordRateLimitRejection() {
	rateLimitRejections.Inc()
}

// RecordCircuitBreakerState records the circuit breaker state for a target
func RecordCircuitBreakerState(target string, state int) {
	circuitBreakerState.WithLabelValues(target).Set(float64(state))
}

// normalizePath groups a request path into a bounded set of label values.
// Anything that does not match a known route collapses into "other": returning
// the raw path would let unauthenticated traffic (scanners probing /.env,
// /wp-login.php, ...) mint an unbounded number of Prometheus time series and
// circuit breakers that are never reclaimed.
func normalizePath(path string) string {
	// Group API paths by their prefix
	prefixes := []string{"/api/users", "/api/products", "/admin/routes", "/admin/stats", "/admin/keys", "/admin/token", pathHealth, pathMetrics}
	for _, prefix := range prefixes {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return prefix
		}
	}
	return "other"
}
