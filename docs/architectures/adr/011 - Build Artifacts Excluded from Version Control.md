# ADR-011: Build Artifacts Excluded from Version Control

## Status
**Proposed** – *Date: 2026-08-02*

## Context

Compiled binaries are tracked in this repository:

```
$ git ls-files bin/
bin/kopiochi
bin/kopiochi-migrate
bin/kopiochi-migrate.exe
```

The largest blobs in history are all binaries:

| Blob | Size |
|---|---|
| `bin/kopiochi` | 38.7 MB |
| `bin/kopiochi` (second version) | 38.7 MB |
| `bin/kopiochi-migrate` | 22.9 MB |
| `kopiochi.exe` (repo root) | 20.1 MB |

That is roughly **120 MB of binaries** permanently in history, leaving `.git` at
**58 MB** for a project containing about 8,000 lines of Go. Stripped Go binaries
do not delta-compress meaningfully, so every committed rebuild added a full copy.
Every clone, CI checkout, and fork pays that cost forever.

The controls that should have prevented this are broken or missing:

- **`.gitignore` has no `bin/` entry.** `make build` writes to `bin/`, so
  `git add .` re-commits a fresh 38 MB blob.
- **`.gitignore` is wrapped in literal markdown ` ``` ` fences** — its first and
  last lines are meaningless patterns, evidence it was pasted from a document
  without cleanup.
- It also omits `keys/`, `*.pem` (see
  [ADR-008](008%20-%20Configuration%20Precedence%20and%20Secret%20Handling.md)),
  `coverage.out`/`coverage.html` (produced by `make test-coverage`), and `*.exe`.
- **`make install-hooks` points `core.hooksPath` at `.githooks/`, but
  `.githooks/` does not exist**, so the hook mechanism is inert.
- Other untracked clutter sits in the root: `claude-agents_1.zip`, `.qwen/`.
- `.vscode/settings.json` is tracked and shows as modified even though
  `.gitignore` lists `.vscode/` — ignore rules do not apply to already-tracked
  files.

Generated documentation is also committed: `docs/api/docs.go` (2,100 LOC),
`swagger.json`, and `swagger.yaml` are all products of `make swagger-docs`.

## Decision

1. **No build artifact is tracked.** `bin/`, `dist/`, `*.exe`, `*.test`,
   `*.prof`, `coverage.*` are ignored.
2. **`.gitignore` is repaired** — fences removed, artifact/secret/tooling
   patterns added.
3. **Tracked artifacts are untracked** with `git rm --cached`, including
   `.vscode/settings.json`.
4. **History is rewritten once** with `git filter-repo` to purge the binary
   blobs, coordinated with the secret purge from ADR-008 so the team re-clones a
   single time.
5. **`.gitattributes` normalises line endings** (`* text=auto eol=lf`), because
   the repository is currently entirely CRLF with no `.gitattributes`, which
   makes `gofmt -l` report 100% of files and destroys the formatting signal.
6. **`.githooks/pre-commit` is created** to reject `*.pem`, `keys/`, `bin/`, and
   `.env`, making the existing `install-hooks` target functional.
7. **CI rejects any added file over 1 MB.**
8. **Generated swagger output is regenerated in CI,** not hand-committed; if it
   must remain tracked, CI verifies regeneration produces no diff.
9. **Release builds strip symbols** (`-ldflags "-s -w"`), and the server binary
   links only what it serves.

## Consequences

### Positive
- **`.git` drops from ~58 MB to roughly 5 MB.** Clones and CI checkouts get
  dramatically faster.
- **Accidental re-commits are blocked** at three layers: ignore file, pre-commit
  hook, CI size check.
- **`gofmt` becomes meaningful** once line endings are normalised — currently the
  only automated quality signal the project has, and it is pure noise.
- **The secret purge and binary purge share one history rewrite** instead of two.
- **Smaller release binaries** from stripping and from dropping dependencies that
  serve unreachable code.

### Negative
- **History rewriting is disruptive.** Every commit SHA changes; open pull
  requests must be recreated; everyone must re-clone (`git pull` cannot recover
  from a rewrite). This requires an announced freeze and a backup mirror.
- **The line-ending normalisation commit touches every file.** It must be a
  standalone commit, and in-flight branches will need an immediate rebase.
- **Anyone who relied on `git pull` for a prebuilt binary** must now run
  `make build` or fetch a CI artifact. This is the correct workflow regardless.
- **Untracking swagger output** means the spec is not browsable directly in the
  repository; CI publishes it as an artifact instead.

## Alternatives Considered

| Alternative | Reason for rejection |
|---|---|
| **Ignore `bin/` going forward, leave history alone** | Every clone still downloads 120 MB forever; the cost is permanent and grows with each fork. |
| **Git LFS for binaries** | Solves size but not the underlying question — build outputs simply do not belong in source control, and LFS adds server-side storage cost and a client requirement. |
| **`git gc --aggressive` / repack** | Cannot remove reachable objects; the blobs remain. |
| **BFG Repo-Cleaner instead of `git filter-repo`** | Both work; `git filter-repo` is the currently recommended tool and handles path and content filters in one pass. |
| **Keep binaries for reproducible deploys** | Deploy artifacts belong in a registry or release page with checksums, not in the source repository. |

## Implementation Plan

1. Add `.gitattributes`; run `git add --renormalize .` as a **standalone commit**.
2. Repair `.gitignore`.
3. `git rm --cached -r bin/ .vscode/settings.json`; delete
   `claude-agents_1.zip`; commit.
4. Create `.githooks/pre-commit`; document `make install-hooks` in the README.
5. Add the CI file-size check.
6. **Coordinated rewrite** (with ADR-008's secret purge): announce a freeze, take
   a backup mirror, run `git filter-repo --path bin/ --path kopiochi.exe --path
   claude-agents_1.zip --invert-paths`, force-push, have everyone re-clone.
7. Untrack `docs/api/*`; add a CI step to regenerate and verify.
8. Add `-s -w` to release `LDFLAGS`; add a `make size` target to track binary size.

Steps 1–5 are safe and immediate. Step 6 requires scheduling.

## Compliance / Enforcement

- CI fails on any added file larger than 1 MB.
- CI fails if `git ls-files` matches `bin/`, `*.exe`, `*.pem`, or `keys/`.
- `.githooks/pre-commit` blocks the same patterns locally.
- `gofmt -l .` must return empty in CI (meaningful after step 1).
- `gitleaks` runs over the working tree and history.

## Related ADRs
- [ADR-008: Configuration Precedence and Secret Handling](008%20-%20Configuration%20Precedence%20and%20Secret%20Handling.md) — shares the history rewrite
- [ADR-004: Consolidate on a Single Extension Framework](004%20-%20Consolidate%20on%20a%20Single%20Extension%20Framework.md) — dependency removal shrinks the binary

## Related Documents
- [Repository hygiene](../06-quality/repository-hygiene.md)
- [Testing strategy](../06-quality/testing-strategy.md)

---

**This ADR serves as a binding architectural decision for the project.**
