package gateway

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// deadCooldown is how long a target stays out of rotation after a proxy error.
// Without it a single transient blip would remove a target for the lifetime of
// the process, since nothing else ever marks a target healthy again.
const deadCooldown = 30 * time.Second

type LoadBalancer struct {
	targets []*Target
	current atomic.Uint64
}

type Target struct {
	URL   *url.URL
	Proxy *httputil.ReverseProxy

	mu sync.RWMutex
	// deadUntil is the instant this target rejoins rotation. The zero value
	// means healthy.
	deadUntil time.Time
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
		}

		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("Proxy error for %s via %s: %v", r.URL.Path, targetURL, err)
			target.SetAlive(false)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte(`{"error": "service unavailable"}`))
		}

		lb.targets = append(lb.targets, target)
	}

	return lb
}

// NextTarget returns the next available target using round-robin. The counter
// advances exactly once per call, so a dead target does not distort the share
// the healthy ones receive.
func (lb *LoadBalancer) NextTarget() *Target {
	n := uint64(len(lb.targets))
	if n == 0 {
		return nil
	}

	start := lb.current.Add(1) - 1
	for i := uint64(0); i < n; i++ {
		if target := lb.targets[(start+i)%n]; target.IsAlive() {
			return target
		}
	}

	// Every target is cooling down. Still serve from the slot this call owns
	// rather than failing outright; the upstream may well have recovered.
	return lb.targets[start%n]
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
	return time.Now().After(t.deadUntil)
}

func (t *Target) SetAlive(alive bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if alive {
		t.deadUntil = time.Time{}
		return
	}
	t.deadUntil = time.Now().Add(deadCooldown)
}
