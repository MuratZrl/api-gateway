package gateway

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"api-gateway/internal/config"
)

type Gateway struct {
	routes  []config.RouteConfig
	proxies map[string]*httputil.ReverseProxy
}

func New(routes []config.RouteConfig) *Gateway {
	proxies := make(map[string]*httputil.ReverseProxy)

	for _, route := range routes {
		target, err := url.Parse(route.Target)
		if err != nil {
			log.Fatalf("Invalid target URL %s: %v", route.Target, err)
		}

		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("Proxy error for %s: %v", r.URL.Path, err)
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte(`{"error": "service unavailable"}`))
		}

		proxies[route.Path] = proxy
	}

	return &Gateway{routes: routes, proxies: proxies}
}

func (g *Gateway) FindRoute(path string) (*config.RouteConfig, *httputil.ReverseProxy) {
	for _, route := range g.routes {
		if strings.HasPrefix(path, route.Path) {
			proxy := g.proxies[route.Path]
			return &route, proxy
		}
	}
	return nil, nil
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route, proxy := g.FindRoute(r.URL.Path)
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

	proxy.ServeHTTP(w, r)
}

func (g *Gateway) UpdateRoutes(routes []config.RouteConfig) {
	gw := New(routes)
	g.routes = gw.routes
	g.proxies = gw.proxies
}
