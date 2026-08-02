# Secret Management

**Status:** Proposed — see [ADR-008](../adr/008%20-%20Configuration%20Precedence%20and%20Secret%20Handling.md)
**Date:** 2026-08-02
**Severity:** High — live credentials and a private key are in the working tree.

---

## Problem

### 1. Credentials committed in `config/default.yaml`

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

`config/default.yaml` is tracked. Both values are in git history. This directly
violates the project's own rule in `CLAUDE.md`:

> No secrets in code or images — load through Viper (env/secret store).

The root cause is mechanical, not careless: as documented in
[configuration model](configuration-model.md), `APP_DB_PASSWORD` **does not
work**, because `db.password` has no registered default and `AutomaticEnv` does
not feed `Unmarshal`. The file was the only path that functioned.

### 2. RSA private key present and not ignored

```
keys/private.pem   1732 bytes   PKCS#8 RSA private key
keys/public.pem     460 bytes
```

`keys/` appears nowhere in `.gitignore`. The key is currently untracked, so it is
**one `git add .` away from entering history permanently**. `config/default.yaml`
points `auth.private_key_path` at it, so it is a real signing key, not a sample.

### 3. `.gitignore` is malformed

The file's first and last lines are literal markdown code fences:

```
```
# Compiled and build artifacts
*.o
...
```
```

A stray ` ``` ` is treated as a filename pattern, so those two lines protect
nothing. The file also lacks `bin/`, `keys/`, and `*.pem` — see
[repository hygiene](../06-quality/repository-hygiene.md).

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
  git-ignored. Provide `make keys-dev`:

```make
keys-dev: ## Generate a local-only RSA keypair for development
	@mkdir -p keys
	@openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out keys/private.pem
	@openssl rsa -pubout -in keys/private.pem -out keys/public.pem
	@chmod 600 keys/private.pem
	@echo "Development keys written to keys/ (git-ignored, DO NOT COMMIT)"
```

- Deployed environments: keys are mounted from the platform secret store at a
  path given by `APP_IDENTITY_PRIVATE_KEY_PATH`, or supplied PEM-inline via
  `APP_IDENTITY_PRIVATE_KEY`.
- Startup verifies the private key file's permissions are not group/world
  readable and fails otherwise.

### Prevention in CI

1. **Secret scanning** — `gitleaks detect --no-git` on the working tree and
   `gitleaks detect` over history, as a required check.
2. **Pre-commit hook** — the `Makefile` already has an `install-hooks` target
   that points `core.hooksPath` at `.githooks/`, but **`.githooks/` does not
   exist**. Create it:

```sh
# .githooks/pre-commit
#!/bin/sh
if git diff --cached --name-only | grep -Eq '\.pem$|^keys/|^bin/|\.env$'; then
    echo "ERROR: refusing to commit secrets or build artifacts (*.pem, keys/, bin/, .env)"
    exit 1
fi
```

3. **`.gitignore` repaired** — remove the markdown fences and add:

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

1. **Rotate the database password** on every environment where `"gaws"` was ever
   used.
2. **Rotate the JWT secret.** Any token signed with the placeholder is forgeable;
   rotating invalidates outstanding tokens, so schedule it with a forced
   re-authentication.
3. **Treat `keys/private.pem` as burned** if it has ever been shared or copied
   outside the developer machine; generate a fresh keypair.
4. **Purge history** in the same pass as the binary purge described in
   [repository hygiene](../06-quality/repository-hygiene.md) — one coordinated
   force-push rather than two.
5. **Then** land the code changes: `BindEnv`, placeholder validation,
   `secret.String`, repaired `.gitignore`, `.githooks/`.

Order matters: rotate first, so the window where a leaked-but-live credential
sits in history is as short as possible.

---

## Related documents

- [ADR-008: Configuration Precedence and Secret Handling](../adr/008%20-%20Configuration%20Precedence%20and%20Secret%20Handling.md)
- [Configuration model](configuration-model.md)
- [Repository hygiene](../06-quality/repository-hygiene.md)
- [Token architecture](../04-security/token-architecture.md)
