# ADR-009: Token Classes and Asymmetric Signing

## Status
**Proposed** – *Date: 2026-08-02*

## Context

Two independent JWT implementations exist, disagreeing on algorithm, claims, and
validation.

**System A** — `internal/plugins/auth/jwt.go`, HS256 with a shared secret. This
is the one wired into `main.go`. Defects:

- The keyfunc returns the secret without inspecting `token.Method`, so the
  signing algorithm is not pinned (`jwt.go:109-111`). golang-jwt v5 rejects
  `alg: none` by default, so this is not immediately exploitable — but it is the
  standard audit finding, and this codebase contains RSA keys as well, which is
  exactly the configuration where algorithm confusion becomes exploitable.
- `iss` and `aud` are never validated. Any token signed with the right secret is
  accepted, including one minted by a different service sharing that secret.
- `GenerateToken(userID, name, email)` never uses `email`, and writes `name` into
  the `Issuer` claim only to overwrite it with `p.issuer` two lines later
  (`jwt.go:158-168`). The middleware then reads `Issuer` back as the user's name
  (`jwt.go:123-125`), so every request sees the name `"kopiochi"`.

**System B** — `modules/identity/infrastructure/token/jwt.go`, RS256 with an RSA
keypair. It is closer to correct: it *does* pin the signing method
(`jwt.go:75-77`). Remaining gaps:

> *An earlier revision of this Context cited System B as
> `extensions/identity/infrastructure/token/jwt.go` and described it as dead code.
> No `extensions/` directory appears in any commit of this repository, and since
> `5f6edfe` this service has been the **live** verifier —
> `modules/identity/transport/middleware.go` calls its `Validate`. The path has
> been repointed and the "dead code" characterisation withdrawn. That makes the
> gaps below the urgent ones, not the deferred ones. This ADR is `Proposed`, so
> the Context is revised in place.*

- No `iss` or `aud` validation, even though `IssueIDToken` sets `aud` to the
  client ID.
- **Three token types share one validation path.** `IssueAccessToken`,
  `IssueIDToken`, and `IssueMFAToken` all produce tokens that pass the same
  `Validate()`. An MFA token — issued *before* the second factor is verified —
  is structurally a valid access token. The only thing preventing privilege
  escalation is every caller remembering to inspect `claims.Scope`.
- `IssueIDToken` hardcodes a 15-minute TTL, ignoring configuration.
- A single keypair with no `kid` header, so rotating the key invalidates every
  outstanding token at once.

## Decision

1. **One token service** for the whole application. System A is deleted.
2. **RS256 (asymmetric) signing.** HMAC is removed.
3. **Every token carries an explicit class claim `cls`:** `access`, `refresh`,
   `mfa`, or `id`.
4. **Validation requires the expected class as a parameter:**
   `Validate(token string, want Class)`. There is no default, so every call site
   must state its expectation. A class mismatch is an error.
5. **Every parse validates** algorithm (`WithValidMethods`), issuer, audience,
   and expiry, with 30 seconds of leeway for clock skew.
6. **Key rotation via `kid`** headers and a published JWKS endpoint, so old and
   new keys coexist during rotation.
7. **All TTLs come from configuration.** No hardcoded durations.
8. **Refresh tokens are opaque random values** (≥256 bits from `crypto/rand`)
   stored as SHA-256 hashes, rotated on every use, with reuse detection revoking
   the whole token family.
9. **Profile claims go in profile claims.** `name` and `email` never occupy
   registered claims like `iss`.

## Consequences

### Positive
- **Privilege escalation via MFA tokens is eliminated** — presenting an MFA token
  to an API endpoint fails at the validation boundary, not by convention.
- **Algorithm confusion is closed** by explicit pinning.
- **Cross-service token acceptance is closed** by `iss`/`aud` validation.
- **Verifiers no longer need a forging key.** With RS256, other services verify
  using the public key; a leaked public key is harmless.
- **Key rotation stops being an outage.** `kid` allows overlapping validity.
- **Stolen refresh tokens are detectable** through reuse detection.
- **The name/email bug disappears** with correct claim mapping.

### Negative
- **RS256 is slower than HS256** (~1ms signing, ~0.1ms verification for RSA-2048).
  Negligible at this scale; verification dominates and is fast.
- **Key management is more involved** than a shared secret: two files, rotation
  procedure, JWKS endpoint. Mitigated by the tooling in
  [ADR-008](008%20-%20Configuration%20Precedence%20and%20Secret%20Handling.md).
- **The `want Class` parameter touches every call site.** That is the point — it
  makes the expectation explicit and reviewable.
- **Rotating the current secret invalidates outstanding tokens,** requiring a
  scheduled re-authentication.

## Alternatives Considered

| Alternative | Reason for rejection |
|---|---|
| **Keep HS256, just pin the algorithm** | Fixes one finding; every verifier still holds a key that can forge tokens, which blocks any future service split. |
| **EdDSA (Ed25519)** | Smaller and faster than RSA and a good choice technically, but RSA keys, loading code, and config paths already exist here. Revisit at the next key rotation. |
| **Opaque tokens with server-side sessions** | Simplest to revoke, but requires a store lookup per request and gives up stateless verification. Retained where it matters: refresh tokens *are* opaque and stored. |
| **Separate services per token type** | Duplicates key handling and parsing; a class claim achieves the separation within one service. |
| **Scope-only distinction (status quo in system B)** | Relies on every caller remembering to check `scope`; one missed check is an authentication bypass. |

## Implementation Plan

1. Add algorithm pinning plus `iss`/`aud` validation to system B — small,
   isolated, and independently shippable. **Do this first.**
2. Introduce the `Class` type and unified `Claims`; stamp `cls` on issue.
3. Change `Validate` to require an expected class; update every call site.
4. Point the auth middleware at the unified service; delete
   `internal/plugins/auth/jwt.go`.
5. Make `IssueIDToken` take a TTL; add `identity.id_token_ttl` to config.
6. Add `kid` headers and the JWKS endpoint.
7. Add refresh-token rotation with reuse detection.
8. Add tests for every rejection path.

## Compliance / Enforcement

- Tests assert rejection of: wrong algorithm, wrong issuer, wrong audience,
  expired token, and **MFA token presented as an access token**.
- Review rejects any `jwt.Parse*` call without `WithValidMethods`.
- Review rejects any `Validate` call whose expected class is not explicit.
- No token value is ever logged; log `jti` instead.
- `govulncheck` runs in CI to catch JWT library advisories.

## Related ADRs
- [ADR-008: Configuration Precedence and Secret Handling](008%20-%20Configuration%20Precedence%20and%20Secret%20Handling.md)
- [ADR-007: API Versioning at the Router Boundary](007%20-%20API%20Versioning%20at%20the%20Router%20Boundary.md)

## Related Documents
- [Token architecture](../04-security/token-architecture.md)
- [Middleware hardening](../04-security/middleware-hardening.md)

---

**This ADR serves as a binding architectural decision for the project.**
