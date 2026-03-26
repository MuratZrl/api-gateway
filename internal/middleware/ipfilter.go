package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"sync"
)

type IPFilter struct {
	mu        sync.RWMutex
	whitelist map[string]bool
	blacklist map[string]bool
	mode      IPFilterMode
}

type IPFilterMode string

const (
	ModeWhitelist IPFilterMode = "whitelist" // Only allow listed IPs
	ModeBlacklist IPFilterMode = "blacklist" // Block listed IPs
	ModeDisabled  IPFilterMode = "disabled"  // No filtering
)

func NewIPFilter(mode IPFilterMode, whitelist, blacklist []string) *IPFilter {
	f := &IPFilter{
		whitelist: make(map[string]bool),
		blacklist: make(map[string]bool),
		mode:      mode,
	}

	for _, ip := range whitelist {
		f.whitelist[ip] = true
	}
	for _, ip := range blacklist {
		f.blacklist[ip] = true
	}

	return f
}

func (f *IPFilter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.mode == ModeDisabled {
			next.ServeHTTP(w, r)
			return
		}

		clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}

		// Also check X-Forwarded-For header
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			clientIP = forwarded
		}

		f.mu.RLock()
		defer f.mu.RUnlock()

		switch f.mode {
		case ModeWhitelist:
			if !f.whitelist[clientIP] {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "ip not allowed",
					"ip":    clientIP,
				})
				return
			}
		case ModeBlacklist:
			if f.blacklist[clientIP] {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "ip blocked",
					"ip":    clientIP,
				})
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// AddToWhitelist adds an IP to the whitelist
func (f *IPFilter) AddToWhitelist(ip string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.whitelist[ip] = true
}

// AddToBlacklist adds an IP to the blacklist
func (f *IPFilter) AddToBlacklist(ip string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blacklist[ip] = true
}

// RemoveFromWhitelist removes an IP from the whitelist
func (f *IPFilter) RemoveFromWhitelist(ip string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.whitelist, ip)
}

// RemoveFromBlacklist removes an IP from the blacklist
func (f *IPFilter) RemoveFromBlacklist(ip string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.blacklist, ip)
}
