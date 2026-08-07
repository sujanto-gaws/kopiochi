package db_test

import (
	"strings"
	"testing"

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

// preCaseInsensitiveVersion is the migration version immediately before 00007.
const preCaseInsensitiveVersion = 6

func TestCaseInsensitiveIdentifiers_IsReversible(t *testing.T) {
	sqlDB, cleanup := testsupport.ScratchPostgres(t)
	t.Cleanup(cleanup)
	dir := testsupport.MigrationsDir(t)

	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(sqlDB, dir))

	hasIndex := func(name string) bool {
		var n int
		require.NoError(t, sqlDB.QueryRow(
			`SELECT count(*) FROM pg_indexes WHERE schemaname = 'public' AND indexname = $1`,
			name).Scan(&n))
		return n == 1
	}
	hasConstraint := func(name string) bool {
		var n int
		require.NoError(t, sqlDB.QueryRow(
			`SELECT count(*) FROM pg_constraint WHERE conname = $1`, name).Scan(&n))
		return n == 1
	}

	require.True(t, hasIndex("idx_auth_users_email_lower"))
	require.True(t, hasIndex("idx_auth_users_username_lower"))
	require.True(t, hasIndex("idx_users_email_lower"))
	require.False(t, hasIndex("idx_auth_users_email"), "the case-sensitive index it replaces is still present")
	require.False(t, hasConstraint("users_email_key"), "the unique constraint it replaces is still present")

	require.NoError(t, goose.Down(sqlDB, dir))

	// Down must leave the schema as 00006 did — not merely drop the new
	// indexes. A Down that removed uniqueness without restoring the old
	// constraint would leave the table with no protection at all.
	require.False(t, hasIndex("idx_auth_users_email_lower"))
	require.False(t, hasIndex("idx_auth_users_username_lower"))
	require.False(t, hasIndex("idx_users_email_lower"))
	require.True(t, hasIndex("idx_auth_users_email"), "the case-sensitive index was not restored")
	require.True(t, hasIndex("idx_users_email"), "the users lookup index was not restored")
	require.True(t, hasConstraint("users_email_key"), "the users unique constraint was not restored")

	require.NoError(t, goose.Up(sqlDB, dir), "the up/down cycle is not repeatable")
	require.True(t, hasIndex("idx_auth_users_email_lower"))
}

// TestCaseInsensitiveIdentifiers_RefusesCollidingData: on a database that
// already holds two accounts differing only by case, there is no correct
// automatic answer — merging them is a decision about someone's data. The
// migration must therefore stop, and say which values collide, rather than
// fail with Postgres' generic "could not create unique index".
func TestCaseInsensitiveIdentifiers_RefusesCollidingData(t *testing.T) {
	sqlDB, cleanup := testsupport.ScratchPostgres(t)
	t.Cleanup(cleanup)
	dir := testsupport.MigrationsDir(t)

	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.UpTo(sqlDB, dir, preCaseInsensitiveVersion))

	_, err := sqlDB.Exec(`
		INSERT INTO auth_users (username, email) VALUES
			('alice', 'alice@example.com'),
			('Alice', 'Alice@Example.com')`)
	require.NoError(t, err, "precondition: the pre-00007 schema must permit the collision")

	err = goose.Up(sqlDB, dir)
	require.Error(t, err, "the migration applied despite colliding rows")
	require.Contains(t, strings.ToLower(err.Error()), "collide",
		"the failure does not explain itself: %v", err)
	require.Contains(t, err.Error(), "auth_users.email",
		"the failure does not name the colliding column: %v", err)
}
