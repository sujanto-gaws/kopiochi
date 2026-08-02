# Repository Hygiene & Build Weight

**Status:** Proposed — see [ADR-011](../adr/011%20-%20Build%20Artifacts%20Excluded%20from%20Version%20Control.md)
**Date:** 2026-08-02

---

## Problem 1: compiled binaries are committed

```
$ git ls-files bin/
bin/kopiochi
bin/kopiochi-migrate
bin/kopiochi-migrate.exe
```

Largest blobs in history:

| Blob | Size |
|---|---|
| `bin/kopiochi` | 38.7 MB |
| `bin/kopiochi` (second version) | 38.7 MB |
| `bin/kopiochi-migrate` | 22.9 MB |
| `kopiochi.exe` (repo root) | 20.1 MB |

That is **~120 MB of binaries** in history, leaving `.git` at **58 MB** for a
project with ~8,000 lines of Go. Every clone downloads all of it, forever.
Git cannot delta-compress stripped Go binaries meaningfully, so each rebuild that
was committed added a full copy.

`.gitignore` has no `bin/` entry, so `make build` followed by `git add .`
re-commits a fresh 38 MB blob.

## Problem 2: `.gitignore` is wrapped in markdown fences

The file begins and ends with a literal ` ``` ` line:

```
```                      ← line 1: a pattern matching a file named ```
# Compiled and build artifacts
*.o
...
*.prof
```                      ← last line: same
```

It was pasted from a chat or document without stripping the fences. Those two
lines are meaningless patterns. The file is also missing:

- `bin/` — the 120 MB problem
- `keys/`, `*.pem` — the private key described in
  [secret management](../03-configuration/secret-management.md)
- `coverage.out`, `coverage.html` — produced by `make test-coverage`
- `*.exe` — the root `kopiochi.exe` precedent

Note it *does* ignore `.vscode/`, yet `.vscode/settings.json` is tracked and
shows as modified — it was committed before the rule existed, and `.gitignore`
does not apply to already-tracked files.

## Problem 3: CRLF everywhere, no `.gitattributes`

Every `.go` file uses CRLF line terminators. There is no `.gitattributes`.
Consequences:

- `gofmt -l .` lists 100% of files — the formatting signal is destroyed
  (see [testing strategy](testing-strategy.md)).
- `make fmt` would rewrite every file, producing an unreviewable diff.
- Any teammate on macOS or Linux generates whole-file diffs on save.

## Problem 4: stray files in the working tree

| File | Issue |
|---|---|
| `claude-agents_1.zip` (17 KB) | Untracked archive in the repository root |
| `.qwen/` | Tool directory, not ignored |
| `bin/kopiochi-migrate.exe` | Windows binary alongside its Linux twin |

## Problem 5: the server binary links code it never serves

`cmd/api` is 38 MB. It statically links:

| Dependency | Used by | Reachable? |
|---|---|---|
| `go-webauthn/webauthn`, `go-tpm`, `fxamacker/cbor` | FIDO2 plugin | **No** — cannot initialise |
| `pquerna/otp`, `boombuler/barcode` | identity MFA | **No** — module not wired |
| `swaggo/swag` v1.8.1, `http-swagger`, 4× `go-openapi/*` | swagger UI | Yes, but a docs concern |
| `pressly/goose/v3` | `cmd/migrate` | Not needed by the server |

154 modules in the graph. `swag` v1.8.1 dates from 2022 and pulls an old
`go-openapi` chain; the current `swag` is v2.

Two YAML libraries are present — `gopkg.in/yaml.v3` and `go.yaml.in/yaml/v3` —
because different dependencies chose different forks.

---

## Target

### Repaired `.gitignore`

```gitignore
# Build artifacts
bin/
dist/
*.exe
*.o
*.obj
*.out
*.test
*.prof

# Coverage
coverage/
coverage.out
coverage.html
htmlcov/
.coverage

# Secrets and keys
keys/
*.pem
*.key
.env
.env.*
!.env.example

# Dependencies
vendor/

# Logs and temp
*.log
*.tmp
*.swp

# Editors and tools
.vscode/
.idea/
.qwen/

# Archives
*.zip
```

No fences. Verify with `git check-ignore -v bin/kopiochi` after editing.

### `.gitattributes`

