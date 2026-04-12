# Database Migrations

> **Version-controlled database schema management with Goose**

This project uses [Goose](https://github.com/pressly/goose) for database migrations. Migrations are version-controlled, reversible, and integrated into the project as a CLI command.

## 📋 Table of Contents

- [Quick Start](#-quick-start)
- [Migration Structure](#-migration-structure)
- [Available Commands](#-available-commands)
- [Creating Migrations](#-creating-migrations)
- [Writing SQL Migrations](#-writing-sql-migrations)
- [Writing Go Migrations](#-writing-go-migrations)
- [Migration Workflow](#-migration-workflow)
- [Best Practices](#-best-practices)
- [CI/CD Integration](#-cicd-integration)
- [Troubleshooting](#-troubleshooting)

## 🚀 Quick Start

### Run Migrations

```bash
# Run all pending migrations
make migrate-up

# Check migration status
make migrate-status
```

### Create a New Migration

```bash
# Create a new SQL migration
make migrate-create NAME=create_products_table

# This creates: migrations/20260412123456_create_products_table.sql
```

### Rollback

```bash
# Rollback the last migration
make migrate-down

# Rollback all migrations
make migrate-reset
```

## 📁 Migration Structure

All migrations are stored in the `migrations/` directory:

```
migrations/
├── 00001_create_users.sql
├── 00002_create_products.sql
└── 20260412123456_create_orders.sql
```

### Migration File Naming

- **Sequential**: `00001_description.sql`, `00002_description.sql`
- **Timestamp-based**: `20260412123456_description.sql` (when using `make migrate-create`)

Both formats work with Goose. Sequential is used in this project for clarity.

## 🎯 Available Commands

### Make Commands

| Command | Description | Example |
|---------|-------------|---------|
| `make migrate-up` | Run all pending migrations | `make migrate-up` |
| `make migrate-down` | Rollback last migration | `make migrate-down` |
| `make migrate-status` | Show migration status | `make migrate-status` |
| `make migrate-reset` | Rollback all migrations | `make migrate-reset` |
| `make migrate-create` | Create new migration | `make migrate-create NAME=add_users_index` |
| `make migrate-install` | Install Goose CLI | `make migrate-install` |

### CLI Commands (Direct)

```bash
# Using the built-in migration command
go run ./cmd/migrate up
go run ./cmd/migrate down
go run ./cmd/migrate status
go run ./cmd/migrate reset
go run ./cmd/migrate create migration_name

# With custom config
go run ./cmd/migrate up --config config/production.yaml

# With custom migrations directory
go run ./cmd/migrate up --dir migrations
```

## 📝 Creating Migrations

### SQL Migrations (Recommended)

Create a new migration:

```bash
make migrate-create NAME=create_orders_table
```

This creates a file: `migrations/TIMESTAMP_create_orders_table.sql`

Edit the file with your SQL:

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    total DECIMAL(10, 2) NOT NULL DEFAULT 0.00,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_orders_user_id ON orders(user_id);
CREATE INDEX idx_orders_status ON orders(status);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_orders_status;
DROP INDEX IF EXISTS idx_orders_user_id;
DROP TABLE IF EXISTS orders;
-- +goose StatementEnd
```

### Go Migrations (Advanced)

For complex migrations requiring Go logic:

```bash
go run ./cmd/migrate create add_users_column --type go
```

This creates a Go migration file:

```go
package migrations

import (
    "context"
    "database/sql"

    "github.com/pressly/goose/v3"
)

func init() {
    goose.AddMigrationContext(UpAddUsersColumn, DownAddUsersColumn)
}

func UpAddUsersColumn(ctx context.Context, db *sql.DB) error {
    _, err := db.ExecContext(ctx, `
        ALTER TABLE users 
        ADD COLUMN IF NOT EXISTS phone VARCHAR(20),
        ADD COLUMN IF NOT EXISTS address TEXT;
    `)
    return err
}

func DownAddUsersColumn(ctx context.Context, db *sql.DB) error {
    _, err := db.ExecContext(ctx, `
        ALTER TABLE users 
        DROP COLUMN IF EXISTS phone,
        DROP COLUMN IF EXISTS address;
    `)
    return err
}
```

## 📖 Writing SQL Migrations

### Up Migration (Apply)

The `Up` section defines changes to apply:

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE products (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    price DECIMAL(10, 2) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
-- +goose StatementEnd
```

### Down Migration (Rollback)

The `Down` section defines how to reverse the changes:

```sql
-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS products;
-- +goose StatementEnd
```

### Best Practices for SQL Migrations

#### 1. **Use IF EXISTS / IF NOT EXISTS**

```sql
CREATE TABLE IF NOT EXISTS users (...);
DROP TABLE IF EXISTS users;
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
DROP INDEX IF EXISTS idx_users_email;
```

#### 2. **Separate Statements**

Use `-- +goose StatementBegin` and `-- +goose StatementEnd` for multi-line statements:

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_orders_user_id ON orders(user_id);
-- +goose StatementEnd
```

#### 3. **Add Indexes Separately**

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE products (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_products_name ON products(name);
-- +goose StatementEnd
```

#### 4. **Data Migrations**

You can also insert initial data:

```sql
-- +goose Up
-- +goose StatementBegin
INSERT INTO users (name, email) VALUES 
    ('Admin User', 'admin@example.com'),
    ('Test User', 'test@example.com')
ON CONFLICT (email) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM users WHERE email IN ('admin@example.com', 'test@example.com');
-- +goose StatementEnd
```

## 🏗️ Migration Workflow

### Development Workflow

```bash
# 1. Create a new migration
make migrate-create NAME=add_products_category

# 2. Edit the migration file
# migrations/TIMESTAMP_add_products_category.sql

# 3. Run the migration
make migrate-up

# 4. Verify the changes
make migrate-status

# 5. Commit the migration file
git add migrations/
git commit -m "migration: add products category column"
```

### Rollback Workflow

```bash
# Made a mistake? Rollback the last migration
make migrate-down

# Fix the migration file
# migrations/TIMESTAMP_migration.sql

# Re-apply
make migrate-up
```

### Reset Workflow (Start Fresh)

```bash
# Rollback all migrations
make migrate-reset

# Re-run from scratch
make migrate-up

# Verify everything works
make migrate-status
```

## 📋 Best Practices

### 1. **Never Edit Applied Migrations**

Once a migration has been applied, never modify it. Instead, create a new migration:

```bash
# ❌ Don't edit an existing migration
# ✅ Do create a new one
make migrate-create NAME=fix_products_column
```

### 2. **Test Rollbacks**

Always test your `Down` migrations:

```bash
# Apply migration
make migrate-up

# Test rollback
make migrate-down

# Re-apply
make migrate-up
```

### 3. **Use Transactions**

Goose runs each migration in a transaction by default. If you need explicit control:

```sql
-- +goose Up
-- +goose StatementBegin
BEGIN;

CREATE TABLE orders (...);

COMMIT;
-- +goose StatementEnd
```

### 4. **Keep Migrations Atomic**

Each migration should do one thing:

- ✅ `create_users_table`
- ✅ `add_users_email_index`
- ✅ `add_users_phone_column`
- ❌ `create_users_products_orders_and_seed_data`

### 5. **Name Migrations Clearly**

Use descriptive names:

- ✅ `add_users_email_index`
- ✅ `create_products_table`
- ❌ `migration1`
- ❌ `update_stuff`

### 6. **Version Control All Migrations**

Never generate migrations in production. Always commit them:

```bash
git add migrations/
git commit -m "migration: create products table"
git push
```

### 7. **Use Foreign Keys Wisely**

```sql
CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE
);
```

Consider:
- `ON DELETE CASCADE` - Delete child records when parent is deleted
- `ON DELETE SET NULL` - Set foreign key to NULL when parent is deleted
- `ON DELETE RESTRICT` - Prevent deletion if child records exist

### 8. **Add Indexes for Performance**

```sql
-- Indexes for common queries
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_products_name ON products(name);
CREATE INDEX idx_orders_user_id ON orders(user_id);
```

## 🚀 CI/CD Integration

### GitHub Actions Example

```yaml
name: Database Migrations

on:
  push:
    branches: [main]

jobs:
  migrate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.25'
      
      - name: Run Migrations
        run: make migrate-up
        env:
          APP_DB_HOST: ${{ secrets.DB_HOST }}
          APP_DB_PORT: 5432
          APP_DB_USER: ${{ secrets.DB_USER }}
          APP_DB_PASSWORD: ${{ secrets.DB_PASSWORD }}
          APP_DB_NAME: ${{ secrets.DB_NAME }}
          APP_DB_SSLMODE: require
```

### Docker Compose Example

```yaml
services:
  api:
    build: .
    depends_on:
      db:
        condition: service_healthy
    command: ["./kopiochi", "serve"]

  migrate:
    build: .
    depends_on:
      db:
        condition: service_healthy
    command: ["./kopiochi-migrate", "up"]

  db:
    image: postgres:16
    environment:
      POSTGRES_DB: kopiochi
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 5s
      timeout: 5s
      retries: 5
```

### Deployment Script Example

```bash
#!/bin/bash
# deploy.sh

echo "Running database migrations..."
make migrate-up CONFIG=config/production.yaml

if [ $? -eq 0 ]; then
    echo "Migrations successful. Deploying application..."
    # Deploy application
    make build
    systemctl restart kopiochi
else
    echo "Migration failed! Aborting deployment."
    exit 1
fi
```

## 🔧 Troubleshooting

### Migration Fails Mid-Way

Goose automatically rolls back failed migrations. If a migration fails:

1. Check the error message
2. Fix the migration file
3. Re-run `make migrate-up`

### Check Migration Status

```bash
make migrate-status
```

Output shows:
- ✅ **Applied** migrations (with timestamp)
- ⬜ **Pending** migrations (not yet applied)

### Reset All Migrations

If you need to start fresh (development only):

```bash
# Rollback all migrations
make migrate-reset

# Re-apply from scratch
make migrate-up
```

### Manual Intervention

In rare cases, you may need to manually fix the migration state:

```sql
-- Check migration version
SELECT * FROM goose_db_version;

-- Set to specific version (use with caution!)
UPDATE goose_db_version SET version_id = 2 WHERE id = 1;
```

### Common Errors

#### "relation already exists"

```sql
-- Use IF NOT EXISTS
CREATE TABLE IF NOT EXISTS users (...);
```

#### "column does not exist"

```sql
-- Add column with migration
ALTER TABLE users ADD COLUMN phone VARCHAR(20);
```

#### "migration failed: duplicate key"

```sql
-- Use ON CONFLICT
INSERT INTO users (email) VALUES ('test@test.com')
ON CONFLICT (email) DO NOTHING;
```

## 📚 Additional Resources

- [Goose Official Documentation](https://github.com/pressly/goose)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
- [SQL Migration Best Practices](https://www.prisma.io/dataguide/types/relational/database-migrations)

---

**Need Help?** Check the [README.md](README.md) for general project information or open an issue on GitHub.
