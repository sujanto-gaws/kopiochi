# Project guide for Claude Code

This file orients Claude Code (and teammates) on the stack, conventions, and the
specialized agents available in `.claude/agents/`. Keep it accurate — it drives
how work gets delegated.

## Tech stack

**Frontend**
- Vue 3 (Composition API, `<script setup lang="ts">`)
- Vite + TypeScript
- Pinia — state management (setup-style stores)
- vue-router — routing (named, lazy-loaded routes; guards for auth)
- axios — API calls via a shared instance with interceptors
- Tailwind CSS v4 — CSS-first config (`@theme` in the stylesheet, `@tailwindcss/vite`)
- shadcn-vue — in-repo UI components under `components/ui/` (built on Reka UI, `cva`, `cn()`)

**Backend**
- Go
- spf13/cobra — CLI/commands (`cmd/` layout: `serve`, `migrate`, …)
- spf13/viper — config from file/env/flags into a typed struct
- go-chi/chi (v5) — HTTP router + middleware
- uptrace/bun — ORM over the pgx v5 driver (`pgdialect`)
- rs/zerolog — structured, leveled logging

**Database**
- PostgreSQL — schema owned via Bun models + Bun migrations (`uptrace/bun/migrate`)

## Conventions

- Type everything on the frontend (props, emits, store state, API responses); avoid `any`.
- Reuse the shared axios instance — no ad-hoc `axios.create()` or raw `fetch` per component.
- Keep server-fetch/shared state in Pinia stores; local UI state stays in components.
- Backend: wrap errors with `%w`, thread `context.Context` through handlers → services → Bun, never ignore returned errors, log at boundaries with zerolog.
- Always parameterized queries via the Bun query builder; never string-built SQL.
- No secrets in code or images — load through Viper (env/secret store).
- Migrations are reversible (Up/Down) and run through the Cobra `migrate` command, never by hand.

## Common commands

**Frontend**
- Dev: `npm run dev`
- Build: `npm run build`
- Type check: `vue-tsc --noEmit`
- Test: `npm run test` (Vitest)

**Backend**
- Build: `go build ./...`
- Vet: `go vet ./...`
- Test: `go test ./...`
- Run server: `go run . serve` (or the built binary)
- Migrate: `go run . migrate` (Bun migrations)
- Vuln scan: `govulncheck ./...`

> Adjust the above to match this repo's actual scripts/targets.

## Specialized agents

Eight agents live in `.claude/agents/`. Delegate to them by role — invoke with
"use the <name> agent to…" or `@agent-<name>`, and run independent ones in
parallel.

| Agent | Model | Scope | Writes code? |
|-------|-------|-------|--------------|
| `system-architect` | opus | Design, API contracts, cross-cutting decisions | No (plans only) |
| `frontend-engineer` | sonnet | Vue / Pinia / vue-router / axios / Tailwind v4 / shadcn-vue | Yes |
| `backend-engineer` | sonnet | Go / Cobra / Viper / chi / Bun / zerolog | Yes |
| `database-engineer` | sonnet | Postgres schema, Bun models + migrations, query tuning | Yes |
| `tester` | sonnet | Writing/improving tests (Go + frontend) | Yes |
| `debugger` | sonnet | Root-cause + minimal fix for failures | Yes |
| `security-auditor` | opus | Vulnerability audit + remediation advice | No (read-only) |
| `devops-ci` | sonnet | Docker, docker-compose, CI/CD, deploys | Yes |

`system-architect` and `security-auditor` are intentionally read-only so they
can't change code.

## Recommended handoff flow

For a typical feature:

1. **system-architect** — turns the requirement into a design and the frontend↔backend API contract, and says which layer owns each piece.
2. **database-engineer** — writes the Bun model + reversible migration for any schema change.
3. **backend-engineer** and **frontend-engineer** — implement against the contract, in parallel (backend endpoints/services; Vue UI + stores + axios calls).
4. **tester** — adds coverage for the new behavior (handler tests, Bun/pgx data tests, Vitest + VTU with Pinia and mocked axios).
5. **debugger** — engaged only when something fails; reproduces, finds root cause, applies the minimal fix.
6. **security-auditor** — reviews sensitive/auth/data-handling changes before shipping.
7. **devops-ci** — updates Docker/compose/CI and ensures migrations run in the pipeline.

Notes:
- Subagents can't spawn other subagents — keep orchestration in the main session.
- Each agent starts with a fresh context, so pass along the relevant plan/contract when handing off.
- Prefer foreground for anything that edits files; background is fine for read-only reviews (architect, security-auditor).
