# Kopiochi

A **Domain-Driven Design (DDD)** Go web API built with modern, production-ready technologies.

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
- ✅ **PostgreSQL** - Production-ready database with connection pooling
- ✅ **Structured Logging** - JSON or console format with configurable levels
- ✅ **Health Check Endpoint** - Ready for Kubernetes/container orchestration
- ✅ **Environment Configuration** - Flexible config via YAML, env vars, or both
- ✅ **Docker Support** - Multi-stage build for optimized container images

## 🛠️ Getting Started

### Prerequisites

- Go 1.25+
- PostgreSQL 14+
- Docker (optional)

### Installation

```bash
# Clone the repository
git clone https://github.com/sujanto-gaws/kopiochi.git
cd kopiochi

# Copy environment example
cp .env.example .env

# Update configuration as needed
# Edit .env or config/default.yaml
```

### Running Locally

```bash
# Start the server
go run ./cmd/api serve

# Or with custom config
go run ./cmd/api serve --config config/default.yaml
```

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

## 📁 Project Structure

```
kopiochi/
├── cmd/
│   └── api/
│       └── main.go          # Application entry point
├── config/
│   └── default.yaml         # Default configuration
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
