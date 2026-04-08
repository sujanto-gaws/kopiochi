package middleware

import "net/http"

// Plugin is the interface that all middleware plugins must implement.
type Plugin interface {
	Name() string
	Initialize(cfg map[string]interface{}) error
	Close() error
	Middleware() func(http.Handler) http.Handler
	Provider() interface{}
}
