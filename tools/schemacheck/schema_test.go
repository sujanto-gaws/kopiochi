// Package schemacheck asserts that every module's Bun models still match the
// schema the migrations produce.
//
// It lives under tools/ rather than in internal/db, where it started, because
// it must import the persistence models of *every* module at once — and
// internal/** is the shared kernel, which may not depend on a business module
// (R3 in docs/architectures/01-modularity/dependency-rules.md). Only cmd/**
// and the cross-cutting checks under tools/ get to know about every module.
//
// depguard caught this the moment the rules were switched on; the test itself
// is unchanged apart from its package clause.
package schemacheck

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
	identitymodels "github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/persistence/models"
	notifmodels "github.com/sujanto-gaws/kopiochi/modules/notification/infrastructure/persistence/models"
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
// internal/testsupport.ScratchPostgres for the source-of-truth precedence
// (TEST_DATABASE_URL, then a disposable docker container, then a clean skip).
func TestModelsMatchMigratedSchema(t *testing.T) {
	bunDB := testsupport.MigratedDB(t)

	cases := []struct {
		table string
		model any
	}{
		{"users", usermodels.UserDBModel{}},
		{"auth_users", identitymodels.BunUser{}},
		{"auth_refresh_tokens", identitymodels.RefreshTokenRow{}},
		{"auth_mfa_backup_codes", identitymodels.MfaBackupCodeRow{}},
		{"notifications", notifmodels.NotificationRow{}},
		{"notification_preferences", notifmodels.NotificationPreferenceRow{}},
	}

	for _, tc := range cases {
		t.Run(tc.table, func(t *testing.T) {
			tbl := bunDB.Table(reflect.TypeOf(tc.model))
			require.Equal(t, tc.table, tbl.Name, "model's bun table tag does not match the table under test")

			var expected []string
			for _, f := range tbl.Fields {
				expected = append(expected, f.Name)
			}

			actual := actualColumns(t, bunDB.DB, tc.table)

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
	defer func() { _ = rows.Close() }()

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
