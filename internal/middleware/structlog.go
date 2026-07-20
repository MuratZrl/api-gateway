package middleware

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

type LogEntry struct {
	Timestamp  string `json:"timestamp"`
	Level      string `json:"level"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	StatusCode int    `json:"status_code"`
	DurationMs int64  `json:"duration_ms"`
	ClientIP   string `json:"client_ip"`
	UserAgent  string `json:"user_agent"`
	RequestID  string `json:"request_id,omitempty"`
	TraceID    string `json:"trace_id,omitempty"`
	BytesSent  int    `json:"bytes_sent"`
	Error      string `json:"error,omitempty"`
}

type StructuredLogger struct {
	writer io.Writer
}

func NewStructuredLogger() *StructuredLogger {
	return &StructuredLogger{
		writer: os.Stdout,
	}
}

type structLogResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (w *structLogResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *structLogResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += n
	return n, err
}

func (sl *StructuredLogger) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		recorder := &structLogResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(recorder, r)

		duration := time.Since(start).Milliseconds()

		clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}

		level := "info"
		if recorder.statusCode >= 500 {
			level = "error"
		} else if recorder.statusCode >= 400 {
			level = "warn"
		}

		entry := LogEntry{
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Level:      level,
			Method:     r.Method,
			Path:       r.URL.Path,
			StatusCode: recorder.statusCode,
			DurationMs: duration,
			ClientIP:   clientIP,
			UserAgent:  r.UserAgent(),
			RequestID:  r.Header.Get("X-Request-ID"),
			TraceID:    r.Header.Get("X-Trace-ID"),
			BytesSent:  recorder.bytesWritten,
		}

		jsonBytes, err := json.Marshal(entry)
		if err == nil {
			_, _ = sl.writer.Write(jsonBytes)
			_, _ = sl.writer.Write([]byte("\n"))
		}
	})
}

// LogInfo writes a structured info log
func LogInfo(message string, fields map[string]interface{}) {
	entry := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"level":     "info",
		"message":   message,
	}
	for k, v := range fields {
		entry[k] = v
	}
	jsonBytes, _ := json.Marshal(entry)
	os.Stdout.Write(jsonBytes)
	os.Stdout.Write([]byte("\n"))
}

// LogError writes a structured error log
func LogError(message string, err error, fields map[string]interface{}) {
	entry := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"level":     "error",
		"message":   message,
		"error":     err.Error(),
	}
	for k, v := range fields {
		entry[k] = v
	}
	jsonBytes, _ := json.Marshal(entry)
	os.Stderr.Write(jsonBytes)
	os.Stderr.Write([]byte("\n"))
}
