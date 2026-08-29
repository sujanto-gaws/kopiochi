package db_test

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
)

// Migration-level tests for 00007_case_insensitive_identifiers.
//
// The repository integration tests cover the resulting *behaviour* (lookups
// ignore case, case-variant duplicates are refused). What they cannot cover is
// the migration itself: that its Down really inverts its Up, and that it
// refuses to run rather than half-applying when the existing data already
// contains rows that collide once case is ignored. Both are properties of the
// SQL, reachable only by driving goose directly.
//
// These skip cleanly when no Postgres is reachable — see
// testsupport.ScratchPostgres.
//
// Both tests drive goose by VERSION and inside a PRIVATE SCHEMA. Neither is
// incidental; see migrationScratch and the version constants below.

// preCaseInsensitiveVersion is the migration version immediately before 00007.
const preCaseInsensitiveVersion = 6

// caseInsensitiveVersion is 00007 itself.
//
// Both tests name it explicitly rather than saying "up" and "down", because
// goose.Down reverts exactly one migration — the NEWEST — and the newest is
// whatever landed last, currently 20260808170025_create_notifications. A test
// that says Down and means "revert 00007" tests the wrong migration the moment
// anyone adds one, which is precisely how these two tests went red.
const caseInsensitiveVersion = 7

// migrationScratch returns a database handle whose session is pinned to a
// private, empty schema, along with the migrations directory and the schema's
// name.
//
// testsupport.ScratchPostgres is NOT scratch when TEST_DATABASE_URL is set —
// the CI configuration — where it hands back the shared database as-is and its
// cleanup only closes the handle. Migration tests need an empty database to
// mean what they say: goose reads its own goose_db_version, so on a shared
// database an UpTo is a no-op against migrations someone else already applied.
//
// A private schema gives that, and the pattern is not new here:
// TestModelsMatchMigratedSchema in schema_test.go already migrates into
// schema_drift_test_<nanos> for the same reason. It also puts these tests out
// of reach of testsupport.TruncateAll, which only enumerates tables in public.
func migrationScratch(t *testing.T) (*sql.DB, string, string) {
	t.Helper()

	sqlDB, cleanup := testsupport.ScratchPostgres(t)
	t.Cleanup(cleanup)

	// search_path is a per-SESSION setting and *sql.DB is a POOL, so a bare
	// SET lands on whichever connection happened to serve it and a later query
	// can still run in public. Capping the pool at one connection is what makes
	// the pin hold for every statement goose and these tests issue.
	sqlDB.SetMaxOpenConns(1)

	schema := fmt.Sprintf("migrations_test_%d", time.Now().UnixNano())
	_, err := sqlDB.Exec(fmt.Sprintf("CREATE SCHEMA %q", schema))
	require.NoError(t, err, "create scratch schema")

	t.Cleanup(func() {
		if _, err := sqlDB.Exec(fmt.Sprintf("DROP SCHEMA %q CASCADE", schema)); err != nil {
			t.Errorf("drop scratch schema %q: %v", schema, err)
		}
	})

	_, err = sqlDB.Exec(fmt.Sprintf("SET search_path TO %q", schema))
	require.NoError(t, err, "pin search_path")

	// Prove the pin took, rather than assuming it: if this ever reads "public",
	// the test is about to migrate the shared database instead of its own.
	var current string
	require.NoError(t, sqlDB.QueryRow("SELECT current_schema()").Scan(&current))
	require.Equal(t, schema, current, "search_path did not pin to the scratch schema")

	assertPublicUntouched(t, sqlDB)

	return sqlDB, testsupport.MigrationsDir(t), schema
}

