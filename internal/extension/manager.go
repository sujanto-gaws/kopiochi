package extension

import (
	"context"
	"fmt"
	"sync"
)

// Manager handles the lifecycle and registration of all extensions.
// Similar to Yii's module manager, it provides centralized extension management.
type Manager struct {
	mu           sync.RWMutex
	extensions   map[string]Extension
	configs      map[string]map[string]interface{}
	services     map[string]interface{}
	serviceMu    sync.RWMutex
	eventHandlers map[string][]func(interface{}) error
	eventMu      sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	logger       Logger
}

// NewManager creates a new extension manager
func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		extensions:    make(map[string]Extension),
		configs:       make(map[string]map[string]interface{}),
		services:      make(map[string]interface{}),
		eventHandlers: make(map[string][]func(interface{}) error),
		ctx:          ctx,
		cancel:       cancel,
		logger:       &defaultLogger{},
	}
}

// Register adds an extension to the manager without initializing it
func (m *Manager) Register(ext Extension) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := ext.ID()
	if _, exists := m.extensions[id]; exists {
		return fmt.Errorf("extension %s is already registered", id)
	}

	m.extensions[id] = ext
	return nil
}

// Init initializes a specific extension with the given configuration
func (m *Manager) Init(id string, config map[string]interface{}) error {
	m.mu.Lock()
	ext, exists := m.extensions[id]
	m.mu.Unlock()

	if !exists {
		return fmt.Errorf("extension %s not found", id)
	}

	if config == nil {
		config = make(map[string]interface{})
	}

	if err := ext.Init(config); err != nil {
		return fmt.Errorf("failed to initialize extension %s: %w", id, err)
	}

	m.mu.Lock()
	m.configs[id] = config
	m.mu.Unlock()

	return nil
}

// Bootstrap bootstraps all initialized extensions
func (m *Manager) Bootstrap() error {
	m.mu.RLock()
	extensions := make([]Extension, 0, len(m.extensions))
	for _, ext := range m.extensions {
		extensions = append(extensions, ext)
	}
	m.mu.RUnlock()

	// First pass: EarlyBootstrap for bootstrappable extensions
	for _, ext := range extensions {
		if bootExt, ok := ext.(BootstrappableExtension); ok {
			if err := bootExt.EarlyBootstrap(m); err != nil {
				return fmt.Errorf("early bootstrap failed for %s: %w", ext.ID(), err)
			}
		}
	}

	// Second pass: Regular bootstrap
	for _, ext := range extensions {
		if err := ext.Bootstrap(m); err != nil {
			return fmt.Errorf("bootstrap failed for %s: %w", ext.ID(), err)
		}
	}

	// Third pass: Register routes for routable extensions
	for _, ext := range extensions {
		if routeExt, ok := ext.(RoutableExtension); ok {
			if err := routeExt.Routes(&routerRegistrar{manager: m}); err != nil {
				return fmt.Errorf("route registration failed for %s: %w", ext.ID(), err)
			}
		}
	}

	// Fourth pass: Register services for service providers
	for _, ext := range extensions {
		if svcExt, ok := ext.(ServiceProvider); ok {
			for _, svc := range svcExt.Services() {
				m.RegisterService(svc.Name, svc.Factory)
			}
		}
	}

	// Fifth pass: Register event listeners
	for _, ext := range extensions {
		if evtExt, ok := ext.(EventListener); ok {
			for _, event := range evtExt.Events() {
				// Wrap the HandleEvent to match the expected signature
				eventName := event
				handler := func(payload interface{}) error {
					return evtExt.HandleEvent(eventName, payload)
				}
				m.On(event, handler)
			}
		}
	}

	return nil
}

// Shutdown shuts down all extensions
func (m *Manager) Shutdown() error {
	m.cancel()

	m.mu.RLock()
	extensions := make([]Extension, 0, len(m.extensions))
	for _, ext := range m.extensions {
		extensions = append(extensions, ext)
	}
	m.mu.RUnlock()

	var firstErr error
	for _, ext := range extensions {
		if err := ext.Shutdown(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("shutdown failed for %s: %w", ext.ID(), err)
		}
	}

	return firstErr
}

// Get retrieves an extension by ID
func (m *Manager) Get(id string) (Extension, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ext, exists := m.extensions[id]
	return ext, exists
}

// List returns all registered extension IDs
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.extensions))
	for id := range m.extensions {
		ids = append(ids, id)
	}
	return ids
}

// Application interface implementation

func (m *Manager) Config() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	allConfig := make(map[string]interface{})
	for id, cfg := range m.configs {
		allConfig[id] = cfg
	}
	return allConfig
}

func (m *Manager) Context() context.Context {
	return m.ctx
}

func (m *Manager) RegisterService(name string, factory func() interface{}) {
	m.serviceMu.Lock()
	defer m.serviceMu.Unlock()

	m.services[name] = factory()
}

func (m *Manager) GetService(name string) (interface{}, bool) {
	m.serviceMu.RLock()
	defer m.serviceMu.RUnlock()

	svc, exists := m.services[name]
	return svc, exists
}

func (m *Manager) AddRoute(method, path string, handler interface{}) {
	// Routes are registered through RouterRegistrar
	// This is a placeholder for actual route registration logic
	m.logger.Info("Route registered", "method", method, "path", path)
}

func (m *Manager) On(event string, handler func(interface{}) error) {
	m.eventMu.Lock()
	defer m.eventMu.Unlock()

	m.eventHandlers[event] = append(m.eventHandlers[event], handler)
}

func (m *Manager) Trigger(event string, payload interface{}) error {
	m.eventMu.RLock()
	handlers := m.eventHandlers[event]
	m.eventMu.RUnlock()

	var firstErr error
	for _, handler := range handlers {
		if err := handler(payload); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func (m *Manager) Logger() Logger {
	return m.logger
}

// SetLogger sets a custom logger for the extension manager
func (m *Manager) SetLogger(logger Logger) {
	m.logger = logger
}

// routerRegistrar implements RouterRegistrar for extensions
type routerRegistrar struct {
	manager *Manager
}

func (r *routerRegistrar) GET(path string, handler interface{}) {
	r.manager.AddRoute("GET", path, handler)
}

func (r *routerRegistrar) POST(path string, handler interface{}) {
	r.manager.AddRoute("POST", path, handler)
}

func (r *routerRegistrar) PUT(path string, handler interface{}) {
	r.manager.AddRoute("PUT", path, handler)
}

func (r *routerRegistrar) DELETE(path string, handler interface{}) {
	r.manager.AddRoute("DELETE", path, handler)
}

func (r *routerRegistrar) Use(middleware ...interface{}) {
	// Middleware registration logic would go here
}

func (r *routerRegistrar) Group(prefix string, fn func()) {
	// Route group logic would go here
	fn()
}

// defaultLogger is a simple default logger implementation
type defaultLogger struct{}

func (l *defaultLogger) Debug(msg string, keysAndValues ...interface{}) {
	fmt.Printf("[DEBUG] %s %v\n", msg, keysAndValues)
}

func (l *defaultLogger) Info(msg string, keysAndValues ...interface{}) {
	fmt.Printf("[INFO] %s %v\n", msg, keysAndValues)
}

func (l *defaultLogger) Warn(msg string, keysAndValues ...interface{}) {
	fmt.Printf("[WARN] %s %v\n", msg, keysAndValues)
}

func (l *defaultLogger) Error(msg string, keysAndValues ...interface{}) {
	fmt.Printf("[ERROR] %s %v\n", msg, keysAndValues)
}
