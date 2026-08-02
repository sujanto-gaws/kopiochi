# ADR-003: Event Sourcing for Specific Bounded Contexts

## Status
**Proposed** – *Date: 2026-05-15*  
(Status may be changed to Accepted or Rejected after evaluation)

## Context
Following ADR-001 (DDD) and ADR-002 (selective CQRS), we have bounded contexts where:

- **Auditability** is a legal or business requirement – e.g., Ordering, Billing, Accounting.
- **Debugging & forensics** – need to reconstruct past states or understand exactly how an aggregate reached its current state.
- **Complex business rules** that depend on the sequence of events rather than just the current state (e.g., fraud detection, workflow sagas).
- **Event-driven architecture** already in place – domain events are published for projection updates (ADR-002).

However, storing only the current state (traditional state‑based persistence) loses the history of changes. When we need that history, we either:
- Manually create audit logs (duplicate effort, error‑prone).
- Use database triggers or change data capture (brittle, not domain‑aware).

Event Sourcing (ES) stores every state change as an immutable event; the current state is derived by replaying events. This aligns naturally with DDD’s **domain events** and CQRS’s **event‑driven projections**.

But ES introduces significant complexity. We need a decision on **whether and where** to adopt Event Sourcing.

## Decision
We will **apply Event Sourcing only to bounded contexts that explicitly require full auditability or event‑replay capabilities** – initially **Ordering** and **Accounting**. Other contexts (Inventory, Billing, Reporting, Authentication) will use **traditional state‑based persistence** (even if they use CQRS).

### Which contexts get Event Sourcing?

| Bounded Context | Event Sourcing? | Rationale |
|----------------|----------------|-----------|
| **Ordering** | ✅ Yes | Legal requirement to keep full order history; complex status transitions; need to support “what‑if” scenario replay. |
| **Accounting** | ✅ Yes | Financial transactions must be immutable, auditable, and replayable for ledger reconstruction. |
| **Billing** | ❌ No (state‑based) | Invoices can be regenerated from orders; no legal need for event‑level audit. Simpler is safer. |
| **Inventory** | ❌ No | High volume of stock adjustments; replaying all events would be inefficient. Use state with periodic snapshots if needed. |
| **Reporting** | ❌ No (read‑only) | Reporting reads from projections built by events, but its own source is not event‑sourced. |

### Implementation details

1. **Event store** – A dedicated database for storing events (e.g., EventStoreDB, PostgreSQL with an events table, or Kafka as source of truth).
   - Each event is immutable, versioned, and contains all data needed to replay the state.
   - Events are stored per aggregate stream (e.g., `Order-{orderId}`).

2. **Aggregate design** – The aggregate:
   - Applies events to build its current state (via `Apply(Event)` methods).
   - Produces new events when commands are executed (before applying them).
   - No direct persistence – the repository only saves the new events.

3. **Event publishing** – After events are stored, they are published to a message bus (e.g., RabbitMQ, Kafka) for:
   - Updating read models (CQRS projections).
   - Notifying other bounded contexts (e.g., Inventory listens to `OrderPlaced`).

4. **Snapshots** – To avoid replaying years of events, we take periodic snapshots of aggregate state (e.g., every 100 events).

5. **Projections** – Same as ADR-002, but the read models are built from the event stream rather than being updated by command handlers.

### Code example (Golang – simplified)

```go
// domain/order.go
type Order struct {
    ID     string
    Status string
    // ... other state
}

func (o *Order) Apply(event OrderEvent) { ... } // rebuilds state

func (o *Order) Cancel() ([]Event, error) {
    if o.Status == "shipped" {
        return nil, errors.New("cannot cancel shipped order")
    }
    return []Event{OrderCancelledEvent{OrderID: o.ID}}, nil
}

// infrastructure/eventstore/postgres.go
type EventStore struct {
    db *sql.DB
}

func (es *EventStore) Save(streamID string, events []Event, expectedVersion int) error {
    // Append to events table with version check (optimistic concurrency)
}

func (es *EventStore) Load(streamID string) ([]Event, error) {
    // Query events ordered by version
}
```

## Consequences

