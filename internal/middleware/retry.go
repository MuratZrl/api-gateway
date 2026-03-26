package middleware

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"time"
)

type RetryConfig struct {
	MaxRetries  int           `yaml:"max_retries" json:"max_retries"`   // Maximum number of retries
	InitialWait time.Duration `yaml:"initial_wait" json:"initial_wait"` // Wait before first retry
	MaxWait     time.Duration `yaml:"max_wait" json:"max_wait"`         // Maximum wait between retries
	Multiplier  float64       `yaml:"multiplier" json:"multiplier"`     // Wait time multiplier (exponential backoff)
}

func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:  3,
		InitialWait: 100 * time.Millisecond,
		MaxWait:     2 * time.Second,
		Multiplier:  2.0,
	}
}

type retryResponseWriter struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
	headers    http.Header
}

func (w *retryResponseWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func (w *retryResponseWriter) WriteHeader(code int) {
	w.statusCode = code
}

func (w *retryResponseWriter) Header() http.Header {
	return w.headers
}

func Retry(cfg *RetryConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only retry idempotent methods
			if r.Method != http.MethodGet && r.Method != http.MethodHead &&
				r.Method != http.MethodOptions && r.Method != http.MethodPut &&
				r.Method != http.MethodDelete {
				next.ServeHTTP(w, r)
				return
			}

			// Buffer the body for potential retries
			var bodyBytes []byte
			if r.Body != nil {
				bodyBytes, _ = io.ReadAll(r.Body)
				r.Body.Close()
			}

			wait := cfg.InitialWait

			for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
				// Restore body for each attempt
				if bodyBytes != nil {
					r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				}

				recorder := &retryResponseWriter{
					ResponseWriter: w,
					body:           &bytes.Buffer{},
					statusCode:     http.StatusOK,
					headers:        make(http.Header),
				}

				next.ServeHTTP(recorder, r)

				// Success or client error — don't retry
				if recorder.statusCode < 500 {
					// Copy headers to actual response
					for key, values := range recorder.headers {
						for _, v := range values {
							w.Header().Add(key, v)
						}
					}
					w.WriteHeader(recorder.statusCode)
					w.Write(recorder.body.Bytes())
					return
				}

				// Server error — retry if attempts remain
				if attempt < cfg.MaxRetries {
					log.Printf("Retry attempt %d/%d for %s %s (status: %d)",
						attempt+1, cfg.MaxRetries, r.Method, r.URL.Path, recorder.statusCode)
					time.Sleep(wait)

					// Exponential backoff
					wait = time.Duration(float64(wait) * cfg.Multiplier)
					if wait > cfg.MaxWait {
						wait = cfg.MaxWait
					}
					continue
				}

				// All retries exhausted
				log.Printf("All %d retries exhausted for %s %s", cfg.MaxRetries, r.Method, r.URL.Path)
				for key, values := range recorder.headers {
					for _, v := range values {
						w.Header().Add(key, v)
					}
				}
				w.WriteHeader(recorder.statusCode)
				w.Write(recorder.body.Bytes())
			}
		})
	}
}
