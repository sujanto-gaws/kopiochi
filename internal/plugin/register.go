package plugin

import (
	"context"
	"net/http"

	"github.com/sujanto-gaws/kopiochi/internal/plugin/auth"
	"github.com/sujanto-gaws/kopiochi/internal/plugin/middleware"
)

// pluginAdapter wraps concrete types to implement plugin.Plugin interface
type middlewarePluginAdapter struct {
	mw middleware.Plugin
}

func (a *middlewarePluginAdapter) Name() string {
	return a.mw.Name()
}

func (a *middlewarePluginAdapter) Initialize(cfg map[string]interface{}) error {
	return a.mw.Initialize(cfg)
}

func (a *middlewarePluginAdapter) Close() error {
	return a.mw.Close()
}

func (a *middlewarePluginAdapter) Middleware() func(http.Handler) http.Handler {
	return a.mw.Middleware()
}

type authPluginAdapter struct {
	auth *auth.JWTPlugin
}

func (a *authPluginAdapter) Name() string {
	return a.auth.Name()
}

func (a *authPluginAdapter) Initialize(cfg map[string]interface{}) error {
	return a.auth.Initialize(cfg)
}

func (a *authPluginAdapter) Close() error {
	return a.auth.Close()
}

func (a *authPluginAdapter) Middleware() func(http.Handler) http.Handler {
	return a.auth.Middleware()
}

func (a *authPluginAdapter) AuthMiddleware() func(http.Handler) http.Handler {
	return a.auth.AuthMiddleware()
}

func (a *authPluginAdapter) ExtractUserID(ctx context.Context) string {
	return a.auth.ExtractUserID(ctx)
}

func (a *authPluginAdapter) Provider() interface{} {
	return a.auth.Provider()
}

// RegisterBuiltinPlugins registers all built-in plugins with the given registry.
// This is a convenience function to register all plugins at once.
func RegisterBuiltinPlugins(registry *Registry) {
	// Authentication plugins
	registry.Register("jwt-auth", func() Plugin {
		return &authPluginAdapter{auth.NewJWTPlugin()}
	})

	// Middleware plugins
	registry.Register("ratelimit", func() Plugin {
		return &middlewarePluginAdapter{middleware.NewRateLimiterPlugin()}
	})
	registry.Register("cors", func() Plugin {
		return &middlewarePluginAdapter{middleware.NewCORSPlugin()}
	})
}
