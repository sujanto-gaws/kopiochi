package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/uptrace/bun"
)

// InTx runs fn inside a transaction, committing if it returns nil and rolling
// back otherwise.
//
// Repositories take bun.IDB, which both *bun.DB and bun.Tx satisfy, so the same
// repository method works inside and outside a transaction with no variant.
// Application services own transaction boundaries; repositories never start
// their own — a repository that opened its own transaction could not be
// composed into a larger atomic operation, which is the only reason to have
// this function.
//
// The rollback is deferred rather than placed on the error path so that a panic
// inside fn does not leave the transaction open until the connection is
// reaped. After a successful Commit the rollback is a no-op, which is why it
// can be unconditional.
func InTx(ctx context.Context, idb bun.IDB, fn func(ctx context.Context, tx bun.Tx) error) error {
	tx, err := idb.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		// Rollback after Commit returns sql.ErrTxDone; that is the expected
		// path, not a problem worth reporting.
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			// Nothing useful to do: fn's error (or the commit's) is the one
			// the caller needs, and shadowing it with a rollback failure would
			// hide the actual cause.
			_ = rbErr
		}
	}()

	if err := fn(ctx, tx); err != nil {
		// Deliberately not wrapped: callers use errors.Is against domain
		// sentinels, and an extra layer here would still satisfy that, but the
		// message reads better without "in tx: " prefixed to every failure.
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
