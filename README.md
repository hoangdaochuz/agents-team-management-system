# AI Agent Kanban System

A kanban board where each ticket is a task an autonomous AI agent works on
inside a registered repo, plus a management layer for agents, skills, and MCP
servers. **MVP — under active design.**

- Spec: [`docs/spec.md`](docs/spec.md)
- Design: [`docs/design.md`](docs/design.md)
- Implementation tasks: [`docs/tasks.md`](docs/tasks.md)
- Decision log / grill transcript: [`.loop-engineering/ai-agent-kanban-brief.md`](.loop-engineering/ai-agent-kanban-brief.md)

## Status

Phase 0 — scaffold only. HTTP server with `/healthz`, Postgres via compose,
per-task runner base image, CI.

## Quick start

```bash
make build test          # build + run unit tests (Go, in backend/)
make compose-up          # app + postgres; visit http://localhost:8080/healthz
make runner              # build the credential-less task-container base image (Phase 5+)
make web-install         # install frontend deps (frontend/)
make web-dev             # Vite dev server on :5173 (proxies /api -> :8080)
make web-build           # typecheck + production build into frontend/dist
```

## Layout

```
frontend/          Vite + React + TS SPA (api client, SSE hook, app shell, pages)
backend/           Go service (self-contained module: cmd/, internal/, runner/, go.mod)
  cmd/server/        entrypoint
  internal/config/   env config
  internal/httpapi/  HTTP server, routes, middleware      (more packages land per phase)
  runner/            Dockerfile for per-task execution sandbox
deploy/            docker-compose.yml, .env.example
docs/              spec, design, tasks
prototype/         static HTML prototype (visual reference for the SPA)
```

> Module path `github.com/aaks/server` is a placeholder — change with
> `go mod edit -module <your-path>` and update imports.
