# Token Architecture

**Status:** Accepted — partially implemented — see [ADR-009](../adr/009%20-%20Token%20Classes%20and%20Asymmetric%20Signing.md)
**Date:** 2026-08-02
**Last verified:** 2026-08-02, after Phase 2 — **there is now one token system,
not two.** A1–A4 are moot (system A was deleted), B1 and B2 are fixed, B3 and B5
remain open, and B4 was already judged acceptable and is untouched.

> **What Phase 1 changed.** The RS256 identity token service moved to
> `modules/identity/infrastructure/token/jwt.go` (`5f6edfe`) and became the
> **live** verifier: `modules/identity/transport/middleware.go` calls its
> `Validate`, and `cmd/api/container.go` builds a second instance for the user
> module. The HS256 jwt-auth plugin was `enabled: false` by default and `main.go`
> no longer derived any auth middleware from it.
>
> **What Phase 2 changed.** 2.4 (`e0da81e`) pinned RS256 with
> `jwt.WithValidMethods` and added `iss`/`aud`/`exp` validation with configurable
> leeway. 2.5 (`946c1c8`) added the `cls` claim and `Validate(token, want Class)`.
> 2.6 (`0cf07d9`) **deleted `internal/plugins/auth/jwt.go`** together with its
> `RegisterBuiltinPlugins` entry, the `APP_JWT_SECRET` binding, its placeholder
> check, the `plugins.auth.jwt` config block, and its `.env.example` entries.
> System A is gone; the section below is retained as the record of why.

---

## Problem (resolved): two token systems, both flawed

| System | Location | Algorithm | State |
|---|---|---|---|
| A — plugin | ~~`internal/plugins/auth/jwt.go`~~ | HS256 (shared secret) | **Deleted** — `0cf07d9` |
| B — identity | `modules/identity/infrastructure/token/jwt.go` | RS256 (RSA keypair) | The only token system |

They issued different claim sets, validated differently, and disagreed about what
a token meant. Neither was complete. There is now one, and the disagreement is
closed by deletion rather than by reconciliation.

---

## Defects in system A — resolved by deleting it (`0cf07d9`)

*None of A1–A4 was fixed in place. The file, its registration, its config block,
and its environment binding were all removed in Phase 2.6, so the HMAC path no
longer exists to be misused. The analysis is kept because it is the rationale for
[ADR-009](../adr/009%20-%20Token%20Classes%20and%20Asymmetric%20Signing.md)'s
decision 2, and because A1 is the reason B's pinning is written the way it is.*

### A1 — Signing algorithm is not pinned

`jwt.go:109-111`:

```go
token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
    return p.secret, nil          // no check of token.Method
})
```

The keyfunc accepts whatever algorithm the token header declares. golang-jwt v5
rejects `alg: none` unless the key is the explicit `UnsafeAllowNoneSignatureType`
sentinel, so this is not immediately exploitable — but it is one refactor away
from being so, and it is the standard finding in any audit.

The concrete risk is real given this codebase contains **both** HMAC and RSA
paths: if an RSA public key is ever passed to an HMAC-accepting parser, an
attacker signs a token with the public key (which is public) and it verifies.
Pinning the algorithm removes the entire class.

**Fix:**

```go
token, err := jwt.ParseWithClaims(tokenStr, claims,
    func(t *jwt.Token) (interface{}, error) { return p.secret, nil },
    jwt.WithValidMethods([]string{"HS256"}),
    jwt.WithIssuer(p.issuer),
    jwt.WithAudience(p.audience),
    jwt.WithLeeway(30*time.Second),
)
```

### A2 — Issuer and audience are never validated

Any token signed with the right secret is accepted, regardless of who issued it
or which service it was minted for. If the same secret is shared with another
service — a common accident — that service's tokens are accepted here.

### A3 — `GenerateToken` discards its arguments

`jwt.go:152-172`:

```go
func (p *JWTPlugin) GenerateToken(userID, name, email string) (string, error) {
    claims := &jwt.RegisteredClaims{
        Subject: userID,
        Issuer:  name,          // name goes into the Issuer field
        ...
    }
    if p.issuer != "" {
        claims.Issuer = p.issuer  // ...and is immediately overwritten
    }
```

- `email` is **never used** at all.
- `name` is written to `Issuer`, then overwritten by `p.issuer` whenever the
  plugin has an issuer configured (which it does by default).

The middleware then reads it back as the user's name (`jwt.go:123-125`):

```go
if claims.Issuer != "" {
    ctx = context.WithValue(ctx, UserNameContextKey, claims.Issuer)
}
```

So every request sees the user's name as `"kopiochi"`. `Issuer` is a registered
claim with a defined meaning; overloading it as a display name is incorrect
regardless of the overwrite bug.

### A4 — String-typed context keys

```go
type contextKey string
const UserIDContextKey contextKey = "user_id"
```

