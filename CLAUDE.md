# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

An **AI Agent Kanban System**: a kanban board where each ticket is a task an autonomous AI agent
executes inside a registered repo, plus a management layer for agents, skills, and MCP servers.
See `docs/spec.md` (requirements), `docs/design.md` (architecture, data model, ADRs), and
`AGENTS.md` (authoritative current state).

**Current state:** the **frontend SPA is fully built** against the declared API contract
(`frontend/src/api/*.ts`), and the **backend is an event-driven microservices implementation**
(see the OpenSpec change `event-driven-microservices-backend`, `AGENTS.md`): 11 Go services,
Kafka event bus, Gateway BFF, multi-tenant Auth/Orgs/Resources/Admin plane, plus the Docker
sandbox driver, MCP client bridging, and the sandbox secret-leak test — all 70 change tasks
checked. UI must always render its full layout in
the error/empty state, never crash.

## Repository layout

```
frontend/   Vite + React + TS SPA (self-contained; port 5173 in dev)
backend/    Go service — self-contained module (go.mod lives HERE, not at repo root)
  cmd/server/         entrypoint
  internal/config/    env config
  internal/httpapi/   HTTP server, routes, middleware  (REST handlers/SSE/agent loop land in later phases)
  runner/             Dockerfile for the per-task credential-less sandbox image
deploy/     docker-compose.yml, .env.example
docs/       spec.md, design.md, tasks.md
prototype/  static HTML prototype — the visual source of truth for the SPA (do not delete)
```

The Go module path is `github.com/aaks/server` and `go.mod` lives in `backend/` — so internal
imports look like `github.com/aaks/server/internal/...` and resolve relative to `backend/`. Do not
move `go.mod` back to the repo root.

## Commands

Most things go through the root `Makefile` (run from repo root):

```bash
make build            # cd backend && go build ./...
make test             # cd backend && go test ./...
make vet              # cd backend && go vet ./...
make lint             # cd backend && golangci-lint run
make run              # cd backend && go run ./cmd/server   (API on :8080)
make compose-up       # docker compose -f deploy/docker-compose.yml up --build  (app + postgres)
make runner           # build the credential-less task-container base image

make web-install      # cd frontend && npm install
make web-dev          # cd frontend && npm run dev   (Vite :5173, proxies /api -> :8080)
make web-build        # cd frontend && npm run build (tsc --noEmit && vite build -> frontend/dist)
make web-typecheck    # cd frontend && npm run typecheck
```

Run a **single Go test**: `cd backend && go test ./internal/httpapi -run TestName -v`
Run a **single package's tests**: `cd backend && go test ./internal/httpapi`

CI (`.github/workflows/ci.yml`) runs the Go job in `working-directory: backend` and a separate
`frontend` job (`npm ci && typecheck && build`). Both must pass.

## Architecture (the load-bearing parts)

**Backend owns everything sensitive; the container is a pure sandbox.** The agent loop, all LLM
provider calls, the MCP client, and all git operations (commit/push/`gh pr create`) run **in the Go
backend on the host**. Per-task containers exist only to run build/test/edit commands via the Docker
exec API against a bind-mounted worktree — they hold **no API keys and no git credentials**
(`design.md` §3.4, §5). This is the core security property; don't break it by pushing credentials
into the container path.

**Worktree-per-task.** Each task moving to `Doing` gets a `git worktree add` on branch
`agent/<task-id>-<slug>` under the managed clone; that worktree is bind-mounted RW into the task
container. Backend and container see the same files. Concurrent Doing tasks use disjoint worktrees.

**Agent loop** (`design.md` §3.1): builds system prompt (agent persona + injected skills), assembles
tools (built-in `read_file`/`write_file`/`list_files`/`run_command` + bridged MCP tools),
reconstructs context from `task_thread`, then loops `provider.Call → dispatch tool_calls → stream
step`. Capped by steps (~50), tokens, and 30-min wall-clock. A separate **reviewer** variant emits
an `APPROVE`/`REQUEST_CHANGES` verdict; on changes the task returns to `Doing` (≤5 rounds) before a
human-initiated PR. PRs are **never auto-merged** (ADR-05).

