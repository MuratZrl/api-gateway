package middleware

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

type State int

const (
	StateClosed   State = iota // Normal operation, requests pass through
	StateOpen                  // Circuit is open, requests are rejected
	StateHalfOpen              // Testing if service recovered
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

type circuitBreaker struct {
	// target is the bounded key this breaker was created under. It is set at
	// construction and never written again, so it is safe to read without mu.
	// It doubles as the label value for gateway_circuit_breaker_state.
	target string

	mu                  sync.RWMutex
	state               State
	failures            int
	successes           int
	maxFailures         int
	timeout             time.Duration
	halfOpenMaxRequests int
	lastFailureTime     time.Time
}

// setState transitions the breaker and mirrors the new state into the
// gateway_circuit_breaker_state gauge, so the gauge can never drift from the
// state machine. Callers must already hold cb.mu for writing.
//
// Taking the Prometheus vector's internal lock while holding cb.mu is safe:
// the gauge is a plain Gauge, so Set never calls back into this package and
// the ordering cb.mu -> vector lock is never reversed.
func (cb *circuitBreaker) setState(s State) {
	cb.state = s
	RecordCircuitBreakerState(cb.target, int(s))
}

type CircuitBreakerManager struct {
	breakers            map[string]*circuitBreaker
	mu                  sync.RWMutex
	maxFailures         int
	timeout             time.Duration
	halfOpenMaxRequests int
}

func NewCircuitBreakerManager(maxFailures, timeoutSec, halfOpenMaxRequests int) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers:            make(map[string]*circuitBreaker),
		maxFailures:         maxFailures,
		timeout:             time.Duration(timeoutSec) * time.Second,
		halfOpenMaxRequests: halfOpenMaxRequests,
	}
}

func (m *CircuitBreakerManager) getBreaker(target string) *circuitBreaker {
	m.mu.RLock()
	cb, exists := m.breakers[target]
	m.mu.RUnlock()

	if exists {
		return cb
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double check after acquiring write lock
	if cb, exists = m.breakers[target]; exists {
		return cb
	}

	cb = &circuitBreaker{
		target:              target,
		state:               StateClosed,
		maxFailures:         m.maxFailures,
		timeout:             m.timeout,
		halfOpenMaxRequests: m.halfOpenMaxRequests,
	}
	// Publish the initial state so the series exists before the first failure.
	RecordCircuitBreakerState(target, int(StateClosed))
	m.breakers[target] = cb
	return cb
}

// allowRequest reports whether the request may proceed, and returns the state
// the decision was made under so callers can report it without a second,
// unsynchronized read of cb.state.
func (cb *circuitBreaker) allowRequest() (bool, State) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case StateClosed:
		return true, cb.state
	case StateOpen:
		if time.Since(cb.lastFailureTime) > cb.timeout {
			return true, cb.state // Will transition to half-open
		}
		return false, cb.state
	case StateHalfOpen:
		return cb.successes < cb.halfOpenMaxRequests, cb.state
	}
	return false, cb.state
}

func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateHalfOpen:
		cb.successes++
		if cb.successes >= cb.halfOpenMaxRequests {
			log.Printf("Circuit breaker: half-open -> closed for %s", cb.target)
			cb.setState(StateClosed)
			cb.failures = 0
			cb.successes = 0
		}
	case StateClosed:
		cb.failures = 0
	}
}

func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		if cb.failures >= cb.maxFailures {
			log.Printf("Circuit breaker: closed -> open for %s (failures: %d)", cb.target, cb.failures)
			cb.setState(StateOpen)
		}
	case StateHalfOpen:
		log.Printf("Circuit breaker: half-open -> open for %s", cb.target)
		cb.setState(StateOpen)
		cb.successes = 0
	}
}

type circuitBreakerRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *circuitBreakerRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (m *CircuitBreakerManager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Key the breaker on the normalized route, not on raw request input.
		// r.URL.Host is attacker-controlled whenever a client sends an
		// absolute-form request target, and a raw path lets any scanner mint
		// an unbounded number of breakers and gauge label values.
		target := normalizePath(r.URL.Path)

		cb := m.getBreaker(target)

		// Check if we should transition from open to half-open
		cb.mu.Lock()
		if cb.state == StateOpen && time.Since(cb.lastFailureTime) > cb.timeout {
			log.Printf("Circuit breaker: open -> half-open for %s", target)
			cb.setState(StateHalfOpen)
			cb.successes = 0
		}
		cb.mu.Unlock()

		allowed, state := cb.allowRequest()
		if !allowed {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"error": "service unavailable", "circuit_breaker": "%s"}`, state)
			return
		}

		recorder := &circuitBreakerRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(recorder, r)

		if recorder.statusCode >= 500 {
			cb.recordFailure()
		} else {
			cb.recordSuccess()
		}
	})
}
