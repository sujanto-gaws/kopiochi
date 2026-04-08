package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimiterPlugin implements Plugin for request rate limiting.
type RateLimiterPlugin struct {
	initialized bool
	mu          sync.Mutex
	requests    map[string]*clientRate
	maxRequests int
	window      time.Duration
}

type clientRate struct {
	count       int
	windowStart time.Time
}

// Name returns the plugin name.
func (p *RateLimiterPlugin) Name() string {
	return "ratelimit"
}

// Initialize sets up the rate limiter with configuration.
func (p *RateLimiterPlugin) Initialize(cfg map[string]interface{}) error {
	// Parse max requests (default: 100)
	if maxReq, ok := cfg["requests"].(float64); ok {
		p.maxRequests = int(maxReq)
	} else {
		p.maxRequests = 100
	}

	// Parse window duration (default: 1m)
	if windowStr, ok := cfg["window"].(string); ok && windowStr != "" {
		d, err := time.ParseDuration(windowStr)
		if err != nil {
			return fmt.Errorf("ratelimit: invalid window duration: %w", err)
		}
		p.window = d
	} else {
		p.window = 1 * time.Minute
	}

	p.requests = make(map[string]*clientRate)
	p.initialized = true
	return nil
}

// Close performs cleanup.
func (p *RateLimiterPlugin) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.requests = nil
	p.initialized = false
	return nil
}

// Middleware returns the rate limiting middleware.
func (p *RateLimiterPlugin) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !p.initialized {
				next.ServeHTTP(w, r)
				return
			}

			// Use client IP as rate limit key
			clientIP := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				clientIP = forwarded
			}

			p.mu.Lock()
			defer p.mu.Unlock()

			now := time.Now()
			client, exists := p.requests[clientIP]

			if !exists || now.Sub(client.windowStart) > p.window {
				// New window
				p.requests[clientIP] = &clientRate{
					count:       1,
					windowStart: now,
				}
				next.ServeHTTP(w, r)
				return
			}

			client.count++

			if client.count > p.maxRequests {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", strconv.Itoa(int(p.window.Seconds())))
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(fmt.Sprintf(`{"error":"rate limit exceeded. Try again in %v"}`, p.window)))
				return
			}

			// Add rate limit headers
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(p.maxRequests))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(p.maxRequests-client.count))

			next.ServeHTTP(w, r)
		})
	}
}

// Provider returns the plugin instance.
func (p *RateLimiterPlugin) Provider() interface{} {
	return p
}

// NewRateLimiterPlugin creates a new rate limiter plugin instance.
func NewRateLimiterPlugin() *RateLimiterPlugin {
	return &RateLimiterPlugin{}
}
