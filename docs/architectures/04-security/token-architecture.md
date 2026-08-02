# Token Architecture

**Status:** Proposed — see [ADR-009](../adr/009%20-%20Token%20Classes%20and%20Asymmetric%20Signing.md)
**Date:** 2026-08-02

---

## Problem: two token systems, both flawed

| System | Location | Algorithm | Used by |
|---|---|---|---|
| A — plugin | `internal/plugins/auth/jwt.go` | HS256 (shared secret) | Live path (`main.go:101`) |
| B — identity | `extensions/identity/infrastructure/token/jwt.go` | RS256 (RSA keypair) | Dead code |

They issue different claim sets, validate differently, and disagree about what a
token means. Neither is complete.

---

## Defects in system A (the live one)

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

## Defects in system B (the dead one)

System B is closer to correct — it **does** pin the algorithm
(`jwt.go:75-77`):

```go
if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
    return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
}
```

Remaining gaps:

### B1 — No issuer or audience validation

`Validate` (line 73) checks the signature and expiry but never `iss` or `aud`.
`IssueIDToken` sets `aud` to the client ID (line 51) yet nothing verifies it.

### B2 — Token classes are indistinguishable at the validation boundary

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

### B3 — `IssueIDToken` ignores configured TTL

Line 54 hardcodes `15 * time.Minute` while `IssueAccessToken` takes a `ttl`
parameter. `auth.access_token_ttl` in config is not honoured for ID tokens.

### B4 — Refresh tokens hashed with unsalted SHA-256

`application/auth.go:67` and `application/helpers.go:14`:

```go
hash := utils.StringHashHex(refreshToken)   // plain SHA-256, no salt
```

This is **acceptable** — provided the refresh token is high-entropy random
(≥128 bits from `crypto/rand`), unsalted SHA-256 resists brute force because
there is nothing to guess. It must be verified that
`internal/utils/generator.go` uses `crypto/rand` and not `math/rand`, and the
constraint must be documented at the call site, because the same helper is one
copy-paste away from being applied to something low-entropy.

### B5 — No key rotation support

`NewJWTService` loads exactly one keypair. Tokens carry no `kid` header, so
rotating the signing key invalidates every outstanding token at once. There is no
JWKS endpoint.

---

## Target design

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

### RS256 everywhere

Asymmetric signing, chosen deliberately:

- The verifier needs only the public key. Other services can verify without
  holding a forging key.
- A leaked public key is harmless; a leaked HMAC secret forges tokens.
- The RSA path already exists (system B) and the key files are already
  configured.

HS256 is removed. Rationale in
[ADR-009](../adr/009%20-%20Token%20Classes%20and%20Asymmetric%20Signing.md).

### Key rotation via `kid` and JWKS

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

### Refresh token rotation and reuse detection

- Refresh tokens are opaque random values (≥256 bits, `crypto/rand`), stored as
  SHA-256 hashes — keeping the existing approach, which is sound.
- Every refresh **rotates**: the old token is invalidated as the new one is issued.
- Presenting an already-used refresh token indicates theft → revoke the entire
  token family for that user and log a security event.
- The existing `refresh_token` store and `user_token_repository` already have the
  shape for this; they need the reuse check.

### TTLs

| Class | TTL | Source |
|---|---|---|
| Access | 15m | `identity.access_token_ttl` |
| Refresh | 168h (7d) | `identity.refresh_token_ttl` |
| MFA | 5m | `identity.mfa_temporary_ttl` |
| ID | 15m | `identity.id_token_ttl` *(new — replaces the hardcoded value)* |

All read from config; none hardcoded. Validation enforces access < refresh.

---

## Migration path

1. Introduce `Class` and the unified `Claims`; add `cls` to newly issued tokens.
2. Add algorithm pinning + `iss`/`aud` validation to system B — **do this first**,
   it is a small, isolated, high-value change.
3. Point the auth middleware at system B's service; delete
   `internal/plugins/auth/jwt.go`.
4. Fix `IssueIDToken` to take a TTL; add `identity.id_token_ttl`.
5. Add `kid` + JWKS.
6. Add refresh reuse detection.
7. Delete system A entirely, including the `AuthPlugin` interface.

Step 2 alone closes the highest-severity token finding and can ship independently.

---

## Related documents

- [ADR-009: Token Classes and Asymmetric Signing](../adr/009%20-%20Token%20Classes%20and%20Asymmetric%20Signing.md)
- [Middleware hardening](middleware-hardening.md)
- [Secret management](../03-configuration/secret-management.md)
