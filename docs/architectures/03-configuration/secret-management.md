# Secret Management

**Status:** Accepted — partially implemented — see [ADR-008](../adr/008%20-%20Configuration%20Precedence%20and%20Secret%20Handling.md)
**Date:** 2026-08-02
**Last verified:** 2026-08-02, after Phase 2
**Severity:** Medium — every code-level problem below is now closed, but the
exposed values remain in git history and rotation is an out-of-repo action that
cannot be verified from here.

Status at a glance, after Phase 0 and Phase 2.9:

| Problem | State |
|---|---|
| 1. Credentials in `config/default.yaml` | **Fixed** in `b74b358` — still in history |
| 2. RSA private key present and not ignored | **Fixed** in `8652534` — still in history |
| 3. `.gitignore` malformed | **Fixed** in `4c72a83` |
| 4. Nothing detects a placeholder | **Fixed** in `acc057d` |
| 5. Secrets can reach the logs | **Fixed** in `acc057d` |
| Rotation of the exposed credentials | **Unverifiable from the repo** — see below |

One item on this page was closed by deletion rather than by fixing: the JWT
signing secret. Phase 2.6 (`0cf07d9`) removed the HS256 plugin, so there is no
HMAC secret in the configuration surface at all — no `plugins.auth.jwt.config.secret`
key, no `APP_JWT_SECRET` binding, no `.env.example` entry. The **rotation**
obligation below is unaffected: `"your-secret-key-change-in-production"` is still
in git history, and any token ever signed with it is still forgeable by anyone
who reads that history. Deleting the code that used it does not un-expose it.

---

## Problem

### 1 (fixed). Credentials committed in `config/default.yaml`

*Removed in `b74b358`. `config/default.yaml:15` and `:50` are now comments
pointing at `APP_DB_PASSWORD` and `APP_JWT_SECRET`, bound explicitly by
`internal/config/config.go:88-93`. The values remain in git history until the
Phase 5.1 rewrite, so they are still to be treated as compromised.*

Historically:

```yaml
db:
  user: "postgres"
  password: "gaws"                                    # line 14

plugins:
  auth:
    jwt:
      config:
        secret: "your-secret-key-change-in-production"  # line 51
```

`config/default.yaml` is tracked, so both values are in git history. This
directly violated the project's own rule in `CLAUDE.md`:

> No secrets in code or images — load through Viper (env/secret store).

The root cause was mechanical, not careless: as documented in
[configuration model](configuration-model.md), `APP_DB_PASSWORD` **did not
work**, because `db.password` had no registered default and `AutomaticEnv` does
not feed `Unmarshal`. The file was the only path that functioned. `b74b358` fixed
the loader and the file together — the explicit `BindEnv` calls had to land first
for the removal to be safe.

### 2 (fixed). RSA private key present and not ignored

```
keys/private.pem   1732 bytes   RSA private key
keys/public.pem     460 bytes
```

`keys/` appeared nowhere in `.gitignore`, so the key was **one `git add .` away
from entering history permanently**. `config/default.yaml` points
`auth.private_key_path` at it, so it is a real signing key, not a sample.

*Fixed in `8652534` (untracked, plus a reproducible `make keys` target) and
`4c72a83` (`.gitignore:36-37` now carries `keys/` and `*.pem`). Note the key is
**PKCS#1**, not PKCS#8 as originally recorded here — `modules/identity/`
`infrastructure/token/jwt.go` parses it with `x509.ParsePKCS1PrivateKey`, which is
why `make keys` uses `openssl genrsa -traditional`.*

### 3 (fixed). `.gitignore` was malformed

The file's first and last lines were literal markdown code fences:

```
```
# Compiled and build artifacts
*.o
...
```
```