The named type prevents collisions with plain-string keys, which is fine. But the
key is exported, so any package can write a value under it and forge identity in
the context. Prefer an unexported key type with exported accessor functions:

```go
type ctxKey struct{}
func UserFrom(ctx context.Context) (Principal, bool) { ... }
func withUser(ctx context.Context, p Principal) context.Context { ... }
```

Only the auth middleware can then set a principal.

---

## Defects in system B — the only token system

System B was already closer to correct — it checked that the algorithm was *some*
RSA variant:

```go
if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
    return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
}
```

*That check is now narrower and enforced earlier. `e0da81e` added
`jwt.WithValidMethods([]string{rs256Alg})`, which rejects any `alg` other than
exactly `RS256` — including `none`, any HMAC variant, and `RS384`/`RS512` —
before the keyfunc is invoked at all. The type assertion above was tightened to
`token.Method != jwt.SigningMethodRS256` and kept as defence in depth, so the
keyfunc still documents and enforces the invariant if it is ever reused on its
own. Tests: `TestValidate_RejectsWrongAlgorithm`,
`TestValidate_RejectsNoneAlgorithm`.*

### B1 (fixed) — No issuer or audience validation

`Validate` (line 75) checks the signature and expiry but never `iss` or `aud`.
`IssueIDToken` sets `aud` to the client ID (line 53) yet nothing verifies it.

*Fixed in `e0da81e`. Every parse now passes `jwt.WithIssuer(s.issuer)`,
`jwt.WithAudience(s.audience)`, `jwt.WithExpirationRequired()`, and
`jwt.WithLeeway(s.leeway)`. `exp` being **required** matters as much as it being
checked: a token minted without one would otherwise never expire.
`NewJWTService` takes `issuer`, `audience`, and `leeway`; leeway comes from
`auth.token_leeway` (default `30s`, validated non-negative) and falls back to a
30-second `defaultLeeway` if a non-positive value is passed. Tests:
`TestValidate_RejectsWrongIssuer`, `TestValidate_RejectsWrongAudience`,
`TestValidate_RejectsExpired`.*

### B2 (fixed) — Token classes are indistinguishable at the validation boundary

Three token types are issued:

| Method | Purpose | Distinguishing claim |
|---|---|---|
| `IssueAccessToken` | API access | *(none)* |
| `IssueIDToken` | OIDC identity | `aud`, `scope: "openid profile email"` |
| `IssueMFAToken` | half-authenticated MFA step | `scope: "mfa"` |

All three pass the same `Validate()`. An MFA token — issued *before* the second
factor is verified — is a structurally valid access token. The only thing
preventing privilege escalation is every caller remembering to inspect
`claims.Scope`. That is a latent authentication bypass waiting for one missed
check.

*Fixed in `946c1c8`. `modules/identity/domain/service.go` defines
`type Class string` with `ClassAccess` / `ClassMFA` / `ClassID`; every issue
method stamps it as the `cls` claim; and the `TokenIssuer` interface signature is
now `Validate(tokenStr string, want Class) (*Claims, error)`. There is no default
`want`, so every call site states its expectation and the compiler enforces that
one is given. A mismatch returns `ErrWrongTokenClass`.*

*Both live call sites were updated: `modules/identity/transport/middleware.go`
requires `domain.ClassAccess`, and
`modules/identity/application/mfa_verify_login.go` requires `domain.ClassMFA`.
An MFA token presented to an API route is therefore rejected at the validation
boundary, not by convention. `TestValidate_RejectsMFATokenAsAccessToken` is the
regression test — one of ADR-009's named compliance requirements — and
`TestValidate_AcceptsGenuineAccessToken` guards the other direction.*

*Note the shipped class set is three, not the four in ADR-009's decision 3:
there is no `ClassRefresh`, because refresh tokens are opaque random values
rather than JWTs and never pass through `Validate` at all.*

### B3 — `IssueIDToken` ignores configured TTL — still open

Line 56 hardcodes `15 * time.Minute` while `IssueAccessToken` takes a `ttl`
parameter. `auth.access_token_ttl` in config is not honoured for ID tokens.

*Unchanged after Phase 2, and `IssueMFAToken` has the same defect: it hardcodes
`5 * time.Minute` while `auth.mfa_temporary_ttl` sits in config being validated
and never read. Both are correctness/consistency bugs rather than security ones —
the hardcoded values are short — but a configured TTL that silently does nothing
is exactly the kind of thing an operator will believe. Deferred with the
`identity.id_token_ttl` work.*

### B4 — Refresh tokens hashed with unsalted SHA-256

