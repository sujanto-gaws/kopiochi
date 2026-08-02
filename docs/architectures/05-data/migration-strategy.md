# Migration Strategy

**Status:** Proposed — see [ADR-010](../adr/010%20-%20Module-Owned%20Database%20Migrations.md)
**Date:** 2026-08-02

---

## Problem 1: the migrations describe a different application

`migrations/` contains exactly two files:

```sql
-- 00001_create_users.sql
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 00002_create_products.sql
CREATE TABLE IF NOT EXISTS products (...);
```

Neither table corresponds to anything in the current codebase:

| Migrations define | Code actually needs |
|---|---|
| `users` (BIGSERIAL id, `name`) | identity's `AppUser` (string/UUID id, `first_name`, `last_name`, `email`, password hash, MFA fields) |
| `products` | *nothing* — no product model exists anywhere |
| — | roles / user-role assignment (`role_repository.go`) |
| — | refresh tokens (`refresh_repository.go`) |
| — | user tokens (`user_token_repository.go`) |
| — | MFA secrets (`mfa_repository.go`) |
| — | aquaculture: farms, ponds, pond groups, pond types (`models.go`, 207 LOC) |

Running every migration produces a database in which **not one repository can
execute a query**. The `products` table is a leftover from the boilerplate this
project started from.

## Problem 2: the Makefile targets are broken

```make
migrate-up: ## Run all pending migrations
	$(GO) run ./cmd/migrate up --config $(CONFIG)
```

`CONFIG` has no default anywhere in the Makefile. `make migrate-up` expands to
`--config ` with an empty value. Compare `run-config`, which documents
`CONFIG=config/production.yaml` as a caller-supplied variable — but `migrate-*`
never got a default the way `run` did (`run` hardcodes no config flag at all and
relies on the cobra default).

Additionally:

```make
db-migrate: ## Run database migrations (placeholder)
	@echo "TODO: Implement migration runner"
```

A TODO placeholder sits 60 lines above four working `migrate-*` targets. Anyone
reading top-to-bottom finds the broken one first.

## Problem 3: no ownership model

All migrations live in one global directory while the code is organised by
module. Nothing indicates which module owns `users`. When two modules need
schema changes in the same release, they collide on sequence numbers.

## Problem 4: `IF NOT EXISTS` masks drift

Every statement uses `CREATE TABLE IF NOT EXISTS`. A migration that "succeeded"
against a database where the table already exists — with a *different* shape —
is recorded as applied while changing nothing. Drift becomes invisible.

## Problem 5: no CI enforcement

Nothing verifies that migrations apply cleanly, that `Down` reverses `Up`, or
that the schema matches the bun models. The drift above could have been caught by
a single CI job.

---

## Target design

### Module-owned migrations, embedded

```
modules/identity/migrations/
├── embed.go
├── 0001_create_app_user.sql
├── 0002_create_role.sql
├── 0003_create_refresh_token.sql
└── 0004_create_mfa_secret.sql

modules/aquaculture/migrations/
├── embed.go
├── 0001_create_farm.sql
└── 0002_create_pond.sql

migrations/                      ← global only
├── 0001_enable_extensions.sql   (pgcrypto, citext)
└── 0002_create_audit_log.sql
```

