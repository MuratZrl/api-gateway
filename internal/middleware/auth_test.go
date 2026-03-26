package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"api-gateway/internal/config"
	"api-gateway/internal/gateway"

	"github.com/golang-jwt/jwt/v5"
)

func newTestGateway() *gateway.Gateway {
	routes := []config.RouteConfig{
		{Path: "/api/public", Target: "http://localhost:9999", Methods: []string{"GET"}, Protected: false},
		{Path: "/api/private", Target: "http://localhost:9999", Methods: []string{"GET"}, Protected: true},
	}
	return gateway.New(routes)
}

func TestAuth_AllowsPublicRoutes(t *testing.T) {
	gw := newTestGateway()
	auth := NewAuth("test-secret", gw, nil)

	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/public", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for public route, got %d", rec.Code)
	}
}

func TestAuth_BlocksProtectedWithoutToken(t *testing.T) {
	gw := newTestGateway()
	auth := NewAuth("test-secret", gw, nil)

	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/private", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for protected route without token, got %d", rec.Code)
	}
}

func TestAuth_AllowsValidJWT(t *testing.T) {
	secret := "test-secret"
	gw := newTestGateway()
	auth := NewAuth(secret, gw, nil)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": "1",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	tokenString, _ := token.SignedString([]byte(secret))

	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/private", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with valid JWT, got %d", rec.Code)
	}
}

func TestAuth_RejectsExpiredJWT(t *testing.T) {
	secret := "test-secret"
	gw := newTestGateway()
	auth := NewAuth(secret, gw, nil)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": "1",
		"exp":     time.Now().Add(-time.Hour).Unix(),
	})
	tokenString, _ := token.SignedString([]byte(secret))

	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/private", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with expired JWT, got %d", rec.Code)
	}
}

func TestAuth_RejectsInvalidFormat(t *testing.T) {
	gw := newTestGateway()
	auth := NewAuth("test-secret", gw, nil)

	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/private", nil)
	req.Header.Set("Authorization", "InvalidFormat token123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with invalid format, got %d", rec.Code)
	}
}
