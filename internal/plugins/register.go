package plugins

import (
	"github.com/sujanto-gaws/kopiochi/internal/plugin"
	"github.com/sujanto-gaws/kopiochi/internal/plugins/auth"
	"github.com/sujanto-gaws/kopiochi/internal/plugins/middleware"
)

// RegisterBuiltinPlugins registers all built-in plugins with the given registry.
// This is a convenience function to register all plugins at once.
// Users can call this during application startup to enable all default plugins.
//
// The legacy HS256 "jwt-auth" plugin (internal/plugins/auth/jwt.go) has been
// removed: the identity module's RS256 JWTService
// (modules/identity/infrastructure/token) is the live token issuer/verifier,
// and no code path derives auth middleware from a registered auth plugin
// anymore. See docs/architectures/04-security/token-architecture.md.
func RegisterBuiltinPlugins(registry *plugin.Registry) {
	// Authentication plugins
	registry.Register("fido2-auth", func() plugin.Plugin {
		return &authPluginAdapter{auth.NewFIDO2Plugin()}
	})

	// Middleware plugins
	registry.Register("ratelimit", func() plugin.Plugin {
		return &middlewarePluginAdapter{middleware.NewRateLimiterPlugin()}
	})
	registry.Register("cors", func() plugin.Plugin {
		return &middlewarePluginAdapter{middleware.NewCORSPlugin()}
	})
}
