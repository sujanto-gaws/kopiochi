package extension

// BaseExtension provides a base implementation of the Extension interface.
// Extensions can embed this struct to avoid implementing all methods.
type BaseExtension struct {
	id     string
	config map[string]interface{}
}

// NewBaseExtension creates a new base extension with the given ID
func NewBaseExtension(id string) *BaseExtension {
	return &BaseExtension{
		id:     id,
		config: make(map[string]interface{}),
	}
}

// ID returns the extension ID
func (b *BaseExtension) ID() string {
	return b.id
}

// Init initializes the extension with configuration
func (b *BaseExtension) Init(config map[string]interface{}) error {
	if config != nil {
		b.config = config
	}
	return nil
}

// Bootstrap is called during application bootstrap
func (b *BaseExtension) Bootstrap(app Application) error {
	// Default implementation does nothing
	return nil
}

// Shutdown is called when the application shuts down
func (b *BaseExtension) Shutdown() error {
	// Default implementation does nothing
	return nil
}

// GetConfig retrieves a configuration value by key
func (b *BaseExtension) GetConfig(key string) (interface{}, bool) {
	val, exists := b.config[key]
	return val, exists
}

// GetString retrieves a configuration value as string
func (b *BaseExtension) GetString(key string, defaultValue string) string {
	if val, exists := b.config[key]; exists {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

// GetInt retrieves a configuration value as int
func (b *BaseExtension) GetInt(key string, defaultValue int) int {
	if val, exists := b.config[key]; exists {
		switch v := val.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return defaultValue
}

// GetBool retrieves a configuration value as bool
func (b *BaseExtension) GetBool(key string, defaultValue bool) bool {
	if val, exists := b.config[key]; exists {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return defaultValue
}
