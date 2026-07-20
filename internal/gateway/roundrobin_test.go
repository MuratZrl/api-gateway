package gateway

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"api-gateway/internal/config"
)

// newNamedBackend returns a server that identifies itself in the body, so a
// test can tell which upstream actually served a request.
func newNamedBackend(t *testing.T, name string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, name)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestGateway_RoundRobinsAcrossTargets(t *testing.T) {
	backendA := newNamedBackend(t, "A")
	backendB := newNamedBackend(t, "B")

	gw := New([]config.RouteConfig{{
		Path:    "/api/lb",
		Targets: []string{backendA.URL, backendB.URL},
		Methods: []string{http.MethodGet},
	}})

	served := make([]string, 0, 6)
	counts := map[string]int{}
	for i := 0; i < 6; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/lb", nil)
		rec := httptest.NewRecorder()
		gw.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d (%s)", i, rec.Code, rec.Body.String())
		}
		served = append(served, rec.Body.String())
		counts[rec.Body.String()]++
	}

	if counts["A"] != 3 || counts["B"] != 3 {
		t.Errorf("expected an even 3/3 split across targets, got %v (%v)", counts, served)
	}

	// Round-robin is an ordering contract, not just a distribution one: a
	// randomised balancer would satisfy the counts above.
	for i := 1; i < len(served); i++ {
		if served[i] == served[i-1] {
			t.Errorf("expected targets to alternate, got %v", served)
			break
		}
	}
}

func TestGateway_SingleTargetStillWorks(t *testing.T) {
	backend := newNamedBackend(t, "only")

	gw := New([]config.RouteConfig{{
		Path:    "/api/single",
		Target:  backend.URL,
		Methods: []string{http.MethodGet},
	}})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/single", nil)
		rec := httptest.NewRecorder()
		gw.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if rec.Body.String() != "only" {
			t.Errorf("expected body %q, got %q", "only", rec.Body.String())
		}
	}
}

func TestGateway_TargetsTakePrecedenceOverTarget(t *testing.T) {
	fallback := newNamedBackend(t, "fallback")
	preferred := newNamedBackend(t, "preferred")

	gw := New([]config.RouteConfig{{
		Path:    "/api/both",
		Target:  fallback.URL,
		Targets: []string{preferred.URL},
		Methods: []string{http.MethodGet},
	}})

	req := httptest.NewRequest(http.MethodGet, "/api/both", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if got := rec.Body.String(); got != "preferred" {
		t.Errorf("expected the targets list to win, got %q", got)
	}
}

func TestGateway_SurvivesOneDeadTarget(t *testing.T) {
	live := newNamedBackend(t, "live")

	// A closed listener: the first dispatch to it fails and puts it into
	// cooldown, after which every request should land on the live backend.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	gw := New([]config.RouteConfig{{
		Path:    "/api/mixed",
		Targets: []string{deadURL, live.URL},
		Methods: []string{http.MethodGet},
	}})

	failures := 0
	for i := 0; i < 6; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/mixed", nil)
		rec := httptest.NewRecorder()
		gw.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			failures++
		}
	}

	// At most the single probe that discovers the target is down.
	if failures > 1 {
		t.Errorf("expected the dead target to drop out of rotation, got %d failures", failures)
	}
}

func TestGateway_UpdateRoutesRejectsBadTargetWithoutExiting(t *testing.T) {
	backend := newNamedBackend(t, "original")

	gw := New([]config.RouteConfig{{
		Path:    "/api/keep",
		Target:  backend.URL,
		Methods: []string{http.MethodGet},
	}})

	// Reachable from POST /admin/routes; before this returned an error it was
	// a log.Fatalf, i.e. a remote kill switch on the whole gateway.
	err := gw.UpdateRoutes([]config.RouteConfig{{
		Path:    "/api/broken",
		Target:  "://not-a-url",
		Methods: []string{http.MethodGet},
	}})
	if err == nil {
		t.Fatal("expected an invalid target to be rejected")
	}

	route, handler := gw.FindRoute("/api/keep")
	if route == nil || handler == nil {
		t.Fatal("a rejected update must leave the previous routing table in place")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/keep", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)
	if rec.Body.String() != "original" {
		t.Errorf("expected the original route to keep serving, got %q", rec.Body.String())
	}
}

func TestGateway_UpdateRoutesRejectsRouteWithNoTarget(t *testing.T) {
	backend := newNamedBackend(t, "x")
	gw := New([]config.RouteConfig{{
		Path:    "/api/x",
		Target:  backend.URL,
		Methods: []string{http.MethodGet},
	}})

	if err := gw.UpdateRoutes([]config.RouteConfig{{
		Path:    "/api/empty",
		Methods: []string{http.MethodGet},
	}}); err == nil {
		t.Fatal("expected a route with neither target nor targets to be rejected")
	}
}

// TestGateway_ConcurrentUpdateAndServe exercises the atomic table swap. Before
// it, UpdateRoutes wrote g.routes and g.proxies unsynchronized while request
// goroutines read them, which is a fatal "concurrent map read and map write"
// throw. Run under -race for this to be meaningful.
func TestGateway_ConcurrentUpdateAndServe(t *testing.T) {
	backend := newNamedBackend(t, "concurrent")
	routes := []config.RouteConfig{{
		Path:    "/api/concurrent",
		Target:  backend.URL,
		Methods: []string{http.MethodGet},
	}}
	gw := New(routes)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			if err := gw.UpdateRoutes(routes); err != nil {
				return
			}
		}
	}()

	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/concurrent", nil)
		rec := httptest.NewRecorder()
		gw.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("request %d during a route swap: expected 200, got %d", i, rec.Code)
			break
		}
	}

	<-done
}
