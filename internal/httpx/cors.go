package httpx

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sujanto-gaws/kopiochi/internal/config"
)

// defaultCORSMaxAge is the preflight cache lifetime used when the config
// leaves max_age unset.
const defaultCORSMaxAge = 5 * time.Minute

// corsPolicy is the immutable, pre-computed form of config.CORS. Building it
// once at construction keeps the request path free of per-request slice
// scans and case folding, and -- more importantly -- makes the policy
// immutable while requests are in flight. The plugin this replaces could be
// re-Initialize()d underneath a live middleware.
type corsPolicy struct {
	allowedOrigins   map[string]bool // lower-cased allowlist
	allowAll         bool            // true only when "*" is explicitly configured
	allowedMethods   string          // pre-joined header value
	allowedHeaders   string          // pre-joined header value
	allowCredentials bool
	maxAge           string // pre-formatted seconds
}

// CORS returns the Cross-Origin Resource Sharing middleware for cfg.
//
// It is constructed directly from typed configuration rather than registered
// in a plugin framework: CORS is a cross-cutting HTTP concern, not a business
// capability, so it needs an `if` in the router and nothing more. See
// docs/architectures/01-modularity/extension-framework.md.
//
// The policy is allowlist-only and deny-by-default: with no AllowedOrigins
// configured, no Origin is ever granted access, and a wildcard is never
// assumed -- it must be listed explicitly as "*". See
// docs/architectures/04-security/middleware-hardening.md for the five defects
// this behaviour replaces (permissive default, arbitrary origin reflection,
// missing Vary: Origin, rejecting non-browser clients with 403, and
// blanket-204'ing every OPTIONS request).
func CORS(cfg config.CORS) func(http.Handler) http.Handler {
	p := newCORSPolicy(cfg)
	return p.middleware
}

func newCORSPolicy(cfg config.CORS) *corsPolicy {
	p := &corsPolicy{
		allowedOrigins:   make(map[string]bool, len(cfg.AllowedOrigins)),
		allowCredentials: cfg.AllowCredentials,
	}

	for _, origin := range cfg.AllowedOrigins {
		if origin == "*" {
			p.allowAll = true
			continue
		}
		p.allowedOrigins[strings.ToLower(origin)] = true
	}

	methods := cfg.AllowedMethods
	if len(methods) == 0 {
		methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	}
	p.allowedMethods = strings.Join(methods, ", ")

	headers := cfg.AllowedHeaders
	if len(headers) == 0 {
		headers = []string{"Accept", "Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization"}
	}
	p.allowedHeaders = strings.Join(headers, ", ")

	maxAge := cfg.MaxAge
	if maxAge <= 0 {
		maxAge = defaultCORSMaxAge
	}
	p.maxAge = strconv.Itoa(int(maxAge.Seconds()))

	return p
}

// middleware implements the policy.
//
// CORS is a browser-enforced mechanism: the server's only job is to decide
// whether to grant the Access-Control-Allow-Origin header, never to reject
// the request itself. A request without an Origin header is not a CORS
// request at all (curl, server-to-server calls, health checks, ...) and is
// passed straight through untouched.
func (p *corsPolicy) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// The response depends on Origin whether or not this turns out to
		// be a CORS request at all: without Vary, a shared cache or CDN can
		// store one origin's Access-Control-Allow-Origin (or its absence)
		// and serve it to a different origin.
		w.Header().Add("Vary", "Origin")

		if origin == "" {
			// Not a CORS request. Do not 403 it, do not treat OPTIONS
			// specially -- let it fall through exactly like any other
			// request, including to the router's 404/405 handling.
			next.ServeHTTP(w, r)
			return
		}

		switch {
		case p.allowAll:
			// Wildcard is only ever reachable here without credentials:
			// config.Config.Validate rejects "*" combined with
			// allow_credentials at config load, and this branch never sets
			// Access-Control-Allow-Credentials regardless, as defense in
			// depth against construction that bypasses Validate (e.g. in
			// tests).
			w.Header().Set("Access-Control-Allow-Origin", "*")
		case p.allowedOrigins[strings.ToLower(origin)]:
			// Echo back only the exact Origin that was already found in the
			// allowlist -- never an arbitrary caller-supplied value.
			w.Header().Set("Access-Control-Allow-Origin", origin)
			if p.allowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
		}
		// Origin present but not allowed: withhold the header and move on.
		// The browser enforces same-origin on its own; rejecting the request
		// here would only break non-browser clients that happen to send
		// Origin, for no security benefit.

		// Only an actual preflight -- OPTIONS plus a declared
		// Access-Control-Request-Method -- gets the 204 preflight response.
		// Any other OPTIONS request (there is no such thing as a blanket
		// "OPTIONS always means preflight") falls through to the router like
		// every other method, so 404/405 behavior stays consistent and
		// downstream middleware still runs.
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			w.Header().Set("Access-Control-Allow-Methods", p.allowedMethods)
			w.Header().Set("Access-Control-Allow-Headers", p.allowedHeaders)
			w.Header().Set("Access-Control-Max-Age", p.maxAge)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
