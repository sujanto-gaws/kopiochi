# Adding Custom Plugins to Kopiochi

This guide shows how to create and integrate your own plugins without modifying the plugin core system.

## 📂 Project Structure

```
internal/
├── plugin/              # 🔒 Plugin CORE (don't modify)
│   ├── plugin.go        #   - Core interfaces
│   ├── registry.go      #   - Plugin registry
│   ├── middleware.go    #   - Middleware chain builder
│   └── initializer.go   #   - Config-driven initialization
│
├── plugins/             # 🔧 Built-in plugins (use as examples)
│   ├── register.go      #   - Built-in plugin registration
│   ├── adapters.go      #   - Type adapters
│   ├── auth/
│   │   └── jwt.go       #   - JWT authentication
│   └── middleware/
│       ├── ratelimit.go #   - Rate limiting
│       └── cors.go      #   - CORS handling
│
└── myplugins/           # ✅ YOUR custom plugins (create this)
    ├── compression.go
    ├── apilogger.go
    └── customauth.go
```

**Key Principle:** The `plugin/` package is the CORE - users should only interact with its interfaces, never modify it.

---

## 🚀 Quick Start: Add Your First Plugin

### Step 1: Create Plugin Directory

```bash
mkdir -p internal/myplugins
```

### Step 2: Implement Your Plugin

Create `internal/myplugins/compression.go`:

```go
package myplugins

import (
    "compress/gzip"
    "net/http"
)

// CompressionPlugin compresses HTTP responses
type CompressionPlugin struct {
    initialized bool
    level       int
}

// Name returns the plugin identifier
func (p *CompressionPlugin) Name() string {
    return "compression"
}

// Initialize sets up the plugin with configuration
func (p *CompressionPlugin) Initialize(cfg map[string]interface{}) error {
    if level, ok := cfg["level"].(float64); ok {
        p.level = int(level)
    } else {
        p.level = gzip.DefaultCompression
    }
    p.initialized = true
    return nil
}

// Close performs cleanup
func (p *CompressionPlugin) Close() error {
    p.initialized = false
    return nil
}

// Middleware returns the HTTP middleware
func (p *CompressionPlugin) Middleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !p.initialized {
                next.ServeHTTP(w, r)
                return
            }

            // Only compress if client accepts gzip
            if !acceptsGzip(r) {
                next.ServeHTTP(w, r)
                return
            }

            // Wrap response writer with gzip
            gz := gzip.NewWriterLevel(w, p.level)
            defer gz.Close()

            w.Header().Set("Content-Encoding", "gzip")
            
            // Note: In production, use a proper gzip response writer
            // that handles the content correctly
            next.ServeHTTP(w, r)
        })
    }
}

// Provider returns the plugin instance
func (p *CompressionPlugin) Provider() interface{} {
    return p
}

func acceptsGzip(r *http.Request) bool {
    ae := r.Header.Get("Accept-Encoding")
    return ae == "gzip" || ae == "*"
}

// NewCompressionPlugin creates a new instance
func NewCompressionPlugin() *CompressionPlugin {
    return &CompressionPlugin{}
}
```

### Step 3: Register Your Plugin

Update `internal/plugins/register.go` to include your plugin:

```go
package plugins

import (
    "github.com/sujanto-gaws/kopiochi/internal/plugin"
    "github.com/sujanto-gaws/kopiochi/internal/plugins/auth"
    "github.com/sujanto-gaws/kopiochi/internal/plugins/middleware"
    "github.com/sujanto-gaws/kopiochi/internal/myplugins"  // ← Add import
)

// RegisterBuiltinPlugins registers all plugins
func RegisterBuiltinPlugins(registry *plugin.Registry) {
    // Authentication plugins
    registry.Register("jwt-auth", func() plugin.Plugin {
        return &authPluginAdapter{auth.NewJWTPlugin()}
    })

    // Middleware plugins
    registry.Register("ratelimit", func() plugin.Plugin {
        return &middlewarePluginAdapter{middleware.NewRateLimiterPlugin()}
    })
    registry.Register("cors", func() plugin.Plugin {
        return &middlewarePluginAdapter{middleware.NewCORSPlugin()}
    })

    // 🎉 Add your custom plugins here
    registry.Register("compression", func() plugin.Plugin {
        return &middlewarePluginAdapter{myplugins.NewCompressionPlugin()}
    })
}
```

### Step 4: Enable in Configuration

Edit `config/default.yaml`:

```yaml
plugins:
  middleware:
    - cors
    - compression  # ← Add your plugin
  
  custom:
    compression:
      level: 6  # Custom configuration
```

### Step 5: Build and Run

```bash
go build ./...
go run ./cmd/api serve
```

---

## 📋 Plugin Interfaces

### MiddlewarePlugin (Most Common)

For HTTP middleware that processes requests/responses:

