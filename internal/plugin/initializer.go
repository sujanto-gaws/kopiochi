package plugin

import (
	"fmt"

	"github.com/sujanto-gaws/kopiochi/internal/config"
)

// InitializeFromConfig initializes all plugins from the application config.
// The registry must have plugins already registered via RegisterBuiltinPlugins().
// It returns the same registry with all plugins initialized and ready to use.
func InitializeFromConfig(registry *Registry, cfg *config.Plugins) (*Registry, error) {

	// Initialize middleware plugins
	for _, mwName := range cfg.Middleware {
		// Middleware plugins don't need special config unless specified in custom
		cfgData := make(map[string]interface{})
		if customCfg, exists := cfg.Custom[mwName]; exists {
			cfgData = customCfg
		}

		if err := registry.Initialize(mwName, cfgData); err != nil {
			return nil, fmt.Errorf("middleware plugin %s: %w", mwName, err)
		}
	}

	// Initialize auth plugins
	for name, authCfg := range cfg.Auth {
		if !authCfg.Enabled {
			continue
		}

		// Use provider name as the plugin name, or fall back to the provider field
		pluginName := authCfg.Provider
		if pluginName == "" {
			pluginName = name
		}

		// Merge config
		cfgData := make(map[string]interface{})
		if authCfg.Config != nil {
			cfgData = authCfg.Config
		}

		if err := registry.Initialize(pluginName, cfgData); err != nil {
			return nil, fmt.Errorf("auth plugin %s: %w", name, err)
		}
	}

	// Initialize cache plugins
	for name, cacheCfg := range cfg.Cache {
		if !cacheCfg.Enabled {
			continue
		}

		pluginName := cacheCfg.Provider
		if pluginName == "" {
			pluginName = name
		}

		cfgData := make(map[string]interface{})
		if cacheCfg.Config != nil {
			cfgData = cacheCfg.Config
		}

		if err := registry.Initialize(pluginName, cfgData); err != nil {
			return nil, fmt.Errorf("cache plugin %s: %w", name, err)
		}
	}

	// Initialize custom plugins
	for name, customCfg := range cfg.Custom {
		// Skip if already initialized as middleware
		if registry.IsInitialized(name) {
			continue
		}

		if err := registry.Initialize(name, customCfg); err != nil {
			return nil, fmt.Errorf("custom plugin %s: %w", name, err)
		}
	}

	return registry, nil
}

// GetMiddlewareNames returns the list of initialized middleware plugin names.
func GetMiddlewareNames(cfg *config.Plugins) []string {
	return cfg.Middleware
}

// GetAuthPlugin returns the primary auth plugin from config.
func GetAuthPlugin(registry *Registry, cfg *config.Plugins) AuthPlugin {
	// Find the first enabled auth plugin
	for name, authCfg := range cfg.Auth {
		if !authCfg.Enabled {
			continue
		}

		pluginName := authCfg.Provider
		if pluginName == "" {
			pluginName = name
		}

		if authPlugin := registry.GetAuth(pluginName); authPlugin != nil {
			return authPlugin
		}
	}

	return nil
}
