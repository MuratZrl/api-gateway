package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

var requestCounter atomic.Int64

type RateLimiter struct {
	client           *redis.Client
	requestsPerMinute int
}

func NewRateLimiter(client *redis.Client, requestsPerMinute int) *RateLimiter {
	return &RateLimiter{
		client:           client,
		requestsPerMinute: requestsPerMinute,
	}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}
		key := fmt.Sprintf("rate_limit:%s", clientIP)

		ctx := context.Background()
		now := time.Now().Unix()
		windowStart := now - 60

		pipe := rl.client.Pipeline()

		// Remove old entries outside the window
		pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart))

		// Count current requests in window
		countCmd := pipe.ZCard(ctx, key)

		// Add current request with unique member
		uniqueID := requestCounter.Add(1)
		pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: fmt.Sprintf("%d:%d", now, uniqueID)})

		// Set expiry on the key
		pipe.Expire(ctx, key, time.Minute)

		_, err := pipe.Exec(ctx)
		if err != nil {
			// If Redis fails, allow the request
			next.ServeHTTP(w, r)
			return
		}

		count := countCmd.Val()
		remaining := int64(rl.requestsPerMinute) - count

		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rl.requestsPerMinute))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", max(0, remaining)))
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", now+60))

		if count >= int64(rl.requestsPerMinute) {
			RecordRateLimitRejection()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "rate limit exceeded", "retry_after": 60}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}
