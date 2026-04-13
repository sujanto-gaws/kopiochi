# Kopiochi

> **🔥 Production-Ready DDD Go API Boilerplate**

A **Domain-Driven Design (DDD)** Go web API boilerplate built with modern, production-ready technologies. Start your next Go project with clean architecture, plugin system, and code generation in seconds.

**[📚 View Boilerplate Guide](BOILERPLATE.md)** | **[🔌 Plugin Documentation](PLUGIN_GUIDE.md)** | **[📝 Swagger API Documentation](SWAGGER.md)** | **[🗄️ Database Migrations](MIGRATIONS.md)**

## 🏗️ Architecture

This project follows **Domain-Driven Design (DDD)** principles with clear separation of concerns:

```
internal/
├── domain/           # Business entities, rules, and domain interfaces
├── application/      # Use cases and application services
└── infrastructure/   # External concerns (HTTP, persistence, etc.)
```

### Layer Responsibilities

| Layer | Purpose |
|-------|---------|
| **Domain** | Core business logic, entities, and repository interfaces |
| **Application** | Use case orchestration and application services |
| **Infrastructure** | HTTP handlers, database repositories, external integrations |

## 🚀 Tech Stack

| Component | Technology |
|-----------|------------|
| **Router** | [chi v5](https://github.com/go-chi/chi) - Lightweight, idiomatic HTTP router |
| **Database** | [bun](https://github.com/uptrace/bun) - SQL ORM for Go |
| **Driver** | [pgx v5](https://github.com/jackc/pgx) - PostgreSQL driver |
| **Config** | [viper](https://github.com/spf13/viper) - Configuration management |
| **CLI** | [cobra](https://github.com/spf13/cobra) - Command-line interface framework |
| **Logging** | [zerolog](https://github.com/rs/zerolog) - Fast, structured logging |

## 📋 Features

- ✅ **Domain-Driven Design** - Clean architecture with separation of concerns
- ✅ **Dependency Injection** - Loose coupling between layers
- ✅ **Plugin System** - Extensible middleware, auth, and cache plugins
- ✅ **Swagger/OpenAPI Documentation** - Auto-generated API documentation
- ✅ **Database Migrations** - Version-controlled schema management with Goose
- ✅ **PostgreSQL** - Production-ready database with connection pooling
- ✅ **Structured Logging** - JSON or console format with configurable levels
- ✅ **Health Check Endpoint** - Ready for Kubernetes/container orchestration
- ✅ **Environment Configuration** - Flexible config via YAML, env vars, or both
- ✅ **Docker Support** - Multi-stage build for optimized container images

## 🛠️ Getting Started

### Quick Start (Recommended)

```bash
# 1. Use as GitHub template or clone
git clone https://github.com/sujanto-gaws/kopiochi.git myapi
cd myapi
rm -rf .git

# 2. Initialize with your project name
make init-project PROJECT=myapi AUTHOR="Your Name"
# Or on Windows:
# .\scripts\init.ps1 -ProjectName myapi -Author "Your Name"

# 3. Start developing
make run
```

**📖 See full setup instructions: [BOILERPLATE.md](BOILERPLATE.md)**

### Prerequisites

- Go 1.25+
- PostgreSQL 14+
- Docker (optional)

### Installation

```bash
# Clone the repository
git clone https://github.com/sujanto-gaws/kopiochi.git
cd kopiochi

# Initialize as your project
make init-project PROJECT=myapi AUTHOR="Your Name"

# Copy environment example
cp .env.example .env

# Update configuration as needed
# Edit .env or config/default.yaml
```

### Running Locally

```bash
# Start the server
make run
# or
go run ./cmd/api serve

# Or with custom config
go run ./cmd/api serve --config config/default.yaml
```

## 💻 Development Workflow

### Generate New Domain (CRUD)

```bash
# Generate Product domain with all CRUD operations
make generate DOMAIN=Product FIELDS="name:string,description:string,price:float64,stock:int"

# This creates:
# ✅ Domain entity with validation
# ✅ Repository interface
# ✅ DTOs (Request/Response)
# ✅ Application service
# ✅ Database model & repository
# ✅ HTTP handlers
# ✅ Routes (manual registration needed)
```

### Common Commands

```bash
make help             # Show all commands
make run              # Start server
make build            # Build binary
make test             # Run tests
make test-coverage    # Run tests with coverage
make lint             # Run linter
make fmt              # Format code
make swagger-docs     # Generate swagger documentation
make migrate-up       # Run database migrations
make migrate-status   # Check migration status
make docker-build     # Build Docker image
```

See [BOILERPLATE.md](BOILERPLATE.md) for complete workflow documentation.

### Running with Docker

```bash
# Build the image
docker build -t kopiochi .

# Run the container
docker run -p 8080:8080 --env-file .env kopiochi
```

## 📡 API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `POST` | `/api/v1/users` | Create a new user |
| `GET` | `/api/v1/users/{id}` | Get user by ID |
| `PUT` | `/api/v1/users/{id}` | Update a user |
| `DELETE` | `/api/v1/users/{id}` | Delete a user |

### 📚 API Documentation (Swagger)

This project includes auto-generated Swagger/OpenAPI documentation for all endpoints.

**Quick Start:**
```bash
# 1. Generate docs
make swagger-docs

# 2. Start server
make run

# 3. Open browser
# Navigate to: http://localhost:8080/swagger/index.html
```

**📖 See complete guide: [SWAGGER.md](SWAGGER.md)**

#### What You Can Do
- ✅ Browse all API endpoints with interactive UI
- ✅ Test endpoints directly from the browser
- ✅ View detailed request/response schemas
- ✅ Authenticate with JWT to test protected endpoints
- ✅ Export code examples in multiple languages
- ✅ Download OpenAPI spec (JSON/YAML)

### Example Requests

**Create User:**
```bash
curl -X POST http://localhost:8080/api/v1/users \
  -H "Content-Type: application/json" \
  -d '{"name":"John Doe","email":"john@example.com"}'
```

**Get User:**
```bash
curl http://localhost:8080/api/v1/users/1
```

## ⚙️ Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `APP_SERVER_HOST` | `0.0.0.0` | Server bind address |
| `APP_SERVER_PORT` | `8080` | Server port |
| `APP_DB_HOST` | `localhost` | PostgreSQL host |
| `APP_DB_PORT` | `5432` | PostgreSQL port |
| `APP_DB_USER` | `postgres` | Database user |
| `APP_DB_PASSWORD` | `postgres` | Database password |
| `APP_DB_NAME` | `kopiochi` | Database name |
| `APP_DB_SSLMODE` | `disable` | SSL mode for database |
| `APP_DB_MAX_CONNS` | `10` | Maximum database connections |
| `APP_DB_MIN_CONNS` | `2` | Minimum database connections |
| `APP_LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `APP_LOG_FORMAT` | `json` | Log format (json, console) |

## �️ Database Migrations

This project uses [Goose](https://github.com/pressly/goose) for version-controlled database migrations.

### Quick Start

```bash
# Run all pending migrations
make migrate-up

# Check migration status
make migrate-status

# Create a new migration
make migrate-create NAME=create_products_table

# Rollback last migration
make migrate-down
```

### Example Migration Commands

```bash
# Run migrations
make migrate-up

# Create migration
make migrate-create NAME=add_users_index

# Check status
make migrate-status
```

**📖 See complete guide: [MIGRATIONS.md](MIGRATIONS.md)**

## �📁 Project Structure

```
kopiochi/
├── cmd/
│   ├── api/
│   │   └── main.go          # Application entry point
│   ├── generator/
│   │   └── main.go          # Code generator for CRUD operations
│   └── migrate/
│       └── main.go          # Database migration CLI
├── config/
│   └── default.yaml         # Default configuration
├── migrations/              # Database migrations (Goose)
│   ├── 00001_create_users.sql
│   └── 00002_create_products.sql
├── internal/
│   ├── application/         # Application layer (use cases)
│   │   └── user/
│   │       └── service.go
│   ├── config/              # Configuration loading
│   ├── db/                  # Database connection setup
│   ├── domain/              # Domain layer (entities, interfaces)
│   │   └── user/
│   │       ├── user.go
│   │       ├── repository.go
│   │       └── service.go
│   ├── infrastructure/      # Infrastructure layer
│   │   ├── http/
│   │   │   └── handlers/
│   │   └── persistence/
│   │       └── repository/
│   ├── logger/              # Logger initialization
│   ├── middleware/          # HTTP middleware
│   └── server/              # Server setup and run
├── .env.example             # Environment variables template
├── Dockerfile               # Docker build configuration
├── go.mod                   # Go module definition
└── README.md
```

## 🔌 Plugin System

Kopiochi includes a powerful, config-driven plugin system that allows you to easily extend functionality without code changes.

### Available Plugins

| Plugin | Type | Description |
|--------|------|-------------|
| `jwt-auth` | Authentication | JWT-based authentication with token generation |
| `fido2-auth` | Authentication | FIDO2/WebAuthn passwordless authentication (passkeys) |
| `ratelimit` | Middleware | Request rate limiting per client IP |
| `cors` | Middleware | Cross-Origin Resource Sharing support |

### Configuration

Enable and configure plugins in `config/default.yaml`:

```yaml
plugins:
  # Middleware plugins (applied in order)
  middleware:
    - cors
    - ratelimit
  
  # Authentication plugins
  auth:
    jwt:
      enabled: false
      provider: jwt-auth
      config:
        secret: "your-secret-key"
        expiry: "24h"
        issuer: "kopiochi"
    
    fido2:
      enabled: false
      provider: fido2-auth
      config:
        rp_id: "localhost"
        rp_origin: "http://localhost:3000"
        rp_name: "kopiochi"
  
  # Cache plugins (coming soon)
  cache: {}
  
  # Custom plugins
  custom: {}
```

### Creating Custom Plugins

1. Create your plugin in `internal/plugin/<category>/`
2. Implement the required interface:
   - **MiddlewarePlugin**: `Name()`, `Initialize()`, `Close()`, `Middleware()`
   - **AuthPlugin**: All middleware methods + `ExtractUserID()`
   - **CachePlugin**: `Get()`, `Set()`, `Delete()`
3. Register it in `internal/plugin/register.go`

Example:
```go
// internal/plugin/middleware/myplugin.go
package middleware

type MyPlugin struct { /* ... */ }

func (p *MyPlugin) Name() string { return "myplugin" }
func (p *MyPlugin) Initialize(cfg map[string]interface{}) error { /* ... */ }
func (p *MyPlugin) Close() error { /* ... */ }
func (p *MyPlugin) Middleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Your middleware logic
            next.ServeHTTP(w, r)
        })
    }
}
func (p *MyPlugin) Provider() interface{} { return p }
```

Then register it:
```go
// internal/plugin/register.go
func RegisterBuiltinPlugins(registry *Registry) {
    // ... existing plugins
    registry.Register("myplugin", func() Plugin {
        return &middlewarePluginAdapter{middleware.NewMyPlugin()}
    })
}
```

## 🧪 Testing

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/domain/user/...
```

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

---

**Built with ❤️ using Go and Domain-Driven Design principles**
