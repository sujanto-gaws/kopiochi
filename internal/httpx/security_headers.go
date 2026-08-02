package httpx

import (
	"net/http"
	"strings"
)

// swaggerPathPrefix is the mount point of the embedded Swagger UI
// (internal/httpx/routes.go). Its bundled index.html renders an inline
// <style> block, an inline <script> that boots SwaggerUIBundle, and
// same-origin swagger-ui-bundle.js / swagger-ui-standalone-preset.js /
// swagger-ui.css / favicon-*.png / doc.json assets (see
// github.com/swaggo/http-swagger/v2's indexTempl). None of that survives
// the API's otherwise-ideal default-src 'none', so this one route gets a
// narrowly relaxed policy instead of loosening it for every response.
const swaggerPathPrefix = "/swagger/"

const (
	// apiCSP applies to every response except the Swagger UI: this is a
	// JSON API, it never renders untrusted HTML, and there is nothing for a
	// browser context to load.
	apiCSP = "default-src 'none'"

	// swaggerCSP is scoped to swaggerPathPrefix only. 'unsafe-inline' is
	// required for the bundled page's inline <style>/<script>; everything
	// else it loads is same-origin, so 'self' covers it -- nothing here
	// reaches a third-party origin. Verified against the real mounted route
	// via httptest in security_headers_test.go.
	swaggerCSP = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:"
)

// SecurityHeadersConfig controls the one header here that depends on the
// deployment rather than being an always-safe default.
type SecurityHeadersConfig struct {
	// EnableHSTS gates Strict-Transport-Security. It must only be true when
	// TLS is actually terminated somewhere in front of this process -- this
	// server always listens plain HTTP itself. Emitting HSTS unconditionally
	// would, at best, be ignored by a plain-HTTP client and, at worst, get
	// cached by a browser against a plain-HTTP dev origin (e.g.
	// http://localhost) and lock a developer out of it. Default: false.
	EnableHSTS bool
}

// SecurityHeaders sets baseline defensive response headers on every
// response, including errors and 404s: any middleware registered via
// chi.Mux.Use wraps the router's default-not-found handling too, since
// chi.Mux.ServeHTTP composes the whole middleware chain around routeHTTP,
// which is what actually decides whether a route matched.
func SecurityHeaders(cfg SecurityHeadersConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")

			if strings.HasPrefix(r.URL.Path, swaggerPathPrefix) {
				h.Set("Content-Security-Policy", swaggerCSP)
			} else {
				h.Set("Content-Security-Policy", apiCSP)
			}

			if cfg.EnableHSTS {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}

			next.ServeHTTP(w, r)
		})
	}
}
