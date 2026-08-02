# ADR-010: Module-Owned Database Migrations

## Status
**Proposed** – *Date: 2026-08-02*

## Context

`CLAUDE.md` requires that migrations be reversible (Up/Down) and run through a
command rather than by hand. The mechanism exists — goose, driven by
`cmd/migrate` — but its content has drifted completely away from the code.

`migrations/` contains two files, `00001_create_users.sql` and
`00002_create_products.sql`. Neither matches anything in the codebase:

| Migrations define | Code actually requires |
|---|---|
| `users` (BIGSERIAL id, `name`) | identity's `AppUser` — string/UUID id, `first_name`, `last_name`, password hash, MFA fields |
| `products` | *nothing* — no product model exists anywhere |
| — | roles and user-role assignment (`role_repository.go`) |
| — | refresh tokens (`refresh_repository.go`) |
| — | user tokens (`user_token_repository.go`) |
| — | MFA secrets (`mfa_repository.go`) |
| — | aquaculture: farms, ponds, pond groups, pond types (207 LOC of bun models) |

Applying every migration produces a database against which **not one repository
can execute a query**. `products` is a leftover from the boilerplate this project
was derived from.

Contributing factors:

- **No ownership.** All migrations sit in one global directory while code is
  organised by module; nothing says which module owns `users`, and two modules
  changing schema in one release collide on sequence numbers.
- **`IF NOT EXISTS` everywhere** means a migration run against a database where
  the table already exists — with a different shape — is recorded as applied
  while changing nothing. Drift becomes invisible.
- **Broken tooling.** `make migrate-up` expands to `--config ` because `CONFIG`
  has no default, and a `db-migrate` target printing "TODO: Implement migration
  runner" sits above four working `migrate-*` targets.
- **No CI verification** that migrations apply, reverse, or match the models.

## Decision

1. **Each module owns its migrations,** embedded in the module:
   `modules/<name>/migrations/*.sql` with a `//go:embed` FS exposed through the
   module contract.
2. **The top-level `migrations/` holds global objects only** — extensions
   (`pgcrypto`, `citext`), shared enums, audit tables.
3. **Each module gets its own goose version table**
   (`goose_db_version_<module>`), so sequence numbers never collide across
   modules.
4. **Ordering is global first, then modules in registration order.**
5. **Migrations never run implicitly at startup.** `cmd/migrate` is a separate
   binary run as an explicit deployment step.
6. **Every migration is reversible.** Where a `Down` cannot restore data, it
   raises an exception rather than silently pretending to succeed.
7. **No `IF NOT EXISTS` in new migrations.** A collision must fail loudly.
8. **Schema conventions are fixed:** singular snake_case table names, `uuid`
   primary keys via `gen_random_uuid()`, `timestamptz` always, audit columns,
   soft delete via `deleted_at`, explicit foreign keys with `ON DELETE` actions,
   and an index on every foreign key.
9. **CI verifies** up → down → up, and that the migrated schema matches the bun
   models.

## Consequences

### Positive
- **Schema travels with the code that uses it.** Moving or deleting a module
  moves or deletes its schema.
- **No cross-module sequence collisions,** so parallel work does not conflict in
  the migration directory.
- **Drift is caught by CI** rather than discovered at runtime — the check that
  would have flagged `users` vs `AppUser` on day one.
- **Reversibility is proven,** not assumed.
- **Serving replicas hold no DDL privileges,** since migration is a separate
  step with its own credentials.
- **Rolling deployments are safe:** no replica races another to migrate on boot.

### Negative
- **Multiple version tables** make "what is applied?" a multi-table question.
  Mitigated by a `migrate status` command that reports all of them.
- **Cross-module foreign keys need care.** A module referencing another module's
  table depends on ordering. Prefer avoiding cross-module FKs; where unavoidable,
  document the dependency and rely on registration order.
- **An explicit deployment step** must exist in every pipeline and runbook.
- **Existing migrations must be reconciled** before new ones are written.

## Alternatives Considered

| Alternative | Reason for rejection |
|---|---|
| **Keep one global migrations directory** | Current state; no ownership, sequence collisions, and schema that drifted from the code without anyone noticing. |
| **Auto-migrate at application startup** | Replicas race; a failed migration crashes a rolling deploy; the serving user needs DDL rights. |
| **Bun's `migrate` package instead of goose** | `CLAUDE.md` mentions Bun migrations, but goose is already in `go.mod` and `cmd/migrate` is written against it. Switching adds churn for no functional gain; the ownership model here is tool-agnostic. |
| **Schema-per-module in Postgres** | Stronger isolation, but complicates cross-module queries, connection search paths, and backups. Reconsider only if modules become separate services. |
| **Declarative schema diffing (Atlas, migra)** | Powerful, but generated diffs need careful review and the team has no experience with it. Revisit later. |

## Implementation Plan

1. Determine whether any environment has applied the existing `users`/`products`
   migrations (`make migrate-status` everywhere).
   - **None applied:** delete both files; module chains start at `0001`.
   - **Some applied:** keep them as global history, add a migration dropping
     `products` and reshaping `users`, then start module chains fresh.
2. Add `migrations/` packages with `//go:embed` to each module; expose via
   `Module.Migrations`.
3. Write identity's real migrations: `app_user`, `role`, `user_role`,
   `refresh_token`, `user_token`, `mfa_secret` — matching the bun models exactly.
4. Write aquaculture's migrations: `farm`, `pond`, `pond_group`, `pond_type`.
5. Add the global chain: `0001_enable_extensions.sql` (pgcrypto).
6. Update `cmd/migrate` to iterate modules with per-module version tables.
7. Fix the Makefile: `CONFIG ?= config/default.yaml`; delete the `db-migrate` and
   `db-seed` placeholders; add `migrate-verify`.
8. Add the CI job: up → down → up, plus the model/schema drift test.

## Compliance / Enforcement

- CI fails if a migration lacks a `-- +goose Down` section.
- CI fails if `down` then `up` does not reproduce the schema.
- The model/schema drift test fails when a bun model has no corresponding column.
- Review rejects `IF NOT EXISTS` in new migrations and any migration file placed
  outside its owning module.
- Applied migrations are immutable: a mistake is corrected by a new migration,
  never by editing an existing one.

## Related ADRs
- [ADR-005: Module Boundaries and Dependency Direction](005%20-%20Module%20Boundaries%20and%20Dependency%20Direction.md)
- [ADR-004: Consolidate on a Single Extension Framework](004%20-%20Consolidate%20on%20a%20Single%20Extension%20Framework.md)
- [ADR-001: Adopt Domain-Driven Design](001%20-%20Adopt%20Domain%20Driven%20Design.md)

## Related Documents
- [Migration strategy](../05-data/migration-strategy.md)
- [Persistence and pooling](../05-data/persistence-and-pooling.md)

---

**This ADR serves as a binding architectural decision for the project.**
