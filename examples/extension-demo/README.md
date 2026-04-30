# Yii-Style Extension System for Go

This document describes the Yii-style extension system implemented for the Kopiochi project, inspired by the Yii framework's module architecture.

## Overview

The extension system provides a modular architecture where functionality can be added through extensions that follow a standardized lifecycle. Unlike VS Code's process-isolated extension model, this system runs extensions in the same process for maximum performance and deep integration.

## Architecture

### Core Components

1. **Extension Interface** (`internal/extension/extension.go`)
   - Base interface defining the extension lifecycle
   - Optional interfaces for specific capabilities (routing, events, services)

2. **Manager** (`internal/extension/manager.go`)
   - Centralized extension management
   - Handles registration, initialization, bootstrapping, and shutdown
   - Implements service container and event dispatcher

3. **BaseExtension** (`internal/extension/base.go`)
   - Helper struct providing common functionality
   - Configuration management utilities

## Extension Lifecycle

```
Register → Init → Bootstrap → (Running) → Shutdown
```

### 1. Register
Add the extension to the manager without initializing it.

### 2. Init
Initialize the extension with configuration parameters.

### 3. Bootstrap
Called after all extensions are initialized. This is where extensions:
- Register services
- Set up event listeners
- Register HTTP routes
- Perform other setup tasks

### 4. Running
Extensions are active and can:
- Provide services to other components
- Handle events
- Serve HTTP requests

### 5. Shutdown
Clean up resources when the application stops.

## Creating an Extension

### Basic Extension

```go
package myext

import "github.com/sujanto-gaws/kopiochi/internal/extension"

type MyExtension struct {
    *extension.BaseExtension
}

func NewMyExtension() *MyExtension {
    return &MyExtension{
        BaseExtension: extension.NewBaseExtension("my-ext"),
    }
}

func (m *MyExtension) Init(config map[string]interface{}) error {
    return m.BaseExtension.Init(config)
}

func (m *MyExtension) Bootstrap(app extension.Application) error {
    app.Logger().Info("MyExtension bootstrapped")
    return nil
}

func (m *MyExtension) Shutdown() error {
    return nil
}
```

### Service Provider Extension

```go
type CacheExtension struct {
    *extension.BaseExtension
    cache map[string]interface{}
}

func (c *CacheExtension) Bootstrap(app extension.Application) error {
    // Register a service
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
```

### Event Listener Extension

```go
type LoggerExtension struct {
    *extension.BaseExtension
}

// Implement EventListener interface
func (e *LoggerExtension) Events() []string {
    return []string{"request.start", "request.end", "error"}
}

func (e *LoggerExtension) HandleEvent(event string, payload interface{}) error {
    fmt.Printf("Event: %s, Payload: %v\n", event, payload)
    return nil
}
```

### Route Provider Extension

```go
type APIExtension struct {
    *extension.BaseExtension
}

// Implement RoutableExtension interface
func (a *APIExtension) Routes(router extension.RouterRegistrar) error {
    router.GET("/api/health", healthHandler)
    router.POST("/api/users", createUserHandler)
    
    router.Group("/api/v1", func() {
        router.GET("/users", listUsersHandler)
    })
    
    return nil
}
```

## Using Extensions

### Registration and Initialization

```go
package main

import (
    "github.com/sujanto-gaws/kopiochi/internal/extension"
    "myapp/extensions"
)

func main() {
    // Create manager
    manager := extension.NewManager()
    
    // Create extensions
    cacheExt := extensions.NewCacheExtension()
    authExt := extensions.NewAuthExtension()
    
    // Register
    manager.Register(cacheExt)
    manager.Register(authExt)
    
    // Initialize with config
    manager.Init("cache", map[string]interface{}{
        "max_size": 1000,
        "ttl": 3600,
    })
    
    manager.Init("auth", map[string]interface{}{
        "secret_key": "your-secret",
    })
    
    // Bootstrap all
    if err := manager.Bootstrap(); err != nil {
        panic(err)
    }
    
    // Use services
    if svc, ok := manager.GetService("cache"); ok {
        cache := svc.(*extensions.CacheExtension)
        cache.Set("key", "value")
    }
    
    // Trigger events
    manager.Trigger("request.start", requestInfo)
    
    // Shutdown on exit
    defer manager.Shutdown()
}
```

## Optional Interfaces

Extensions can implement these optional interfaces to gain additional capabilities:

| Interface | Purpose |
|-----------|---------|
| `BootstrappableExtension` | Early bootstrap phase |
| `EventListener` | Listen to application events |
| `RoutableExtension` | Register HTTP routes |
| `ServiceProvider` | Provide services to container |

## Configuration

Extensions receive configuration during initialization:

```go
func (e *MyExtension) Init(config map[string]interface{}) error {
    // Access configuration values
    maxSize := e.GetInt("max_size", 100)
    enabled := e.GetBool("enabled", true)
    name := e.GetString("name", "default")
    
    return nil
}
```

## Comparison with Other Systems

### vs VS Code Extensions
- **Same process** vs isolated processes
- **Deep integration** vs sandboxed API
- **Type-safe** vs dynamic messaging
- **Faster** but less fault-isolated

### vs Yii Modules
- Similar lifecycle (init, bootstrap, run)
- Similar service container pattern
- Similar event system
- Go's type safety vs PHP's flexibility

## Best Practices

1. **Keep extensions focused**: Each extension should do one thing well
2. **Use base class**: Embed `BaseExtension` to reduce boilerplate
3. **Handle errors gracefully**: Return meaningful errors from lifecycle methods
4. **Clean up resources**: Always implement `Shutdown()` properly
5. **Document dependencies**: Make it clear what your extension requires
6. **Use optional interfaces**: Only implement what you need

## Example Extensions

See `/workspace/examples/extension-demo/main.go` for complete working examples:
- `HelloExtension` - Basic extension with service registration
- `CacheExtension` - Service provider example
- `EventLoggerExtension` - Event listener example

## Running the Demo

```bash
cd /workspace
go run ./examples/extension-demo/main.go
```

Expected output:
```
=== Yii-Style Extension System Demo ===

✓ Extensions registered: [hello cache event-logger]
✓ Extensions initialized
[INFO] HelloExtension bootstrapped [message Welcome to Kopiochi!]
[INFO] CacheExtension bootstrapped []
[INFO] EventLoggerExtension bootstrapped []

✓ All extensions bootstrapped successfully

✓ Service call result: Welcome to Kopiochi! Developer
✓ Cache service working: map[name:John Doe]

✓ Triggering events...
[INFO] Event received: event=request.start, payload=map[path:/api/users]
[INFO] Event received: event=request.end, payload=map[status:200]

✓ Shutting down...
[INFO] HelloExtension shutting down: Welcome to Kopiochi!

=== Demo Complete ===
```
