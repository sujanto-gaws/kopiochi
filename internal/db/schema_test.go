package db

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"github.com/sujanto-gaws/kopiochi/internal/testutil"
	identitymodels "github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/persistence/models"
	usermodels "github.com/sujanto-gaws/kopiochi/modules/user/infrastructure/persistence/models"
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
// at credentials for whatever may be listening on localhost:5432. See
// internal/testutil.ScratchPostgres for the source-of-truth precedence
// (TEST_DATABASE_URL, then a disposable docker container, then a clean skip).
func TestModelsMatchMigratedSchema(t *testing.T) {
	sqlDB, cleanup := testutil.ScratchPostgres(t)
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