```gitattributes
* text=auto eol=lf

*.go     text eol=lf
*.sql    text eol=lf
*.yaml   text eol=lf
*.yml    text eol=lf
*.md     text eol=lf
Makefile text eol=lf
*.sh     text eol=lf

*.ps1    text eol=crlf

*.png binary
*.jpg binary
*.pem binary
```

Then normalise once:

```bash
git add --renormalize .
git commit -m "chore: normalise line endings to LF"
```

Do this as a **standalone commit** — it touches every file and must not be mixed
with logic changes.

### Purge binaries from history

```bash
git clone --mirror <url> kopiochi-mirror.git
cd kopiochi-mirror.git

git filter-repo \
  --path bin/ \
  --path kopiochi.exe \
  --path claude-agents_1.zip \
  --invert-paths

git push --force --all
git push --force --tags
```

⚠️ **This rewrites history.** It requires coordination:

1. Announce a freeze; everyone pushes outstanding work.
2. Take a backup mirror before rewriting.
3. Run the purge and force-push.
4. Everyone re-clones — `git pull` will not recover from a rewrite.
5. Combine with the secret purge from
   [secret management](../03-configuration/secret-management.md) so history is
   rewritten **once**, not twice.

Expected result: `.git` drops from 58 MB to roughly 5 MB.

Also run `git rm --cached .vscode/settings.json` so the existing ignore rule
finally applies.

### Untrack and regenerate swagger output

`docs/api/docs.go` (2,100 LOC), `swagger.json`, and `swagger.yaml` are generated
by `make swagger-docs`. Committing them creates review noise and merge conflicts.
Generate in CI and publish as an artifact; keep the generated files out of the
tree. If they must stay committed for the swagger UI to build, add a CI check
that regeneration produces no diff — a stale spec is worse than no spec.

### Shrink the binary

1. Delete the unreachable FIDO2 plugin and the extension frameworks; drop
   `go-webauthn`, `go-tpm`, and `fxamacker/cbor` from `go.mod`.
2. Keep MFA dependencies only if the MFA feature is actually being wired; the
   identity module needs `pquerna/otp` when it is.
3. Move swagger UI serving behind a build tag or serve the spec statically, so
   `swag` is a development dependency rather than a runtime one:

```go
//go:build swagger
```

4. Keep `goose` in `cmd/migrate` only — it should never link into `cmd/api`.
5. Strip symbols in release builds:

```make
LDFLAGS = -ldflags "-s -w -X .../internal/version.Version=$(VERSION)"
```

6. Run `go mod tidy` after the deletions and confirm the module count falls.

Track the result:

```make
size: ## Report binary size
	@go build $(LDFLAGS) -o /tmp/kopiochi ./cmd/api && ls -lh /tmp/kopiochi
```

### Dockerfile

A `Dockerfile` exists (708 bytes) but `docker-compose.yml` does not, while
`make docker-compose-up` references it and `make docker-run` requires a `.env`
that is also absent. Either add both files or remove the targets — a Makefile
target that cannot work is worse than no target.

The image must be built from a multi-stage build with a distroless or scratch
final stage, must not copy `keys/`, and must run as a non-root user.

### Prevention

- `.githooks/pre-commit` rejecting `*.pem`, `keys/`, `bin/`, `.env` — the
  `install-hooks` Makefile target already exists but `.githooks/` does not.
- CI job failing on any added file over 1 MB:

```bash
git diff --name-only origin/main...HEAD | while read -r f; do
  [ -f "$f" ] || continue
  size=$(wc -c < "$f")
  [ "$size" -gt 1048576 ] && { echo "ERROR: $f is $((size/1024))KB"; exit 1; }
done
```

---

## Sequencing

1. `.gitattributes` + renormalise (standalone commit) — unblocks `gofmt`.
2. Repaired `.gitignore`; `git rm --cached` the tracked artifacts.
3. `.githooks/` + CI size check — stop the bleeding before the rewrite.
4. **One coordinated history rewrite** removing binaries *and* secrets.
5. Dependency and binary-size reduction, after the dead code is deleted.

---

## Related documents

- [ADR-011: Build Artifacts Excluded from Version Control](../adr/011%20-%20Build%20Artifacts%20Excluded%20from%20Version%20Control.md)
- [Secret management](../03-configuration/secret-management.md)
- [Testing strategy](testing-strategy.md)
- [Extension framework](../01-modularity/extension-framework.md)