```go
// modules/identity/migrations/embed.go
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

The module exposes it through the module contract:

```go
return &module.Module{
    Name:       "identity",
    Migrations: migrations.FS,
    Routes:     handler.Routes,
}, nil
```

### Namespaced version tables

Each module gets its own goose version table, so sequence numbers never collide:

```go
func Migrate(ctx context.Context, db *sql.DB, mods []*module.Module) error {
    // Global first — extensions and shared types other modules depend on.
    if err := runSet(ctx, db, globalFS, "goose_db_version"); err != nil {
        return fmt.Errorf("global migrations: %w", err)
    }
    for _, m := range mods {
        if m.Migrations == nil {
            continue
        }
        table := "goose_db_version_" + m.Name
        if err := runSet(ctx, db, m.Migrations, table); err != nil {
            return fmt.Errorf("module %s migrations: %w", m.Name, err)
        }
    }
    return nil
}
```

goose supports this via `goose.SetTableName`. Ordering is: global, then modules
in registration order.

### Migrations are never run implicitly at startup

The server does **not** migrate on boot. Reasons: concurrent replicas would race;
a failed migration should not crash a rolling deployment; and the database user
serving traffic should not hold DDL privileges.

`cmd/migrate` stays a separate binary, run as an explicit deployment step.

### Reversibility

Every migration has a real `Down`, per the `CLAUDE.md` rule. Where a `Down` would
lose data irrecoverably, it fails loudly rather than pretending:

```sql
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'irreversible: 0007 dropped the legacy password column';
END $$;
-- +goose StatementEnd
```

### Drop `IF NOT EXISTS`

New migrations use plain `CREATE TABLE`. A collision then fails loudly, which is
the desired behaviour — it means the database is not in the state the migration
chain claims.

### Schema conventions

| Convention | Rule |
|---|---|
| Table names | singular, snake_case: `app_user`, `refresh_token` |
| Primary keys | `uuid` with `gen_random_uuid()` (pgcrypto), matching the string IDs in the domain |
| Timestamps | `timestamptz` always, never `timestamp` |
| Audit columns | `created_at`, `created_by`, `updated_at`, `updated_by` — the domain entities already carry these |
| Soft delete | `deleted_at timestamptz NULL`, with partial indexes `WHERE deleted_at IS NULL` |
| Foreign keys | always declared, with an explicit `ON DELETE` action |
| Indexes | every FK gets one; add composites for real query patterns |

Note the current `users` migration uses `BIGSERIAL` while the domain entities use
string IDs — the new `app_user` table must use `uuid` to match.

### Indexes the code needs

Derived from the repository queries that exist today:

```sql
CREATE UNIQUE INDEX idx_app_user_email_lower ON app_user (lower(email)) WHERE deleted_at IS NULL;
CREATE INDEX idx_refresh_token_user_id      ON refresh_token (user_id);
CREATE UNIQUE INDEX idx_refresh_token_hash  ON refresh_token (token_hash);
CREATE INDEX idx_refresh_token_expires_at   ON refresh_token (expires_at);   -- cleanup sweeps
CREATE INDEX idx_user_role_user_id          ON user_role (user_id);
CREATE INDEX idx_pond_farm_id               ON pond (farm_id);
```

The `lower(email)` unique index matters: the existing lookup is by email, and
without it, `User@example.com` and `user@example.com` become two accounts.

### Recovering from the current state

The existing `users`/`products` migrations cannot be edited in place if any
environment has applied them. Two paths:

**Path A — no environment has applied them (most likely).** Delete both files and
start the module chains at `0001`. Simplest; verify with
`make migrate-status` against every environment first.

**Path B — some environment has applied them.** Keep them as the global chain's
history, add `0003_drop_products.sql` and a migration that reshapes `users` into
`app_user`, then start module chains fresh.

Confirm which applies before writing any new migration.

---

## Makefile repair

```make
CONFIG ?= config/default.yaml      # default so `make migrate-up` works standalone

migrate-up:      ## Apply all pending migrations
	$(GO) run ./cmd/migrate up --config $(CONFIG)

migrate-status:  ## Show migration status
	$(GO) run ./cmd/migrate status --config $(CONFIG)

migrate-verify:  ## Apply, roll back, and re-apply against a scratch database (CI)
	$(GO) run ./cmd/migrate verify --config $(CONFIG)
```

Delete the `db-migrate` and `db-seed` TODO placeholders — they duplicate working
targets and mislead.

---

## CI enforcement

A job that runs against a disposable Postgres service:

1. `migrate up` from empty → must succeed.
2. `migrate down` to zero → must succeed (proves reversibility).
3. `migrate up` again → must succeed (proves idempotent chains).
4. **Schema/model drift check** — a test that creates every bun model's table via
   `db.NewCreateTable().Model(...)` into a scratch schema and diffs it against
   the migrated schema. This is the check that would have caught `users` vs
   `AppUser` immediately.
5. Reject any new migration file that lacks a `-- +goose Down` section.

---

## Related documents

- [ADR-010: Module-Owned Database Migrations](../adr/010%20-%20Module-Owned%20Database%20Migrations.md)
- [Persistence and pooling](persistence-and-pooling.md)
- [Module layout](../01-modularity/module-layout.md)
- [Testing strategy](../06-quality/testing-strategy.md)
