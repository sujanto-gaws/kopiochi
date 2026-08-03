// Package testsupport holds test-only helpers shared across package
// boundaries: a disposable Postgres, config fixtures, token minting and HTTP
// request helpers.
//
// It lives as regular .go files rather than _test.go so other packages' tests
// can import it, and it is imported by no production code path.
//
// # It may not import modules/**
//
// Dependency rule R3 applies here like anywhere else under internal/ — see
// docs/architectures/01-modularity/dependency-rules.md. That is why auth.go
// mints tokens with golang-jwt directly instead of reusing the identity
// module's JWTService. The duplication is deliberate: a helper built from the
// production code cannot detect a change in the production code, so this one
// encodes the wire format independently and fails if the two drift apart.
package testsupport

import (
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	// Registers the "pgx" database/sql driver that ScratchPostgres opens.
	//
	// This blank import is load-bearing and was missing: the helper this
	// replaces relied on some other package in the test binary's import graph
	// pulling in internal/db, and tools/schemacheck did not. That test would
	// have failed with `sql: unknown driver "pgx"` the moment a database was
	// reachable — it passed only because it skipped. A helper that opens a
	// connection must register its own driver.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// ScratchPostgres returns a *sql.DB connected to a throwaway Postgres
// database, plus a cleanup func. Database access, in order of preference:
//
//  1. TEST_DATABASE_URL env var — used as-is. This is what CI sets, so its
//     Postgres service container is used rather than nesting containers.
//  2. A disposable `docker run postgres:16` container with a throwaway
//     password, started and torn down entirely within the calling test.
//  3. Neither available — the test SKIPS cleanly (it does not fail), and is
//     unverified against a real server in that environment.
//
// The plan called for testcontainers-go here. It is not used: the value it
// adds over this is lifecycle management for containers a test could otherwise
// leak, and in CI neither path runs at all because TEST_DATABASE_URL is set.
// Adding a Docker API client and its dependency tree to buy that was not worth
// it — see the note in the Phase 4 section of the remediation plan.
func ScratchPostgres(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		sqlDB, err := sql.Open("pgx", dsn)
		require.NoError(t, err)
		require.NoError(t, sqlDB.Ping())
		return sqlDB, func() { _ = sqlDB.Close() }
	}

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("neither TEST_DATABASE_URL nor a docker binary is available; skipping (unverified against a real server)")
	}

	port, err := freeTCPPort()
	require.NoError(t, err)

	containerName := fmt.Sprintf("kopiochi-scratch-pg-%d", time.Now().UnixNano())
	runCmd := exec.Command("docker", "run", "-d", "--rm",
		"--name", containerName,
		"-e", "POSTGRES_USER=test",
		"-e", "POSTGRES_PASSWORD=test", // throwaway, container-local only
		"-e", "POSTGRES_DB=test",
		"-p", fmt.Sprintf("%d:5432", port),
		"postgres:16",
	)
	out, err := runCmd.CombinedOutput()
	if err != nil {
		t.Skipf("could not start disposable postgres container (docker unavailable/unusable in this environment): %v: %s", err, out)
	}
	containerID := strings.TrimSpace(string(out))

	cleanup := func() {
		_ = exec.Command("docker", "rm", "-f", containerID).Run()
	}

	dsn := fmt.Sprintf("postgres://test:test@127.0.0.1:%d/test?sslmode=disable", port)

	deadline := time.Now().Add(60 * time.Second)
	var sqlDB *sql.DB
	for {
		sqlDB, err = sql.Open("pgx", dsn)
		if err == nil {
			if pingErr := sqlDB.Ping(); pingErr == nil {
				break
			}
			_ = sqlDB.Close()
		}
		if time.Now().After(deadline) {
			cleanup()
			t.Fatalf("disposable postgres container never became ready: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	return sqlDB, func() {
		_ = sqlDB.Close()
		cleanup()
	}
}

// MigratedDB returns a *bun.DB backed by ScratchPostgres with every migration
// in migrations/ applied, and registers its teardown with t.
//
// It skips, via ScratchPostgres, when no database is reachable. Callers do not
// need to handle that case; they simply do not run.
func MigratedDB(t *testing.T) *bun.DB {
	t.Helper()

	sqlDB, cleanup := ScratchPostgres(t)
	t.Cleanup(cleanup)

	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(sqlDB, MigrationsDir(t)))

	return bun.NewDB(sqlDB, pgdialect.New())
}

// TruncateAll empties every table in the public schema except goose's version
// table, so a test can start from a known-empty database without paying to
// re-migrate.
//
// RESTART IDENTITY resets sequences (otherwise ids leak across tests and
// assertions on them become order-dependent) and CASCADE handles the foreign
// keys between auth_users and its child tables.
func TruncateAll(t *testing.T, db *bun.DB) {
	t.Helper()

	rows, err := db.QueryContext(t.Context(), `
		SELECT tablename FROM pg_tables
		WHERE schemaname = 'public' AND tablename <> 'goose_db_version'`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var tables []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		tables = append(tables, `"`+name+`"`)
	}
	require.NoError(t, rows.Err())

	if len(tables) == 0 {
		return
	}

	_, err = db.ExecContext(t.Context(),
		"TRUNCATE "+strings.Join(tables, ", ")+" RESTART IDENTITY CASCADE")
	require.NoError(t, err)
}

// MigrationsDir returns the absolute path to the repository's migrations/
// directory.
//
// It is resolved from this source file's own location rather than from the
// test's working directory, because that differs per package — cmd/api's tests
// needed "../../migrations" and a module's would need something else, so every
// caller was encoding its own depth into a relative path.
func MigrationsDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(RepoRoot(t), "migrations")
}

// RepoRoot returns the absolute path to the repository root.
func RepoRoot(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed; cannot locate the repository root")

	// thisFile is <root>/internal/testsupport/db.go.
	root, err := filepath.Abs(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	require.NoError(t, err)
	return root
}

// freeTCPPort asks the OS for an ephemeral port and immediately releases it,
// so the disposable container can be told to bind there. Small TOCTOU race
// in theory; acceptable for a test helper.
func freeTCPPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}
