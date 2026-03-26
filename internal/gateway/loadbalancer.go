package gateway

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
)

type LoadBalancer struct {
	targets []*Target
	current atomic.Int64
}

type Target struct {
	URL   *url.URL
	Proxy *httputil.ReverseProxy
	mu    sync.RWMutex
	alive bool
}

func NewLoadBalancer(targets []string) *LoadBalancer {
	lb := &LoadBalancer{}

	for _, t := range targets {
		targetURL, err := url.Parse(t)
		if err != nil {
			continue
		}

		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		target := &Target{
			URL:   targetURL,
			Proxy: proxy,
			alive: true,
		}

		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			target.SetAlive(false)
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte(`{"error": "service unavailable"}`))
		}

		lb.targets = append(lb.targets, target)
	}

	return lb
}

// NextTarget returns the next available target using round-robin
func (lb *LoadBalancer) NextTarget() *Target {
	if len(lb.targets) == 0 {
		return nil
	}

	// Try all targets at most once
	for range lb.targets {
		idx := lb.current.Add(1) % int64(len(lb.targets))
		target := lb.targets[idx]
		if target.IsAlive() {
			return target
		}
	}

	// If no alive target found, return the next one anyway
	idx := lb.current.Add(1) % int64(len(lb.targets))
	return lb.targets[idx]
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := lb.NextTarget()
	if target == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error": "no targets available"}`))
		return
	}
	target.Proxy.ServeHTTP(w, r)
}

func (lb *LoadBalancer) Targets() []*Target {
	return lb.targets
}

func (t *Target) IsAlive() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.alive
}

func (t *Target) SetAlive(alive bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.alive = alive
}
