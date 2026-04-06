package plugin

import (
	"context"
	"net/http"
)

// Plugin is the base interface for all plugins in Kopiochi.
// Every plugin must have a unique name and support lifecycle management.
type Plugin interface {
	// Name returns the unique identifier for this plugin
	Name() string
	
	// Initialize sets up the plugin with the given configuration
	Initialize(cfg map[string]interface{}) error
	
	// Close performs cleanup when the plugin is shut down
	Close() error
}

// MiddlewarePlugin is a plugin that provides HTTP middleware functionality.
// It can be injected into the chi router middleware chain.
type MiddlewarePlugin interface {
	Plugin
	
	// Middleware returns the HTTP middleware handler
	Middleware() func(http.Handler) http.Handler
}

// ProviderPlugin is a plugin that provides a service or capability provider.
// Examples: cache provider, auth provider, storage provider.
type ProviderPlugin interface {
	Plugin
	
	// Provider returns the provider instance
	Provider() interface{}
}

// AuthPlugin is a specialized plugin for authentication providers.
type AuthPlugin interface {
	ProviderPlugin
	
	// AuthMiddleware returns authentication middleware
	AuthMiddleware() func(http.Handler) http.Handler
	
	// ExtractUserID extracts user ID from request context
	ExtractUserID(ctx context.Context) string
}

// CachePlugin is a specialized plugin for caching providers.
type CachePlugin interface {
	ProviderPlugin
	
	// Get retrieves a value from cache
	Get(ctx context.Context, key string) (interface{}, error)
	
	// Set stores a value in cache
	Set(ctx context.Context, key string, value interface{}) error
	
	// Delete removes a value from cache
	Delete(ctx context.Context, key string) error
}
