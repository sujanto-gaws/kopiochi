package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestRateLimitAllowsConcurrentRequests is task 1.1(c) from the remediation
// plan (docs/architectures/06-quality/testing-strategy.md:86-105) — the
// highest-value test in this batch. Two requests dispatched concurrently
// must both be inside the wrapped handler at the same time; a rate limiter
// should gate *how many* requests get through, not serialize them one at a
// time regardless of limit.
//
// This was a documented failure against the pre-Phase-2.1 implementation of
// RateLimiterPlugin.Middleware, which did:
//
//	p.mu.Lock()
//	defer p.mu.Unlock()
//	...
//	next.ServeHTTP(w, r)
//
// holding the single mutex for the *entire* duration of next.ServeHTTP, so a
// second concurrent request could not even start running the handler until
// the first request's ServeHTTP call had completely returned. Phase 2.1
// rewrote the limiter as a token bucket whose lock is released (inside
// allow()) before next.ServeHTTP is ever called, so this now passes and runs
// unconditionally as part of the default test suite -- see
// docs/architectures/04-security/rate-limiting.md.
func TestRateLimitAllowsConcurrentRequests(t *testing.T) {
	p := NewRateLimiterPlugin()
	if err := p.Initialize(map[string]interface{}{
		"requests": float64(100),
		"window":   "1m",
	}); err != nil {
		t.Fatalf("initialize rate limiter: %v", err)
	}

	release := make(chan struct{})
	entered := make(chan struct{}, 2)

	h := p.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
	}))

	req := func() *http.Request {
		return httptest.NewRequest(http.MethodGet, "/", nil)
	}

	go h.ServeHTTP(httptest.NewRecorder(), req())
	go h.ServeHTTP(httptest.NewRecorder(), req())

	// Both handlers must be inside simultaneously. Current code deadlocks
	// here: the second ServeHTTP call is still blocked on the limiter's
	// mutex, held by the first call across its own ServeHTTP.
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("handler blocked: rate limiter holds its lock across ServeHTTP")
		}
	}
	close(release)
}
