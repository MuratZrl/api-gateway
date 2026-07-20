package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewCache(client *redis.Client, ttlSeconds int) *Cache {
	return &Cache{
		client: client,
		ttl:    time.Duration(ttlSeconds) * time.Second,
	}
}

type cacheResponseWriter struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (w *cacheResponseWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *cacheResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (c *Cache) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only cache GET requests
		if r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}

		// Don't cache admin endpoints, nor the gateway's own operational
		// endpoints. Caching /metrics hands Prometheus a stale exposition
		// payload for a full TTL and makes every scrape increment the cache
		// counters, which would poison the very metrics served there.
		if strings.HasPrefix(r.URL.Path, "/admin") || r.URL.Path == pathMetrics || r.URL.Path == pathHealth {
			next.ServeHTTP(w, r)
			return
		}

		// Generate cache key from method + path + query
		hash := sha256.Sum256([]byte(r.Method + r.URL.RequestURI()))
		cacheKey := fmt.Sprintf("cache:%x", hash)

		ctx := context.Background()

		// Try to get from cache
		cached, err := c.client.Get(ctx, cacheKey).Bytes()
		if err == nil {
			RecordCacheHit()
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.Write(cached)
			return
		}

		// A Redis error is counted as a miss: the request is served from the
		// upstream either way, which is what the hit-rate ratio measures.
		RecordCacheMiss()

		// Cache miss — forward request and capture response
		recorder := &cacheResponseWriter{
			ResponseWriter: w,
			body:           &bytes.Buffer{},
			statusCode:     http.StatusOK,
		}

		recorder.Header().Set("X-Cache", "MISS")
		next.ServeHTTP(recorder, r)

		// Only cache successful responses
		if recorder.statusCode == http.StatusOK {
			c.client.Set(ctx, cacheKey, recorder.body.Bytes(), c.ttl)
		}
	})
}
