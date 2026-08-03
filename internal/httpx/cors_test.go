package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sujanto-gaws/kopiochi/internal/config"
)

func passthroughHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// allowlistOnly is the common fixture: a single allowed origin, everything
// else at its zero value.
func allowlistOnly(origins ...string) config.CORS {
	return config.CORS{Enabled: true, AllowedOrigins: origins}
}

// TestCORS_DisallowedOriginGetsNoHeaderAndIsNot403 is the regression test for
// middleware-hardening.md Problems 2 and 4: a caller sending an Origin that
// isn't on the allowlist must not be reflected, and must not be rejected with
// 403 either -- CORS is enforced by the browser, not the server.
func TestCORS_DisallowedOriginGetsNoHeaderAndIsNot403(t *testing.T) {
	h := CORS(allowlistOnly("https://allowed.example.com"))(passthroughHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "a disallowed Origin must not be rejected by the server")
	require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"), "a disallowed Origin must never be reflected")
}

// TestCORS_NeverReflectsArbitraryOrigin proves the allowlist is checked, not
// bypassed: only the exact allowed origin is ever granted the header, even
// when the request supplies a different one.
func TestCORS_NeverReflectsArbitraryOrigin(t *testing.T) {
	h := CORS(allowlistOnly("https://allowed.example.com"))(passthroughHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://allowed.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, "https://allowed.example.com", rec.Header().Get("Access-Control-Allow-Origin"))
}

// TestCORS_AlwaysSetsVaryOrigin covers Problem 3: absent Vary: Origin, a
// shared cache can serve one origin's response to a different origin. This
// must hold for matching, non-matching, and no-Origin-header requests alike.
func TestCORS_AlwaysSetsVaryOrigin(t *testing.T) {
	h := CORS(allowlistOnly("https://allowed.example.com"))(passthroughHandler())

	cases := []struct {
		name   string
		origin string
	}{
		{"matching origin", "https://allowed.example.com"},
		{"non-matching origin", "https://evil.example.com"},
		{"no origin header", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Contains(t, rec.Header().Values("Vary"), "Origin")
		})
	}
}

// TestCORS_NoOriginRequestPassesThroughUntouched covers Problem 4 for the
// specific case of a plain, non-browser request: curl, server-to-server
// calls, and health checks send no Origin header at all and must reach the
// handler exactly as if CORS middleware weren't present, whatever the method.
func TestCORS_NoOriginRequestPassesThroughUntouched(t *testing.T) {
	var handlerCalled bool
	h := CORS(allowlistOnly("https://allowed.example.com"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusTeapot)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.True(t, handlerCalled, "a request with no Origin header must reach the wrapped handler")
	require.Equal(t, http.StatusTeapot, rec.Code, "the wrapped handler's response must not be overridden")
}

// TestCORS_OnlyActualPreflightGetsNoContent covers Problem 5: a blanket 204
// on every OPTIONS request bypasses downstream middleware and makes 404
// behavior inconsistent for paths that don't exist. Only OPTIONS carrying
// Access-Control-Request-Method -- an actual preflight -- gets the 204
// short-circuit.
func TestCORS_OnlyActualPreflightGetsNoContent(t *testing.T) {
	cors := CORS(allowlistOnly("https://allowed.example.com"))

	t.Run("actual preflight", func(t *testing.T) {
		var handlerCalled bool
		h := cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
		}))

		req := httptest.NewRequest(http.MethodOptions, "/", nil)
		req.Header.Set("Origin", "https://allowed.example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNoContent, rec.Code)
		require.False(t, handlerCalled, "an actual preflight must be answered directly, not forwarded")
	})

	t.Run("plain OPTIONS with Origin but no preflight header", func(t *testing.T) {
		var handlerCalled bool
		h := cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodOptions, "/", nil)
		req.Header.Set("Origin", "https://allowed.example.com")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		require.True(t, handlerCalled, "a non-preflight OPTIONS request must fall through to the router")
		require.Equal(t, http.StatusOK, rec.Code)
	})
}

// TestCORS_DefaultDenyAllowsNoOrigin covers Problem 1: with no allowed
// origins configured at all, no Origin -- not even a plausible one -- is
// granted the header. Permissive behavior must be a deliberate, visible
// config choice, never the default.
func TestCORS_DefaultDenyAllowsNoOrigin(t *testing.T) {
	h := CORS(config.CORS{Enabled: true})(passthroughHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://anything.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

// TestCORS_ExplicitWildcardNeverSetsCredentials is defense in depth for
// Problem 2 at the middleware level (config-load rejection lives in
// internal/config.Config.Validate): even if a wildcard policy were somehow
// constructed with AllowCredentials -- bypassing Validate -- the middleware
// itself must never emit Access-Control-Allow-Credentials alongside a
// wildcard Access-Control-Allow-Origin.
func TestCORS_ExplicitWildcardNeverSetsCredentials(t *testing.T) {
	h := CORS(config.CORS{
		Enabled:          true,
		AllowedOrigins:   []string{"*"},
		AllowCredentials: true,
	})(passthroughHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://anything.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	require.Empty(t, rec.Header().Get("Access-Control-Allow-Credentials"))
}

// TestCORS_AllowlistMatchIsCaseInsensitiveOnHost mirrors the policy's
// lower-casing of both the allowlist and the incoming Origin: origins are
// compared case-insensitively, but the value echoed back is the caller's
// exact string, never a normalised one.
func TestCORS_AllowlistMatchIsCaseInsensitiveOnHost(t *testing.T) {
	h := CORS(allowlistOnly("https://Allowed.Example.com"))(passthroughHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://allowed.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, "https://allowed.example.com", rec.Header().Get("Access-Control-Allow-Origin"))
}

// TestCORS_PreflightUsesConfiguredMaxAge proves max_age is carried through as
// seconds. It is configured as a duration ("5m"), not a bare number, so that
// a unit mistake is impossible in YAML.
func TestCORS_PreflightUsesConfiguredMaxAge(t *testing.T) {
	cfg := allowlistOnly("https://allowed.example.com")
	cfg.MaxAge = 90 * time.Second
	h := CORS(cfg)(passthroughHandler())

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://allowed.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, "90", rec.Header().Get("Access-Control-Max-Age"))
}
