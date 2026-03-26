package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCircuitBreaker_ClosedState(t *testing.T) {
	cbm := NewCircuitBreakerManager(3, 10, 2)

	handler := cbm.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 in closed state, got %d", rec.Code)
	}
}

func TestCircuitBreaker_OpensAfterMaxFailures(t *testing.T) {
	maxFailures := 3
	cbm := NewCircuitBreakerManager(maxFailures, 30, 2)

	failHandler := cbm.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	// Send enough failures to open the circuit
	for i := 0; i < maxFailures; i++ {
		req := httptest.NewRequest(http.MethodGet, "/fail-test", nil)
		rec := httptest.NewRecorder()
		failHandler.ServeHTTP(rec, req)
	}

	// Next request should be rejected by circuit breaker
	req := httptest.NewRequest(http.MethodGet, "/fail-test", nil)
	rec := httptest.NewRecorder()
	failHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when circuit is open, got %d", rec.Code)
	}
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	cbm := NewCircuitBreakerManager(3, 30, 2)
	callCount := 0

	handler := cbm.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))

	// 2 failures, then 1 success
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/reset-test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// Should still be closed (success reset the failure count)
	req := httptest.NewRequest(http.MethodGet, "/reset-test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusServiceUnavailable {
		t.Error("circuit should be closed after success reset")
	}
}

func TestCircuitBreakerState_String(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("State(%d).String() = %s, want %s", tt.state, got, tt.expected)
		}
	}
}