> **Withdrawn and re-pointed.** An earlier revision cited
> `utils.StringHashHex(refreshToken)` at `application/auth.go:67` and
> `application/helpers.go:14`, and asked for `internal/utils/generator.go` to be
> checked for `crypto/rand`. No `internal/utils` package appears in any commit of
> this repository and no symbol `StringHashHex` has ever existed in Go source. The
> citations could not be substantiated and have been withdrawn. The finding itself
> is real; the actual code is below.

`modules/identity/domain/refresh_token.go`:

```go
func HashToken(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(h[:])
}
```

This is **acceptable** — provided the refresh token is high-entropy random
(≥128 bits from `crypto/rand`), unsalted SHA-256 resists brute force because
there is nothing to guess. Two follow-ups stand: verify that whatever generates
the plaintext refresh token uses `crypto/rand` and not `math/rand`, and document
the high-entropy precondition on `HashToken` itself, because a function with that
name invites reuse on something low-entropy. It sits in the domain package that
owns the concept rather than in a shared helpers package, which limits the blast
radius — keep it there.

### B5 — No key rotation support — still open

`NewJWTService` loads exactly one keypair. Tokens carry no `kid` header, so
rotating the signing key invalidates every outstanding token at once. There is no
JWKS endpoint.

*Unchanged after Phase 2 — scheduled as Phase 5.5. It is now the largest
remaining gap in this document, and it is more pressing than it was: with HS256
gone, the RSA keypair is the only signing material there is.*

---

## Target design

*Shipped in Phase 2.4/2.5/2.6, with the deviations noted per item below.*

### One token service, three explicit classes

```go
type Class string

const (
    ClassAccess  Class = "access"
    ClassRefresh Class = "refresh"
    ClassMFA     Class = "mfa"     // half-authenticated; NEVER grants API access
    ClassID      Class = "id"
)

type Claims struct {
    jwt.RegisteredClaims
    Class  Class    `json:"cls"`
    Scopes []string `json:"scp,omitempty"`
    Email  string   `json:"email,omitempty"`
    Name   string   `json:"name,omitempty"`
}
```

Every token carries `cls`. Validation **requires the expected class**:

```go
func (s *Service) Validate(tokenStr string, want Class) (*Claims, error) {
    var claims Claims
    token, err := jwt.ParseWithClaims(tokenStr, &claims, s.keyFunc,
        jwt.WithValidMethods([]string{"RS256"}),
        jwt.WithIssuer(s.issuer),
        jwt.WithAudience(s.audience),
        jwt.WithExpirationRequired(),
        jwt.WithLeeway(30*time.Second),
    )
    if err != nil {
        return nil, fmt.Errorf("parse token: %w", err)
    }
    if !token.Valid {
        return nil, ErrInvalidToken
    }
    if claims.Class != want {
        return nil, fmt.Errorf("%w: got %q, want %q", ErrWrongTokenClass, claims.Class, want)
    }
    return &claims, nil
}
```

Presenting an MFA token to an API endpoint now fails at the type level, not by
convention. The `want` parameter has no default — every call site must state its
expectation.

*Shipped in `e0da81e` + `946c1c8` with three differences from the sketch. The
class set is `access` / `mfa` / `id` — no `ClassRefresh`, since refresh tokens
are opaque and never parsed as JWTs. `Class` and `Claims` live in
`modules/identity/domain/service.go`, so the domain owns the vocabulary and the
infrastructure implements it. And the parse still uses `jwt.MapClaims` with the
values copied into the domain `Claims` afterwards, rather than a struct
embedding `jwt.RegisteredClaims` — the same validations run either way, since
they come from the `jwt.With*` parser options rather than from the claims type.*

### RS256 everywhere — done

Asymmetric signing, chosen deliberately:

- The verifier needs only the public key. Other services can verify without
  holding a forging key.
- A leaked public key is harmless; a leaked HMAC secret forges tokens.
- The RSA path already exists (system B) and the key files are already
  configured.

HS256 is removed. Rationale in
[ADR-009](../adr/009%20-%20Token%20Classes%20and%20Asymmetric%20Signing.md).

*Done in `0cf07d9`. There is no HMAC code, no HMAC secret, and no config key for
one anywhere in the tree: `internal/plugins/auth/jwt.go` is deleted, the
`jwt-auth` registration is gone from `internal/plugins/register.go`, the
`APP_JWT_SECRET` `BindEnv` and its placeholder check are gone from
`internal/config`, the `plugins.auth.jwt` block is gone from
`config/default.yaml`, and the `APP_JWT_*` entries are gone from `.env.example`.
The `AuthPlugin` interface itself still exists — `fido2-auth` implements it —
and its removal is Phase 3.6.*

### Key rotation via `kid` and JWKS — not started (Phase 5.5)

```go
token.Header["kid"] = s.activeKeyID
```

The verifier resolves the key by `kid` from a small key set, so old and new keys
coexist during rotation. Publish `/.well-known/jwks.json` for the public keys.
Rotation becomes: add new key → start signing with it → wait one access-token TTL
→ retire the old key.

