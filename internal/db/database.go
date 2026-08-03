// Package db owns the database connection: DSN construction, the pgx pool,
// the bun ORM on top of it, and the plain *sql.DB the migration runner needs.
//
// See docs/architectures/05-data/persistence-and-pooling.md for the six
// defects Phase 3.8 addresses here.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	// stdlib is imported for OpenDBFromPool below and, just as importantly,
	// for its side effect: it registers the "pgx" driver name that OpenSQL
	// passes to sql.Open. Removing either use silently breaks the other, so
	// neither is safe to "clean up" without checking both.
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"github.com/sujanto-gaws/kopiochi/internal/config"
)

// DB is the process's database handle: the bun ORM the application uses, the
// pgx pool underneath it, and the *sql.DB wrapper that bridges them.
//
// All three are held together because all three must be closed, in order.
// Previously NewDB returned the bun DB and the pool and callers closed only
// the pool, leaking the sql.DB wrapper — the single-ownership problem
// docs/architectures/02-composition/lifecycle-and-shutdown.md describes.
type DB struct {
	// Bun is what repositories and modules receive.
	Bun *bun.DB
	// Pool is exposed because /readyz pings it directly (it satisfies
	// httpx.Pinger). Nothing else should reach for it.
	Pool *pgxpool.Pool

	sqlDB *sql.DB
}

// Close releases the handle's resources in reverse order of construction:
// the sql.DB wrapper first, then the pool it sits on. Closing the pool first
// would leave the wrapper holding connections that no longer exist.
//
// It is safe to call more than once; sql.DB.Close and pgxpool.Close are both
// idempotent.
func (d *DB) Close() error {
	var err error
	if d.sqlDB != nil {
		if cerr := d.sqlDB.Close(); cerr != nil {
			err = fmt.Errorf("close sql.DB: %w", cerr)
		}
	}
	if d.Pool != nil {
		d.Pool.Close()
	}
	return err
}

// Open builds the connection pool, verifies it with a ping, and returns the
// bun ORM on top of it.
//
// ctx bounds the whole operation. Callers must supply one with a deadline:
// this used to use context.Background() for both the pool construction and
// the ping, so an unreachable database blocked boot indefinitely instead of
// failing fast.
func Open(ctx context.Context, cfg config.DB) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(BuildDSN(cfg))
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.ConnMaxLifetime
	poolCfg.MaxConnIdleTime = cfg.ConnMaxIdleTime
	poolCfg.HealthCheckPeriod = cfg.HealthCheckPeriod
	// Per-connection dial deadline. Without it a network partition makes
	// every acquisition hang rather than error, including the ones on the
	// request path long after startup.
	poolCfg.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pgxpool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	sqlDB := stdlib.OpenDBFromPool(pool)

	// stdlib.OpenDBFromPool returns a *sql.DB that sits on top of the pgx
	// pool with its own, separate limits. Left at Go's defaults, MaxIdleConns
	// is 2 — so against a 10-connection pool every request past the second
	// acquires a connection and then immediately discards it, churning
	// connections continuously under any concurrency at all.
	//
	// Both limits are set to MaxConns rather than to different values: the
	// pool below is what actually bounds real connections, so the wrapper
	// only needs to avoid being the tighter constraint.
	sqlDB.SetMaxOpenConns(int(cfg.MaxConns))
	sqlDB.SetMaxIdleConns(int(cfg.MaxConns))
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	return &DB{
		Bun:   bun.NewDB(sqlDB, pgdialect.New()),
		Pool:  pool,
		sqlDB: sqlDB,
	}, nil
}

// OpenSQL opens a plain *sql.DB for the migration runner, which takes a
// database/sql handle rather than a pool.
//
// The "pgx" driver name is registered by the stdlib import above.
func OpenSQL(ctx context.Context, cfg config.DB) (*sql.DB, error) {
	sqlDB, err := sql.Open("pgx", BuildDSN(cfg))
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Migrations are serial by nature; one connection is enough, and capping
	// it keeps a migration run from opening a poolful of connections against
	// a database that may be under load.
	sqlDB.SetMaxOpenConns(1)

	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return sqlDB, nil
}

// BuildDSN assembles the PostgreSQL connection URL.
//
// It uses net/url rather than fmt.Sprintf so that credentials are
// percent-escaped. Measured against the format string this replaces, a
// password containing / ? # or % makes pgxpool.ParseConfig fail outright:
//
//	pw="pa/ss"  ->  failed to parse as URL (invalid port ":pa" after host)
//	pw="pa%ss"  ->  failed to parse as URL (invalid URL escape "%ss")
//
// The error names the host and the port, so it reads as a network or
// configuration fault and sends you looking in the wrong place entirely.
// That turns generated passwords — which secret-management.md recommends —
// into an operational blocker.
//
// (persistence-and-pooling.md illustrated this with "p@ss" being misparsed
// into a host of "ss". That particular case is wrong: pgx splits userinfo on
// the *last* @, so a bare @ has always worked. The four characters above are
// the real ones. The document has been corrected.)
//
// net.JoinHostPort also brackets IPv6 hosts, which the old "%s:%d" did not.
func BuildDSN(cfg config.DB) string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.User, cfg.Password.Reveal()),
		Host:   net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Path:   "/" + cfg.Name,
	}

	q := u.Query()
	q.Set("sslmode", cfg.SSLMode)
	// Names the connection in pg_stat_activity, so a DBA looking at a busy
	// server can tell which service the connections belong to.
	q.Set("application_name", "kopiochi")
	u.RawQuery = q.Encode()

	return u.String()
}
