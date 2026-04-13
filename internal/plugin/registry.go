package plugin

import (
	"fmt"
	"sync"
)

// Registry manages all plugins in the application.
// It provides centralized plugin discovery, initialization, and lifecycle management.
type Registry struct {
	mu        sync.RWMutex
	plugins   map[string]Plugin
	factories map[string]PluginFactory
}

// PluginFactory is a function that creates a new plugin instance.
type PluginFactory func() Plugin

// NewRegistry creates a new plugin registry.
func NewRegistry() *Registry {
	return &Registry{
		plugins:   make(map[string]Plugin),
		factories: make(map[string]PluginFactory),
	}
}

// Register adds a plugin factory to the registry.
// This should typically be called during initialization or startup.
func (r *Registry) Register(name string, factory PluginFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[name]; exists {
		// Plugin already registered, skip silently
		return
	}

	r.factories[name] = factory
}

// Get retrieves an initialized plugin by name.
// Returns nil if the plugin is not found or not initialized.
func (r *Registry) Get(name string) Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.plugins[name]
}

// GetMiddleware retrieves a plugin as a MiddlewarePlugin.
// Returns nil if the plugin doesn't implement MiddlewarePlugin interface.
func (r *Registry) GetMiddleware(name string) MiddlewarePlugin {
	plugin := r.Get(name)
	if plugin == nil {
		return nil
	}

	mw, ok := plugin.(MiddlewarePlugin)
	if !ok {
		return nil
	}

	return mw
}

// GetAuth retrieves a plugin as an AuthPlugin.
// Returns nil if the plugin doesn't implement AuthPlugin interface.
func (r *Registry) GetAuth(name string) AuthPlugin {
	plugin := r.Get(name)
	if plugin == nil {
		return nil
	}

	auth, ok := plugin.(AuthPlugin)
	if !ok {
		return nil
	}

	return auth
}

// GetCache retrieves a plugin as a CachePlugin.
// Returns nil if the plugin doesn't implement CachePlugin interface.
func (r *Registry) GetCache(name string) CachePlugin {
	plugin := r.Get(name)
	if plugin == nil {
		return nil
	}

	cache, ok := plugin.(CachePlugin)
	if !ok {
		return nil
	}

	return cache
}

// Initialize creates and initializes a plugin with the given configuration.
func (r *Registry) Initialize(name string, cfg map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	factory, exists := r.factories[name]
	if !exists {
		return fmt.Errorf("plugin factory not found: %s", name)
	}

	// Create plugin instance
	plugin := factory()

	// Initialize with configuration
	if err := plugin.Initialize(cfg); err != nil {
		return fmt.Errorf("failed to initialize plugin %s: %w", name, err)
	}

	r.plugins[name] = plugin
	return nil
}

// InitializeAll initializes multiple plugins from a configuration map.
// The config should be in format: {"plugin_name": {"key": "value"}, ...}
func (r *Registry) InitializeAll(configs map[string]map[string]interface{}) []error {
	var errs []error

	for name, cfg := range configs {
		if err := r.Initialize(name, cfg); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

// Close shuts down all initialized plugins and cleans up resources.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var firstErr error

	for name, plugin := range r.plugins {
		if err := plugin.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to close plugin %s: %w", name, err)
		}
		delete(r.plugins, name)
	}

	return firstErr
}

// List returns names of all registered plugins (initialized or not).
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}

	return names
}

// ListInitialized returns names of all initialized plugins.
func (r *Registry) ListInitialized() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.plugins))
	for name := range r.plugins {
		names = append(names, name)
	}

	return names
}

// IsInitialized checks if a specific plugin has been initialized.
func (r *Registry) IsInitialized(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.plugins[name]
	return exists
}