```go
type MyPlugin struct {
    // Your fields
}

func (p *MyPlugin) Name() string {
    return "myplugin"
}

func (p *MyPlugin) Initialize(cfg map[string]interface{}) error {
    // Parse configuration
    return nil
}

func (p *MyPlugin) Close() error {
    // Cleanup resources
    return nil
}

func (p *MyPlugin) Middleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Your logic here
            
            // Call next handler
            next.ServeHTTP(w, r)
        })
    }
}

func (p *MyPlugin) Provider() interface{} {
    return p
}
```

### AuthPlugin (For Authentication)

For authentication providers with user context extraction:

```go
type MyAuthPlugin struct {
    // Your fields
}

// Implement all MiddlewarePlugin methods, plus:

func (p *MyAuthPlugin) AuthMiddleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Validate credentials
            
            // Add user to context
            ctx := context.WithValue(r.Context(), "user_id", userID)
            r = r.WithContext(ctx)
            
            next.ServeHTTP(w, r)
        })
    }
}

func (p *MyAuthPlugin) ExtractUserID(ctx context.Context) string {
    if id, ok := ctx.Value("user_id").(string); ok {
        return id
    }
    return ""
}
```

### CachePlugin (For Caching)

For cache providers (Redis, Memcached, etc.):

```go
type MyCachePlugin struct {
    client *redis.Client
}

func (p *MyCachePlugin) Get(ctx context.Context, key string) (interface{}, error) {
    return p.client.Get(ctx, key).Result()
}

func (p *MyCachePlugin) Set(ctx context.Context, key string, value interface{}) error {
    return p.client.Set(ctx, key, value, 10*time.Minute).Err()
}

func (p *MyCachePlugin) Delete(ctx context.Context, key string) error {
    return p.client.Del(ctx, key).Err()
}
```

---

## 🎨 Real-World Examples

### Example 1: Request Logger Plugin

```go
package myplugins

import (
    "net/http"
    "time"

    "github.com/rs/zerolog/log"
)

type RequestLoggerPlugin struct {
    initialized bool
    includeBody bool
}

func (p *RequestLoggerPlugin) Name() string {
    return "request-logger"
}

func (p *RequestLoggerPlugin) Initialize(cfg map[string]interface{}) error {
    if includeBody, ok := cfg["include_body"].(bool); ok {
        p.includeBody = includeBody
    }
    p.initialized = true
    return nil
}

func (p *RequestLoggerPlugin) Close() error {
    p.initialized = false
    return nil
}

func (p *RequestLoggerPlugin) Middleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            
            // Log request
            log.Info().
                Str("method", r.Method).
                Str("path", r.URL.Path).
                Str("remote_addr", r.RemoteAddr).
                Time("started_at", start).
                Msg("request started")
            
            // Call next
            next.ServeHTTP(w, r)
            
            // Log duration
            log.Info().
                Str("method", r.Method).
                Str("path", r.URL.Path).
                Dur("duration_ms", time.Since(start)).
                Msg("request completed")
        })
    }
}

func (p *RequestLoggerPlugin) Provider() interface{} {
    return p
}

func NewRequestLoggerPlugin() *RequestLoggerPlugin {
    return &RequestLoggerPlugin{}
}
```

### Example 2: API Key Authentication

```go
package myplugins

import (
    "context"
    "net/http"
    "strings"
)

type APIKeyPlugin struct {
    initialized bool
    validKeys   map[string]bool
    headerName  string
}

func (p *APIKeyPlugin) Name() string {
    return "api-key-auth"
}

func (p *APIKeyPlugin) Initialize(cfg map[string]interface{}) error {
    p.validKeys = make(map[string]bool)
    
    if keys, ok := cfg["keys"].([]interface{}); ok {
        for _, key := range keys {
            if keyStr, ok := key.(string); ok {
                p.validKeys[keyStr] = true
            }
        }
    }
    
    if headerName, ok := cfg["header_name"].(string); ok {
        p.headerName = headerName
    } else {
        p.headerName = "X-API-Key"
    }
    
    p.initialized = true
    return nil
}

func (p *APIKeyPlugin) Close() error {
    p.initialized = false
    p.validKeys = nil
    return nil
}

func (p *APIKeyPlugin) Middleware() func(http.Handler) http.Handler {
    return p.AuthMiddleware()
}

func (p *APIKeyPlugin) AuthMiddleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            apiKey := r.Header.Get(p.headerName)
            if apiKey == "" {
                // Try Authorization header as fallback
                auth := r.Header.Get("Authorization")
                if strings.HasPrefix(auth, "Bearer ") {
                    apiKey = strings.TrimPrefix(auth, "Bearer ")
                }
            }
            
            if apiKey == "" || !p.validKeys[apiKey] {
                http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
                return
            }
            
            // Add API key to context
            ctx := context.WithValue(r.Context(), "api_key", apiKey)
            r = r.WithContext(ctx)
            
            next.ServeHTTP(w, r)
        })
    }
}

func (p *APIKeyPlugin) ExtractUserID(ctx context.Context) string {
    if key, ok := ctx.Value("api_key").(string); ok {
        return key
    }
    return ""
}

func (p *APIKeyPlugin) Provider() interface{} {
    return p
}

func NewAPIKeyPlugin() *APIKeyPlugin {
    return &APIKeyPlugin{}
}
```

