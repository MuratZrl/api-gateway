package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIPFilter_DisabledMode(t *testing.T) {
	f := NewIPFilter(ModeDisabled, nil, nil)

	handler := f.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 in disabled mode, got %d", rec.Code)
	}
}

func TestIPFilter_WhitelistAllows(t *testing.T) {
	f := NewIPFilter(ModeWhitelist, []string{"10.0.0.1"}, nil)

	handler := f.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for whitelisted IP, got %d", rec.Code)
	}
}

func TestIPFilter_WhitelistBlocks(t *testing.T) {
	f := NewIPFilter(ModeWhitelist, []string{"10.0.0.1"}, nil)

	handler := f.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.2:1234"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-whitelisted IP, got %d", rec.Code)
	}
}

func TestIPFilter_BlacklistBlocks(t *testing.T) {
	f := NewIPFilter(ModeBlacklist, nil, []string{"192.168.1.100"})

	handler := f.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:1234"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for blacklisted IP, got %d", rec.Code)
	}
}

func TestIPFilter_BlacklistAllows(t *testing.T) {
	f := NewIPFilter(ModeBlacklist, nil, []string{"192.168.1.100"})

	handler := f.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.200:1234"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for non-blacklisted IP, got %d", rec.Code)
	}
}

func TestIPFilter_AddRemove(t *testing.T) {
	f := NewIPFilter(ModeBlacklist, nil, nil)

	f.AddToBlacklist("1.2.3.4")

	handler := f.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 after AddToBlacklist, got %d", rec.Code)
	}

	f.RemoveFromBlacklist("1.2.3.4")

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)

	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200 after RemoveFromBlacklist, got %d", rec2.Code)
	}
}
