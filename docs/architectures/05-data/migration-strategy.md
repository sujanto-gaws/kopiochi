# Migration Strategy

**Status:** Partially implemented — see [ADR-010](../adr/010%20-%20Module-Owned%20Database%20Migrations.md)
**Date:** 2026-08-02
**Last verified:** 2026-08-02, after Phase 1

---

## Problem 1 (partially fixed): the migrations describe a different application

*`fbddccb` added three migrations that match the live bun models exactly:*

```
migrations/00003_create_auth_users.sql
migrations/00004_create_auth_refresh_tokens.sql
migrations/00005_create_auth_mfa_backup_codes.sql
```

*`cmd/api/login_e2e_test.go` proves the identity repositories now execute
against a migrated database, and `internal/db/schema_test.go` guards the model /
schema correspondence. `00002_create_products.sql` remains an orphan — nothing in
the codebase maps to `products`. `00001_create_users.sql` is **not** an orphan:
its columns match `internal/infrastructure/persistence/models/user.go`
(`UserDBModel`) exactly, and that model backs the live `user` module.*

*Two notes on the shipped migrations, both of which contradict rules stated later
in this document and in ADR-010: the tables are plural
(`auth_users`, not `auth_user`), and all three still use `IF NOT EXISTS`. Either
the convention or the migrations need a follow-up.*

At review time, `migrations/` contained exactly two files:

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

Neither table corresponded to the identity stack:

> **Withdrawn.** The drift table here previously listed identity's `AppUser`
> entity and four repositories — `role_repository.go`, `refresh_repository.go`,
> `user_token_repository.go`, `mfa_repository.go` — plus a set of aquaculture
> models. No type named `AppUser` appears in any commit of this repository
> (`git grep AppUser` across `git rev-list --all` returns nothing), none of the
> four filenames has ever existed under any path, and there has never been an
> aquaculture model. Those rows could not be substantiated and have been
> withdrawn. The table below is re-derived from the bun models actually present.

| Migrations define | Code actually needs |
|---|---|
| `users` (BIGSERIAL id, `name`) | `internal/infrastructure/persistence/models/user.go` — also `users`, so this one does correspond |
| `products` | *nothing* — no product model exists anywhere |
| — | `auth_users` (`modules/identity/.../models/auth_models.go`) — string id, password hash, MFA and role columns |
| — | `auth_refresh_tokens` — user id, token hash, expiry |
| — | `auth_mfa_backup_codes` |

Running every migration as it stood produced a database in which **no identity
repository could execute a query**. The `products` table is a leftover from the
boilerplate this project started from. `fbddccb` closed the identity half by
adding `00003`–`00005`.

## Problem 2 (fixed): the Makefile targets were broken

*Both halves fixed in `657b2dc`: `Makefile:14` now declares
`CONFIG?=config/default.yaml`, so `make migrate-up` works standalone, and the
`db-migrate`/`db-seed` placeholder targets were deleted.*

```make
migrate-up: ## Run all pending migrations
	$(GO) run ./cmd/migrate up --config $(CONFIG)
```

`CONFIG` had no default anywhere in the Makefile, so `make migrate-up` expanded
to `--config ` with an empty value. Compare `run-config`, which documents
`CONFIG=config/production.yaml` as a caller-supplied variable — but `migrate-*`
never got a default the way `run` did (`run` hardcodes no config flag at all and
relies on the cobra default).

Additionally:

```make
db-migrate: ## Run database migrations (placeholder)
	@echo "TODO: Implement migration runner"
```

A TODO placeholder sat 60 lines above four working `migrate-*` targets, so anyone
reading top-to-bottom found the broken one first.

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
modules/identity/migrations/     ← the shipped 00003–00005, relocated
├── embed.go
├── 0001_create_auth_users.sql
├── 0002_create_auth_refresh_tokens.sql
└── 0003_create_auth_mfa_backup_codes.sql

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
| Table names | singular, snake_case: `refresh_token`, not `auth_refresh_tokens` |
| Primary keys | `uuid` with `gen_random_uuid()` (pgcrypto), matching the string IDs in the domain |
| Timestamps | `timestamptz` always, never `timestamp` |
| Audit columns | `created_at`, `created_by`, `updated_at`, `updated_by` — the domain entities already carry these |
| Soft delete | `deleted_at timestamptz NULL`, with partial indexes `WHERE deleted_at IS NULL` |
| Foreign keys | always declared, with an explicit `ON DELETE` action |
| Indexes | every FK gets one; add composites for real query patterns |

Note the `00001_create_users` migration uses `BIGSERIAL` while the identity
domain entities use string IDs — any table backing them must use `uuid` to match.

### Indexes the code needs

Derived from the repository queries that exist today
(`modules/identity/infrastructure/persistence/repository/`):

