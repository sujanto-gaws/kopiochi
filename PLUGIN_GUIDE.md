# Kopiochi Plugin System Guide

## Overview

Kopiochi's plugin system provides a flexible, config-driven way to extend the API with middleware, authentication, cache providers, and custom functionality. Plugins are initialized at startup based on YAML configuration and can be easily enabled/disabled without code changes.

## Architecture

```
internal/plugin/
├── plugin.go              # Core plugin interfaces
├── registry.go            # Plugin registry & lifecycle management
├── middleware.go          # Middleware chain builder
├── initializer.go         # Config-driven plugin initialization
├── register.go            # Built-in plugin registration
├── auth/
│   └── jwt.go             # JWT authentication plugin
└── middleware/
    ├── types.go           # Shared middleware types
    ├── ratelimit.go       # Rate limiting plugin
    └── cors.go            # CORS plugin
```

## Plugin Types

### 1. MiddlewarePlugin

Applied to all HTTP requests through the chi router middleware chain.

**Interface:**
```go
type MiddlewarePlugin interface {
    Name() string
    Initialize(cfg map[string]interface{}) error
    Close() error
    Middleware() func(http.Handler) http.Handler
}
```

**Built-in Examples:**
- `cors` - Cross-Origin Resource Sharing
- `ratelimit` - Request rate limiting

### 2. AuthPlugin

Specialized plugin for authentication with user context extraction.

**Interface:**
```go
type AuthPlugin interface {
    MiddlewarePlugin
    AuthMiddleware() func(http.Handler) http.Handler
    ExtractUserID(ctx context.Context) string
}
```

**Built-in Examples:**
- `jwt-auth` - JWT Bearer token authentication

### 3. CachePlugin

For caching providers (Redis, Memcached, etc.).

**Interface:**
```go
type CachePlugin interface {
    ProviderPlugin
    Get(ctx context.Context, key string) (interface{}, error)
    Set(ctx context.Context, key string, value interface{}) error
    Delete(ctx context.Context, key string) error
}
```

**Planned:**
- Redis cache provider
- Memcached provider

## Configuration

### Basic Setup

Edit `config/default.yaml`:

```yaml
plugins:
  # Middleware plugins - applied in order
  middleware:
    - cors
    - ratelimit
  
  # Authentication plugins
  auth:
    jwt:
      enabled: true
      provider: jwt-auth
      config:
        secret: "${JWT_SECRET}"  # Use env var
        expiry: "24h"
        issuer: "kopiochi"
  
  # Cache plugins
  cache:
    redis:
      enabled: false
      provider: redis
      config:
        host: "localhost"
        port: 6379
  
  # Custom plugins
  custom:
    myplugin:
      some_option: "value"
```

### Environment Variables

Plugin configuration can be overridden via environment variables:

```bash
# JWT Auth
APP_PLUGINS_AUTH_JWT_SECRET=my-secret
APP_PLUGINS_AUTH_JWT_EXPIRY=48h

# Rate Limiting
APP_PLUGINS_MIDDLEWARE='["cors","ratelimit"]'
```

## Using Built-in Plugins

### JWT Authentication

**Enable in config:**
```yaml
plugins:
  auth:
    jwt:
      enabled: true
      provider: jwt-auth
      config:
        secret: "your-256-bit-secret"
        expiry: "24h"
```

**Generate tokens:**
```go
// In your handler
jwtPlugin := plugin.GetAuthPlugin(registry, &cfg.Plugins)
if jwtPlugin != nil {
    token, err := jwtPlugin.(*auth.JWTPlugin).GenerateToken(
        "user-123", 
        "John Doe", 
        "john@example.com",
    )
}
```

**Protect routes:**
```go
// The middleware automatically validates tokens
// and adds user info to context
r.Use(jwtPlugin.AuthMiddleware())

// Extract user ID in handler
userID := jwtPlugin.ExtractUserID(r.Context())
```

### Rate Limiting

**Enable in config:**
```yaml
plugins:
  middleware:
    - ratelimit
  
  custom:
    ratelimit:
      requests: 100      # Max requests per window
      window: "1m"       # Time window
```

**Behavior:**
- Tracks requests per client IP
- Returns `429 Too Many Requests` when limit exceeded
- Adds `X-RateLimit-Limit` and `X-RateLimit-Remaining` headers
- Respects `X-Forwarded-For` for proxied requests

### CORS

**Enable in config:**
```yaml
plugins:
  middleware:
    - cors
  
  custom:
    cors:
      allowed_origins:
        - "https://example.com"
        - "https://app.example.com"
      allowed_methods:
        - "GET"
        - "POST"
        - "PUT"
        - "DELETE"
      allowed_headers:
        - "Authorization"
        - "Content-Type"
      allow_credentials: true
      max_age: 300
```

**Defaults:**
- `allowed_origins`: `["*"]` (allow all)
- `allowed_methods`: `["GET", "POST", "PUT", "DELETE", "OPTIONS"]`
- `allowed_headers`: Common headers including `Authorization`
- `allow_credentials`: `false`
- `max_age`: `300` (5 minutes)

## Creating Custom Plugins

### Step 1: Create Plugin File

Create your plugin in `internal/plugin/<category>/`:

