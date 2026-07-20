package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const gatewayURL = "http://localhost:8080"

func waitForGateway(t *testing.T) {
	t.Helper()
	for i := 0; i < 30; i++ {
		resp, err := http.Get(gatewayURL + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("gateway did not become ready within 30 seconds")
}

func TestIntegration_HealthCheck(t *testing.T) {
	waitForGateway(t)

	resp, err := http.Get(gatewayURL + "/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %s", body["status"])
	}
}

func TestIntegration_ProxyToUserService(t *testing.T) {
	waitForGateway(t)

	resp, err := http.Get(gatewayURL + "/api/users")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var users []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&users)
	if len(users) == 0 {
		t.Error("expected at least one user")
	}
}

func TestIntegration_ProxyToProductService(t *testing.T) {
	waitForGateway(t)

	resp, err := http.Get(gatewayURL + "/api/products")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var products []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&products)
	if len(products) == 0 {
		t.Error("expected at least one product")
	}
}

// TestIntegration_LoadBalancedRouteServes exercises the load-balancer dispatch
// path in the deployed binary: /api/users is declared with a `targets` list in
// configs/gateway.yaml, so every request here goes through LoadBalancer.
// NextTarget rather than a bare single-host proxy.
//
// Distribution across two *distinct* backends is asserted deterministically in
// internal/gateway (TestGateway_RoundRobinsAcrossTargets); it is not observable
// here, because the compose stack runs one replica per service and the
// responses carry no upstream identity. Round-robin over N targets that all
// resolve to the same container is indistinguishable from a single target.
func TestIntegration_LoadBalancedRouteServes(t *testing.T) {
	waitForGateway(t)

	// A broken multi-target dispatch surfaces as intermittent 502s rather than
	// a consistent failure, so probe repeatedly with cache-busting queries.
	for i := 0; i < 5; i++ {
		path := fmt.Sprintf("/api/users?_lb_test=%d_%d", time.Now().UnixNano(), i)
		resp, err := http.Get(gatewayURL + path)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("request %d: expected 200 from load-balanced route, got %d (%s)", i, resp.StatusCode, body)
		}

		var users []map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&users)
		resp.Body.Close()

		if len(users) == 0 {
			t.Errorf("request %d: expected at least one user from the load-balanced route", i)
		}
	}
}

func TestIntegration_UnknownRouteReturns404(t *testing.T) {
	waitForGateway(t)

	resp, err := http.Get(gatewayURL + "/api/nonexistent")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown route, got %d", resp.StatusCode)
	}
}

func TestIntegration_CacheHeaders(t *testing.T) {
	waitForGateway(t)

	// Unique path to avoid cache pollution from other tests
	path := fmt.Sprintf("/api/users?_cache_test=%d", time.Now().UnixNano())

	// First request - MISS
	resp1, err := http.Get(gatewayURL + path)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	resp1.Body.Close()

	if resp1.Header.Get("X-Cache") != "MISS" {
		t.Errorf("expected X-Cache: MISS on first request, got %s", resp1.Header.Get("X-Cache"))
	}

	// Second request - HIT
	resp2, err := http.Get(gatewayURL + path)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	resp2.Body.Close()

	if resp2.Header.Get("X-Cache") != "HIT" {
		t.Errorf("expected X-Cache: HIT on second request, got %s", resp2.Header.Get("X-Cache"))
	}
}

func TestIntegration_RateLimitHeaders(t *testing.T) {
	waitForGateway(t)

	resp, err := http.Get(gatewayURL + "/api/users")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("X-RateLimit-Limit") == "" {
		t.Error("expected X-RateLimit-Limit header")
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "" {
		t.Error("expected X-RateLimit-Remaining header")
	}
}

func TestIntegration_TransformHeaders(t *testing.T) {
	waitForGateway(t)

	// Use unique query to avoid cache hit
	path := fmt.Sprintf("/api/users?_transform_test=%d", time.Now().UnixNano())
	resp, err := http.Get(gatewayURL + path)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("X-Powered-By") != "API Gateway" {
		t.Errorf("expected X-Powered-By: API Gateway, got %s", resp.Header.Get("X-Powered-By"))
	}
}

func TestIntegration_ValidationRejectsInvalidBody(t *testing.T) {
	waitForGateway(t)

	// Missing required "name" field
	body := `{"email": "test@example.com"}`
	resp, err := http.Post(gatewayURL+"/api/users", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid body, got %d", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(respBody), "validation failed") {
		t.Error("expected validation error message")
	}
}

func TestIntegration_ValidationAcceptsValidBody(t *testing.T) {
	waitForGateway(t)

	body := `{"name": "Integration Test User", "email": "integration@test.com"}`
	resp, err := http.Post(gatewayURL+"/api/users", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201 for valid body, got %d", resp.StatusCode)
	}
}

func TestIntegration_AdminStats(t *testing.T) {
	waitForGateway(t)

	// Make a request first to ensure stats exist
	http.Get(gatewayURL + "/api/users")

	resp, err := http.Get(gatewayURL + "/admin/stats")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for admin stats, got %d", resp.StatusCode)
	}
}

func TestIntegration_CreateAPIKey(t *testing.T) {
	waitForGateway(t)

	body := `{"name": "integration-test-key"}`
	resp, err := http.Post(gatewayURL+"/admin/keys", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}

	var apiKey map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&apiKey)
	if apiKey["key"] == nil || apiKey["key"] == "" {
		t.Error("expected API key to be generated")
	}
}

func TestIntegration_GenerateJWTToken(t *testing.T) {
	waitForGateway(t)

	body := `{"user_id": "1", "role": "admin"}`
	resp, err := http.Post(gatewayURL+"/admin/token", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var tokenResp map[string]string
	json.NewDecoder(resp.Body).Decode(&tokenResp)
	if tokenResp["token"] == "" {
		t.Error("expected token in response")
	}
}
