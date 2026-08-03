package db

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func pgErr(code string) error {
	return &pgconn.PgError{Code: code, Message: "boom", ConstraintName: "some_constraint"}
}

func TestTranslate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   error
		want error
	}{
		{"nil stays nil", nil, nil},
		{"no rows", sql.ErrNoRows, ErrNotFound},
		{"wrapped no rows", fmt.Errorf("scan: %w", sql.ErrNoRows), ErrNotFound},
		{"unique violation", pgErr("23505"), ErrConflict},
		{"foreign key violation", pgErr("23503"), ErrInvalidRef},
		{"not null violation", pgErr("23502"), ErrNotNull},
		{"check violation", pgErr("23514"), ErrCheckViolation},
		{"serialization failure", pgErr("40001"), ErrSerialization},
		{"deadlock", pgErr("40P01"), ErrSerialization},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := Translate(tc.in)
			if tc.want == nil {
				if got != nil {
					t.Fatalf("Translate(nil) = %v, want nil", got)
				}
				return
			}
			if !errors.Is(got, tc.want) {
				t.Fatalf("Translate(%v) = %v, want errors.Is(_, %v)", tc.in, got, tc.want)
			}
		})
	}
}

// TestTranslate_KeepsTheOriginalReachable is why Translate joins rather than
// replaces: the caller needs a value it can errors.Is against, and the
// operator still needs the constraint name to know *which* uniqueness was
// violated.
func TestTranslate_KeepsTheOriginalReachable(t *testing.T) {
	t.Parallel()

	got := Translate(pgErr("23505"))

	var pg *pgconn.PgError
	if !errors.As(got, &pg) {
		t.Fatal("the underlying *pgconn.PgError is no longer reachable via errors.As")
	}
	if pg.ConstraintName != "some_constraint" {
		t.Errorf("constraint name = %q, want it preserved", pg.ConstraintName)
	}
}

// TestTranslate_LeavesUnknownErrorsAlone: silently mapping an unrecognised
// driver error onto a sentinel would make an unrelated failure look like a
// business outcome.
func TestTranslate_LeavesUnknownErrorsAlone(t *testing.T) {
	t.Parallel()

	original := errors.New("connection refused")
	if got := Translate(original); !errors.Is(got, original) {
		t.Errorf("Translate(%v) = %v, want it returned unchanged", original, got)
	}

	unknownCode := pgErr("XX000")
	got := Translate(unknownCode)
	for _, sentinel := range []error{ErrNotFound, ErrConflict, ErrInvalidRef, ErrNotNull, ErrCheckViolation, ErrSerialization} {
		if errors.Is(got, sentinel) {
			t.Errorf("an unrecognised code was mapped onto %v", sentinel)
		}
	}
}

// TestIsRetryable: retrying a unique violation reproduces the unique
// violation, and retrying a foreign-key violation hides a caller bug. Only
// contention is worth another attempt.
func TestIsRetryable(t *testing.T) {
	t.Parallel()

	if !IsRetryable(Translate(pgErr("40001"))) {
		t.Error("a serialization failure is not reported as retryable")
	}
	if !IsRetryable(Translate(pgErr("40P01"))) {
		t.Error("a deadlock is not reported as retryable")
	}
	for _, code := range []string{"23505", "23503", "23502", "23514"} {
		if IsRetryable(Translate(pgErr(code))) {
			t.Errorf("code %s is reported as retryable; retrying it cannot help", code)
		}
	}
	if IsRetryable(nil) {
		t.Error("nil is reported as retryable")
	}
}