```go
// internal/plugin/middleware/compression.go
package middleware

import (
    "compress/gzip"
    "net/http"
)

// CompressionPlugin compresses HTTP responses
type CompressionPlugin struct {
    initialized bool
    level       int
}

func (p *CompressionPlugin) Name() string {
    return "compression"
}

func (p *CompressionPlugin) Initialize(cfg map[string]interface{}) error {
    if level, ok := cfg["level"].(float64); ok {
        p.level = int(level)
    } else {
        p.level = gzip.DefaultCompression
    }
    p.initialized = true
    return nil
}

func (p *CompressionPlugin) Close() error {
    p.initialized = false
    return nil
}

func (p *CompressionPlugin) Middleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !p.initialized {
                next.ServeHTTP(w, r)
                return
            }
            
            // Add compression wrapper
            gz := gzip.NewWriterLevel(w, p.level)
            defer gz.Close()
            
            // Set content-encoding header
            w.Header().Set("Content-Encoding", "gzip")
            
            // Call next handler
            next.ServeHTTP(w, r)
        })
    }
}

func (p *CompressionPlugin) Provider() interface{} {
    return p
}

// NewCompressionPlugin creates a new instance
func NewCompressionPlugin() *CompressionPlugin {
    return &CompressionPlugin{}
}
```

### Step 2: Register the Plugin

Add to `internal/plugin/register.go`:

```go
func RegisterBuiltinPlugins(registry *Registry) {
    // ... existing plugins
    
    // Register compression middleware
    registry.Register("compression", func() Plugin {
        return &middlewarePluginAdapter{middleware.NewCompressionPlugin()}
    })
}
```

### Step 3: Enable in Configuration

```yaml
plugins:
  middleware:
    - cors
    - compression  # Add your plugin
```

### Step 4: Build and Run

```bash
go build ./...
go run ./cmd/api serve
```

## Programmatic Access

### Access Plugin Registry

```go
// In main.go or anywhere you have access to registry
registry, err := plugin.InitializeFromConfig(&cfg.Plugins)
if err != nil {
    log.Fatal(err)
}

// Get a specific plugin
authPlugin := registry.GetAuth("jwt-auth")
cachePlugin := registry.GetCache("redis")

// Use plugin methods
userID := authPlugin.ExtractUserID(ctx)
```

### Access from Handlers

Pass the registry to your handlers:

```go
type UserHandler struct {
    service  *appUser.Service
    registry *plugin.Registry
}

func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
    // Extract authenticated user
    userID := h.registry.GetAuth("jwt-auth").ExtractUserID(r.Context())
    
    // Your logic...
}
```

## Plugin Lifecycle

1. **Registration**: Plugins are registered with the registry via `RegisterBuiltinPlugins()`
2. **Initialization**: Config is loaded and plugins are initialized via `InitializeFromConfig()`
3. **Execution**: Middleware plugins are applied to requests through the middleware chain
4. **Shutdown**: Plugins are closed during graceful shutdown

## Best Practices

### ✅ Do

- Use config-driven activation (YAML + env vars)
- Keep plugins focused on single responsibilities
- Use interfaces for loose coupling
- Validate configuration in `Initialize()`
- Clean up resources in `Close()`
- Handle uninitialized state gracefully

### ❌ Don't

- Create import cycles (plugin package → auth → plugin)
- Block in middleware without timeouts
- Store request state in plugin instances (use context)
- Panic on invalid config (return errors instead)
- Forget to close plugins on shutdown

## Troubleshooting

### Plugin Not Initializing

**Check:**
1. Plugin name matches in config and registration
2. No errors in `InitializeFromConfig()`
3. `enabled: true` for auth/cache plugins

**Debug:**
```go
log.Info().Strs("available", registry.List()).Msg("registered plugins")
log.Info().Strs("initialized", registry.ListInitialized()).Msg("active plugins")
```

### Import Cycle Error

**Cause:** Plugin package imports auth/middleware which imports plugin package.

**Solution:** Use adapter pattern (see `register.go`) or define interfaces in the plugin subpackages.

### Middleware Not Applied

**Check:**
1. Plugin name in `plugins.middleware` array
2. Order matters - middleware is applied in array order
3. Check `middlewareChain.Len() > 0`

## Advanced: Custom Plugin Types

### Database Provider Plugin

```go
type DatabasePlugin interface {
    plugin.ProviderPlugin
    GetDB() *sql.DB
    Migrate() error
    Seed() error
}
```

### Event Bus Plugin

```go
type EventBusPlugin interface {
    plugin.ProviderPlugin
    Publish(event string, data interface{}) error
    Subscribe(event string, handler func(interface{})) error
}
```

### Metrics Plugin

```go
type MetricsPlugin interface {
    plugin.MiddlewarePlugin
    RecordMetric(name string, value float64)
    GetMetrics() map[string]interface{}
}
```

## Future Enhancements

- [ ] Redis cache plugin
- [ ] API Key authentication plugin
- [ ] OAuth2 plugin
- [ ] Request validation plugin
- [ ] Audit logging plugin
- [ ] GraphQL plugin
- [ ] WebSocket plugin
- [ ] gRPC plugin support

## Contributing

To contribute a new plugin:

1. Create plugin in `internal/plugin/<category>/`
2. Implement required interfaces
3. Add to `RegisterBuiltinPlugins()`
4. Update documentation
5. Add tests
6. Submit PR

---

For questions or issues, please open an issue on GitHub.
