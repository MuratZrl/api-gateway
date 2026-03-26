package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTransform_AddsRequestHeaders(t *testing.T) {
	rules := map[string]*TransformRule{
		"/api/": {
			AddRequestHeaders: map[string]string{
				"X-Custom": "test-value",
			},
		},
	}

	var capturedHeader string
	handler := Transform(rules)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get("X-Custom")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if capturedHeader != "test-value" {
		t.Errorf("expected X-Custom: test-value, got %s", capturedHeader)
	}
}

func TestTransform_RemovesRequestHeaders(t *testing.T) {
	rules := map[string]*TransformRule{
		"/api/": {
			RemoveRequestHeaders: []string{"X-Remove-Me"},
		},
	}

	var headerPresent bool
	handler := Transform(rules)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headerPresent = r.Header.Get("X-Remove-Me") != ""
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Remove-Me", "should-be-gone")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if headerPresent {
		t.Error("expected X-Remove-Me header to be removed")
	}
}

func TestTransform_AddsResponseHeaders(t *testing.T) {
	rules := map[string]*TransformRule{
		"/api/": {
			AddResponseHeaders: map[string]string{
				"X-Powered-By": "Test Gateway",
			},
		},
	}

	handler := Transform(rules)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get("X-Powered-By") != "Test Gateway" {
		t.Errorf("expected X-Powered-By: Test Gateway, got %s", rec.Header().Get("X-Powered-By"))
	}
}

func TestTransform_AddsBodyFields(t *testing.T) {
	rules := map[string]*TransformRule{
		"/api/": {
			AddBodyFields: map[string]interface{}{
				"source": "gateway",
			},
		},
	}

	var capturedBody map[string]interface{}
	handler := Transform(rules)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		w.WriteHeader(http.StatusOK)
	}))

	body := `{"name": "test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/test", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if capturedBody["source"] != "gateway" {
		t.Errorf("expected source: gateway in body, got %v", capturedBody["source"])
	}
	if capturedBody["name"] != "test" {
		t.Errorf("expected name: test preserved in body, got %v", capturedBody["name"])
	}
}

func TestTransform_NoRuleMatchPassesThrough(t *testing.T) {
	rules := map[string]*TransformRule{
		"/api/": {
			AddResponseHeaders: map[string]string{"X-Test": "value"},
		},
	}

	handler := Transform(rules)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-Test") != "" {
		t.Error("non-matching path should not get transform headers")
	}
}
