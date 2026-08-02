# Module Layout

**Status:** Proposed — see [ADR-005](../adr/005%20-%20Module%20Boundaries%20and%20Dependency%20Direction.md)
**Date:** 2026-08-02

---

## Problem: two layouts for business code

> **Withdrawn.** An earlier revision of this document opened with a
> "three generations of layout" table and a file-by-file inventory naming
> `internal/modules/aquaculture/**`, `extensions/identity/**`, and four
> `aquaculture`/`aqualculture` files under `internal/`. None of those paths appear
> in any commit of this repository — `git log --all --diff-filter=A` returns
> nothing for each of them — and no aquaculture file has ever existed. Those
> claims could not be substantiated and have been withdrawn, together with the
> aquaculture consolidation plan and the identity move-mapping table that were
> built on them. What the tree actually contains is described below.

Business code lives in two places:

| Location | State |
|---|---|
| `modules/identity/` | Live. Wired through `cmd/api/container.go` (`BuildApp`), served under `/api/v1/auth/*`. Already matches the target shape below. |
| `internal/domain/`, `internal/application/`, `internal/infrastructure/persistence/` | Live but layer-first. Holds the profile-user stack (`internal/domain/user`, `internal/application/user`) that `cmd/api/container.go` wires as the `user` module, plus `internal/domain/ofbizuser`. |

Alongside them sits one genuinely dead copy of an identity capability:

```
internal/extension/identity/extension.go     109 LOC
internal/extension/identity/models.go        135 LOC
internal/extension/identity/repository.go    391 LOC
internal/extension/identity/service.go       441 LOC
                                           -------
                                           1,076 LOC
```

It is written against the Yii-style `Manager` framework in `internal/extension/`,
has no transport layer at all, and is imported by nothing —
`grep -rn "extension/identity" --include=*.go .` returns no hits. It is not an
older generation of the live auth stack; it is a parallel one that was never
reachable.

The remaining work is therefore narrower than a three-way consolidation: move the
profile-user stack into a module of its own, and delete
`internal/extension/identity/`.

---

## Target: one location, one shape

```
modules/<capability>/
├── module.go                  constructor + Config; the only exported surface
├── domain/
│   ├── entity.go              entities and value objects
│   ├── repository.go          repository INTERFACES (implemented in infrastructure)
│   └── errors.go              domain sentinel errors
├── application/
│   ├── <usecase>.go           one file per use case
│   ├── dto.go                 input/output types
│   └── service.go             orchestration
├── infrastructure/
│   ├── persistence/
│   │   ├── models.go          bun models — never leave this package
│   │   ├── repository.go      implements domain.Repository
│   │   └── mapping.go         model ⇄ domain conversion
│   └── <adapter>/             token service, hasher, external clients
├── transport/
│   ├── handler.go             HTTP handlers
│   ├── routes.go              func (h *Handler) Routes(r chi.Router)
│   └── dto.go                 request/response shapes + validation
└── migrations/
    ├── embed.go               //go:embed *.sql
    └── 0001_<name>.sql
```

`modules/` sits at the repository root, not under `internal/`. Modules are the
product; `internal/` is the platform they run on.

---

## Rules

1. **One capability per module.** Not `user` + `auth` + `role` as separate
   modules — those are one bounded context (consistent with
   [ADR-001](../adr/001%20-%20Adopt%20Domain%20Driven%20Design.md)).

2. **`domain/` is pure.** Standard library and `internal/platform` only. No bun,
   chi, viper, zerolog. If `domain` cannot be unit-tested without a database, the
   boundary is wrong.

3. **Repository interfaces live in `domain/`,** implementations in
   `infrastructure/persistence/`. This is already done correctly in
   `modules/identity/domain/repository.go` — preserve it in any further move.

4. **Bun models never escape `infrastructure/persistence/`.** Map to domain types
   at the boundary. A `bun:"table:..."` tag appearing in `application` or
   `transport` is a review rejection.

5. **`transport/` owns its own DTOs.** Domain entities are never serialised
   directly to JSON — that couples the wire format to the schema and leaks fields
   like password hashes.

6. **Cross-module calls go through `application`,** never by importing another
   module's `domain` or `infrastructure`. Prefer an interface declared by the
   consumer and satisfied at the composition root.

---

## Naming

| Element | Convention | Example |
|---|---|---|
| Module directory | lowercase, singular, no underscores | `modules/identity/` |
| File | snake_case describing content | `refresh_token.go` |
| Use case file | verb-noun | `login.go`, `verify_mfa.go` |
| Migration | `NNNN_verb_noun.sql` | `0001_create_refresh_token.sql` |
| Interface | consumer-side, no `I` prefix | `UserRepository` |
| Implementation | technology-qualified | `PostgresUserRepository` |

---

## Migration mapping

### identity — done

`modules/identity/` already has the shape above: `module.go`, `domain/`,
`application/` (one file per use case), `infrastructure/{persistence,token,hasher,mfa}/`,
and `transport/`. It was moved — not rewritten — out of
`internal/{domain,application,infrastructure}/auth/**` in `5f6edfe` / `6d0c1b7`.
The one part of the shape it does not yet have is `migrations/`: identity's
migrations still live in the global `migrations/` directory as
`00003`–`00005`. See [migration strategy](../05-data/migration-strategy.md).

### profile-user — outstanding

The `user` module is still assembled by `cmd/api/container.go` out of
`internal/domain/user`, `internal/application/user`,
`internal/infrastructure/persistence/repository/user.go`, and
`internal/infrastructure/http/handlers/user.go`. Those four layer-first
directories are the last business code outside `modules/`. Moving them to
`modules/user/` is a mechanical, like-for-like move of the kind identity already
had.

### `internal/extension/identity/` — delete

1,076 LOC, no importers, no transport layer. Nothing in it needs preserving; it
is a parallel implementation, not an earlier revision of anything live. See
[extension framework](extension-framework.md).

---

## Definition of done

- [ ] `modules/` contains one directory per business capability, each with the
      full shape.
- [ ] No business code remains under `internal/domain/`,
      `internal/application/`, or `internal/infrastructure/persistence/`.
- [ ] `internal/extension/` no longer exists.
- [ ] `go build ./...` and the import linter both pass.
- [ ] Each module's routes respond in the route-table smoke test.

---

## Related documents

- [ADR-005: Module Boundaries and Dependency Direction](../adr/005%20-%20Module%20Boundaries%20and%20Dependency%20Direction.md)
- [Extension framework](extension-framework.md)
- [Dependency rules](dependency-rules.md)
- [Migration strategy](../05-data/migration-strategy.md)
