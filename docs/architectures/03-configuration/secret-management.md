# Secret Management

**Status:** Partially implemented — see [ADR-008](../adr/008%20-%20Configuration%20Precedence%20and%20Secret%20Handling.md)
**Date:** 2026-08-02
**Last verified:** 2026-08-02, after Phase 0
**Severity:** Medium — the working tree is clean (Phase 0), but every value below
remains in git history, nothing rejects a placeholder secret, and rotation of the
exposed credentials is an out-of-repo action that cannot be verified from here.

Phase 0 status at a glance:

| Problem | State |
|---|---|
| 1. Credentials in `config/default.yaml` | **Fixed** in `b74b358` — still in history |
| 2. RSA private key present and not ignored | **Fixed** in `8652534` — still in history |
| 3. `.gitignore` malformed | **Fixed** in `4c72a83` |
| 4. Nothing detects a placeholder | **Open** — Phase 2.9 |
| 5. Secrets can reach the logs | **Open** — Phase 2.9 |
| Rotation of the exposed credentials | **Unverifiable from the repo** — see below |

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

### 4. Nothing detects a placeholder

`"your-secret-key-change-in-production"` is a functioning HMAC key as far as the
code is concerned. Ship it and every token in production is forgeable by anyone
who has read the repository.

### 5. Secrets can reach the logs

`internal/config/config.go` produces a `Config` whose `DB.Password` is a plain
`string`. Any `log.Info().Interface("config", cfg)` — a natural thing to add
while debugging — prints the password. Nothing prevents it.

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

### Redacted secret type

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
   the same reason.
3. ⚠️ **Treat `keys/private.pem` as burned** if it has ever been shared or copied
   outside the developer machine; generate a fresh keypair with `make keys`.
   **Outstanding.**
4. ⏳ **Purge history** in the same pass as the binary purge described in
   [repository hygiene](../06-quality/repository-hygiene.md) — one coordinated
   force-push rather than two. Scheduled as Phase 5.1; not started.
5. **Then** land the code changes. Partially done: `BindEnv` (`b74b358`),
   repaired `.gitignore` (`4c72a83`), and `.githooks/` (`9c302ad`) have shipped.
   Placeholder validation and `secret.String` remain — Phase 2.9.

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
