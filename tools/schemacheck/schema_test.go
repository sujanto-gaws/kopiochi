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

// TestNullableColumnsMapToNilableFields — BL33, the class E10 was one instance of.
//
// A nullable column mapped to a plain Go value collapses two different facts
// into one: "this was never set" and "this was set to the zero value" arrive at
// the caller identically. Whether that matters depends entirely on what the
// caller then does with the zero value, which is not a property any reviewer can
// see from the model.
//
// E10 is what it costs when the answer is bad. mfa_secret is nullable and was a
// plain string, so an account that never ran MFA setup presented as one whose
// secret is "" — and the TOTP code derived from the empty secret is computable
// by anyone with a clock. The second factor was a public constant. password_hash
// has the identical shape and happens to fail closed, which is luck about what
// bcrypt does with "", not a difference in the models.
//
// So the rule is mechanical rather than case-by-case: a nullable column needs a
// Go type that has a nil. Pointers qualify; so do maps and slices, which carry
// their own absent state. Anything else is a value that cannot say "absent".
//
// bun's `nullzero` tag deliberately does NOT satisfy this. It makes WRITES send
// NULL for a zero value; it does nothing for reads, where NULL still scans to
// the zero value and the ambiguity survives in the direction that bit E10.
func TestNullableColumnsMapToNilableFields(t *testing.T) {
	bunDB := testsupport.MigratedDB(t)

	// Deliberate exceptions live here, one line each, with the reason. An empty
	// map is the goal state: every entry is a column where somebody decided the
	// zero value and absence mean the same thing, and wrote down why.
	exempt := map[string]string{
		"auth_users.name": "a person with no name and a person whose name is \"\" are the " +
			"same thing to every consumer: it is displayed, never compared, never " +
			"authenticated against, and no branch anywhere reads it. Unlike password_hash " +
			"and mfa_secret, nothing downstream turns the zero value into a decision.",
	}

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
			nullable := nullableColumns(t, bunDB.DB, tc.table)
			tbl := bunDB.Table(reflect.TypeOf(tc.model))

			var checked int
			for _, f := range tbl.Fields {
				if !nullable[f.Name] {
					continue
				}
				checked++

				key := tc.table + "." + f.Name
				if why, ok := exempt[key]; ok {
					t.Logf("%s: exempt — %s", key, why)
					continue
				}
				if canBeNil(f.StructField.Type) {
					continue
				}

				t.Errorf("%s is NULLABLE but maps to %s, which has no nil: "+
					"a row where this was never set is indistinguishable from one set to "+
					"the zero value. Make it a pointer, make the column NOT NULL, or add "+
					"an exemption here saying why the two mean the same thing (BL33, E10).",
					key, f.StructField.Type)
			}

			if checked == 0 {
				t.Logf("%s has no nullable columns", tc.table)
			}
		})
	}
}

// canBeNil reports whether a value of t can represent absence.
func canBeNil(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface:
		return true
	default:
		return false
	}
}

// nullableColumns returns the columns information_schema reports as nullable.
func nullableColumns(t *testing.T, sqlDB *sql.DB, table string) map[string]bool {
	t.Helper()

	rows, err := sqlDB.Query(
		`SELECT column_name FROM information_schema.columns
		  WHERE table_schema = 'public' AND table_name = $1 AND is_nullable = 'YES'`,
		table,
	)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	out := map[string]bool{}
	for rows.Next() {
		var c string
		require.NoError(t, rows.Scan(&c))
		out[c] = true
	}
	require.NoError(t, rows.Err())
	return out
}
