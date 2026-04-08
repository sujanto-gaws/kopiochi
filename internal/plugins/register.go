package plugins

import (
	"github.com/sujanto-gaws/kopiochi/internal/plugin"
	"github.com/sujanto-gaws/kopiochi/internal/plugins/auth"
	"github.com/sujanto-gaws/kopiochi/internal/plugins/middleware"
)

// RegisterBuiltinPlugins registers all built-in plugins with the given registry.
// This is a convenience function to register all plugins at once.
// Users can call this during application startup to enable all default plugins.
func RegisterBuiltinPlugins(registry *plugin.Registry) {
	// Authentication plugins
	registry.Register("jwt-auth", func() plugin.Plugin {
		return &authPluginAdapter{auth.NewJWTPlugin()}
	})

	// Middleware plugins
	registry.Register("ratelimit", func() plugin.Plugin {
		return &middlewarePluginAdapter{middleware.NewRateLimiterPlugin()}
	})
	registry.Register("cors", func() plugin.Plugin {
		return &middlewarePluginAdapter{middleware.NewCORSPlugin()}
	})
}
