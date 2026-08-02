package middleware

import (
	"net/http"
	"strconv"
	"strings"
)

// CORSPlugin implements Plugin for Cross-Origin Resource Sharing.
//
// Defaults are allowlist-only and deny-by-default: with no "allowed_origins"
// configured (or an empty list), no Origin is ever granted access. A
// wildcard is never assumed and must be listed explicitly. See
// docs/architectures/04-security/middleware-hardening.md for the five
// defects this replaces (permissive default, arbitrary origin reflection,
// missing Vary: Origin, rejecting non-browser clients with 403, and
// blanket-204'ing every OPTIONS request).
type CORSPlugin struct {
	initialized bool
	// allowedOrigins is the explicit allowlist, lower-cased. A request
	// Origin is only ever granted the header if it matches an entry here or
	// allowAll is set -- it is never reflected otherwise.
	allowedOrigins   map[string]bool
	allowAll         bool // true only when "*" is explicitly configured
	allowedMethods   []string
	allowedHeaders   []string
	allowCredentials bool
	maxAge           int
}

// Name returns the plugin name.
func (p *CORSPlugin) Name() string {
	return "cors"
}

// Initialize sets up CORS with configuration.
func (p *CORSPlugin) Initialize(cfg map[string]interface{}) error {
	// Parse allowed origins. Absent or empty "allowed_origins" means no
	// origin is allowed -- permissive behavior must be a deliberate,
	// explicit config choice, never a default (see config/default.yaml,
	// plugins.custom, which ships empty).
	p.allowedOrigins = make(map[string]bool)
	p.allowAll = false
	if origins, ok := cfg["allowed_origins"].([]interface{}); ok {
		for _, origin := range origins {
			originStr, ok := origin.(string)
			if !ok {
				continue
			}
			if originStr == "*" {
				p.allowAll = true
				continue
			}
			p.allowedOrigins[strings.ToLower(originStr)] = true
		}
	}

	// Parse allowed methods
	if methods, ok := cfg["allowed_methods"].([]interface{}); ok {
		for _, method := range methods {
			if methodStr, ok := method.(string); ok {
				p.allowedMethods = append(p.allowedMethods, methodStr)
			}
		}
	} else {
		p.allowedMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	}

	// Parse allowed headers
	if headers, ok := cfg["allowed_headers"].([]interface{}); ok {
		for _, header := range headers {
			if headerStr, ok := header.(string); ok {
				p.allowedHeaders = append(p.allowedHeaders, headerStr)
			}
		}
	} else {
		p.allowedHeaders = []string{"Accept", "Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization"}
	}

	// Parse allow credentials. Rejecting "*" combined with allow_credentials
	// is done at config load, in internal/config.Config.Validate -- by the
	// time this Initialize runs (during plugin registry initialization,
	// itself after config.Load has already succeeded), that combination has
	// already failed the process closed. Middleware below still never emits
	// Access-Control-Allow-Credentials on the wildcard path, as defense in
	// depth against this plugin being initialized directly (e.g. in tests)
	// without going through Config.Validate.
	if creds, ok := cfg["allow_credentials"].(bool); ok {
		p.allowCredentials = creds
	}

	// Parse max age
	if maxAge, ok := cfg["max_age"].(float64); ok {
		p.maxAge = int(maxAge)
	} else {
		p.maxAge = 300 // Default 5 minutes
	}

	p.initialized = true
	return nil
}

// Close performs cleanup.
func (p *CORSPlugin) Close() error {
	p.initialized = false
	return nil
}

// Middleware returns the CORS middleware.
//
// CORS is a browser-enforced mechanism: the server's only job is to decide
// whether to grant the Access-Control-Allow-Origin header, never to reject
// the request itself. A request without an Origin header is not a CORS
// request at all (curl, server-to-server calls, health checks, ...) and is
// passed straight through untouched.
func (p *CORSPlugin) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// The response depends on Origin whether or not this turns out
			// to be a CORS request at all: without Vary, a shared cache or
			// CDN can store one origin's Access-Control-Allow-Origin (or its
			// absence) and serve it to a different origin.
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
				// internal/config.Config.Validate rejects "*" combined with
				// allow_credentials at config load, and this branch never
				// sets Access-Control-Allow-Credentials regardless, as
				// defense in depth.
				w.Header().Set("Access-Control-Allow-Origin", "*")
			case p.allowedOrigins[strings.ToLower(origin)]:
				// Echo back only the exact Origin that was already found in
				// the allowlist -- never an arbitrary caller-supplied value.
				w.Header().Set("Access-Control-Allow-Origin", origin)
				if p.allowCredentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
			}
			// Origin present but not allowed: withhold the header and move
			// on. The browser enforces same-origin on its own; rejecting
			// the request here would only break non-browser clients that
			// happen to send Origin, for no security benefit.

			// Only an actual preflight -- OPTIONS plus a declared
			// Access-Control-Request-Method -- gets the 204 preflight
			// response. Any other OPTIONS request (there is no such thing
			// as a blanket "OPTIONS always means preflight") falls through
			// to the router like every other method, so 404/405 behavior
			// stays consistent and downstream middleware still runs.
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Add("Vary", "Access-Control-Request-Method")
				w.Header().Add("Vary", "Access-Control-Request-Headers")
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(p.allowedMethods, ", "))
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(p.allowedHeaders, ", "))
				w.Header().Set("Access-Control-Max-Age", strconv.Itoa(p.maxAge))
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Provider returns the plugin instance.
func (p *CORSPlugin) Provider() interface{} {
	return p
}

// NewCORSPlugin creates a new CORS plugin instance.
func NewCORSPlugin() *CORSPlugin {
	return &CORSPlugin{}
}