### Positive
- **Complete audit trail** – Every change is recorded immutably; no data loss.
- **Temporal queries** – Reconstruct aggregate state at any point in time.
- **Event replay** – Rebuild read models, debug production issues, or run “what‑if” scenarios.
- **Decoupling** – Write side only appends events; no direct updates to read models or other contexts (eventual consistency becomes natural).
- **Resilience to bugs** – Fix a projection bug by replaying events; no data corruption.

### Negative
- **Complexity** – Event versioning, event schema evolution, snapshotting, concurrency control (optimistic locking).
- **Storage overhead** – Events accumulate; requires snapshots and eventual archival.
- **Eventual consistency** – Strong consistency is harder (e.g., reading your own writes may require waiting for projection).
- **Learning curve** – Teams must understand event sourcing patterns and avoid anti‑patterns (e.g., using events as primary API, storing huge payloads).
- **Performance** – Loading aggregates requires replaying events (mitigated by snapshots).
- **Tooling** – Standard ORMs (GORM, Entity Framework) don’t support ES; need custom code or specialised stores.

## Alternatives Considered

| Alternative | Why rejected (for contexts requiring audit) |
|-------------|----------------------------------------------|
| **State‑based + audit log table** | Duplicates logic; audit log is separate from domain model; hard to replay state; no built‑in concurrency control across events. |
| **Database change data capture (CDC)** | Low‑level (row‑based), not domain‑event aware; brittle schema changes; cannot express business semantics. |
| **Event Sourcing only for CQRS projections** | Already possible without ES (using domain events). ES is about the **source of truth**, not just projection updates. |
| **Temporal database (e.g., SQL:2011)** | Stores row versions, but no domain event semantics; poor fit for aggregate replay. |

## Implementation Plan (for Ordering context as pilot)

1. **Design event schemas** – `OrderCreated`, `ItemAdded`, `OrderCancelled`, `PaymentConfirmed`, etc.
2. **Set up event store** – Use PostgreSQL with a simple events table (stream_id, version, event_type, event_data, timestamp) as first iteration. Evaluate EventStoreDB later.
3. **Implement aggregate root** – `Order` with `Apply` and command methods that produce events.
4. **Implement event repository** – Load/save event streams with version checking.
5. **Implement snapshotting** – After N events, store a snapshot row to speed up loading.
6. **Implement event publisher** – After saving events, publish to Kafka or RabbitMQ.
7. **Rebuild read models** – Change CQRS projections to subscribe to events from the event store (not from command side).
8. **Gradual rollout** – Start with Ordering; evaluate for Accounting after lessons learned.

## Risks & Mitigations

| Risk | Mitigation |
|------|-------------|
| **Event schema evolution** | Use protobuf or JSON schema with versioning; events are immutable – new fields added as optional; never change or delete existing fields. |
| **Performance degradation** | Snapshot frequently (e.g., every 50 events); use asynchronous projections; consider splitting aggregates if streams grow too long. |
| **Concurrency conflicts** | Implement optimistic concurrency using expected version; retry logic in command handlers. |
| **Event store becomes single point of failure** | Replicate event store; use cloud‑native event stores with high availability (e.g., EventStoreDB cluster, Aurora PostgreSQL with multi‑AZ). |
| **Projection lag** | Monitor lag; for critical reads, command handler can wait for projection (sacrifice eventual consistency) or read from write side. |
| **Deleting events (GDPR compliance)** | Rare – use anonymisation events (e.g., `CustomerAnonymised`), not physical deletion. For hard deletion, consult legal; ES makes deletion hard by design (append‑only). |

## Decision Criteria for Future Contexts

Only add Event Sourcing if **all** of the following are true:
1. Full audit trail is a legal or critical business requirement.
2. Need to reconstruct past states or debug via event replay.
3. Team has capacity to manage the complexity.
4. Domain events are already well‑defined (DDD in place).

Otherwise, **stick with state‑based persistence** (even with CQRS).

## Related ADRs
- **ADR-001** (DDD) – Provides domain events and aggregates; required for ES.
- **ADR-002** (CQRS) – ES works naturally with CQRS, but ES can exist without CQRS (though not recommended).
- **Future ADR** – May decide on specific event store technology (e.g., EventStoreDB vs PostgreSQL vs Kafka as source of truth).

---

**This ADR is a proposal awaiting team review. Acceptance will apply ES only to Ordering and Accounting as pilot contexts, with a mandatory review after 3 months.**