### Claim mapping

| Claim | Source | Notes |
|---|---|---|
| `sub` | user ID | Stable, never an email |
| `iss` | `identity.issuer` | Validated on every parse |
| `aud` | `identity.audience` | Validated on every parse |
| `exp` | `iat` + class TTL | Required |
| `iat`, `nbf` | issue time | 30s leeway for clock skew |
| `jti` | random | Enables refresh-token replay detection |
| `cls` | token class | Required, validated |
| `scp` | granted scopes | Authorisation decisions |
| `name`, `email` | profile | ID tokens only — **not** in `iss` |

As shipped, `sub`, `iss`, `aud`, `exp`, `iat`, and `cls` all land as described
and `iss`/`aud`/`exp`/`cls` are all validated. Three rows are not yet true:
`nbf` is not set, `jti` is not set (it is only needed once refresh-token replay
detection lands, Phase 5.6), and scopes are carried as a single `scope` string
rather than a `scp` array. `name` and `email` appear on access tokens as well as
ID tokens — a deliberate convenience for the request logger and handlers, and
harmless now that they occupy their own claims rather than `iss`; the
overwrite-`Issuer` bug (A3) died with system A.

### Refresh token rotation and reuse detection

- Refresh tokens are opaque random values (≥256 bits, `crypto/rand`), stored as
  SHA-256 hashes — keeping the existing approach, which is sound.
- Every refresh **rotates**: the old token is invalidated as the new one is issued.
- Presenting an already-used refresh token indicates theft → revoke the entire
  token family for that user and log a security event.
- The existing `RefreshTokenStore`
  (`modules/identity/domain/repository.go`, implemented by
  `infrastructure/persistence/repository/refresh_token_store.go`) already has the
  shape for this — `Store`, `FindValid`, `RevokeAllForUser`. It needs the reuse
  check and a per-family identifier.

### TTLs

| Class | TTL | Source |
|---|---|---|
| Access | 15m | `identity.access_token_ttl` |
| Refresh | 168h (7d) | `identity.refresh_token_ttl` |
| MFA | 5m | `identity.mfa_temporary_ttl` |
| ID | 15m | `identity.id_token_ttl` *(new — replaces the hardcoded value)* |

All read from config; none hardcoded. Validation enforces access < refresh.

*Partly true as shipped. The config keys are under `auth.`, not `identity.` —
`auth.access_token_ttl`, `auth.refresh_token_ttl`, `auth.mfa_temporary_ttl` —
and `Config.Validate` does enforce access < refresh, plus each being positive
(`TestValidate_RejectsNonPositiveTTLs`). But only the access TTL is actually
read at issue time: the MFA and ID TTLs are still hardcoded in `jwt.go`, so
`auth.mfa_temporary_ttl` is validated and then ignored. See B3.*

---

## Migration path

1. ✅ `946c1c8` — Introduce `Class` and the unified `Claims`; add `cls` to newly
   issued tokens.
2. ✅ `e0da81e` — Add algorithm pinning + `iss`/`aud` validation to system B.
   Shipped as step 2 of the phase, before the deletion, exactly as advised.
3. ✅ `6d0c1b7` (Phase 1) / `0cf07d9` — Point the auth middleware at system B's
   service; delete `internal/plugins/auth/jwt.go`.
4. ⏳ Fix `IssueIDToken` to take a TTL; add `identity.id_token_ttl`. Extend it to
   `IssueMFAToken` and `auth.mfa_temporary_ttl`, which has the same defect.
5. ⏳ Add `kid` + JWKS — Phase 5.5.
6. ⏳ Add refresh reuse detection — Phase 5.6.
7. ◐ Delete system A entirely, including the `AuthPlugin` interface. The plugin
   is gone; the interface survives because `fido2-auth` implements it, and goes
   with the framework deletion in Phase 3.6.

Step 2 alone closed the highest-severity token finding, and did.

---

## Tests

`modules/identity/infrastructure/token/jwt_test.go` (`e0da81e`, `946c1c8`)
covers every rejection path ADR-009 names as a compliance requirement:

```
TestValidate_RejectsWrongAlgorithm          TestValidate_RejectsWrongAudience
TestValidate_RejectsNoneAlgorithm           TestValidate_RejectsExpired
TestValidate_RejectsWrongIssuer             TestValidate_RejectsMFATokenAsAccessToken
                                            TestValidate_AcceptsGenuineAccessToken
```

They need no infrastructure — a generated keypair and a clock — which is why
they were cheap to write and should stay comprehensive as the service grows.

---

## Related documents

- [ADR-009: Token Classes and Asymmetric Signing](../adr/009%20-%20Token%20Classes%20and%20Asymmetric%20Signing.md)
- [Middleware hardening](middleware-hardening.md)
- [Secret management](../03-configuration/secret-management.md)
