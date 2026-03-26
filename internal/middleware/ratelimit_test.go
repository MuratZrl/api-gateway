package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func newTestRedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: "localhost:6380",
	})
}

func flushTestRedis(t *testing.T) {
	t.Helper()
	client := newTestRedisClient()
	client.FlushDB(context.Background())
}

func TestRateLimiter_AllowsRequestsUnderLimit(t *testing.T) {
	flushTestRedis(t)
	client := newTestRedisClient()
	rl := NewRateLimiter(client, 100)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ip := fmt.Sprintf("192.168.%d.%d:12345", time.Now().Unix()%255, time.Now().UnixNano()%255)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = ip
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	if rec.Header().Get("X-RateLimit-Limit") == "" {
		t.Error("expected X-RateLimit-Limit header")
	}
}

func TestRateLimiter_BlocksWhenLimitExceeded(t *testing.T) {
	flushTestRedis(t)
	client := newTestRedisClient()
	limit := 5
	rl := NewRateLimiter(client, limit)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ip := fmt.Sprintf("10.%d.%d.99:12345", time.Now().Unix()%255, time.Now().UnixNano()%255)

	for i := 0; i < limit+2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test-block", nil)
		req.RemoteAddr = ip
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if i < limit && rec.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, rec.Code)
		}
		if i >= limit && rec.Code != http.StatusTooManyRequests {
			t.Errorf("request %d: expected 429, got %d", i, rec.Code)
		}
	}
}
