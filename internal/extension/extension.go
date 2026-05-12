package extension

import (
	"context"
	"net/http"
)

// Extension is the base interface for all extensions, similar to Yii modules.
// Extensions can provide services, register routes, listen to events, and perform initialization.
type Extension interface {
	// ID returns the unique identifier for this extension
	ID() string

	// Init initializes the extension with the given configuration.
	// Called once when the extension is loaded.
	Init(config map[string]interface{}) error

	// Bootstrap is called after Init during application bootstrap phase.
	// This is where extensions should register routes, events, and other components.
	Bootstrap(app Application) error

	// Shutdown is called when the application is shutting down.
	// Extensions should clean up resources here.
	Shutdown() error
}

// BootstrappableExtension is an optional interface for extensions that need early bootstrapping.
type BootstrappableExtension interface {
	Extension
	// EarlyBootstrap is called before regular Bootstrap for critical extensions.
	EarlyBootstrap(app Application) error
}

// EventListener is an interface for extensions that want to listen to application events.
type EventListener interface {
	Extension
	// Events returns a list of event names this extension wants to listen to
	Events() []string
	// HandleEvent processes an event with the given payload
	HandleEvent(event string, payload interface{}) error
}

// RoutableExtension is an optional interface for extensions that provide HTTP routes.
type RoutableExtension interface {
	Extension
	// Routes registers HTTP routes with the application router
	Routes(router RouterRegistrar) error
}

// ServiceProvider is an optional interface for extensions that provide services to the container.
type ServiceProvider interface {
	Extension
	// Services returns a list of service providers to register
	Services() []Service
}

// Service represents a service provided by an extension
type Service struct {
	Name    string
	Factory func() interface{}
}

// Application defines the interface that extensions interact with
type Application interface {
	// Config returns the application configuration
	Config() map[string]interface{}

	// Context returns the application context
	Context() context.Context

	// RegisterService registers a service in the application container
	RegisterService(name string, factory func() interface{})

	// GetService retrieves a service from the container
	GetService(name string) (interface{}, bool)

	// AddRoute registers an HTTP route with the application router.
	AddRoute(method, path string, handler http.HandlerFunc)

	// On registers an event listener
	On(event string, handler func(interface{}) error)

	// Trigger triggers an event with the given payload
	Trigger(event string, payload interface{}) error

	// Logger returns the application logger
	Logger() Logger
}

// RouterRegistrar is the interface for registering HTTP routes
type RouterRegistrar interface {
	// GET registers a GET route
	GET(path string, handler http.HandlerFunc)
	// POST registers a POST route
	POST(path string, handler http.HandlerFunc)
	// PUT registers a PUT route
	PUT(path string, handler http.HandlerFunc)
	// DELETE registers a DELETE route
	DELETE(path string, handler http.HandlerFunc)
	// Use registers middleware for a route group
	Use(mw ...func(http.Handler) http.Handler)
	// Group creates a route group with a common prefix
	Group(prefix string, fn func())
}

// Logger is a simple logger interface for extensions
type Logger interface {
	Debug(msg string, keysAndValues ...interface{})
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
}
