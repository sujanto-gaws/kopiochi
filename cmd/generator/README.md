# DDD CRUD Generator

Automatically generates complete CRUD operations for any domain entity following DDD architecture.

## Usage

```bash
go run cmd/generator/main.go -domain <DomainName> -fields "<field:type pairs>" [options]
```

## Examples

### Basic Product Domain
```bash
go run cmd/generator/main.go \
  -domain Product \
  -fields "name:string,description:string,price:float64,stock:int,category:string"
```

### Order Domain
```bash
go run cmd/generator/main.go \
  -domain Order \
  -fields "customerId:int64,total:float64,status:string,orderDate:time" \
  -table orders
```

### Custom Output
```bash
go run cmd/generator/main.go \
  -domain Category \
  -fields "name:string,slug:string" \
  -module github.com/yourname/yourproject \
  -output internal
```

## Flags

| Flag | Description | Default |
|------|-------------|---------|
| `-domain` | Domain name (required) | - |
| `-fields` | Comma-separated field:type pairs | `name:string,description:string` |
| `-module` | Go module path | `github.com/sujanto-gaws/kopiochi` |
| `-output` | Output directory | `internal` |
| `-table` | Database table name | Auto-pluralized domain |
| `-author` | Author name | - |

## Supported Field Types

| Type | Go Type |
|------|---------|
| `string`, `text`, `varchar` | `string` |
| `int`, `integer` | `int64` |
| `float`, `decimal` | `float64` |
| `bool`, `boolean` | `bool` |
| `time`, `datetime` | `time.Time` |
| `uuid` | `string` |

## Generated Structure

```
<domain>/
├── domain/
│   ├── entity.go      # Pure domain entity + validation
│   ├── repository.go  # Repository interface
│   └── dto.go         # Request/Response DTOs
├── application/
│   └── service.go     # Use case logic
└── infrastructure/
    ├── repository.go  # DB implementation
    ├── model.go       # ORM model
    └── handler.go     # HTTP handlers
```

## Generated Endpoints

- `POST   /api/v1/<domain>s` - Create
- `GET    /api/v1/<domain>s/{id}` - Get by ID
- `GET    /api/v1/<domain>s` - List (with pagination)
- `PUT    /api/v1/<domain>s/{id}` - Update
- `DELETE /api/v1/<domain>s/{id}` - Delete

## Integration

After generation, move files to your project:

```bash
# Generate
go run cmd/generator/main.go -domain Product -fields "name:string,price:float64"

# Move to internal (if not using -output internal)
mv tmp/product internal/

# Add routes in routes.go
import "github.com/yourname/project/internal/infrastructure/http/handlers"

r.Post("/products", productHandler.CreateProduct())
r.Get("/products/{id}", productHandler.GetProduct())
# ... etc
```

## Features

✅ Pure domain entities (no infrastructure concerns)
✅ Request/Response DTOs with JSON tags
✅ Automatic validation
✅ Pagination support for list endpoints
✅ Error handling with standardized responses
✅ Database model separation
✅ Mapper functions between layers
✅ Graceful shutdown support
