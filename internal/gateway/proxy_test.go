package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"api-gateway/internal/config"
)

func TestGateway_FindRoute(t *testing.T) {
	routes := []config.RouteConfig{
		{Path: "/api/users", Target: "http://localhost:8081", Methods: []string{"GET", "POST"}},
		{Path: "/api/products", Target: "http://localhost:8082", Methods: []string{"GET"}},
	}
	gw := New(routes)

	route, proxy := gw.FindRoute("/api/users/123")
	if route == nil {
		t.Fatal("expected to find route for /api/users/123")
	}
	if route.Path != "/api/users" {
		t.Errorf("expected path /api/users, got %s", route.Path)
	}
	if proxy == nil {
		t.Error("expected proxy to be non-nil")
	}
}

func TestGateway_FindRoute_NotFound(t *testing.T) {
	routes := []config.RouteConfig{
		{Path: "/api/users", Target: "http://localhost:8081", Methods: []string{"GET"}},
	}
	gw := New(routes)

	route, _ := gw.FindRoute("/api/unknown")
	if route != nil {
		t.Error("expected nil for unknown route")
	}
}

func TestGateway_ReturnsNotFoundForUnknownPath(t *testing.T) {
	routes := []config.RouteConfig{
		{Path: "/api/users", Target: "http://localhost:8081", Methods: []string{"GET"}},
	}
	gw := New(routes)

	req := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
	rec := httptest.NewRecorder()

	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown path, got %d", rec.Code)
	}
}

func TestGateway_ReturnsMethodNotAllowed(t *testing.T) {
	routes := []config.RouteConfig{
		{Path: "/api/users", Target: "http://localhost:8081", Methods: []string{"GET"}},
	}
	gw := New(routes)

	req := httptest.NewRequest(http.MethodDelete, "/api/users", nil)
	rec := httptest.NewRecorder()

	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for disallowed method, got %d", rec.Code)
	}
}
