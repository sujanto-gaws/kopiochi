# ADR-008: Configuration Precedence and Secret Handling

## Status
**Proposed** – *Date: 2026-08-02*

## Context

`CLAUDE.md` states the rule plainly:

> No secrets in code or images — load through Viper (env/secret store).

The repository violates it. `config/default.yaml` — a tracked file — contains a
live database password (`"gaws"`, line 14) and a JWT signing secret
(`"your-secret-key-change-in-production"`, line 51). `keys/private.pem` sits in
the working tree and is not covered by `.gitignore`, so it is one `git add .`
away from entering history permanently.

The cause is mechanical rather than careless. `internal/config/config.go` sets up
environment binding:

```go
v.SetEnvPrefix("APP")
v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
v.AutomaticEnv()
```

and then calls `v.Unmarshal(&cfg)`. **`AutomaticEnv` does not participate in
`Unmarshal`.** Viper builds the unmarshal source from `AllSettings()`, which
enumerates keys known from defaults, the config file, and explicit `BindEnv`
calls. `AutomaticEnv` only affects direct `v.Get()` lookups; it cannot enumerate
the environment.

The defaults registered at lines 84-111 cover `db.host`, `db.port`, `db.sslmode`,
`db.max_conns`, and `db.min_conns` — but **not** `db.user`, `db.password`, or
`db.name`. Those keys are therefore unknown to Viper, so `APP_DB_PASSWORD` is
silently ignored. The YAML file was the only path that worked.

Further gaps:

- **No validation.** `Load` returns as soon as decoding succeeds. Nothing catches
  that `server.request_timeout` (60s) exceeds `server.write_timeout` (30s) — a
  live misconfiguration that truncates long responses.
- **No placeholder detection.** The shipped JWT secret is a functioning HMAC key;
  deploying it makes every token forgeable by anyone who has read the repository.
- **Unchecked type assertions** in plugin config silently fall back to defaults
  (`ratelimit.go:33`), so a YAML type error changes behaviour with no diagnostic.
- **Secrets can reach logs.** `Config.DB.Password` is a plain `string`; a single
  `log.Debug().Interface("config", cfg)` prints it.

## Decision

1. **Precedence is `flags > environment > file > defaults`,** documented and
   tested.
2. **Every configuration key gets an explicit default registration,** even when
   the default is the zero value, so environment binding functions.
3. **Secrets are additionally bound with `BindEnv`** and never appear in any
   committed file.
4. **`Config.Validate()` runs during `Load` and fails closed** on missing
   required values, timeout inversions, and TTL ordering violations.
5. **Known placeholders are rejected at startup** — an application carrying
   `"gaws"` or `"your-secret-key-change-in-production"` cannot boot.
6. **Secrets use a redacting type** (`secret.String`) whose `String`, `GoString`,
   and `MarshalJSON` return `[REDACTED]`; the real value requires an explicit
   `.Reveal()`.
7. **Strict decoding:** `ErrorUnused: true` so a misspelled key is an error, and
   `WeaklyTypedInput: false` so `"500"` is not silently accepted as `500`.
8. **Typed config per module** replaces `map[string]interface{}`.
9. **Key material never lives in the repository.** Development keys are generated
   locally into a git-ignored `keys/`; deployed environments mount them from a
   secret store.

## Consequences

### Positive
- **`APP_DB_PASSWORD` works**, removing the reason secrets were put in YAML.
- **Leaked defaults cannot be deployed** — the placeholder check fails the boot.
- **Misconfiguration surfaces at startup** with a clear message, not as a
  puzzling failure under load.
- **Typos become errors** rather than silently-defaulted values.
- **Accidental secret logging is structurally difficult.**
- **`config/default.yaml` becomes safe to read publicly.**

### Negative
- **Local development needs environment setup.** Mitigated by a committed
  `config/example.env` documenting every variable and a git-ignored
  `config/local.yaml` for non-secret overrides.
- **`Validate()` is code that must be maintained** as config grows.
- **`.Reveal()` at every use site** is mildly verbose — deliberately, since each
  call is a greppable review point.
- **Rotation is required**, not optional: the exposed values must be treated as
  compromised, which means a coordinated credential change and a forced
  re-authentication when the JWT secret rotates.

## Alternatives Considered

| Alternative | Reason for rejection |
|---|---|
| **Keep secrets in git-ignored YAML** | An ignored file is one `git add -f` or one careless `.gitignore` edit from exposure, and it does not solve distribution to deployed environments. |
| **`.env` files loaded by the app** | Better than committed YAML but still a plaintext file on disk; environment or a secret store is the standard boundary. |
| **Direct secret-store integration (Vault, AWS Secrets Manager)** | The right long-term answer, but it adds a dependency and a failure mode. The environment is the interface; the platform can inject from any store. |
| **Keep `AutomaticEnv` and use `v.Get()` everywhere instead of `Unmarshal`** | Loses the typed config struct and scatters string keys through the codebase. |
| **Only fix the missing defaults** | Restores env binding but leaves no validation, no placeholder detection, and no redaction. |

## Implementation Plan

1. **Rotate first.** Change the database password everywhere `"gaws"` was used;
   rotate the JWT secret; regenerate the RSA keypair if it has left the developer
   machine.
2. Register defaults for every key; add `BindEnv` for each secret.
3. Add `secret.String` and change secret-bearing fields to use it.
4. Implement `Validate()`, including placeholder rejection and the timeout
   inversion check.
5. Enable strict decoding; fix any keys it rejects.
6. Strip secrets from `config/default.yaml`; add `config/example.env`.
7. Repair `.gitignore` (`keys/`, `*.pem`, `.env*`); add `.githooks/pre-commit`.
8. Add tests: env override, validation failures, unknown-key rejection.
9. Purge history in the coordinated rewrite described in
   [ADR-011](011%20-%20Build%20Artifacts%20Excluded%20from%20Version%20Control.md).

## Compliance / Enforcement

- `gitleaks` runs in CI over both the working tree and history.
- `.githooks/pre-commit` blocks `*.pem`, `keys/`, `.env`, and `bin/`.
- A test asserts `config/default.yaml` contains no non-empty `password` or
  `secret` value.
- Review rejects any new `string`-typed secret field; use `secret.String`.
- Every new config key must have a registered default and appear in
  `config/example.env`.

## Related ADRs
- [ADR-009: Token Classes and Asymmetric Signing](009%20-%20Token%20Classes%20and%20Asymmetric%20Signing.md)
- [ADR-011: Build Artifacts Excluded from Version Control](011%20-%20Build%20Artifacts%20Excluded%20from%20Version%20Control.md)
- [ADR-004: Consolidate on a Single Extension Framework](004%20-%20Consolidate%20on%20a%20Single%20Extension%20Framework.md)

## Related Documents
- [Configuration model](../03-configuration/configuration-model.md)
- [Secret management](../03-configuration/secret-management.md)

---

**This ADR serves as a binding architectural decision for the project.**
