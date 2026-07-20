package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// The metric collectors are package-level globals shared by every test in this
// package, so these tests assert on the delta they cause rather than on
// absolute counter values, which depend on test execution order.

func TestCacheMetrics_MissThenHitAreRecorded(t *testing.T) {
	flushTestRedis(t)
	client := newTestRedisClient()
	cache := NewCache(client, 30)

	handler := cache.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data": "metrics"}`))
	}))

	missesBefore := testutil.ToFloat64(cacheMisses)
	hitsBefore := testutil.ToFloat64(cacheHits)

	path := fmt.Sprintf("/api/cache-metrics-%d", time.Now().UnixNano())
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	if got := testutil.ToFloat64(cacheMisses) - missesBefore; got != 1 {
		t.Errorf("expected 1 cache miss recorded, got %v", got)
	}
	if got := testutil.ToFloat64(cacheHits) - hitsBefore; got != 1 {
		t.Errorf("expected 1 cache hit recorded, got %v", got)
	}
}

func TestCache_SkipsOperationalEndpoints(t *testing.T) {
	client := newTestRedisClient()
	cache := NewCache(client, 30)

	upstreamCalls := 0
	handler := cache.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Write([]byte(`{"served": true}`))
	}))

	// Caching /metrics would serve Prometheus a stale exposition payload for a
	// full TTL; caching /health would mask a failing gateway.
	for _, path := range []string{"/metrics", "/health"} {
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Header().Get("X-Cache") != "" {
				t.Errorf("%s should not be cached, got X-Cache: %s", path, rec.Header().Get("X-Cache"))
			}
		}
	}

	if upstreamCalls != 4 {
		t.Errorf("expected all 4 requests to reach the upstream, got %d", upstreamCalls)
	}
}

func TestRateLimitMetrics_RejectionsAreRecorded(t *testing.T) {
	flushTestRedis(t)
	client := newTestRedisClient()

	limit := 3
	rl := NewRateLimiter(client, limit)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	before := testutil.ToFloat64(rateLimitRejections)

	ip := fmt.Sprintf("10.9.%d.%d:5555", time.Now().Unix()%255, time.Now().UnixNano()%255)
	rejected := 0
	for i := 0; i < limit+2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/ratelimit-metrics", nil)
		req.RemoteAddr = ip
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code == http.StatusTooManyRequests {
			rejected++
		}
	}

	if rejected == 0 {
		t.Fatal("expected at least one request to be rate limited")
	}
	if got := testutil.ToFloat64(rateLimitRejections) - before; int(got) != rejected {
		t.Errorf("expected %d rejections recorded, got %v", rejected, got)
	}
}

func TestCircuitBreakerMetrics_TracksEveryStateTransition(t *testing.T) {
	// A path that normalizePath maps to a stable, bounded label, kept distinct
	// from the other metric tests so the gauge is not shared.
	const target = "/admin/routes"

	maxFailures := 3
	halfOpenMaxRequests := 2
	cbm := NewCircuitBreakerManager(maxFailures, 1, halfOpenMaxRequests)

	upstreamFails := true
	handler := cbm.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if upstreamFails {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	do := func() {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
	gauge := func() float64 {
		return testutil.ToFloat64(circuitBreakerState.WithLabelValues(target))
	}

	// First request creates the breaker and publishes the initial closed state.
	do()
	if got := gauge(); got != float64(StateClosed) {
		t.Fatalf("expected gauge %d (closed) after first request, got %v", StateClosed, got)
	}

	// Drive it to maxFailures: closed -> open.
	for i := 1; i < maxFailures; i++ {
		do()
	}
	if got := gauge(); got != float64(StateOpen) {
		t.Fatalf("expected gauge %d (open) after %d failures, got %v", StateOpen, maxFailures, got)
	}

	// After the timeout elapses the next request flips open -> half-open, and
	// the upstream is healthy again so it counts as one half-open success.
	time.Sleep(1100 * time.Millisecond)
	upstreamFails = false
	do()
	if got := gauge(); got != float64(StateHalfOpen) {
		t.Fatalf("expected gauge %d (half-open) after timeout, got %v", StateHalfOpen, got)
	}

	// The remaining half-open success closes it again.
	do()
	if got := gauge(); got != float64(StateClosed) {
		t.Fatalf("expected gauge %d (closed) after recovery, got %v", StateClosed, got)
	}
}

func TestNormalizePath_BoundsUnknownPaths(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/api/users/123", "/api/users"},
		{"/api/products", "/api/products"},
		{"/health", "/health"},
		{"/metrics", "/metrics"},
		{"/.env", "other"},
		{"/wp-login.php", "other"},
		{"/", "other"},
	}

	for _, tt := range tests {
		if got := normalizePath(tt.path); got != tt.want {
			t.Errorf("normalizePath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
