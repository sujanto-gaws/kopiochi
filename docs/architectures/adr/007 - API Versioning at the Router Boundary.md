# ADR-007: API Versioning at the Router Boundary

## Status
**Proposed** – *Date: 2026-08-02*

## Context

The intended API surface is `/api/v1/...`. The implementation does not produce it.

`internal/infrastructure/http/routes/routes.go:25-29`:

```go
r.Route("/api/v1", func(r chi.Router) {   // inner r shadows the outer *chi.Mux
    for _, reg := range registrars {
        reg.RegisterRoutes(g)              // registers onto g, not onto the inner r
    }
})
```

The inner `r` — the only router mounted at `/api/v1` — is shadowed and never
used. Registrars receive `g`, a `handlers.RouterGroup` constructed in
`cmd/api/main.go:106-113` from `v1 := r.With()`. `chi.Mux.With()` returns an
*inline* router that shares the parent's tree and carries **no path prefix**.

So any route a registrar adds mounts at `/` — `/login`, not `/api/v1/login`. The
`r.Route("/api/v1", ...)` block mounts an empty sub-router and contributes
nothing. The defect is currently invisible only because the container is empty
(see [ADR-006](006%20-%20Explicit%20Compile-Time%20Dependency%20Injection.md)) and
the loop body never runs.

Two further problems come from the same design:

- **Auth binding fails open.** `main.go:99-112` resolves the auth middleware once
  and falls back to `protected = v1` when no auth plugin is initialised. With
  `plugins.auth.jwt.enabled: false` — the shipped default — every route a module
  considers protected is served unauthenticated, with nothing logged.
- **Routing knowledge is split across three files.** The prefix lives in
  `routes.go`, the public/protected split in `main.go`, and the paths in the
  handler. No single file shows a module's actual URL structure, which is how the
  shadowing survived review.

## Decision

1. **The version group router is passed to modules as a parameter.**
   `Module.Routes` has the signature `func(chi.Router)` and receives the router
   mounted at `/api/v1`. No module closes over a router built elsewhere.
2. **Path-based versioning.** Versions appear in the URL (`/api/v1/...`), not in
   headers or media types.
3. **Modules own their public/protected split**, declaring `r.Group` blocks with
   the middleware each group needs.
4. **Auth middleware is a typed constructor dependency and is never nil.** A
   module that needs authentication fails to construct when a verifier cannot be
   built — fail closed, never fail open.
5. **Operational endpoints are unversioned:** `/healthz`, `/readyz`, `/metrics`.
6. **A route-table test asserts the real mounted paths** using `chi.Walk`.
7. **`RouterGroup` and `RouteRegistrar` are deleted.**

Versioning policy:

| Rule | Detail |
|---|---|
| Bump the major version | Only for breaking changes; additive fields do not |
| Coexistence | `/api/v1` and `/api/v2` may be mounted simultaneously |
| Deprecation | `Deprecation` and `Sunset` headers on the outgoing version |
| Removal | Only after the announced sunset date |

## Consequences

### Positive
- **The shadowing bug becomes unrepresentable** — there is no second router in
  scope to register against by mistake.
- **Fail-open authentication is eliminated.**
- **A module's URL structure is readable in one file.**
- **Per-route authorisation becomes expressible** (public / authenticated /
  admin), which the single global `Protected` router could not do.
- **Versions can coexist**, enabling incremental client migration.

### Negative
- **Modules must know their sub-path** (`/auth`, `/farms`). Acceptable: it is
  their own namespace, and collisions surface immediately in the route-table test.
- **Per-module auth wiring** repeats a line or two per group. That repetition is
  what makes the protection visible.
- **Path versioning duplicates handlers across versions** when v2 arrives. Shared
  application services keep the duplication at the transport layer only.

## Alternatives Considered

| Alternative | Reason for rejection |
|---|---|
| **Header-based versioning** (`Accept: application/vnd.kopiochi.v1+json`) | Harder to test, curl, cache, and log; invisible in access logs and route tables. |
| **Query-parameter versioning** (`?version=1`) | Pollutes every URL; poor cache semantics; easy to omit accidentally. |
| **No versioning** | Any breaking change breaks every client at once. |
| **Fix the shadowing, keep `RouterGroup`** | Removes the immediate bug but keeps routing split across three files and keeps the fail-open auth fallback. |
| **Version per module** (`/identity/v1`, `/aquaculture/v2`) | Clients must track several version axes; whole-API versioning is simpler at this scale. |

## Implementation Plan

1. Add `Routes func(chi.Router)` to the module contract.
2. Replace `routes.Setup` with `httpx.Mount`, passing `v1` into `m.Routes`.
3. Move each module's public/protected split into its own `Routes`.
4. Make the auth middleware a constructor dependency of the modules that need it.
5. Split `/api/health` into `/healthz` and `/readyz` (the latter pinging the DB).
6. Delete `handlers.RouterGroup` and `handlers.RouteRegistrar`.
7. Add `TestRouteTable` — it must fail before step 2 and pass after.

## Compliance / Enforcement

- `TestRouteTable` asserts every expected path, including the `/api/v1` prefix.
- A test asserts that a protected route without a token returns 401 — the guard
  against fail-open regressions.
- Review rejects any route registered outside a module's `Routes` function.
- No handler may be mounted directly on the root mux except the operational
  endpoints and the swagger UI.

## Related ADRs
- [ADR-006: Explicit Compile-Time Dependency Injection](006%20-%20Explicit%20Compile-Time%20Dependency%20Injection.md)
- [ADR-004: Consolidate on a Single Extension Framework](004%20-%20Consolidate%20on%20a%20Single%20Extension%20Framework.md)
- [ADR-009: Token Classes and Asymmetric Signing](009%20-%20Token%20Classes%20and%20Asymmetric%20Signing.md)

## Related Documents
- [Routing and versioning](../02-composition/routing-and-versioning.md)
- [Middleware hardening](../04-security/middleware-hardening.md)

---

**This ADR serves as a binding architectural decision for the project.**
