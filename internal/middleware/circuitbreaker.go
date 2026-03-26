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
	mu                  sync.RWMutex
	state               State
	failures            int
	successes           int
	maxFailures         int
	timeout             time.Duration
	halfOpenMaxRequests int
	lastFailureTime     time.Time
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
		state:               StateClosed,
		maxFailures:         m.maxFailures,
		timeout:             m.timeout,
		halfOpenMaxRequests: m.halfOpenMaxRequests,
	}
	m.breakers[target] = cb
	return cb
}

func (cb *circuitBreaker) allowRequest() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(cb.lastFailureTime) > cb.timeout {
			return true // Will transition to half-open
		}
		return false
	case StateHalfOpen:
		return cb.successes < cb.halfOpenMaxRequests
	}
	return false
}

func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateHalfOpen:
		cb.successes++
		if cb.successes >= cb.halfOpenMaxRequests {
			log.Printf("Circuit breaker: half-open -> closed")
			cb.state = StateClosed
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
			log.Printf("Circuit breaker: closed -> open (failures: %d)", cb.failures)
			cb.state = StateOpen
		}
	case StateHalfOpen:
		log.Printf("Circuit breaker: half-open -> open")
		cb.state = StateOpen
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
		// Use the host as the circuit breaker key
		target := r.URL.Host
		if target == "" {
			target = r.URL.Path
		}

		cb := m.getBreaker(target)

		// Check if we should transition from open to half-open
		cb.mu.Lock()
		if cb.state == StateOpen && time.Since(cb.lastFailureTime) > cb.timeout {
			log.Printf("Circuit breaker: open -> half-open for %s", target)
			cb.state = StateHalfOpen
			cb.successes = 0
		}
		cb.mu.Unlock()

		if !cb.allowRequest() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(fmt.Sprintf(`{"error": "service unavailable", "circuit_breaker": "%s"}`, cb.state)))
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
