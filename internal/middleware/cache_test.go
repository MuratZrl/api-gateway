package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCache_MissOnFirstRequest(t *testing.T) {
	flushTestRedis(t)
	client := newTestRedisClient()
	cache := NewCache(client, 30)

	callCount := 0
	handler := cache.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data": "test"}`))
	}))

	path := fmt.Sprintf("/api/cache-miss-%d", time.Now().UnixNano())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get("X-Cache") != "MISS" {
		t.Errorf("expected X-Cache: MISS, got %s", rec.Header().Get("X-Cache"))
	}
	if callCount != 1 {
		t.Errorf("expected handler to be called once, got %d", callCount)
	}
}

func TestCache_HitOnSecondRequest(t *testing.T) {
	flushTestRedis(t)
	client := newTestRedisClient()
	cache := NewCache(client, 30)

	callCount := 0
	handler := cache.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data": "cached"}`))
	}))

	path := fmt.Sprintf("/api/cache-hit-%d", time.Now().UnixNano())

	// First request - MISS
	req1 := httptest.NewRequest(http.MethodGet, path, nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	// Second request - HIT
	req2 := httptest.NewRequest(http.MethodGet, path, nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Header().Get("X-Cache") != "HIT" {
		t.Errorf("expected X-Cache: HIT on second request, got %s", rec2.Header().Get("X-Cache"))
	}
	if callCount != 1 {
		t.Errorf("expected handler to be called once (cached), got %d", callCount)
	}
}

func TestCache_SkipsPostRequests(t *testing.T) {
	client := newTestRedisClient()
	cache := NewCache(client, 30)

	handler := cache.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/cache-post-test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 for POST (skip cache), got %d", rec.Code)
	}
	if rec.Header().Get("X-Cache") != "" {
		t.Error("POST requests should not have X-Cache header")
	}
}

func TestCache_SkipsAdminRoutes(t *testing.T) {
	client := newTestRedisClient()
	cache := NewCache(client, 30)

	handler := cache.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get("X-Cache") != "" {
		t.Error("admin routes should not be cached")
	}
}
