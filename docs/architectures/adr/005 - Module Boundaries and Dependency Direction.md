# ADR-005: Module Boundaries and Dependency Direction

## Status
**Proposed** – *Date: 2026-08-02*

## Context

[ADR-001](001%20-%20Adopt%20Domain%20Driven%20Design.md) commits the project to DDD
with strict layer separation and dependency inversion, and states that code
review must verify the domain layer has no dependency on infrastructure. In
practice, neither the layering nor the location of modules has held.

**Three directory layouts coexist**, none complete:

| Generation | Location | State |
|---|---|---|
| gen-1 | `internal/domain/`, `internal/application/`, `internal/infrastructure/persistence/` | 4 aquaculture files; 2 are **0 bytes** |
| gen-2 | `internal/modules/aquaculture/` | 7 files; **6 are empty** |
| gen-3 | `extensions/identity/` | 26 files, complete, but unreachable from `cmd/` |

A migration was evidently started twice and abandoned twice, each time leaving
the previous generation in place. Nothing indicates which is authoritative.

**The dependency direction is inverted.** Eleven files under
`extensions/identity/` import `internal/utils`, including
`extensions/identity/domain/repository.go` and
`extensions/identity/domain/service.go` — the layer ADR-001 requires to be pure.
An `extensions/` tree that imports the host's `internal/` packages can never be
extracted or versioned independently, which defeats the purpose of separating it.

**`internal/utils` is a bucket, not a boundary.** It mixes HTTP response helpers,
pagination, string manipulation, hashing, and ID generation in one package —
which is precisely how the domain layer came to depend on HTTP helpers.

**Nothing enforces any of this.** There is no import linter, no architecture
test, no `depguard` configuration. The drift happened because nothing objected.

## Decision

1. **One location for business code: `modules/<capability>/`** at the repository
   root. `internal/` holds only platform code; `modules/` holds the product.
2. **One layout inside every module:** `domain/`, `application/`,
   `infrastructure/`, `transport/`, `migrations/`, plus `module.go`.
3. **Layer dependency rules** (enforced, not advisory):

   | Package | May import | Must NOT import |
   |---|---|---|
   | `domain` | stdlib, `internal/platform` | bun, chi, viper, zerolog, sibling layers |
   | `application` | `domain`, stdlib, `internal/platform` | bun, chi, `infrastructure`, `transport` |
   | `infrastructure` | `domain`, bun, pgx | `application`, `transport` |
   | `transport` | `application`, chi | `infrastructure`, domain models on the wire |

4. **Modules do not import each other.** Cross-module needs are expressed as an
   interface declared by the consumer and satisfied at the composition root.
5. **Platform never imports modules.** `internal/platform`, `internal/httpx`,
   `internal/db`, and `internal/config` must not reference `modules/**`. Only
   `cmd/**` sees both.
6. **`internal/utils` is dissolved** into named packages: `internal/httpx`
   (HTTP helpers), `internal/platform/paging`, `internal/platform/crypto`,
   `internal/platform/id`.
7. **Enforcement is mechanical**: `depguard` rules in `.golangci.yml` for the
   layer and platform rules, and a Go test walking the import graph for the
   module-isolation rule.

## Consequences

### Positive
- **One obvious place** for new code; the "which layout?" question disappears.
- **ADR-001's layering becomes real**, because CI fails when it is violated.
- **Modules become extractable** — no `internal/` imports blocking a future split.
- **Dead generations get deleted**, removing ~200 LOC of empty files and the
  confusion they cause.
- **Regressions are caught at PR time**, not at the next architecture review.

### Negative
- **A large, mostly-mechanical move.** Many files change path; open branches will
  conflict. Mitigated by doing it in one PR, immediately after the line-ending
  normalisation.
- **`internal/platform` risks becoming the new `utils`.** Mitigated by requiring
  each sub-package to be named for a concept, never `common`, `shared`, or
  `helpers`.
- **Aquaculture needs real work**, not just moving: its repository implementation
  has never existed. The consolidation surfaces that debt rather than creating it.
- **`depguard` upkeep** as new dependencies are added.

## Alternatives Considered

| Alternative | Reason for rejection |
|---|---|
| **Keep `extensions/` for pluggable capabilities, `internal/modules/` for core** | This is the current state; the distinction was never applied consistently and produced two dead trees. |
| **Flat `internal/` with layer-first packages** (gen-1) | Layer-first grouping scatters one capability across four directories and scales poorly; conflicts with the bounded-context emphasis of ADR-001. |
| **One Go module per business module** | Real isolation, but version-bump overhead for every cross-cutting change. Revisit only if teams need independent release cadences. |
| **Convention documented, enforced by review** | Already the status quo. It produced three layouts and an inverted dependency. |

## Implementation Plan

1. Create `internal/platform/{paging,crypto,id}` and `internal/httpx`; move
   `internal/utils` contents into them; re-point the 11 identity imports; delete
   `internal/utils`.
2. Add the `depguard` configuration and the module-isolation test — **before**
   the bulk move, so the new structure is protected from the first commit.
3. Move `extensions/identity/**` → `modules/identity/**`
   (`infrastructure/http/` → `transport/`).
4. Consolidate the three aquaculture fragments into `modules/aquaculture/`;
   delete the empty files; write the missing repository.
5. Delete `internal/domain/`, `internal/application/`, `internal/modules/`,
   `extensions/`.
6. Correct the `aqualculture` misspellings during the move.

## Compliance / Enforcement

- `golangci-lint` with `depguard` rules for layer purity and platform independence.
- `tools/archtest` test asserting no module imports another module.
- CI fails on any file of 0 bytes.
- Review checklist: no bun model outside `infrastructure/persistence`; no domain
  entity serialised directly in `transport`.

## Related ADRs
- [ADR-001: Adopt Domain-Driven Design](001%20-%20Adopt%20Domain%20Driven%20Design.md) — this ADR makes its layering rules enforceable
- [ADR-004: Consolidate on a Single Extension Framework](004%20-%20Consolidate%20on%20a%20Single%20Extension%20Framework.md)
- [ADR-010: Module-Owned Database Migrations](010%20-%20Module-Owned%20Database%20Migrations.md)

## Related Documents
- [Module layout](../01-modularity/module-layout.md)
- [Dependency rules](../01-modularity/dependency-rules.md)

---

**This ADR serves as a binding architectural decision for the project.**
