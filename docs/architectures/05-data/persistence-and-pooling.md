# Persistence & Connection Pooling

**Status:** Proposed
**Date:** 2026-08-02
**Last verified:** 2026-08-02 — Problems 1–6 are all still live (Phase 3.8 has
not run). The one item resolved since this was written is the health endpoint;
see the note at the end.

---

## What is already correct

Worth stating, because these should not be "improved" into something worse:

- **No string-built SQL anywhere.** A sweep for `fmt.Sprintf` near SQL keywords
  across `internal/` and `modules/` returns nothing. All queries use the bun
  query builder, satisfying the `CLAUDE.md` rule on parameterised queries.
- **Repository interfaces live in the domain layer**
  (`modules/identity/domain/repository.go`) with implementations in
  `modules/identity/infrastructure/persistence/repository/`. The dependency
  inversion is right.
- **pgxpool + bun over `stdlib`** is a sound combination.
- **Password hashing uses bcrypt** with `DefaultCost`
  (`modules/identity/infrastructure/hasher/bcrypt.go`), and verification uses
  `bcrypt.CompareHashAndPassword` — constant-time and correct.

> *An earlier revision cited these last two under `extensions/identity/...`. No
> `extensions/` directory appears in any commit of this repository; the paths have
> been repointed at the live equivalents rather than withdrawn, since the code
> itself is real.*

---

## Problem 1: the DSN does not escape credentials

`internal/db/database.go:65-67`:

```go
func BuildDSN(host string, port int, user, pass, name, ssl string) string {
    return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s", user, pass, host, port, name, ssl)
}
```

Any password containing `/`, `?`, `#`, or `%` produces a URL that
`pgxpool.ParseConfig` rejects outright, with an error naming the *host*:

```
pw="pa/ss"  ->  failed to parse as URL (invalid port ":pa" after host)
pw="pa?ss"  ->  failed to parse as URL (invalid port ":pa" after host)
pw="pa#ss"  ->  failed to parse as URL (invalid port ":pa" after host)
pw="pa%ss"  ->  failed to parse as URL (invalid URL escape "%ss")
```

Because the error points at the host and port, it reads as a network or
configuration fault and sends you looking in the wrong place entirely.

> **Correction.** This paragraph previously claimed the character set was
> `@ : / ? # %`, and that a password of `p@ss` "makes `pgxpool.ParseConfig` read
> the host as `ss`". Measured against the real implementation, that is wrong on
> both counts: `p@ss` and `pa:ss` both parse correctly, because pgx splits
> userinfo on the *last* `@` and a `:` inside userinfo is unambiguous. The four
> characters listed above are the ones that actually break, and they break by
> failing to parse rather than by misparsing. The defect is real and the fix is
> unchanged; only the example was wrong. See `internal/db/dsn_test.go`, which
> covers all six characters.

This becomes an operational blocker the moment a generated password is used,
which is precisely what
[secret management](../03-configuration/secret-management.md) recommends.

**Fix:**

```go
func BuildDSN(cfg config.DB) string {
    u := &url.URL{
        Scheme: "postgres",
        User:   url.UserPassword(cfg.User, cfg.Password.Reveal()),
        Host:   net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
        Path:   "/" + cfg.Name,
    }
    q := u.Query()
    q.Set("sslmode", cfg.SSLMode)
    q.Set("application_name", "kopiochi")   // shows up in pg_stat_activity
    u.RawQuery = q.Encode()
    return u.String()
}
```

`net.JoinHostPort` also handles IPv6 hosts, which the current `%s:%d` format
breaks.

## Problem 2: the `sql.DB` wrapper is unconfigured

`database.go:42-46`:

```go
sqldb := stdlib.OpenDBFromPool(pool)
db := bun.NewDB(sqldb, pgdialect.New())
return db, pool, nil
```

`stdlib.OpenDBFromPool` returns a `*sql.DB` that sits **on top of** the pgxpool.
It has its own limits, and the Go defaults are:

| Setting | Default | Effect here |
|---|---|---|
| `MaxOpenConns` | unlimited | `sql.DB` may open more than the pool intends |
| `MaxIdleConns` | **2** | With `db.max_conns: 10`, connections beyond the second are closed and reopened constantly |
| `ConnMaxLifetime` | unlimited | Ignores the pool's own `MaxConnLifetime` |

The `MaxIdleConns: 2` default over a 10-connection pool causes continuous
connection churn under concurrency — each request beyond the second acquires and
then discards a connection.

**Fix:**

```go
sqldb := stdlib.OpenDBFromPool(pool)
sqldb.SetMaxOpenConns(int(cfg.MaxConns))
sqldb.SetMaxIdleConns(int(cfg.MaxConns))     // match: the pool already bounds real connections
sqldb.SetConnMaxLifetime(cfg.ConnMaxLifetime)
sqldb.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
```

## Problem 3: the `sql.DB` is never closed

`NewDB` returns `(*bun.DB, *pgxpool.Pool, error)`. Callers close the pool but
never the `sql.DB` wrapper. Return a closer that shuts down both, in order, and
register it on the lifecycle stack described in
[lifecycle and shutdown](../02-composition/lifecycle-and-shutdown.md).

## Problem 4: pool sizing is hardcoded

