package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/sujanto-gaws/kopiochi/internal/config"
	"github.com/sujanto-gaws/kopiochi/internal/metrics"
)

func routerTestConfig() config.Server {
	return config.Server{RequestTimeout: 5 * time.Second}
}

// mustRouter builds a router and registers its closer with t.
func mustRouter(t *testing.T, sec config.Security, m *metrics.Metrics) http.Handler {
	t.Helper()

	r, closeRouter, err := NewRouter(routerTestConfig(), sec, zerolog.Nop(), m)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	t.Cleanup(func() { _ = closeRouter() })

	r.Get("/thing", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	return r
}

func TestNewRouter_ServesAndAppliesTheCoreStack(t *testing.T) {
	t.Parallel()

	r := mustRouter(t, config.Security{}, nil)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/thing", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// SecurityHeaders is part of the core stack; its absence would mean the
	// stack was not applied at all.
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q; the core middleware stack is not applied", got)
	}
	if rec.Header().Get("X-Request-Id") == "" {
		t.Error("no request id was generated")
	}
}

// TestNewRouter_RecoversPanics proves Recovery is wired in NewRouter, not just
// that it works in isolation.
func TestNewRouter_RecoversPanics(t *testing.T) {
	t.Parallel()

	r, closeRouter, err := NewRouter(routerTestConfig(), config.Security{}, zerolog.Nop(), nil)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	t.Cleanup(func() { _ = closeRouter() })
	r.Get("/boom", func(w http.ResponseWriter, r *http.Request) { panic("boom") })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != ProblemContentType {
		t.Errorf("Content-Type = %q, want %q — chi's Recoverer is still in use", ct, ProblemContentType)
	}
}

func TestNewRouter_NotFoundAndMethodNotAllowedAreWired(t *testing.T) {
	t.Parallel()

	r := mustRouter(t, config.Security{}, nil)

	for _, tc := range []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"unknown path", http.MethodGet, "/nope", http.StatusNotFound},
		{"wrong method", http.MethodPost, "/thing", http.StatusMethodNotAllowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); ct != ProblemContentType {
				t.Errorf("Content-Type = %q, want %q", ct, ProblemContentType)
			}
		})
	}
}

func TestNewRouter_CORSOnlyWhenEnabled(t *testing.T) {
	t.Parallel()

	sec := config.Security{CORS: config.CORS{
		Enabled:        true,
		AllowedOrigins: []string{"https://app.example.com"},
		AllowedMethods: []string{"GET"},
	}}

	t.Run("enabled", func(t *testing.T) {
		t.Parallel()

		r := mustRouter(t, sec, nil)
		req := httptest.NewRequest(http.MethodGet, "/thing", nil)
		req.Header.Set("Origin", "https://app.example.com")

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
			t.Errorf("Access-Control-Allow-Origin = %q, want the allowed origin", got)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()

		r := mustRouter(t, config.Security{}, nil)
		req := httptest.NewRequest(http.MethodGet, "/thing", nil)
		req.Header.Set("Origin", "https://app.example.com")

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q with CORS disabled, want none", got)
		}
	})
}

// TestNewRouter_RejectsAnInvalidRateLimitConfig: the limiter has no fallback
// defaults by design, so a nonsensical value must fail construction rather
// than silently produce a limiter that allows everything.
func TestNewRouter_RejectsAnInvalidRateLimitConfig(t *testing.T) {
	t.Parallel()

	sec := config.Security{RateLimit: config.RateLimit{
		Enabled: true,
		Rate:    0, // invalid
		Burst:   10,
		TTL:     time.Minute,
		MaxKeys: 10,
	}}

	r, closeRouter, err := NewRouter(routerTestConfig(), sec, zerolog.Nop(), nil)
	if err == nil {
		if closeRouter != nil {
			_ = closeRouter()
		}
		t.Fatal("NewRouter() = nil error for an invalid rate limit config")
	}
	if r != nil {
		t.Error("NewRouter() returned a router alongside an error")
	}
}

// TestNewRouter_CloserIsSafeWhenNothingWasConstructed: main pushes the closer
// onto the lifecycle stack unconditionally, so it must tolerate a router that
// built no resources.
func TestNewRouter_CloserIsSafeWhenNothingWasConstructed(t *testing.T) {
	t.Parallel()

	_, closeRouter, err := NewRouter(routerTestConfig(), config.Security{}, zerolog.Nop(), nil)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	if err := closeRouter(); err != nil {
		t.Errorf("closeRouter() = %v, want nil", err)
	}
	// Idempotent: the stack may run it after an earlier failure path already
	// did.
	if err := closeRouter(); err != nil {
		t.Errorf("second closeRouter() = %v, want nil", err)
	}
}

// TestNewRouter_RateLimiterCloserStopsTheSweep: the limiter owns an eviction
// goroutine, and the router's closer is the only thing that stops it.
func TestNewRouter_RateLimiterCloserStopsTheSweep(t *testing.T) {
	t.Parallel()

	sec := config.Security{RateLimit: config.RateLimit{
		Enabled: true, Rate: 100, Burst: 100, TTL: time.Minute, MaxKeys: 100,
	}}

	_, closeRouter, err := NewRouter(routerTestConfig(), sec, zerolog.Nop(), nil)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	if err := closeRouter(); err != nil {
		t.Errorf("closeRouter() = %v, want nil", err)
	}
}

// TestNewRouter_MetricsAreOptional: nil disables instrumentation entirely, and
// a non-nil registry must actually see traffic.
func TestNewRouter_MetricsAreOptional(t *testing.T) {
	t.Parallel()

	m := metrics.New()
	r := mustRouter(t, config.Security{}, m)

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/thing", nil))

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics scrape returned %d", rec.Code)
	}
	if body := rec.Body.String(); !contains(body, "kopiochi_http_requests_total") {
		t.Errorf("the router's traffic was not recorded:\n%s", body)
	}
}

// TestNewRouter_CallerMiddlewareRuns confirms the variadic middleware is
// actually applied, not accepted and dropped.
func TestNewRouter_CallerMiddlewareRuns(t *testing.T) {
	t.Parallel()

	called := false
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			next.ServeHTTP(w, r)
		})
	}

	r, closeRouter, err := NewRouter(routerTestConfig(), config.Security{}, zerolog.Nop(), nil, mw)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	t.Cleanup(func() { _ = closeRouter() })
	r.Get("/thing", func(w http.ResponseWriter, r *http.Request) {})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/thing", nil))

	if !called {
		t.Error("caller-supplied middleware was never invoked")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
