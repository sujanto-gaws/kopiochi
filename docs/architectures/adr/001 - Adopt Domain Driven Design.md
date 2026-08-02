# ADR-001: Adopt Domain-Driven Design (DDD)

## Status
**Accepted** – *Date: 2026-05-15*

## Context
Our system handles complex business logic with evolving domain rules, multiple bounded contexts (e.g., order management, inventory, billing), and cross-functional teams. We need an architectural approach that:

- Keeps the core domain model pure and independent of technical infrastructure.
- Improves communication between domain experts and developers via a shared ubiquitous language.
- Allows scalability of development teams by splitting the system into bounded contexts.

Alternative styles considered:
- **Transaction Script** (too simplistic, leads to business logic duplication).
- **Active Record** (couples domain logic with persistence, hard to test).
- **CRUD-based layered architecture** (anemic domain model, business logic leaks into services).

## Decision
We will apply **Domain-Driven Design (DDD)** as the primary architectural approach for the core domain modules. Specifically:

1. **Strict layer separation** – Domain, Application, Infrastructure, and Presentation (API) layers with dependency inversion.
2. **Aggregates & Entities** – Encapsulate business rules and ensure transactional consistency.
3. **Value Objects** – Immutable, self-validating domain concepts.
4. **Domain Events** – Capture meaningful business occurrences and enable loose coupling between bounded contexts.
5. **Repositories** – Defined as interfaces in the Domain layer, implemented in Infrastructure.
6. **Domain Services** – Stateless operations that don’t naturally belong to an entity or value object.
7. **Bounded Contexts** – Each major business capability (e.g., Sales, Shipping, Billing) has its own context, with explicit Context Maps (e.g., shared kernel, anti-corruption layer).

We will **not** implement DDD across the whole system; simple CRUD parts (e.g., admin settings, reporting) may use a simpler pattern to avoid over-engineering.

## Consequences

### Positive
- **Business alignment** – Models directly reflect domain expert’s language; reduces misunderstandings.
- **Maintainability** – Changes to business rules are localised within domain models, not scattered across controllers or database code.
- **Testability** – Domain layer can be unit-tested without infrastructure dependencies.
- **Team scalability** – Different bounded contexts can be owned by different teams, with clear contracts.
- **Technology flexibility** – Infrastructure can be swapped (e.g., from Entity Framework to Dapper, from JWT to OAuth) without touching the domain.

### Negative
- **Learning curve** – Developers must understand aggregates, value objects, domain events, and tactical DDD patterns.
- **Initial overhead** – More design up‑front (event storming, context mapping, aggregate identification) compared to CRUD.
- **Persistence complexity** – Mapping aggregates to relational databases (e.g., handling value object columns, aggregate roots) requires careful implementation.
- **Risk of over‑engineering** – Applying DDD to simple sub‑domains (e.g., read‑only lookup tables) creates unnecessary complexity. We will mitigate by using DDD only for **core** and **supporting** sub‑domains, not generic ones.

## Alternatives Considered

| Pattern | Reason for rejection |
|---------|----------------------|
| **Transaction Script** | Business logic duplicates; large service classes become unmaintainable as rules grow. |
| **Active Record** | Couples data access with behavior; violates Single Responsibility; hard to test domain in isolation. |
| **CQRS without DDD** | Fine for query/command separation but lacks domain modelling guidance; may lead to anemic domain. We may combine DDD + CQRS later. |
| **Event Sourcing only** | Useful but not mandatory; DDD does not require event sourcing. We can add it per bounded context if needed. |

## Implementation Plan
1. **Event storming** – With domain experts to define bounded contexts, aggregates, and core flows.
2. **Define domain model** – Entity, value object, aggregate, domain event, and repository interfaces in `Domain` project.
3. **Implement application layer** – Use cases, command/query DTOs, orchestrate domain objects.
4. **Implement infrastructure** – Repositories, token service, email sender, etc., referencing domain interfaces.
5. **API (Presentation)** – Controllers that call application layer, map DTOs.
6. **Establish context mapping** – For each bounded context (e.g., Sales → Shipping) define anti-corruption layer or shared kernel.

## Compliance / Enforcement
- Code reviews must verify domain layer has **no** dependency on infrastructure or application services.
- Repository interfaces are defined in Domain, implemented in Infrastructure.
- Use static analysis (e.g., NetArchTest) to enforce layer dependency rules.
- DTOs are not allowed in Domain; they reside in Application/API.

## Related ADRs
- (Future) ADR-002: CQRS for specific bounded contexts
- (Future) ADR-003: Event Sourcing for the Ordering context

---

**This ADR serves as a binding architectural decision for the project.**