package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

type TransformRule struct {
	// Headers to add to the request before forwarding
	AddRequestHeaders map[string]string `yaml:"add_request_headers" json:"add_request_headers"`
	// Headers to remove from the request before forwarding
	RemoveRequestHeaders []string `yaml:"remove_request_headers" json:"remove_request_headers"`
	// Headers to add to the response before returning to client
	AddResponseHeaders map[string]string `yaml:"add_response_headers" json:"add_response_headers"`
	// Headers to remove from the response before returning to client
	RemoveResponseHeaders []string `yaml:"remove_response_headers" json:"remove_response_headers"`
	// Fields to add to the JSON request body
	AddBodyFields map[string]interface{} `yaml:"add_body_fields" json:"add_body_fields"`
	// Fields to remove from the JSON request body
	RemoveBodyFields []string `yaml:"remove_body_fields" json:"remove_body_fields"`
}

type transformResponseWriter struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
	rule       *TransformRule
}

func (w *transformResponseWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func (w *transformResponseWriter) WriteHeader(code int) {
	w.statusCode = code
}

func Transform(rules map[string]*TransformRule) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Find matching rule by path prefix
			var rule *TransformRule
			for path, tr := range rules {
				if len(r.URL.Path) >= len(path) && r.URL.Path[:len(path)] == path {
					rule = tr
					break
				}
			}

			if rule == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Transform request headers
			for key, value := range rule.AddRequestHeaders {
				r.Header.Set(key, value)
			}
			for _, key := range rule.RemoveRequestHeaders {
				r.Header.Del(key)
			}

			// Transform request body (only for POST/PUT/PATCH with JSON)
			if (r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch) &&
				r.Body != nil && (len(rule.AddBodyFields) > 0 || len(rule.RemoveBodyFields) > 0) {

				bodyBytes, err := io.ReadAll(r.Body)
				r.Body.Close()
				if err == nil && len(bodyBytes) > 0 {
					var bodyMap map[string]interface{}
					if json.Unmarshal(bodyBytes, &bodyMap) == nil {
						for key, value := range rule.AddBodyFields {
							bodyMap[key] = value
						}
						for _, key := range rule.RemoveBodyFields {
							delete(bodyMap, key)
						}
						if newBody, err := json.Marshal(bodyMap); err == nil {
							bodyBytes = newBody
						}
					}
				}
				r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				r.ContentLength = int64(len(bodyBytes))
			}

			// Capture response for transformation
			recorder := &transformResponseWriter{
				ResponseWriter: w,
				body:           &bytes.Buffer{},
				statusCode:     http.StatusOK,
				rule:           rule,
			}

			next.ServeHTTP(recorder, r)

			// Transform response headers
			for key, value := range rule.AddResponseHeaders {
				w.Header().Set(key, value)
			}
			for _, key := range rule.RemoveResponseHeaders {
				w.Header().Del(key)
			}

			w.WriteHeader(recorder.statusCode)
			w.Write(recorder.body.Bytes())
		})
	}
}
