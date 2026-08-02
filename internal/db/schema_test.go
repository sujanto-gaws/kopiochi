package db

import (
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	usermodels "github.com/sujanto-gaws/kopiochi/internal/infrastructure/persistence/models"
	identitymodels "github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/persistence/models"
)

// TestModelsMatchMigratedSchema is task 1.1(e) from the remediation plan
// (docs/architectures/06-quality/testing-strategy.md:115-119): it applies
// every migration in migrations/ to a scratch Postgres database and then
// asserts, per model, that the set of columns Bun expects for that table
// exactly matches the set of columns information_schema reports — in both
// directions (a column the model expects but the DB lacks, and a column the
// DB has that no model field maps to, are both schema drift).
//
// Database access: this test needs a real Postgres server. It never guesses
// at credentials for whatever may be listening on localhost:5432. In order
// of preference:
//
//  1. TEST_DATABASE_URL env var — used as-is, migrations applied to it.
//  2. A disposable `docker run postgres:16` container with a throwaway
//     password, started and torn down entirely within this test.
//  3. Neither available — the test SKIPS cleanly (it does not fail), and is
//     unverified against a real server in that environment.
func TestModelsMatchMigratedSchema(t *testing.T) {
	sqlDB, cleanup := scratchPostgres(t)
	defer cleanup()

	migrationsDir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	require.NoError(t, err)

	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(sqlDB, migrationsDir))

	bunDB := bun.NewDB(sqlDB, pgdialect.New())

	cases := []struct {
		table string
		model any
	}{
		{"users", usermodels.UserDBModel{}},
		{"auth_users", identitymodels.BunUser{}},
		{"auth_refresh_tokens", identitymodels.RefreshTokenRow{}},
		{"auth_mfa_backup_codes", identitymodels.MfaBackupCodeRow{}},
	}

	for _, tc := range cases {
		t.Run(tc.table, func(t *testing.T) {
			tbl := bunDB.Table(reflect.TypeOf(tc.model))
			require.Equal(t, tc.table, tbl.Name, "model's bun table tag does not match the table under test")

			var expected []string
			for _, f := range tbl.Fields {
				expected = append(expected, f.Name)
			}

			actual := actualColumns(t, sqlDB, tc.table)

			require.ElementsMatch(t, expected, actual,
				"table %q: model columns %v vs information_schema columns %v", tc.table, expected, actual)
		})
	}
}

// actualColumns returns every column information_schema reports for the
// given table in the "public" schema.
func actualColumns(t *testing.T, sqlDB *sql.DB, table string) []string {
	t.Helper()

	rows, err := sqlDB.Query(
		`SELECT column_name FROM information_schema.columns WHERE table_schema = 'public' AND table_name = $1`,
		table,
	)
	require.NoError(t, err)
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var c string
		require.NoError(t, rows.Scan(&c))
		cols = append(cols, c)
	}
	require.NoError(t, rows.Err())
	require.NotEmpty(t, cols, "table %q has no columns — does it exist after migrations ran?", table)
	return cols
}

// scratchPostgres returns a *sql.DB connected to a throwaway Postgres
// database, plus a cleanup func. See TestModelsMatchMigratedSchema's doc
// comment for the source-of-truth precedence.
func scratchPostgres(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		sqlDB, err := sql.Open("pgx", dsn)
		require.NoError(t, err)
		require.NoError(t, sqlDB.Ping())
		return sqlDB, func() { _ = sqlDB.Close() }
	}

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("neither TEST_DATABASE_URL nor a docker binary is available; skipping schema drift test (unverified against a real server)")
	}

	port, err := freeTCPPort()
	require.NoError(t, err)

	containerName := fmt.Sprintf("kopiochi-schema-test-%d", time.Now().UnixNano())
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
