package gateway

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	"api-gateway/internal/config"
)

// routingTable is an immutable snapshot of the routes and the handlers that
// serve them. It is swapped atomically, so a request either sees the whole old
// table or the whole new one, and the old table stays alive until its last
// in-flight reader is finished with it.
type routingTable struct {
	routes []config.RouteConfig

	// handlers is index-parallel to routes. Keying by path instead would
	// collapse two routes that share a path onto a single handler, pairing one
	// route's methods with another route's upstream.
	handlers []http.Handler
}

type Gateway struct {
	table atomic.Pointer[routingTable]
}

// routeTargets resolves the upstreams for a route. A targets list wins when
// present; a lone target keeps the original single-upstream schema working, so
// existing configs and admin-created routes behave exactly as before.
func routeTargets(route config.RouteConfig) ([]string, error) {
	targets := route.Targets
	if len(targets) == 0 && route.Target != "" {
		targets = []string{route.Target}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("route %q: no target and no targets", route.Path)
	}

	for _, t := range targets {
		u, err := url.Parse(t)
		if err != nil {
			return nil, fmt.Errorf("route %q: invalid target %q: %w", route.Path, t, err)
		}
		if u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("route %q: target %q needs a scheme and a host", route.Path, t)
		}
	}

	return targets, nil
}

func buildRoutingTable(routes []config.RouteConfig) (*routingTable, error) {
	snapshot := make([]config.RouteConfig, len(routes))
	copy(snapshot, routes)

	handlers := make([]http.Handler, len(snapshot))
	for i, route := range snapshot {
		targets, err := routeTargets(route)
		if err != nil {
			return nil, err
		}
		// A single-target route is just a load balancer with one target, which
		// keeps one code path for dispatch, error handling and health state.
		handlers[i] = NewLoadBalancer(targets)
	}

	return &routingTable{routes: snapshot, handlers: handlers}, nil
}

func New(routes []config.RouteConfig) *Gateway {
	table, err := buildRoutingTable(routes)
	if err != nil {
		// Startup only: a broken static config should fail loudly and early.
		log.Fatalf("gateway: %v", err)
	}

	g := &Gateway{}
	g.table.Store(table)
	return g
}

// FindRoute returns the first route whose path prefixes the request path,
// together with the handler serving it. The returned RouteConfig is a shallow
// copy; its slice fields share backing arrays with the live table, so callers
// must treat them as read-only.
func (g *Gateway) FindRoute(path string) (*config.RouteConfig, http.Handler) {
	table := g.table.Load()
	if table == nil {
		return nil, nil
	}

	for i := range table.routes {
		if strings.HasPrefix(path, table.routes[i].Path) {
			route := table.routes[i]
			return &route, table.handlers[i]
		}
	}
	return nil, nil
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route, handler := g.FindRoute(r.URL.Path)
	if route == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "route not found"}`))
		return
	}

	// Check if HTTP method is allowed
	methodAllowed := false
	for _, m := range route.Methods {
		if m == r.Method {
			methodAllowed = true
			break
		}
	}
	if !methodAllowed {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte(`{"error": "method not allowed"}`))
		return
	}

	handler.ServeHTTP(w, r)
}

// UpdateRoutes atomically swaps in a new routing table, leaving the current one
// untouched if the new set does not build. It returns an error rather than
// exiting: this is reachable from the admin API, where a malformed target must
// not be able to take the process down.
func (g *Gateway) UpdateRoutes(routes []config.RouteConfig) error {
	table, err := buildRoutingTable(routes)
	if err != nil {
		return err
	}

	g.table.Store(table)
	return nil
}
