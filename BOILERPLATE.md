# Kopiochi Boilerplate Guide

## 🎯 Overview

**Kopiochi** is a production-ready boilerplate for building Go web APIs using **Domain-Driven Design (DDD)** principles. It provides a clean, extensible foundation with modern tooling and best practices.

### Why Use This Boilerplate?

✅ **Clean Architecture** - Strict DDD layer separation  
✅ **Production-Ready** - Graceful shutdown, structured logging, health checks  
✅ **Plugin System** - Extensible middleware, auth, and cache  
✅ **Code Generator** - Auto-generate CRUD domains in seconds  
✅ **Best Practices** - Dependency injection, interface-based design  
✅ **Modern Stack** - chi router, bun ORM, pgx, viper, zerolog  

---

## 🚀 Quick Start

### Option 1: Use GitHub Template (Recommended)

1. Click **"Use this template"** on GitHub
2. Create your new repository
3. Clone and initialize:

```bash
git clone https://github.com/YOUR_USERNAME/YOUR_PROJECT.git
cd YOUR_PROJECT

# Initialize the boilerplate
make init-project PROJECT=myapi AUTHOR="Your Name"
```

### Option 2: Clone and Initialize

```bash
# Clone the repository
git clone https://github.com/sujanto-gaws/kopiochi.git myapi
cd myapi

# Remove original git history
rm -rf .git

# Initialize with your project details
make init-project PROJECT=myapi AUTHOR="Your Name"

# Make initial commit
git init
git add .
git commit -m "Initial commit: myapi boilerplate"
```

### Option 3: Manual Setup

```bash
# Clone
git clone https://github.com/sujanto-gaws/kopiochi.git myapi
cd myapi
rm -rf .git

# Manually edit these files:
# - go.mod: Change module path
# - config/default.yaml: Update project name
# - .env.example: Update database name
# - README.md: Update project details
# - All *.go files: Replace import paths

# Or use the scripts directly:
./scripts/init.sh --project-name myapi --author "Your Name"
# or on Windows:
.\scripts\init.ps1 -ProjectName myapi -Author "Your Name"
```

---

## 🛠️ Initialization

### Using Make (Linux/Mac)

```bash
# Basic initialization
make init-project PROJECT=myapi AUTHOR="John Doe"

# With custom database name
make init-project PROJECT=myapi AUTHOR="John Doe" DB_NAME=myapi_db

# Keep example User CRUD domain
# (Edit scripts/init.sh and set REMOVE_EXAMPLE=false)
```

### Using PowerShell (Windows)

```powershell
# Basic initialization
.\scripts\init.ps1 -ProjectName myapi -Author "John Doe"

# With custom module path
.\scripts\init.ps1 -ProjectName myapi -Author "John Doe" `
  -ModulePath "github.com/john/myapi" -DBName myapi_db
```

### Using Bash Script (Linux/Mac)

```bash
# Basic initialization
./scripts/init.sh --project-name myapi --author "John Doe"

# With all options
./scripts/init.sh \
  --project-name myapi \
  --author "John Doe" \
  --module-path github.com/john/myapi \
  --db-name myapi_db

# Keep example domain
./scripts/init.sh --project-name myapi --keep-example
```

### What Gets Changed

The initialization script updates:

1. **Module Path** - All Go import paths
2. **Project Name** - README, config, CLI name
3. **Database Name** - Config and .env.example
4. **Author** - LICENSE copyright
5. **Removes Example Domain** - User CRUD (optional)
6. **Resets Git History** - Fresh start (optional)

---

## 📁 Project Structure

```
myapi/
├── cmd/
│   ├── api/              # Main API server entry point
│   │   └── main.go
│   └── generator/        # Code generator for new domains
│       └── main.go
├── config/
│   └── default.yaml      # Default configuration
├── internal/
│   ├── domain/           # Domain layer (entities, interfaces)
│   ├── application/      # Application layer (use cases)
│   ├── infrastructure/   # Infrastructure layer (HTTP, DB)
│   │   ├── http/
│   │   │   ├── handlers/
│   │   │   └── routes/
│   │   └── persistence/
│   │       └── repository/
│   ├── plugin/           # Plugin system
│   │   ├── auth/
│   │   └── middleware/
│   ├── config/           # Configuration loader
│   ├── db/               # Database connection
│   ├── logger/           # Logger setup
│   ├── middleware/       # HTTP middleware
│   └── server/           # Server setup
├── scripts/
│   ├── init.ps1          # Windows initialization script
│   └── init.sh           # Linux/Mac initialization script
├── Makefile              # Development commands
├── .env.example          # Environment variables template
├── Dockerfile            # Docker build configuration
├── go.mod                # Go module definition
└── README.md             # Project documentation
```

---

## 🔧 Development Workflow

### 1. Initial Setup

```bash
# After initialization
make run              # Start the server
make test             # Run tests
make build            # Build binary
```

### 2. Generate Your First Domain

You have two options for generating CRUD domains:

#### Option A: Generate from Explicit Fields

```bash
# Generate Product CRUD
make generate DOMAIN=Product FIELDS="name:string,description:string,price:float64,stock:int"
```

#### Option B: Generate from Existing Database Table

If you already have a table in your database, the generator can read the schema directly:

```bash
# Read schema from database (uses DB config from config/default.yaml)
go run cmd/generator/main.go -domain Product -table products

