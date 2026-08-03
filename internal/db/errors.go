package db

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// Sentinels a repository may return instead of a driver error.
//
// These live in internal/db rather than in a module's domain package because
// they describe what the *database* said, not what a business rule means. A
// module maps them onto its own vocabulary — the identity module turns a
// conflict on auth_users into "username taken", the user module turns the same
// code on users into "email already registered" — and only the module knows
// which. Returning pgconn.PgError upward instead would put the driver's error
// type in the domain and force the transport layer to know Postgres error
// codes.
var (
	// ErrNotFound is sql.ErrNoRows, normalised.
	ErrNotFound = errors.New("db: not found")
	// ErrConflict is a unique constraint violation (23505).
	ErrConflict = errors.New("db: conflict")
	// ErrInvalidRef is a foreign key violation (23503) — a reference to a row
	// that does not exist, or a delete blocked by a child row.
	ErrInvalidRef = errors.New("db: invalid reference")
	// ErrNotNull is a not-null violation (23502).
	ErrNotNull = errors.New("db: missing required value")
	// ErrCheckViolation is a CHECK constraint violation (23514).
	ErrCheckViolation = errors.New("db: check constraint violated")
	// ErrSerialization is a serialization failure or deadlock (40001, 40P01).
	// It is the one entry here that is *retryable*: the transaction failed
	// because of concurrent activity, not because the request was wrong.
	ErrSerialization = errors.New("db: serialization failure")
)

// Translate converts a driver error into one of the sentinels above, leaving
// anything it does not recognise untouched.
//
// It wraps rather than replaces, so the original Postgres error — with its
// constraint name, table and detail — is still reachable through errors.As for
// a log line. The caller gets a value it can errors.Is against; the operator
// still gets the specifics.
func Translate(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}

	// Codes are from the PostgreSQL "Appendix A. Error Codes" table. Written
	// as constants rather than inline strings so a typo is a compile error
	// somewhere rather than a silently unmatched case here.
	switch pgErr.Code {
	case pgCodeUniqueViolation:
		return joinSentinel(ErrConflict, err)
	case pgCodeForeignKeyViolation:
		return joinSentinel(ErrInvalidRef, err)
	case pgCodeNotNullViolation:
		return joinSentinel(ErrNotNull, err)
	case pgCodeCheckViolation:
		return joinSentinel(ErrCheckViolation, err)
	case pgCodeSerializationFailure, pgCodeDeadlockDetected:
		return joinSentinel(ErrSerialization, err)
	}
	return err
}

const (
	pgCodeNotNullViolation     = "23502"
	pgCodeForeignKeyViolation  = "23503"
	pgCodeUniqueViolation      = "23505"
	pgCodeCheckViolation       = "23514"
	pgCodeSerializationFailure = "40001"
	pgCodeDeadlockDetected     = "40P01"
)

// joinSentinel returns an error that satisfies errors.Is for the sentinel and
// errors.As for the underlying *pgconn.PgError.
func joinSentinel(sentinel, cause error) error {
	return errors.Join(sentinel, cause)
}

// IsRetryable reports whether err describes a failure that may succeed if the
// whole transaction is attempted again.
//
// Only serialization failures and deadlocks qualify. Retrying a unique
// violation just produces the same unique violation, and retrying a foreign
// key violation hides a bug in the caller.
func IsRetryable(err error) bool {
	return errors.Is(err, ErrSerialization)
}