A stray ` ``` ` is treated as a filename pattern, so those two lines protected
nothing. The file also lacked `bin/`, `keys/`, and `*.pem`.

*Fixed in `4c72a83`; see [repository hygiene](../06-quality/repository-hygiene.md).*

### 4 (fixed). Nothing detects a placeholder

`"your-secret-key-change-in-production"` is a functioning HMAC key as far as the
code is concerned. Ship it and every token in production is forgeable by anyone
who has read the repository.

*Fixed in `acc057d`. `isPlaceholderSecret` (`config.go:200-208`) rejects three
things: the empty string, anything with a `CHANGEME` prefix (case-insensitive),
and the two values that were actually committed to this repository — `"postgres"`
and `"your-secret-key-change-in-production"`. `Config.Validate` applies it to
`db.password`, so a deployment carrying a leaked default cannot boot. Test:
`TestLoad_RejectsPlaceholderSecrets`.*

*Two design points worth keeping. The prefix check is deliberately broader than
an exact-match blacklist, because `.env.example` ships `CHANGEME_SET_YOUR_OWN_DB_PASSWORD`
and someone editing the trailing wording without generating a real secret should
still be caught. And the check is applied **only** to fields that must hold a
real secret — never to `db.user`, where `"postgres"` is an entirely ordinary
value that would otherwise be rejected.*

*The JWT-secret half of this problem was closed differently: `0cf07d9` deleted
the secret, so its placeholder check went with it.*

### 5 (fixed). Secrets can reach the logs

`internal/config/config.go` produces a `Config` whose `DB.Password` is a plain
`string`. Any `log.Info().Interface("config", cfg)` — a natural thing to add
while debugging — prints the password. Nothing prevents it.

*Fixed in `acc057d`. `internal/platform/secret` provides `secret.String`, and
`Config.DB.Password` is now that type instead of `string`. `String()`,
`GoString()`, and `MarshalJSON()` all return `[REDACTED]`, so `%v`, `%s`, `%#v`,
`Interface("config", cfg)`, and any JSON config dump print the placeholder
rather than the value. `Reveal()` is the only way out and is greppable in
review. `UnmarshalJSON` is provided so the type can still be populated from a
legitimate JSON source — only marshalling is one-way.*

*`mapstructure` decodes into it with no custom hook, because the underlying kind
is `string` and reflect's `SetString` path handles it; that is asserted rather
than assumed, in `TestString_MapstructureDecode`. `TestString_RedactsInsideAStruct`
covers the case this was written for — the whole `Config` being formatted at
once.*

---

## Target design

### Classification

| Class | Examples | Where it lives |
|---|---|---|
| Public config | ports, timeouts, log level, pool sizes | `config/default.yaml`, committed |
| Environment-specific | hostnames, origins, DB name | env vars or `config/local.yaml` (ignored) |
| **Secret** | DB password, JWT/HMAC secret, private keys | env var or secret store — **never a file in the repo** |

### Secrets never come from committed files

```go
// internal/config/config.go
_ = v.BindEnv("db.password", "APP_DB_PASSWORD")
_ = v.BindEnv("identity.jwt_secret", "APP_IDENTITY_JWT_SECRET")
```

*As shipped there is exactly one secret to bind — `db.password` — and its error
is checked rather than discarded. The second line never became real: RS256 needs
no shared secret, so the only key material is the PEM files, handled below.*

`config/default.yaml` after cleanup:

```yaml
db:
  host: "localhost"
  port: 5432
  user: ""          # APP_DB_USER
  password: ""      # APP_DB_PASSWORD — never set here
  name: ""          # APP_DB_NAME
  sslmode: "require"
```

### Fail closed on missing or placeholder secrets

```go
var placeholders = map[string]bool{
    "":                                     true,
    "changeme":                             true,
    "gaws":                                 true,
    "your-secret-key-change-in-production": true,
    "postgres":                             true,
}

func (c *Config) validateSecrets() error {
    var errs []error

    if placeholders[c.DB.Password] {
        errs = append(errs, errors.New(
            "db.password is empty or a known placeholder; set APP_DB_PASSWORD"))
    }
    if s := c.Identity.JWTSecret; s != "" {
        if placeholders[s] {
            errs = append(errs, errors.New(
                "identity.jwt_secret is a known placeholder; set APP_IDENTITY_JWT_SECRET"))
        }
        if len(s) < 32 {
            errs = append(errs, errors.New("identity.jwt_secret must be at least 32 bytes"))
        }
    }
    return errors.Join(errs...)
}
```

A deployment carrying a leaked default now **cannot start**.

*Shipped in `acc057d`, with `"changeme"` generalised to a `CHANGEME` **prefix**
match. The `identity.jwt_secret` branch — placeholder plus minimum length — has
no subject after `0cf07d9` and did not ship.*

> **`"gaws"` is not in the shipped list.** `legacyPlaceholderSecrets`
> (`config.go:190-193`) holds `"postgres"` and
> `"your-secret-key-change-in-production"`; the sketch above also listed
> `"gaws"`, which is the DB password that was actually committed. A deployment
> still using it therefore boots without complaint. Whether to add it is a real
> trade-off — it is a short, plausible value that someone may have chosen
> legitimately, so an exact-match rule risks refusing a valid credential — but
> the gap should be a decision on the record rather than an omission. Rotation
> (below) closes it either way, and is the thing that actually matters.

### Redacted secret type — shipped

Make leaking a secret through logs or JSON structurally difficult:

```go
// internal/platform/secret/secret.go
package secret

type String string

func (s String) String() string                 { return "[REDACTED]" }
func (s String) GoString() string               { return "[REDACTED]" }
func (s String) MarshalJSON() ([]byte, error)   { return []byte(`"[REDACTED]"`), nil }
func (s String) Reveal() string                 { return string(s) }
```

