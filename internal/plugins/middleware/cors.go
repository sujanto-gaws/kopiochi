package middleware

import (
	"fmt"
	"net/http"
	"strings"
)

// CORSPlugin implements Plugin for Cross-Origin Resource Sharing.
type CORSPlugin struct {
	initialized      bool
	allowedOrigins   []string
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
	// Parse allowed origins
	if origins, ok := cfg["allowed_origins"].([]interface{}); ok {
		for _, origin := range origins {
			if originStr, ok := origin.(string); ok {
				p.allowedOrigins = append(p.allowedOrigins, originStr)
			}
		}
	} else {
		p.allowedOrigins = []string{"*"} // Default: allow all
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

	// Parse allow credentials
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
func (p *CORSPlugin) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			for _, allowedOrigin := range p.allowedOrigins {
				if allowedOrigin == "*" || allowedOrigin == origin {
					allowed = true
					break
				}
			}

			if !allowed && origin != "" {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":"origin not allowed"}`))
				return
			}

			// Set CORS headers
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			} else if len(p.allowedOrigins) > 0 && p.allowedOrigins[0] == "*" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}

			w.Header().Set("Access-Control-Allow-Methods", strings.Join(p.allowedMethods, ", "))
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(p.allowedHeaders, ", "))
			w.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%d", p.maxAge))

			if p.allowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			// Handle preflight requests
			if r.Method == "OPTIONS" {
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