```sql
CREATE UNIQUE INDEX idx_auth_users_email_lower  ON auth_users (lower(email));
CREATE INDEX idx_refresh_token_user_id          ON auth_refresh_tokens (user_id);
CREATE UNIQUE INDEX idx_refresh_token_hash      ON auth_refresh_tokens (token_hash);
CREATE INDEX idx_refresh_token_expires_at       ON auth_refresh_tokens (expires_at);   -- cleanup sweeps
```

The `lower(email)` unique index matters: `FindByEmail` is on the login path, and
without it `User@example.com` and `user@example.com` become two accounts.

> **Corrected 2026-08-30 (E19).** The first index above carried
> `WHERE deleted_at IS NULL` until now. Migration `00007` created it **without**
> that clause, and **`deleted_at` exists in no migration in this repository** —
> there is no soft-delete column anywhere, so the "Soft delete" convention in the
> table above describes something the schema does not implement. A partial index
> in a doc that is a full index in the database is the kind of difference nobody
> notices until a query plan does.

### Recovering from the current state

> **Rewritten 2026-08-30 (E19, E22).** The two paths this section used to offer
> were both wrong, and the wronger of the two was the one it called most likely.
> They are quoted at the end so the correction is checkable.

**There is one path, and it does not depend on what any environment has
applied.** Migrations are append-only: ADR-010 makes a mistake something you
correct with a *new* migration, never by editing or deleting an existing one,
and this repository has never done otherwise — `git log --diff-filter=MRD --
migrations/` returns nothing. `00001` and `00002` stay exactly where they are,
applied or not.

Everything after them is an ordinary forward migration:

- **`users` was reshaped**, not moved and not deleted, by
  `20260830090000_users_becomes_identity_profile.sql`. It is now the profile of
  an identity, keyed by `auth_users.id`. The `BIGSERIAL`/uuid mismatch noted
  above is exactly why it had to be: a profile whose key is unrelated to any
  identity gives a handler nothing to compare a caller against, which is how the
  same table carried an IDOR (E16).
- **`products` remains**, unused, and removing it is an ordinary migration
  whenever someone wants it gone. It is a boilerplate leftover with no bun model
  and no Go code referencing it anywhere.

**Write the migration so the answer to "has any environment applied this?" does
not change it.** The reshape back-fills by joining `lower(email)` against
`auth_users` and *raises* on any row it cannot map, naming the values. On an
empty table the back-fill is a no-op and the result is indistinguishable from
starting clean; on a populated one it refuses to discard a profile it cannot
attach to an identity. `make migrate-status` against every environment is still
worth running before you ship — as a pre-deploy check, not as a decision you are
blocked on.

<details>
<summary>What this section said before, and why both paths were wrong</summary>

> **Path A — no environment has applied them (most likely).** Delete both files
> and start the module chains at `0001`.
>
> **Path B — some environment has applied them.** Keep them as the global
> chain's history and add a migration dropping `products`, then start module
> chains fresh. `users` itself is still backed by a live bun model
> (`internal/infrastructure/persistence/models/user.go`) and moves with the
> profile-user module rather than being reshaped.

**Path A contradicted ADR-010 outright** — it told you to delete applied
migration files, which is the one thing the immutability rule forbids, and it
was labelled the likely case.

**Path B's claim that `users` "moves rather than being reshaped"** is the
opposite of what happened, and it cited
`internal/infrastructure/persistence/models/user.go` as a live model — **a file
that does not exist**; Phase 3.6b moved it. E19 raised the contradiction, and it
was ruled on rather than resolved locally: the same file already stated, two
paragraphs above, the `BIGSERIAL`/uuid constraint that forces the reshape.

</details>

---

## Makefile repair — mostly shipped

*`657b2dc` added the `CONFIG ?=` default and deleted the placeholders.
`migrate-verify` does not exist yet.*

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
targets and mislead. *Done in `657b2dc`.*

---

## CI enforcement

A job that runs against a disposable Postgres service:

1. `migrate up` from empty → must succeed.
2. `migrate down` to zero → must succeed (proves reversibility).
3. `migrate up` again → must succeed (proves idempotent chains).
4. **Schema/model drift check** — a test that creates every bun model's table via
   `db.NewCreateTable().Model(...)` into a scratch schema and diffs it against
   the migrated schema. This is the check that would have caught the missing
   `auth_users` table immediately. *Shipped in part as
   `internal/db/schema_test.go` (`d92480c`).*
5. Reject any new migration file that lacks a `-- +goose Down` section.

---

## Related documents

- [ADR-010: Module-Owned Database Migrations](../adr/010%20-%20Module-Owned%20Database%20Migrations.md)
- [Persistence and pooling](persistence-and-pooling.md)
- [Module layout](../01-modularity/module-layout.md)
- [Testing strategy](../06-quality/testing-strategy.md)
