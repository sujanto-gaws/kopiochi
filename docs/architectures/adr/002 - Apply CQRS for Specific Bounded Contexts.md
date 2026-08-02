# ADR-002: Apply CQRS for Specific Bounded Contexts

## Status
**Accepted** – *Date: 2026-05-15*

## Context
Following ADR-001 (Adopt Domain-Driven Design), we have multiple bounded contexts (e.g., Ordering, Inventory, Billing, Reporting). We observe that:

- Some contexts have **significantly different read and write workloads** (e.g., Ordering has high-volume writes for checkout, but complex reads for order history dashboards).
- The domain model (aggregates, entities) is optimised for **write-side invariants** but becomes awkward and inefficient for queries (e.g., joining across aggregates, projection performance).
- Using the same domain model for both commands and queries leads to:
  - Over‑fetching data or multiple specialised query methods on repositories.
  - Accidental modification side‑effects in read‑only operations.
  - Difficulty optimising database schemas (e.g., OLTP for writes, denormalised views for reads).

We need to decide whether to introduce **CQRS** (Command Query Responsibility Segregation) – separating the models for writes and reads – in some bounded contexts.

## Decision
We will apply **CQRS** selectively to **specific bounded contexts** where read/write asymmetry or performance requirements justify the added complexity. Other contexts remain with a traditional uniform model (shared domain entity for reads and writes).

### Which contexts get CQRS?
- **Ordering** – Yes: high write throughput (checkout, status updates); complex read models (dashboard, order history with customer, payment, shipment details).
- **Inventory** – Maybe: writes (stock adjustments) are frequent, reads (stock availability) are also frequent but simple. Will start without CQRS, evaluate later.
- **Billing** – No: writes and reads are both moderate and transaction‑oriented; single model suffices.
- **Reporting** – Yes (query‑only): essentially a pure read model; no commands. This is “CQRS lite”.

### Implementation details
1. **Write side (Command model)**
   - Uses DDD aggregates, repositories, domain events as defined in ADR-001.
   - Command handlers (Application layer) validate and invoke aggregate methods.
   - Data persisted to a normalised write database (e.g., PostgreSQL with ACID).

2. **Read side (Query model)**
   - Uses **separate, simplified read models** (DTOs/POCOs) optimised for specific UI or API needs.
   - Read models are populated via **projections** – listening to domain events from the write side and updating denormalised read tables or a separate read store.
   - Queries bypass aggregates and repositories; go directly to a read‑optimised database (e.g., same DB with views/materialised views, or separate read replica, or even a different technology like Elasticsearch).

3. **Synchronisation**
   - Domain events published from the write side are consumed by event handlers that update the read models (eventual consistency).
   - For scenarios requiring immediate consistency (e.g., command that returns data just written), the command handler can directly query the write side (but prefer sending IDs and letting client re-fetch from read side).

4. **No shared model between command and query** – separate classes, separate data access.

### Code example
```go
// ========== Write side (Domain + Application) ==========
// domain/order.go
type Order struct {  // Aggregate
    ID      string
    UserID  string
    Items   []OrderItem
    Status  string
}

// application/command/create_order.go
type CreateOrderCommand struct {
    UserID string
    Items  []OrderItem
}

type OrderCommandHandler struct {
    repo OrderRepository
}

func (h *OrderCommandHandler) Handle(ctx context.Context, cmd CreateOrderCommand) error {
    order := &Order{
        ID:     uuid.New().String(),
        UserID: cmd.UserID,
        Items:  cmd.Items,
        Status: "pending",
    }
    // Domain invariants, validations, events...
    return h.repo.Save(ctx, order)
}

// ========== Read side (separate module) ==========
// readmodel/order.go
type OrderReadModel struct {  // Denormalised DTO
    ID         string
    UserID     string
    UserName   string
    ItemsCount int
    TotalPrice float64
    Status     string
    CreatedAt  time.Time
}

// readmodel/queries.go
type OrderQueries interface {
    GetByID(ctx context.Context, id string) (*OrderReadModel, error)
    GetHistory(ctx context.Context, userID string, page, pageSize int) ([]OrderListItem, error)
}

// infrastructure/readdb/order_queries.go
type PostgresOrderQueries struct {
    db *sql.DB
}

func (q *PostgresOrderQueries) GetByID(ctx context.Context, id string) (*OrderReadModel, error) {
    // Direct query on denormalised read table
    row := q.db.QueryRowContext(ctx, `SELECT id, user_id, user_name, items_count, total_price, status, created_at FROM order_read_models WHERE id = $1`, id)
    // ... scan and return
}
```

## Consequences

### Positive
- **Scalability** – Write and read sides can be scaled independently (e.g., more read replicas).
- **Performance** – Queries no longer need to reconstruct complex aggregates or join across many tables.
- **Simpler read logic** – Read models directly match UI needs; no bloated repositories with dozens of specialised query methods.
- **Security** – Separation reduces risk of accidental data modification through read operations.
- **Flexibility** – Different storage technologies can be used per side (e.g., EventStore for writes, Elasticsearch for reads).

### Negative
- **Increased complexity** – Two models, two data access strategies, eventual consistency.
- **Eventual consistency** – Write‑side changes are not instantly visible on the read side (milliseconds to seconds). Not acceptable for all scenarios (e.g., strict real‑time inventory check).
- **More code** – Command handlers, event handlers, projections, read model definitions.
- **Operational overhead** – Monitoring event processing lag, handling failures in projections.
- **Not suitable for simple CRUD** – Overkill for contexts with trivial read/write symmetry.

## Alternatives Considered

| Alternative | Why rejected |
|-------------|---------------|
| **Single model (traditional DDD)** | Led to performance issues, complex join-heavy queries, and repository bloat in Ordering context. |
| **CQRS with Event Sourcing** | Too heavy for initial implementation; adds event store, replay logic, etc. We will evaluate separately (ADR-003). |
| **Read‑only views on same DB** | Still couples read model to write schema; doesn’t solve query optimisation fully. But acceptable for simple contexts – we use this for Billing. |
| **Separate databases without CQRS** | No clear separation of models; still leak write abstractions. CQRS enforces discipline. |

## Implementation Plan
1. **Identify bounded contexts** requiring CQRS (start with Ordering).
2. **Design command model** (already exists from ADR-001).
3. **Design read model schema** (denormalised tables) and query interfaces.
4. **Implement projections** – event handlers that listen to domain events (e.g., `OrderPlaced`, `OrderShipped`) and update read tables.
5. **Implement query endpoints** – API controllers that use only read-side services.
6. **Handle eventual consistency** – UI feedback (e.g., “Your order was placed, it may take a few seconds to appear in history”).
7. **Monitor and iterate** – Add contexts like Reporting later.

## Risks & Mitigations
| Risk | Mitigation |
|------|-------------|
| Eventual consistency unacceptable for some operations | Keep those operations on the write side (e.g., after placing order, redirect to “order confirmed” page that reads from write side once). |
| Projection failures lead to stale reads | Implement retry logic, dead‑letter queue, alerting, and ability to rebuild read models from events. |
| Developers confused by two models | Clear documentation, code organisation (separate folders: `CommandModel`, `QueryModel`), and team training. |

## Related ADRs
- **ADR-001** (Adopt DDD) – Prerequisite; defines bounded contexts and domain events.
- **ADR-003** (Event Sourcing) – Future decision; may be combined with CQRS but not required.

---

**This ADR is binding for the Ordering and Reporting bounded contexts. Other contexts default to single‑model DDD unless changed via a future ADR amendment.**
