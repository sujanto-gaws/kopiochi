package main

import (
	"fmt"

	"github.com/sujanto-gaws/kopiochi/internal/extension"
)

// HelloExtension is a simple example extension
type HelloExtension struct {
	*extension.BaseExtension
	message string
}

func NewHelloExtension() *HelloExtension {
	return &HelloExtension{
		BaseExtension: extension.NewBaseExtension("hello"),
		message:       "Hello, World!",
	}
}

func (h *HelloExtension) Init(config map[string]interface{}) error {
	if err := h.BaseExtension.Init(config); err != nil {
		return err
	}

	if msg, ok := config["message"]; ok {
		if str, ok := msg.(string); ok && str != "" {
			h.message = str
		}
	}

	return nil
}

func (h *HelloExtension) Bootstrap(app extension.Application) error {
	app.Logger().Info("HelloExtension bootstrapped", "message", h.message)

	app.RegisterService("hello-service", func() interface{} {
		return h
	})

	return nil
}

func (h *HelloExtension) Shutdown() error {
	fmt.Printf("[INFO] HelloExtension shutting down: %s\n", h.message)
	return nil
}

func (h *HelloExtension) SayHello(name string) string {
	return h.message + " " + name
}

// CacheExtension provides caching functionality
type CacheExtension struct {
	*extension.BaseExtension
	cache map[string]interface{}
}

func NewCacheExtension() *CacheExtension {
	return &CacheExtension{
		BaseExtension: extension.NewBaseExtension("cache"),
		cache:         make(map[string]interface{}),
	}
}

func (c *CacheExtension) Init(config map[string]interface{}) error {
	if err := c.BaseExtension.Init(config); err != nil {
		return err
	}

	c.cache = make(map[string]interface{})
	return nil
}

func (c *CacheExtension) Bootstrap(app extension.Application) error {
	app.Logger().Info("CacheExtension bootstrapped")

	app.RegisterService("cache", func() interface{} {
		return c
	})

	return nil
}

func (c *CacheExtension) Get(key string) (interface{}, bool) {
	val, exists := c.cache[key]
	return val, exists
}

func (c *CacheExtension) Set(key string, value interface{}) {
	c.cache[key] = value
}

func (c *CacheExtension) Delete(key string) {
	delete(c.cache, key)
}

func (c *CacheExtension) Shutdown() error {
	c.cache = nil
	return nil
}

// EventLoggerExtension listens to events
type EventLoggerExtension struct {
	*extension.BaseExtension
}

func NewEventLoggerExtension() *EventLoggerExtension {
	return &EventLoggerExtension{
		BaseExtension: extension.NewBaseExtension("event-logger"),
	}
}

func (e *EventLoggerExtension) Events() []string {
	return []string{"request.start", "request.end", "error"}
}

func (e *EventLoggerExtension) HandleEvent(event string, payload interface{}) error {
	fmt.Printf("[INFO] Event received: event=%s, payload=%v\n", event, payload)
	return nil
}

func (e *EventLoggerExtension) Bootstrap(app extension.Application) error {
	app.Logger().Info("EventLoggerExtension bootstrapped")
	return nil
}

func main() {
	fmt.Println("=== Yii-Style Extension System Demo ===\n")

	// Create extension manager
	manager := extension.NewManager()

	// Create and register extensions
	helloExt := NewHelloExtension()
	cacheExt := NewCacheExtension()
	eventLoggerExt := NewEventLoggerExtension()

	// Register extensions
	if err := manager.Register(helloExt); err != nil {
		fmt.Printf("Failed to register hello extension: %v\n", err)
		return
	}

	if err := manager.Register(cacheExt); err != nil {
		fmt.Printf("Failed to register cache extension: %v\n", err)
		return
	}

	if err := manager.Register(eventLoggerExt); err != nil {
		fmt.Printf("Failed to register event logger extension: %v\n", err)
		return
	}

	fmt.Println("✓ Extensions registered:", manager.List())

	// Initialize extensions with configuration
	if err := manager.Init("hello", map[string]interface{}{
		"message": "Welcome to Kopiochi!",
	}); err != nil {
		fmt.Printf("Failed to initialize hello extension: %v\n", err)
		return
	}

	if err := manager.Init("cache", map[string]interface{}{
		"max_size": 1000,
		"ttl":      3600,
	}); err != nil {
		fmt.Printf("Failed to initialize cache extension: %v\n", err)
		return
	}

	fmt.Println("✓ Extensions initialized")

	// Bootstrap all extensions
	if err := manager.Bootstrap(); err != nil {
		fmt.Printf("Failed to bootstrap extensions: %v\n", err)
		return
	}

	fmt.Println("\n✓ All extensions bootstrapped successfully\n")

	// Use services provided by extensions
	if svc, exists := manager.GetService("hello-service"); exists {
		if hello, ok := svc.(*HelloExtension); ok {
			greeting := hello.SayHello("Developer")
			fmt.Printf("✓ Service call result: %s\n", greeting)
		}
	}

	if svc, exists := manager.GetService("cache"); exists {
		if cache, ok := svc.(*CacheExtension); ok {
			cache.Set("user:1", map[string]string{"name": "John Doe"})
			if val, found := cache.Get("user:1"); found {
				fmt.Printf("✓ Cache service working: %v\n", val)
			}
		}
	}

	// Trigger events
	fmt.Println("\n✓ Triggering events...")
	manager.Trigger("request.start", map[string]string{"path": "/api/users"})
	manager.Trigger("request.end", map[string]string{"status": "200"})

	// Shutdown
	fmt.Println("\n✓ Shutting down...")
	if err := manager.Shutdown(); err != nil {
		fmt.Printf("Error during shutdown: %v\n", err)
	}

	fmt.Println("\n=== Demo Complete ===")
}
