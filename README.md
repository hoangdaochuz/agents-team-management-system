# AI Agent Kanban System

A kanban board where each ticket is a task an autonomous AI agent works on
inside a registered repo, plus a management layer for agents, skills, and MCP
servers. **MVP — under active development.**

- Spec: [`docs/spec.md`](docs/spec.md)
- Design: [`docs/design.md`](docs/design.md)
- Implementation tasks: [`docs/tasks.md`](docs/tasks.md)
- Decision log / grill transcript: [`.loop-engineering/ai-agent-kanban-brief.md`](.loop-engineering/ai-agent-kanban-brief.md)
- Agent guide: [`CLAUDE.md`](CLAUDE.md)

## Status

| Layer | State |
|---|---|
| **Frontend SPA** | ✅ Complete — all 7 screens built from [`prototype/`](prototype/), against the declared API contract |
| **Backend API** | 🚧 Phase 0 — only `GET /healthz` exists; the REST + SSE surface is declared in `frontend/src/api/` but not yet implemented |
| **Agent engine** | ⏳ Planned — agent loop, multi-provider LLM, container sandbox, MCP bridging (see `docs/tasks.md` Phases 3–8) |

The frontend is built ahead of the backend **on purpose**: it declares the full
typed API contract (`frontend/src/api/types.ts` + per-entity client modules), and
every page renders its full layout in an error/empty state today. As the Go
backend implements each endpoint, the same UI lights up with real data — no
frontend rework needed.

## Quick start

```bash
# Backend (Go) — run from repo root via Makefile
make build test          # build + run unit tests (in backend/)
make run                 # API on http://localhost:8080  (/healthz works today)
make compose-up          # app + postgres via docker compose
make runner              # build the credential-less task-container base image (Phase 5+)

# Frontend (Vite + React + TS)
make web-install         # npm install (in frontend/)
make web-dev             # Vite dev server on http://localhost:5173 (proxies /api -> :8080)
make web-build           # tsc --noEmit && vite build  -> frontend/dist
make web-typecheck       # tsc --noEmit
```

For local UI development you typically run `make run` (backend) in one terminal
and `make web-dev` (frontend) in another. With only `/healthz` implemented, the
kanban/dashboard/etc. show their layout with friendly "backend not implemented
yet" states.

## Layout

```
frontend/          Vite + React + TS SPA
  src/api/           typed API contract (types.ts + per-entity clients) — the contract of record
  src/components/    ui/ primitives, shell/ (sidebar+topbar), tasks/, agents/
  src/pages/         Launcher, Dashboard, Kanban, TaskDetail, Agents, History, Settings
  src/hooks/         useTaskStream (SSE step stream)
  src/styles.css     design system ported verbatim from prototype/assets/app.css
backend/           Go service — self-contained module (go.mod lives here)
  cmd/server/        entrypoint
  internal/config/   env config
  internal/httpapi/  HTTP server, routes, middleware      (more packages land per phase)
  runner/            Dockerfile for per-task execution sandbox
deploy/            docker-compose.yml, .env.example
docs/              spec, design, tasks
prototype/         static HTML prototype — visual source of truth for the SPA
```

> The Go module path is `github.com/aaks/server` and `go.mod` lives in
> `backend/` (not the repo root) — internal imports resolve relative to
> `backend/`. It's a placeholder; rename with
> `cd backend && go mod edit -module <your-path>` and update imports.

## Architecture (essentials)

- **Backend owns everything sensitive; the task container is a credential-less
  sandbox.** The agent loop, all LLM provider calls, the MCP client, and every
  git operation run in the Go backend on the host. Per-task containers only run
  build/test/edit commands against a bind-mounted worktree — no API keys, no git
  credentials inside them.
- **Worktree-per-task.** Moving a task to `Doing` creates a
  `git worktree add` on branch `agent/<task-id>-<slug>`; that worktree is shared
  between backend and container.
- **Live monitoring over SSE.** `GET /api/tasks/:id/stream` emits `step` events
  carrying the full step shape (`message`/`tool_call`/`tool_result`/`reasoning`).
- **PR-only merge, human-gated** — code lands via a human-opened PR, never
  auto-merged.

See [`docs/design.md`](docs/design.md) for the full data model, agent execution
model, security model, and ADRs.

## CI

[`.github/workflows/ci.yml`](.github/workflows/ci.yml) runs two jobs on every
push/PR: a Go job (`backend/`: `go vet`, `go build`, `go test -race`,
`golangci-lint`) and a frontend job (`frontend/`: `npm ci`, typecheck, build).