# With custom config file
go run cmd/generator/main.go -domain Product -table products -config config/production.yaml
```

The generator will:
- Connect to your database using the config from `config/default.yaml`
- Read column names, data types, and nullable constraints from `information_schema.columns`
- Automatically skip internal columns (`id`, `created_at`, `updated_at`)
- Map PostgreSQL types to Go types (`varchar` → `string`, `bigint` → `int64`, etc.)
- Convert `snake_case` column names to `CamelCase` for Go field names

#### Generated Files

Both methods generate the following files:

```
internal/
├── domain/product/
│   ├── entity.go          # Domain entity with validation
│   ├── repository.go      # Repository interface
│   └── dto.go             # Request/Response DTOs
├── application/product/
│   └── service.go         # Business logic layer
└── infrastructure/
    ├── persistence/
    │   ├── repository/
    │   │   └── product_repository.go  # Repository implementation
    │   └── models/
    │       └── product_model.go       # Database model
    └── http/
        └── handlers/
            └── product_handler.go     # HTTP handlers
```

#### Auto-Wiring

The generator automatically:
1. **Updates routes** - Adds handler parameter and route definitions to `internal/infrastructure/http/routes/routes.go`
2. **Wires dependencies** - Adds repository, service, and handler initialization to `cmd/api/main.go`
3. **Registers routes** - Adds the new handler to `routes.Setup()` call

No manual file moving or route editing required!

### 3. Add Routes

Edit `internal/infrastructure/http/routes/routes.go`:

```go
func Setup(r *chi.Mux, userHandler *handlers.UserHandler, productHandler *handlers.ProductHandler) {
    // ... existing routes
    
    r.Route("/api/v1/products", func(r chi.Router) {
        r.Post("/", productHandler.CreateProduct)
        r.Get("/", productHandler.GetProducts)
        r.Get("/{id}", productHandler.GetProductByID)
        r.Put("/{id}", productHandler.UpdateProduct)
        r.Delete("/{id}", productHandler.DeleteProduct)
    })
}
```

### 4. Enable Plugins

Edit `config/default.yaml`:

```yaml
plugins:
  middleware:
    - cors
    - ratelimit
  
  auth:
    jwt:
      enabled: true
      provider: jwt-auth
      config:
        secret: "${JWT_SECRET}"
        expiry: "24h"
```

### 5. Build and Deploy

```bash
# Build binary
make build

# Build Docker image
make docker-build

# Run Docker container
make docker-run
```

---

## 📋 Common Commands

| Command | Description |
|---------|-------------|
| `make help` | Show all available commands |
| `make run` | Start the server |
| `make build` | Build the binary |
| `make test` | Run all tests |
| `make test-coverage` | Run tests with coverage report |
| `make lint` | Run linter |
| `make fmt` | Format code |
| `make generate DOMAIN=X FIELDS=Y` | Generate CRUD domain |
| `make docker-build` | Build Docker image |
| `make clean` | Remove build artifacts |

See the full [Makefile](Makefile) for all available commands.

---

## 🎨 Customization

### Adding New Middleware

1. Create in `internal/middleware/`:

```go
// internal/middleware/mycustom.go
package middleware

import "net/http"

func MyCustomMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Your logic
        next.ServeHTTP(w, r)
    })
}
```

2. Add to router in `internal/server/server.go`:

```go
func NewRouter() *chi.Mux {
    r := chi.NewRouter()
    
    r.Use(middleware.Recoverer)
    r.Use(middleware.RequestID)
    r.Use(MyCustomMiddleware)  // Add here
    
    return r
}
```

### Adding New Plugins

See [PLUGIN_GUIDE.md](PLUGIN_GUIDE.md) for detailed instructions.

### Changing Database

The boilerplate uses PostgreSQL with Bun ORM. To switch:

1. Update `internal/db/database.go` with new driver
2. Change dialect in bun initialization
3. Update `config/default.yaml` accordingly

Supported by Bun: PostgreSQL, MySQL, SQLite, MSSQL

---

## 🚦 CI/CD Setup

### GitHub Actions Example

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  build:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:14
        env:
          POSTGRES_PASSWORD: postgres
          POSTGRES_DB: myapi_test
        ports:
          - 5432:5432
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.25'
    
    - name: Install dependencies
      run: go mod download
    
    - name: Run tests
      run: go test -v ./...
      env:
        APP_DB_HOST: localhost
        APP_DB_PORT: 5432
        APP_DB_USER: postgres
        APP_DB_PASSWORD: postgres
        APP_DB_NAME: myapi_test
    
    - name: Build
      run: go build -v ./cmd/api
```

---

## 📚 Learning Resources

### Domain-Driven Design

- [Domain-Driven Design Distilled](https://www.infoq.com/minibooks/domain-driven-design-quickly/)
- [Clean Architecture by Robert C. Martin](https://www.amazon.com/Clean-Architecture-Craftsmans-Software-Structure/dp/0134494164)

### Go Best Practices

- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Effective Go](https://golang.org/doc/effective_go)

### Technologies Used

- [chi router](https://github.com/go-chi/chi)
- [Bun ORM](https://bun.uptrace.dev/)
- [Viper](https://github.com/spf13/viper)
- [Zerolog](https://github.com/rs/zerolog)
- [Cobra CLI](https://github.com/spf13/cobra)

---

## 🤝 Contributing

Found a bug or have a feature request? Please open an issue or submit a PR.

### Development Setup

```bash
# Fork and clone
git clone https://github.com/YOUR_USERNAME/kopiochi.git
cd kopiochi

# Install dev dependencies
make deps-update

# Run tests
make test

# Make your changes and submit PR
```

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## ☕ Support

If you find this boilerplate useful, consider:

- ⭐ Starring the repository
- 🐛 Reporting bugs
- 💡 Suggesting features
- 🔀 Contributing improvements

---

**Built with ❤️ using Go and Domain-Driven Design principles**
