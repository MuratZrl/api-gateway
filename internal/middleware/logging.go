package middleware

import (
	"context"
	"log"
	"net/http"
	"time"

	"api-gateway/internal/models"
	"api-gateway/internal/repository"
)

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func Logging(repo *repository.MongoRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(recorder, r)

			duration := time.Since(start).Milliseconds()

			log.Printf("[%s] %s %s - %d (%dms)",
				r.RemoteAddr, r.Method, r.URL.Path, recorder.statusCode, duration)

			// Save to MongoDB asynchronously
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				reqLog := &models.RequestLog{
					Method:     r.Method,
					Path:       r.URL.Path,
					StatusCode: recorder.statusCode,
					Duration:   duration,
					ClientIP:   r.RemoteAddr,
					UserAgent:  r.UserAgent(),
				}
				if err := repo.InsertLog(ctx, reqLog); err != nil {
					log.Printf("Failed to save request log: %v", err)
				}
			}()
		})
	}
}