**Realtime (SSE).** The Runner persists steps to Postgres *and* publishes them to Kafka
(`step.*` topics, partitioned by `task_id`). `GET /api/tasks/:id/stream` (Gateway) replays
persisted steps from the Runner's internal endpoint, then tails the Kafka topic, deduping by
`step.id`. The SSE **event is named `step`** and carries the full `Step` shape `{id, run_id,
seq, kind, payload, created_at}` — `kind` is `message|tool_call|tool_result|reasoning`. (The
`message`/`tool_call`/etc. in the design doc are `kind` values, not separate SSE event names.)

## Frontend conventions

**Stack:** React 18 + TypeScript (strict, `noUnusedLocals`/`noUnusedParameters`) + Vite +
React Router v6 + TanStack Query (server state) + Zustand (UI state). No CSS framework, no UI lib.

**The API contract is declared in the frontend; backend implements later.** All HTTP goes through
`frontend/src/api/client.ts`'s `request<T>` (base `/api`, throws `ApiError`). Domain types live in
`api/types.ts`; one thin module per entity (`tasks.ts`, `agents.ts`, `runs.ts`, …). When adding an
endpoint, add the type + the client function here first — the backend will catch up. Vite proxies
`/api` → `localhost:8080` in dev (`vite.config.ts`); in production the Go binary is meant to serve
the embedded bundle (not yet implemented — currently dev-only).

**Design system = ported CSS.** `frontend/src/styles.css` is a verbatim port of
`prototype/assets/app.css` (Apple design system). The class names (`.card`, `.tcard`, `.kanban`,
`.btn`, `.badge`, `.tabs`, `.terminal`, `.kpi`, …) **are the styling API** — match them, don't invent
CSS or add a framework. When implementing a screen, open the matching `prototype/*.html` and mirror
its DOM/class structure. `lib/icons.tsx` is the typed `<Icon name="..." />` set, also ported from
the prototype.

**UI primitives** live in `frontend/src/components/ui/` and are thin wrappers that emit those class
names (Button, Badge/StatusBadge, Card, Progress, Avatar, Field/Input/Select/Textarea, Tabs,
Segmented, Switch, Modal, Toast/useToast, KPI/Sparkline, EmptyState, AsyncBoundary). Two gotchas:
- **`Card`, `Badge`, `Progress` do not accept a `style` prop** (only `className`/`flush`/etc.). For
  inline styling either wrap in a `<span style>` or use a raw `<section className="card">`.
- **`Badge` requires children** (can't self-close); use `<Badge tone="accent" dot pulse>{" "}</Badge>`.

**Data fetching pattern.** Use `useQuery`/`useMutation` from TanStack Query and wrap rendered lists
in `<AsyncBoundary>` — it renders loading, a friendly "backend not implemented yet" message on
error (the common case today), empty, and data states. Invalidation keys are plain arrays, e.g.
`["tasks"]`, `["task", id]`, `["agents"]`, `["feedback", taskId]`.

**Routing.** `App.tsx`: `/` is the standalone Launcher (outside the app shell); all other routes
render inside `<AppLayout>` (Sidebar + Topbar + Outlet). The kanban uses **native HTML5
drag-and-drop** (no DnD library) calling `tasks.patchStatus`.

**Imports:** code uses **relative** imports (`../components/ui`), even though `tsconfig.json`
declares an `@/*` → `src/*` path alias. Prefer the relative style to match the existing code.

## Working in this repo

- When adding backend REST handlers, the endpoint paths and request/response shapes must match what
  `frontend/src/api/*.ts` already declares — that file pair is the contract of record.
- `DESIGN.md` at repo root is an Apple design-system *analysis* (the prototype's source); `docs/design.md`
  is the system design doc. They are different documents.
- `docs/tasks.md` scopes the remaining backend phases; its file paths are prefixed with `backend/`
  and `frontend/` to match the post-restructure layout.
