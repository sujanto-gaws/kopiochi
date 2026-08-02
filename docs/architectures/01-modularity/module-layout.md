# Module Layout

**Status:** Proposed — see [ADR-005](../adr/005%20-%20Module%20Boundaries%20and%20Dependency%20Direction.md)
**Date:** 2026-08-02

---

## Problem: three layouts, none complete

| Generation | Location | Files | State |
|---|---|---|---|
| gen-1 | `internal/domain/`, `internal/application/`, `internal/infrastructure/persistence/` | 4 aquaculture files | 2 are **0 bytes**; filenames misspelled `aqualculture` |
| gen-2 | `internal/modules/aquaculture/` | 7 | **6 are empty**; only `persistence/models.go` (207 LOC) has content |
| gen-3 | `extensions/identity/` | 26 | Complete (~2,000 LOC), **unreachable** |

Concretely:

```
internal/domain/aquaculture_entity.go                       119 LOC   Farm, Pond entities
internal/application/aqualculture_service.go                104 LOC   service
internal/infrastructure/persistence/aquaculture_repository.go 0 LOC   EMPTY
internal/infrastructure/persistence/aqualculture_models.go    0 LOC   EMPTY

internal/modules/aquaculture/domain/entity.go                 0 LOC   EMPTY
internal/modules/aquaculture/domain/repository.go             0 LOC   EMPTY
internal/modules/aquaculture/domain/service.go                0 LOC   EMPTY
internal/modules/aquaculture/application/service_impl.go      0 LOC   EMPTY
internal/modules/aquaculture/infrastructure/persistence/repository_impl.go  1 LOC
internal/modules/aquaculture/infrastructure/persistence/models.go  207 LOC
```

A migration was started twice and abandoned twice, each time leaving the previous
generation in place. Nothing indicates which is authoritative.

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

1. **One capability per module.** `identity`, `aquaculture`. Not `user` +
   `auth` + `role` as separate modules — those are one bounded context
   (consistent with [ADR-001](../adr/001%20-%20Adopt%20Domain%20Driven%20Design.md)).

2. **`domain/` is pure.** Standard library and `internal/platform` only. No bun,
   chi, viper, zerolog. If `domain` cannot be unit-tested without a database, the
   boundary is wrong.

3. **Repository interfaces live in `domain/`,** implementations in
   `infrastructure/persistence/`. This is already done correctly in
   `extensions/identity/domain/repository.go` — preserve it in the move.

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
| Migration | `NNNN_verb_noun.sql` | `0001_create_app_user.sql` |
| Interface | consumer-side, no `I` prefix | `UserRepository` |
| Implementation | technology-qualified | `PostgresUserRepository` |

Existing misspellings (`aqualculture_service.go`,
`aqualculture_models.go`) are corrected during the move.

---

## Migration mapping

### identity — move, do not rewrite

| From | To |
|---|---|
| `extensions/identity/domain/entity.go` | `modules/identity/domain/entity.go` |
| `extensions/identity/domain/repository.go` | `modules/identity/domain/repository.go` |
| `extensions/identity/application/*.go` | `modules/identity/application/` |
| `extensions/identity/infrastructure/persistence/*.go` | `modules/identity/infrastructure/persistence/` |
| `extensions/identity/infrastructure/token/jwt.go` | `modules/identity/infrastructure/token/jwt.go` |
| `extensions/identity/infrastructure/hasher/bcrypt.go` | `modules/identity/infrastructure/hasher/bcrypt.go` |
| `extensions/identity/infrastructure/http/*.go` | `modules/identity/transport/` |
| `extensions/identity/extension.go` | `modules/identity/module.go` *(rewritten against the new contract)* |

The 11 imports of `internal/utils` are re-pointed at `internal/platform` — see
[dependency rules](dependency-rules.md).

### aquaculture — consolidate three fragments into one

| From | Action |
|---|---|
| `internal/domain/aquaculture_entity.go` | → `modules/aquaculture/domain/entity.go` (**the only real domain code**) |
| `internal/application/aqualculture_service.go` | → `modules/aquaculture/application/service.go` |
| `internal/modules/aquaculture/infrastructure/persistence/models.go` | → `modules/aquaculture/infrastructure/persistence/models.go` (**the only real model code**) |
| `internal/infrastructure/persistence/aquaculture_repository.go` (0 B) | delete |
| `internal/infrastructure/persistence/aqualculture_models.go` (0 B) | delete |
| `internal/modules/aquaculture/**` (6 empty files) | delete |

The surviving content is one entity file, one service file, and one model file.
The repository implementation must be **written** — it has never existed.

---

## Definition of done

- [ ] `modules/` contains exactly two directories, each with the full shape.
- [ ] `internal/domain/`, `internal/application/`, `internal/modules/`,
      `extensions/` no longer exist.
- [ ] No file in the repository is 0 bytes.
- [ ] `go build ./...` and the import linter both pass.
- [ ] Each module's routes respond in the route-table smoke test.

---

## Related documents

- [ADR-005: Module Boundaries and Dependency Direction](../adr/005%20-%20Module%20Boundaries%20and%20Dependency%20Direction.md)
- [Extension framework](extension-framework.md)
- [Dependency rules](dependency-rules.md)
- [Migration strategy](../05-data/migration-strategy.md)
