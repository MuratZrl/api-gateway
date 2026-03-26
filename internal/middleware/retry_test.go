package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRetry_NoRetryOnSuccess(t *testing.T) {
	cfg := &RetryConfig{
		MaxRetries:  3,
		InitialWait: 10 * time.Millisecond,
		MaxWait:     100 * time.Millisecond,
		Multiplier:  2.0,
	}

	callCount := 0
	handler := Retry(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if callCount != 1 {
		t.Errorf("expected 1 call on success, got %d", callCount)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRetry_RetriesOnServerError(t *testing.T) {
	cfg := &RetryConfig{
		MaxRetries:  3,
		InitialWait: 10 * time.Millisecond,
		MaxWait:     100 * time.Millisecond,
		Multiplier:  2.0,
	}

	callCount := 0
	handler := Retry(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("error"))
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if callCount != 3 {
		t.Errorf("expected 3 calls (2 retries + success), got %d", callCount)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 after retry success, got %d", rec.Code)
	}
}

func TestRetry_ExhaustsAllRetries(t *testing.T) {
	cfg := &RetryConfig{
		MaxRetries:  2,
		InitialWait: 10 * time.Millisecond,
		MaxWait:     100 * time.Millisecond,
		Multiplier:  2.0,
	}

	callCount := 0
	handler := Retry(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// 1 original + 2 retries = 3
	if callCount != 3 {
		t.Errorf("expected 3 calls (1 + 2 retries), got %d", callCount)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 after all retries exhausted, got %d", rec.Code)
	}
}

func TestRetry_NoRetryOnPOST(t *testing.T) {
	cfg := &RetryConfig{
		MaxRetries:  3,
		InitialWait: 10 * time.Millisecond,
		MaxWait:     100 * time.Millisecond,
		Multiplier:  2.0,
	}

	callCount := 0
	handler := Retry(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if callCount != 1 {
		t.Errorf("POST should not retry, expected 1 call, got %d", callCount)
	}
}

func TestRetry_NoRetryOnClientError(t *testing.T) {
	cfg := &RetryConfig{
		MaxRetries:  3,
		InitialWait: 10 * time.Millisecond,
		MaxWait:     100 * time.Millisecond,
		Multiplier:  2.0,
	}

	callCount := 0
	handler := Retry(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusBadRequest)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if callCount != 1 {
		t.Errorf("client errors should not retry, expected 1 call, got %d", callCount)
	}
}
