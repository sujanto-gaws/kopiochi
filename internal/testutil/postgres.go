// Package testutil holds test-only helpers that need to be shared across
// package boundaries (e.g. internal/db and cmd/api both need a disposable
// Postgres instance). It is not imported by any production code path — only
// by _test.go files — but lives as regular .go files (not _test.go) so it is
// importable from other packages' tests.
package testutil

import (
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ScratchPostgres returns a *sql.DB connected to a throwaway Postgres
// database, plus a cleanup func. Database access, in order of preference:
//
//  1. TEST_DATABASE_URL env var — used as-is.
//  2. A disposable `docker run postgres:16` container with a throwaway
//     password, started and torn down entirely within the calling test.
//  3. Neither available — the test SKIPS cleanly (it does not fail), and is
//     unverified against a real server in that environment.
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

// freeTCPPort asks the OS for an ephemeral port and immediately releases it,
// so the disposable container can be told to bind there. Small TOCTOU race
// in theory; acceptable for a test helper.
func freeTCPPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