```go
type DB struct {
    Password secret.String `mapstructure:"password"`
}
```

`log.Info().Interface("config", cfg)` prints `[REDACTED]`. Only an explicit
`.Reveal()` — greppable in review — exposes the value.

### Key material handling

- Keys are **not** stored in the repository, in any branch, ever.
- Local development: generate per-developer keys into `keys/`, which is
  git-ignored. Shipped in `8652534` as `make keys` (`Makefile:83-98`), which also
  refuses to overwrite an existing keypair rather than silently invalidating live
  tokens:

```make
keys: ## Generate a fresh RSA keypair into keys/ for JWT signing
	@mkdir -p $(KEYS_DIR)
	# -traditional forces PKCS#1 ("BEGIN RSA PRIVATE KEY"), which is what
	# x509.ParsePKCS1PrivateKey reads. OpenSSL 3.x defaults to PKCS#8.
	openssl genrsa -traditional -out $(KEYS_DIR)/private.pem 2048
	openssl rsa -in $(KEYS_DIR)/private.pem -pubout -out $(KEYS_DIR)/public.pem
```

  > This document originally proposed `make keys-dev` using `openssl genpkey`,
  > which emits PKCS#8. That recipe produces a key the application cannot parse.
  > Use `make keys`.

- Deployed environments: keys are mounted from the platform secret store at a
  path given by `APP_IDENTITY_PRIVATE_KEY_PATH`, or supplied PEM-inline via
  `APP_IDENTITY_PRIVATE_KEY`.
- Startup verifies the private key file's permissions are not group/world
  readable and fails otherwise.

### Prevention in CI

1. **Secret scanning** — `gitleaks detect --no-git` on the working tree and
   `gitleaks detect` over history, as a required check.
2. **Pre-commit hook** — done. The `Makefile`'s `install-hooks` target points
   `core.hooksPath` at `.githooks/`, and `.githooks/pre-commit` was added in
   `9c302ad` (marked executable in the index by `1d3379e`):

```sh
# .githooks/pre-commit
#!/bin/sh
if git diff --cached --name-only | grep -Eq '\.pem$|^keys/|^bin/|\.env$'; then
    echo "ERROR: refusing to commit secrets or build artifacts (*.pem, keys/, bin/, .env)"
    exit 1
fi
```

3. **`.gitignore` repaired** — done in `4c72a83`. The fences were removed and the
   file now carries:

```gitignore
# Secrets and keys
keys/
*.pem
*.key
.env
.env.*
!.env.example

# Build artifacts
bin/
*.exe
```

---

## Remediation for what is already exposed

The committed values must be treated as compromised — rotation, not deletion, is
the fix. Deleting them from the file leaves them in history.

1. ⚠️ **Rotate the database password** on every environment where `"gaws"` was
   ever used. **Outstanding.** This happens outside the repository, so no commit
   can demonstrate it; it must be confirmed by whoever owns the environments.
2. ⚠️ **Rotate the JWT secret.** Any token signed with the placeholder is
   forgeable; rotating invalidates outstanding tokens, so schedule it with a
   forced re-authentication. **Outstanding**, and unverifiable from the repo for
   the same reason. *Note that `0cf07d9` deleting the HS256 plugin does not
   discharge this: it means nothing in the current code will accept such a token,
   which is a good outcome, but any deployment still running a pre-`0cf07d9`
   build accepts them, and the value is still in history.*
3. ⚠️ **Treat `keys/private.pem` as burned** if it has ever been shared or copied
   outside the developer machine; generate a fresh keypair with `make keys`.
   **Outstanding.**
4. ⏳ **Purge history** in the same pass as the binary purge described in
   [repository hygiene](../06-quality/repository-hygiene.md) — one coordinated
   force-push rather than two. Scheduled as Phase 5.1; not started.
5. ✅ **Then** land the code changes. **Done.** `BindEnv` (`b74b358`, extended in
   `acc057d`), repaired `.gitignore` (`4c72a83`), `.githooks/` (`9c302ad`), and
   — in Phase 2.9 (`acc057d`) — placeholder validation, `secret.String`, and
   `Config.Validate()` called from `Load`.

Order matters: rotate first, so the window where a leaked-but-live credential
sits in history is as short as possible. Steps 1–3 are the ones still open, and
they are the ones that actually close the exposure — removing the values from the
working tree (done) does not.

---

## Related documents

- [ADR-008: Configuration Precedence and Secret Handling](../adr/008%20-%20Configuration%20Precedence%20and%20Secret%20Handling.md)
- [Configuration model](configuration-model.md)
- [Repository hygiene](../06-quality/repository-hygiene.md)
- [Token architecture](../04-security/token-architecture.md)
