# ADR-009: Token Classes and Asymmetric Signing

## Status
**Accepted — partially implemented** – *Date: 2026-08-02; status updated
2026-08-02 after Phase 2.4/2.5/2.6 (`e0da81e`, `946c1c8`, `0cf07d9`)*

Decisions 1, 2, 4, 5, and 9 have shipped and are covered by tests; decision 3
shipped in a reduced form. Decisions 6, 7, and 8 have not — see
[Implementation status](#implementation-status). This ADR was `Proposed` and is
therefore revised in place; its Context and Decision remain append-only from
here.

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

> *Appended 2026-08-02, after Phase 2.* The Context above describes the state at
> the time of the decision and is left as written. **System A no longer exists:**
> `0cf07d9` deleted `internal/plugins/auth/jwt.go`, its `RegisterBuiltinPlugins`
> entry, the `APP_JWT_SECRET` binding and placeholder check, the
> `plugins.auth.jwt` config block, and its `.env.example` entries. So A1–A3 were
> resolved by deletion rather than by fixing, and the "two independent JWT
> implementations" premise no longer holds. Of system B's four gaps, the first
> two — no `iss`/`aud` validation and three token types sharing one validation
> path — are fixed; the hardcoded ID-token TTL and the missing `kid` are not. See
> [Implementation status](#implementation-status).

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

1. ✅ `e0da81e` — Add algorithm pinning plus `iss`/`aud` validation to system B.
   Shipped first, as advised.
2. ✅ `946c1c8` — Introduce the `Class` type and unified `Claims`; stamp `cls` on
   issue.
3. ✅ `946c1c8` — Change `Validate` to require an expected class; update every
   call site (`transport/middleware.go` → `ClassAccess`,
   `application/mfa_verify_login.go` → `ClassMFA`).
4. ✅ `6d0c1b7` (Phase 1) / `0cf07d9` — Point the auth middleware at the unified
   service; delete `internal/plugins/auth/jwt.go`.
5. ⏳ Make `IssueIDToken` take a TTL; add `identity.id_token_ttl` to config.
   `IssueMFAToken` has the same defect and should be fixed alongside it —
   `auth.mfa_temporary_ttl` is validated and then ignored.
6. ⏳ Add `kid` headers and the JWKS endpoint — Phase 5.5.
7. ⏳ Add refresh-token rotation with reuse detection — Phase 5.6.
8. ✅ `e0da81e`, `946c1c8` — Add tests for every rejection path.

## Implementation status

| # | Decision | State |
|---|---|---|
| 1 | One token service; system A deleted | ✅ `0cf07d9`. The `AuthPlugin` *interface* survives because `fido2-auth` implements it; it goes with Phase 3.6 |
| 2 | RS256 signing; HMAC removed | ✅ No HMAC code, secret, or config key remains anywhere in the tree |
| 3 | Every token carries `cls`: `access`, `refresh`, `mfa`, `id` | ◐ Three classes shipped — `access`, `mfa`, `id`. **There is no `refresh` class**, because refresh tokens are opaque random values rather than JWTs and never pass through `Validate`. The decision's four-value enumeration was wrong about its own decision 8 |
| 4 | `Validate(token, want Class)` with no default | ✅ Signature enforced by the `TokenIssuer` interface, so a call site cannot omit it |
| 5 | Every parse validates alg, iss, aud, exp, with leeway | ✅ `WithValidMethods`, `WithIssuer`, `WithAudience`, `WithExpirationRequired`, `WithLeeway`. Leeway is configurable via `auth.token_leeway` rather than hardcoded at 30s — 30s is the default and the fallback |
| 6 | Key rotation via `kid` + JWKS | ⏳ Phase 5.5. Now the largest remaining token gap: with HS256 gone, the RSA keypair is the only signing material |
| 7 | All TTLs from configuration | ⏳ Access-token TTL is passed in; ID (15m) and MFA (5m) are still hardcoded in `jwt.go` |
| 8 | Opaque refresh tokens, rotated, with reuse detection | ◐ Opaque and SHA-256-hashed already; rotation and reuse detection are Phase 5.6 |
| 9 | Profile claims go in profile claims | ✅ `name` and `email` are their own claims; nothing writes to `iss`. The A3 overwrite bug died with system A |

## Compliance / Enforcement

- Tests assert rejection of: wrong algorithm, wrong issuer, wrong audience,
  expired token, and **MFA token presented as an access token**.
  ✅ *All five shipped in `modules/identity/infrastructure/token/jwt_test.go`,
  plus `TestValidate_RejectsNoneAlgorithm` and
  `TestValidate_AcceptsGenuineAccessToken`.*
- Review rejects any `jwt.Parse*` call without `WithValidMethods`.
  ✅ *There is exactly one `jwt.ParseWithClaims` in the tree and it has one.*
- Review rejects any `Validate` call whose expected class is not explicit.
  ✅ *Structurally enforced — the parameter is required, so this is now a
  compiler rule rather than a review rule.*
- No token value is ever logged; log `jti` instead.
  ◐ *No token value is logged. There is no `jti` to log yet (decision 7 of the
  claim mapping in [token architecture](../04-security/token-architecture.md)).*
- `govulncheck` runs in CI to catch JWT library advisories.
  ⏳ *No CI exists — Phase 4.4.*

## Related ADRs
- [ADR-008: Configuration Precedence and Secret Handling](008%20-%20Configuration%20Precedence%20and%20Secret%20Handling.md)
- [ADR-007: API Versioning at the Router Boundary](007%20-%20API%20Versioning%20at%20the%20Router%20Boundary.md)

## Related Documents
- [Token architecture](../04-security/token-architecture.md)
- [Middleware hardening](../04-security/middleware-hardening.md)

---

**This ADR serves as a binding architectural decision for the project.**
