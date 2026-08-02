package httpx

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

// TestSecurityHeadersPresent covers the baseline, always-on headers on a
// real response, including a non-JSON-API-like plain handler, to prove
// SecurityHeaders itself sets them regardless of what's downstream.
func TestSecurityHeadersPresent(t *testing.T) {
	h := SecurityHeaders(SecurityHeadersConfig{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	require.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
	require.Equal(t, apiCSP, rec.Header().Get("Content-Security-Policy"))
	require.Empty(t, rec.Header().Get("Strict-Transport-Security"), "HSTS must default off")
}

// TestSecurityHeadersPresent_On404 proves the headers survive onto a
// response the router itself produces (no matching route), not just onto
// responses a handler explicitly writes -- SecurityHeaders wraps chi's
// default NotFound handling the same as any other request.
func TestSecurityHeadersPresent_On404(t *testing.T) {
	r := chi.NewRouter()
	r.Use(SecurityHeaders(SecurityHeadersConfig{}))
	r.Get("/exists", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	require.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
	require.Equal(t, apiCSP, rec.Header().Get("Content-Security-Policy"))
}

// TestSecurityHeaders_HSTSGatedByConfig covers the config gate: HSTS must be
// absent by default and present only when explicitly enabled, since this
// server always listens plain HTTP and unconditional HSTS would be wrong in
// that deployment.
func TestSecurityHeaders_HSTSGatedByConfig(t *testing.T) {
	newHandler := func(cfg SecurityHeadersConfig) http.Handler {
		return SecurityHeaders(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	}

	t.Run("disabled by default", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		newHandler(SecurityHeadersConfig{EnableHSTS: false}).ServeHTTP(rec, req)

		require.Empty(t, rec.Header().Get("Strict-Transport-Security"))
	})

	t.Run("enabled explicitly", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		newHandler(SecurityHeadersConfig{EnableHSTS: true}).ServeHTTP(rec, req)

		require.Equal(t, "max-age=31536000; includeSubDomains", rec.Header().Get("Strict-Transport-Security"))
	})
}

// TestSecurityHeaders_SwaggerCSPIsRelaxed verifies the CSP chosen for
// /swagger/* actually lets the real, mounted Swagger UI render: this exact
// concern is called out in docs/architectures/04-security/middleware-hardening.md
// ("careful: a restrictive CSP will break it"), so this test renders the
// real route (via Mount, the same wiring cmd/api uses) through httptest
// rather than asserting the header value in isolation.
func TestSecurityHeaders_SwaggerCSPIsRelaxed(t *testing.T) {
	r := chi.NewRouter()
	r.Use(SecurityHeaders(SecurityHeadersConfig{}))
	Mount(r, nil, Deps{})

	srv := httptest.NewServer(r)
	defer srv.Close()

	client := srv.Client()

	t.Run("index.html renders with the relaxed swagger CSP", func(t *testing.T) {
		resp, err := client.Get(srv.URL + "/swagger/index.html")
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, swaggerCSP, resp.Header.Get("Content-Security-Policy"))
		require.Contains(t, resp.Header.Get("Content-Type"), "text/html")

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		// Confirms this is genuinely the Swagger UI bundle page, not an
		// error page or an empty body swallowed by a CSP-caused failure
		// upstream of the browser.
		require.Contains(t, string(body), "SwaggerUIBundle")
		require.Contains(t, string(body), `id="swagger-ui"`)
	})

	t.Run("same-origin JS/CSS assets the page references are actually servable", func(t *testing.T) {
		for _, asset := range []string{
			"/swagger/swagger-ui-bundle.js",
			"/swagger/swagger-ui-standalone-preset.js",
			"/swagger/swagger-ui.css",
		} {
			resp, err := client.Get(srv.URL + asset)
			require.NoError(t, err)
			resp.Body.Close()
			require.Equalf(t, http.StatusOK, resp.StatusCode, "asset %s must be servable for the page to render", asset)
		}
	})

	t.Run("non-swagger routes keep the strict API CSP", func(t *testing.T) {
		resp, err := client.Get(srv.URL + "/healthz")
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, apiCSP, resp.Header.Get("Content-Security-Policy"))
	})
}
