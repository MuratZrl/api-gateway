package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestValidator() *RequestValidator {
	minPrice := 0.0
	maxPrice := 1000.0
	return NewRequestValidator(map[string]*ValidationSchema{
		"POST /api/users": {
			Fields: map[string]FieldRule{
				"name":  {Type: "string", Required: true, MinLen: 2, MaxLen: 50},
				"email": {Type: "string", Required: true, MinLen: 5},
			},
		},
		"POST /api/products": {
			Fields: map[string]FieldRule{
				"name":  {Type: "string", Required: true},
				"price": {Type: "number", Required: true, Min: &minPrice, Max: &maxPrice},
			},
		},
	})
}

func TestValidation_PassesValidRequest(t *testing.T) {
	v := newTestValidator()
	handler := v.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	body := `{"name": "Test User", "email": "test@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for valid request, got %d", rec.Code)
	}
}

func TestValidation_RejectsRequiredFieldMissing(t *testing.T) {
	v := newTestValidator()
	handler := v.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	body := `{"email": "test@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing required field, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "validation failed" {
		t.Errorf("expected 'validation failed' error, got %v", resp["error"])
	}
}

func TestValidation_RejectsShortString(t *testing.T) {
	v := newTestValidator()
	handler := v.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	body := `{"name": "A", "email": "test@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for too short string, got %d", rec.Code)
	}
}

func TestValidation_RejectsWrongType(t *testing.T) {
	v := newTestValidator()
	handler := v.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	body := `{"name": 123, "email": "test@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for wrong type, got %d", rec.Code)
	}
}

func TestValidation_RejectsNumberOutOfRange(t *testing.T) {
	v := newTestValidator()
	handler := v.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	body := `{"name": "Laptop", "price": 5000}`
	req := httptest.NewRequest(http.MethodPost, "/api/products", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for number out of range, got %d", rec.Code)
	}
}

func TestValidation_SkipsGETRequests(t *testing.T) {
	v := newTestValidator()
	handler := v.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for GET request (skip validation), got %d", rec.Code)
	}
}

func TestValidation_RejectsInvalidJSON(t *testing.T) {
	v := newTestValidator()
	handler := v.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString("not json"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", rec.Code)
	}
}