---

## 🔧 Advanced: Plugin with Database Access

If your plugin needs database access:

```go
package myplugins

import (
    "database/sql"
    "net/http"
)

type AuditLoggerPlugin struct {
    initialized bool
    db          *sql.DB
    tableName   string
}

func (p *AuditLoggerPlugin) Initialize(cfg map[string]interface{}) error {
    // Get database connection from config
    if db, ok := cfg["db"].(*sql.DB); ok {
        p.db = db
    } else {
        return fmt.Errorf("audit-logger: db connection required")
    }
    
    if table, ok := cfg["table"].(string); ok {
        p.tableName = table
    } else {
        p.tableName = "audit_logs"
    }
    
    p.initialized = true
    return nil
}

func (p *AuditLoggerPlugin) Middleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Log to database
            _, err := p.db.Exec(
                fmt.Sprintf("INSERT INTO %s (method, path, timestamp) VALUES (?, ?, NOW())", p.tableName),
                r.Method,
                r.URL.Path,
            )
            if err != nil {
                log.Error().Err(err).Msg("failed to log audit trail")
            }
            
            next.ServeHTTP(w, r)
        })
    }
}
```

Register it with database connection:

```go
// In main.go
pluginRegistry := plugin.NewRegistry()

// Pass database connection to plugin
plugins.RegisterBuiltinPlugins(pluginRegistry)

// Or manually register custom plugin with DB
pluginRegistry.Register("audit-logger", func() plugin.Plugin {
    return &middlewarePluginAdapter{
        myplugins.NewAuditLoggerPlugin(db.DB),  // Pass your DB connection
    }
})
```

---

## ✅ Best Practices

### Do's

✅ **Keep plugins focused** - One responsibility per plugin  
✅ **Validate configuration** - Return errors in `Initialize()`  
✅ **Handle uninitialized state** - Gracefully skip if not initialized  
✅ **Use context** - Pass request-scoped data via context  
✅ **Document configuration** - Show example YAML in comments  
✅ **Write tests** - Test plugin logic in isolation  

### Don'ts

❌ **Don't modify `internal/plugin/`** - It's the core system  
❌ **Don't create global state** - Use plugin instances  
❌ **Don't block indefinitely** - Use timeouts in middleware  
❌ **Don't panic on errors** - Return errors or log them  
❌ **Don't forget cleanup** - Implement `Close()` properly  

---

## 📦 Organizing Multiple Custom Plugins

For large projects, organize like this:

```
internal/
├── myplugins/
│   ├── middleware/
│   │   ├── compression.go
│   │   ├── requestlogger.go
│   │   └── ratelimit_custom.go
│   ├── auth/
│   │   ├── apikey.go
│   │   └── oauth2.go
│   └── cache/
│       └── redis.go
```

Update `internal/plugins/register.go`:

```go
package plugins

import (
    "github.com/sujanto-gaws/kopiochi/internal/plugin"
    "github.com/sujanto-gaws/kopiochi/internal/myplugins/auth"
    "github.com/sujanto-gaws/kopiochi/internal/myplugins/middleware"
)

func RegisterBuiltinPlugins(registry *plugin.Registry) {
    // Built-in plugins
    registry.Register("jwt-auth", ...)
    
    // Custom middleware
    registry.Register("compression", func() plugin.Plugin {
        return &middlewarePluginAdapter{middleware.NewCompressionPlugin()}
    })
    
    // Custom auth
    registry.Register("api-key", func() plugin.Plugin {
        return &authPluginAdapter{auth.NewAPIKeyPlugin()}
    })
}
```

---

## 🔍 Debugging Plugins

### List All Registered Plugins

```go
// In main.go or handlers
registered := pluginRegistry.List()
log.Info().Strs("plugins", registered).Msg("all registered plugins")
```

### Check If Plugin Is Initialized

```go
if pluginRegistry.IsInitialized("compression") {
    log.Info().Msg("compression plugin is active")
}
```

### Get Plugin Instance

```go
// Get as middleware plugin
mwPlugin := pluginRegistry.GetMiddleware("compression")
if mwPlugin != nil {
    // Use the middleware
}

// Get as auth plugin
authPlugin := pluginRegistry.GetAuth("jwt-auth")
if authPlugin != nil {
    userID := authPlugin.ExtractUserID(ctx)
}
```

---

## 📝 Summary

1. **Create your plugin** in `internal/myplugins/` (or any custom package)
2. **Implement the interface** (`Name()`, `Initialize()`, `Close()`, `Middleware()`)
3. **Register it** in `internal/plugins/register.go`
4. **Enable it** in `config/default.yaml`
5. **Build and run!**

The plugin core (`internal/plugin/`) remains untouched - you only interact with its interfaces.

For questions, see [PLUGIN_GUIDE.md](../PLUGIN_GUIDE.md) or open an issue.
