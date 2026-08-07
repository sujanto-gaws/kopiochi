package db_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// TestModelsMatchMigratedSchema is the priority-1 schema-drift test: it
// applies the real migration chain (migrations/) into a disposable schema
// and asserts every column the identity bun models declare
// (internal/infrastructure/persistence/auth/models) actually exists in the
// migrated database. This is the check that would have caught "users" (the
// old migration) vs "auth_users" (what the code needs) immediately.
//
// It requires a reachable Postgres instance; when one is not configured it
// skips rather than failing the whole suite (matching the CI job in
// docs/architectures/06-quality/testing-strategy.md, which runs this against
// a disposable Postgres service).
func TestModelsMatchMigratedSchema(t *testing.T) {
	sqlDB, err := sql.Open("pgx", testDSN())
	if err != nil {
		t.Skipf("schema drift test: cannot open database handle, skipping: %v", err)
	}
	defer sqlDB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Skipf("schema drift test: database not reachable, skipping: %v", err)
	}

	schemaName := fmt.Sprintf("schema_drift_test_%d", time.Now().UnixNano())
	if _, err := sqlDB.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schemaName)); err != nil {
		t.Fatalf("create scratch schema: %v", err)
	}
	defer sqlDB.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schemaName))

	if _, err := sqlDB.Exec(fmt.Sprintf(`SET search_path TO %q`, schemaName)); err != nil {
		t.Fatalf("set search_path: %v", err)
	}

	if err := goose.Up(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	// Column sets declared by the identity bun models — see
	// internal/infrastructure/persistence/auth/models/auth_models.go.
	wantColumns := map[string][]string{
		"auth_users": {
			"id", "username", "email", "name", "roles", "permissions",
			"password_hash", "mfa_enabled", "mfa_secret",
			"failed_login_attempts", "locked_until", "created_at", "updated_at",
		},
		"auth_refresh_tokens": {
			"id", "user_id", "token_hash", "expires_at", "revoked", "created_at",
		},
		"auth_mfa_backup_codes": {
			"id", "user_id", "code_hash", "used", "created_at",
		},
	}

	const q = `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2 AND column_name = $3
	)`

	for table, cols := range wantColumns {
		for _, col := range cols {
			var exists bool
			if err := sqlDB.QueryRowContext(ctx, q, schemaName, table, col).Scan(&exists); err != nil {
				t.Fatalf("query information_schema for %s.%s: %v", table, col, err)
			}
			if !exists {
				t.Errorf("migrated schema is missing column %s.%s declared on the bun model", table, col)
			}
		}
	}
}

func testDSN() string {
	host := envOr("APP_DB_HOST", "localhost")
	port := envOr("APP_DB_PORT", "5432")
	user := envOr("APP_DB_USER", "postgres")
	pass := envOr("APP_DB_PASSWORD", "postgres")
	name := envOr("APP_DB_NAME", "kopiochi")
	ssl := envOr("APP_DB_SSLMODE", "disable")
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, name, ssl)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
