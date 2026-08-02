# Kopiochi — Architecture Documentation

This directory holds the architecture documentation for Kopiochi: where the system
is today, where it should go, and the binding decisions that get it there.

## How this is organised

Documents are grouped by **topic**. Each topic folder is self-contained and can be
read on its own; cross-references are relative links.

| Folder | Topic | Read it when… |
|--------|-------|---------------|
| [`00-overview/`](00-overview/) | Current state, target architecture, principles | You are new to the codebase or need the big picture |
| [`01-modularity/`](01-modularity/) | Extension framework, module layout, dependency rules | You are adding a module or moving code between layers |
| [`02-composition/`](02-composition/) | DI container, routing/versioning, lifecycle | You are wiring a new handler or changing startup/shutdown |
| [`03-configuration/`](03-configuration/) | Config model, secret management | You are adding a setting or handling a credential |
| [`04-security/`](04-security/) | Tokens, middleware hardening, rate limiting | You touch auth, headers, or anything internet-facing |
| [`05-data/`](05-data/) | Persistence, pooling, migrations | You change the schema or the data access layer |
| [`06-quality/`](06-quality/) | Testing, observability, repo hygiene | You are setting up CI, tests, or cleaning the repo |
| [`07-roadmap/`](07-roadmap/) | Sequenced remediation plan | You are planning the next sprint |
| [`adr/`](adr/) | Architecture Decision Records | You need the *why* behind a binding decision |

## Document types

- **Topic documents** (`00-` … `07-`) describe the current problem, the target
  design, and the migration path. They are living documents — update them as the
  system changes.
- **ADRs** (`adr/`) record a single decision at a point in time. They are
  append-only: an ADR is never edited after acceptance, only superseded by a
  later ADR.

## Decision index

| ADR | Title | Status |
|-----|-------|--------|
| [001](adr/001%20-%20Adopt%20Domain%20Driven%20Design.md) | Adopt Domain-Driven Design | Accepted |
| [002](adr/002%20-%20Apply%20CQRS%20for%20Specific%20Bounded%20Contexts.md) | Apply CQRS for Specific Bounded Contexts | Accepted |
| [003](adr/003%20-%20Event%20Sourcing%20for%20Specific%20Bounded%20Contexts.md) | Event Sourcing for Specific Bounded Contexts | Accepted |
| [004](adr/004%20-%20Consolidate%20on%20a%20Single%20Extension%20Framework.md) | Consolidate on a Single Extension Framework | Accepted — partially implemented |
| [005](adr/005%20-%20Module%20Boundaries%20and%20Dependency%20Direction.md) | Module Boundaries and Dependency Direction | Proposed |
| [006](adr/006%20-%20Explicit%20Compile-Time%20Dependency%20Injection.md) | Explicit Compile-Time Dependency Injection | Accepted — implemented |
| [007](adr/007%20-%20API%20Versioning%20at%20the%20Router%20Boundary.md) | API Versioning at the Router Boundary | Accepted — implemented |
| [008](adr/008%20-%20Configuration%20Precedence%20and%20Secret%20Handling.md) | Configuration Precedence and Secret Handling | Proposed |
| [009](adr/009%20-%20Token%20Classes%20and%20Asymmetric%20Signing.md) | Token Classes and Asymmetric Signing | Proposed |
| [010](adr/010%20-%20Module-Owned%20Database%20Migrations.md) | Module-Owned Database Migrations | Proposed |
| [011](adr/011%20-%20Build%20Artifacts%20Excluded%20from%20Version%20Control.md) | Build Artifacts Excluded from Version Control | Accepted — partially implemented |

**Status vocabulary.** `Proposed` — decided in principle, no code yet.
`Accepted — partially implemented` — some implementation steps have shipped;
the ADR lists which. `Accepted — implemented` — every implementation step in the
ADR has shipped and is covered by a test. An ADR's status changes as its
implementation steps land; its Context and Decision are still append-only.

## Scope note

These documents were produced from a full review of the repository, and Phases 0
and 1 of the [remediation plan](07-roadmap/remediation-plan.md) have since landed
against them.

The review's headline finding — that the application compiled but **served no
business routes** — no longer applies: `cmd/api/container.go` (`BuildApp`) wires
the `identity` and `user` modules and `internal/httpx.Mount` serves them under a
real `/api/v1` (`ef76759`, `4fdc609`), proven end to end by
`cmd/api/login_e2e_test.go`. A large fraction of hand-written Go is still
unreachable from `cmd/`. The topic documents below address the structural causes,
not just the symptoms.

⚠️ Several of the review's claims about the *source layout* — specific directory
names and file inventories — do not match any commit in this repository's
history and are being re-verified. Until that pass lands, treat file paths in
these documents as indicative and check them against the tree.

Start with [`00-overview/current-state.md`](00-overview/current-state.md), then
[`07-roadmap/remediation-plan.md`](07-roadmap/remediation-plan.md) for the
sequenced fix list.