```go
poolCfg.MaxConnLifetime = time.Hour           // line 30
poolCfg.MaxConnIdleTime = 30 * time.Minute    // line 31
```

Not configurable, while `MaxConns`/`MinConns` are. Move both to config with the
current values as defaults.

Also missing: `poolCfg.HealthCheckPeriod` and a `ConnConfig.ConnectTimeout`. A
default connect timeout matters — without one, a network partition makes startup
hang instead of failing fast.

## Problem 5: startup contexts have no deadline

```go
pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)   // line 33
if err := pool.Ping(context.Background()); err != nil {             // line 38
```

`context.Background()` means an unreachable database blocks boot indefinitely.
Take a `ctx` parameter and have the caller apply a startup timeout.

## Problem 6: `OpenDB` uses an unregistered driver

`database.go:50-62`:

```go
db, err := sql.Open("pgx", dsn)
```

This requires `github.com/jackc/pgx/v5/stdlib` to be imported for its driver
registration side effect. It *is* imported in this file for
`OpenDBFromPool`, so it works today — but the dependency is implicit and would
break silently if that call were refactored away. Make it explicit with a blank
import comment, or build the migration `*sql.DB` from a pool as well.

---

## Repository patterns

### Transactions

There is currently no transaction helper. Multi-step use cases — create user then
assign role, verify MFA then rotate refresh token — need atomicity:

```go
// internal/db/tx.go
func InTx(ctx context.Context, db bun.IDB, fn func(ctx context.Context, tx bun.Tx) error) error {
    tx, err := db.BeginTx(ctx, &sql.TxOptions{})
    if err != nil {
        return fmt.Errorf("begin tx: %w", err)
    }
    defer func() { _ = tx.Rollback() }()   // no-op after a successful commit

    if err := fn(ctx, tx); err != nil {
        return err
    }
    return tx.Commit()
}
```

Repositories accept `bun.IDB`, so the same implementation works inside and
outside a transaction. Application services own transaction boundaries;
repositories never start their own.

### Model / domain mapping

Bun models stay inside `infrastructure/persistence`. Repositories return domain
types. This keeps `bun:"..."` tags out of the domain and lets the schema evolve
independently of the business model — consistent with
[module layout](../01-modularity/module-layout.md).

### Error translation

Postgres errors must not leak upward as driver types:

```go
func translate(err error) error {
    if errors.Is(err, sql.ErrNoRows) {
        return domain.ErrNotFound
    }
    var pgErr *pgconn.PgError
    if errors.As(err, &pgErr) {
        switch pgErr.Code {
        case "23505":  return domain.ErrConflict      // unique_violation
        case "23503":  return domain.ErrInvalidRef    // foreign_key_violation
        case "40001":  return domain.ErrSerialization // retryable
        }
    }
    return err
}
```

The transport layer maps domain errors to status codes, so the HTTP layer never
inspects driver types.

> *An earlier revision claimed
> `extensions/identity/infrastructure/persistence/errors.go` "already exists" and
> only needed extending. No file by that path — or any `persistence/errors.go` —
> appears in any commit of this repository. The claim has been withdrawn: this
> translation layer must be **written**. The nearest existing thing is
> `modules/identity/application/errors.go`, which declares application-level
> sentinels but does no driver-error translation.*

### Context propagation

Every repository method takes `context.Context` as its first parameter and passes
it to bun. This is already followed; keep it, and never use
`context.Background()` inside a repository.

### Query performance

- Add `bundebug.NewQueryHook` in development to surface N+1 patterns.
- Log queries slower than a configurable threshold at `warn`.
- The identity model has an obvious N+1 risk: loading users then roles per user.
  Use bun `Relation()` eager loading, and add the composite index that supports
  it (see [migration strategy](migration-strategy.md)).

---

## Health checks

Split liveness from readiness, per
[routing and versioning](../02-composition/routing-and-versioning.md):

```go
func Ready(pool *pgxpool.Pool) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
        defer cancel()

        if err := pool.Ping(ctx); err != nil {
            problem.Write(w, http.StatusServiceUnavailable, "database_unavailable",
                "Service Unavailable", "Database is not reachable.")
            return
        }
        stat := pool.Stat()
        httpx.JSON(w, http.StatusOK, map[string]any{
            "status":              "ready",
            "db_conns_acquired":   stat.AcquiredConns(),
            "db_conns_idle":       stat.IdleConns(),
            "db_conns_total":      stat.TotalConns(),
        })
    }
}
```

The old `/api/health` (`routes.go:17`) checked nothing, which is why an
application with no reachable routes and an unused database still reported
healthy. *Split in `40887de`: `internal/httpx/health.go` now serves `/healthz`
(liveness only) and `/readyz`, which does a real `Ping` with a 2s timeout and
fails closed when the pinger is nil. The shipped `/readyz` returns only
`{"status","version"}` — the pool statistics proposed above are not reported
yet, and remain worth adding.*

---

## Related documents

- [Migration strategy](migration-strategy.md)
- [Lifecycle and shutdown](../02-composition/lifecycle-and-shutdown.md)
- [Configuration model](../03-configuration/configuration-model.md)
- [Module layout](../01-modularity/module-layout.md)