// assertPublicUntouched records public's migration version now and re-checks it
// when the test ends. These tests migrate up and down repeatedly; if the schema
// pin ever regresses, that happens to the shared database and the damage shows
// up somewhere else entirely. This turns that into a failure here.
func assertPublicUntouched(t *testing.T, sqlDB *sql.DB) {
	t.Helper()

	read := func() (int64, bool) {
		var version int64
		err := sqlDB.QueryRow(
			`SELECT max(version_id) FROM public.goose_db_version`).Scan(&version)
		if err != nil {
			// No version table in public: nothing has migrated there, which is
			// itself a state worth preserving.
			return 0, false
		}
		return version, true
	}

	before, had := read()
	t.Cleanup(func() {
		after, has := read()
		require.Equal(t, had, has, "public.goose_db_version appeared or vanished during the test")
		require.Equal(t, before, after, "the test changed public's migration version")
	})
}

func TestCaseInsensitiveIdentifiers_IsReversible(t *testing.T) {
	sqlDB, dir, schema := migrationScratch(t)

	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.UpTo(sqlDB, dir, caseInsensitiveVersion))

	hasIndex := func(name string) bool {
		var n int
		require.NoError(t, sqlDB.QueryRow(
			`SELECT count(*) FROM pg_indexes WHERE schemaname = $1 AND indexname = $2`,
			schema, name).Scan(&n))
		return n == 1
	}
	hasConstraint := func(name string) bool {
		var n int
		require.NoError(t, sqlDB.QueryRow(
			`SELECT count(*) FROM pg_constraint c
			 JOIN pg_namespace n ON n.oid = c.connamespace
			 WHERE n.nspname = $1 AND c.conname = $2`,
			schema, name).Scan(&n))
		return n == 1
	}

	require.True(t, hasIndex("idx_auth_users_email_lower"))
	require.True(t, hasIndex("idx_auth_users_username_lower"))
	require.True(t, hasIndex("idx_users_email_lower"))
	require.False(t, hasIndex("idx_auth_users_email"), "the case-sensitive index it replaces is still present")
	require.False(t, hasConstraint("users_email_key"), "the unique constraint it replaces is still present")

	require.NoError(t, goose.DownTo(sqlDB, dir, preCaseInsensitiveVersion))

	// Down must leave the schema as 00006 did — not merely drop the new
	// indexes. A Down that removed uniqueness without restoring the old
	// constraint would leave the table with no protection at all.
	require.False(t, hasIndex("idx_auth_users_email_lower"))
	require.False(t, hasIndex("idx_auth_users_username_lower"))
	require.False(t, hasIndex("idx_users_email_lower"))
	require.True(t, hasIndex("idx_auth_users_email"), "the case-sensitive index was not restored")
	require.True(t, hasIndex("idx_users_email"), "the users lookup index was not restored")
	require.True(t, hasConstraint("users_email_key"), "the users unique constraint was not restored")

	require.NoError(t, goose.UpTo(sqlDB, dir, caseInsensitiveVersion), "the up/down cycle is not repeatable")
	require.True(t, hasIndex("idx_auth_users_email_lower"))
}

// TestCaseInsensitiveIdentifiers_RefusesCollidingData: on a database that
// already holds two accounts differing only by case, there is no correct
// automatic answer — merging them is a decision about someone's data. The
// migration must therefore stop, and say which values collide, rather than
// fail with Postgres' generic "could not create unique index".
func TestCaseInsensitiveIdentifiers_RefusesCollidingData(t *testing.T) {
	sqlDB, dir, _ := migrationScratch(t)

	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.UpTo(sqlDB, dir, preCaseInsensitiveVersion))

	_, err := sqlDB.Exec(`
		INSERT INTO auth_users (username, email) VALUES
			('alice', 'alice@example.com'),
			('Alice', 'Alice@Example.com')`)
	require.NoError(t, err, "precondition: the pre-00007 schema must permit the collision")

	err = goose.UpTo(sqlDB, dir, caseInsensitiveVersion)
	require.Error(t, err, "the migration applied despite colliding rows")
	require.Contains(t, strings.ToLower(err.Error()), "collide",
		"the failure does not explain itself: %v", err)
	require.Contains(t, err.Error(), "auth_users.email",
		"the failure does not name the colliding column: %v", err)
}
