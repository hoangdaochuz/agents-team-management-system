# AGENTS.md

AI Agent Kanban System — a kanban board where each ticket is a task executed by an AI agent inside a registered repo, plus management layers for agents/skills/MCP servers. Docs: `docs/spec.md` (requirements), `docs/design.md` (architecture), `docs/tasks.md` (backend phases; both carry supersede banners — the OpenSpec change is authoritative). `CLAUDE.md` has the long-form agent guide.

## Current state (verified)

- **Frontend SPA is complete** (React 18 + Vite + TS strict): all 15 prototype pages; the full API contract is declared in `frontend/src/api/*.ts` (`types.ts` + 17 entity modules, ~60 endpoints + 1 SSE stream). Typecheck/build pass.
- **Backend is implemented** as an event-driven microservices backend (OpenSpec change `event-driven-microservices-backend`): all 11 services (gateway, project, task, agent, catalog, settings, runner, auth, orgs, resources, admin) are implemented, wired into `deploy/docker-compose.yml`, and `go build ./...`, `go vet ./...`, `go test ./...` pass. `docs/design.md`/`docs/tasks.md` carry supersede banners pointing at the OpenSpec change (which is authoritative).
- **`docker compose up` runs the whole stack**: Postgres (10 logical DBs, `deploy/postgres/01-create-databases.sql`) + Kafka KRaft (single node, auto-create topics, 6 partitions) + all service containers + the Gateway on :8080. Frontend is served separately.
- **All change tasks are done** (70/70 checked in `openspec/changes/event-driven-microservices-backend/tasks.md`): Docker sandbox driver (Runner ships `simulated`/`llm` drivers; `RUNNER_SANDBOX=docker|local` exec sandbox with worktree bind-mounted at /workspace), MCP client bridging, and the task-7.4 sandbox secret-leak test (`services/runner/internal/sandbox/secret_leak_test.go`, opt-in via `AAKS_SANDBOX_TEST_DOCKER=1`, verifies no provider key/git token reaches container env/filesystem/logs). Container E2E suite `deploy/e2e/e2e.sh` (`make e2e`) passes 37/37 locally but needs a docker daemon — it has not been executed in CI yet.
- `backend/cmd/server` is the Phase-0 scaffold binary — `make run` boots it (`/healthz` only). The real entrypoint is the **gateway**: `go run ./services/gateway/cmd` (needs `UPSTREAM_*` env vars).

## Commands (from repo root)

```bash
make build test vet lint    # Go targets; they cd into backend/ themselves
make run                    # Phase-0 scaffold binary — NOT the gateway
make compose-up compose-down
make web-install web-dev web-build web-typecheck
```

- Single Go test: `cd backend && go test ./services/gateway/internal/httpapi -run TestName -v`
- Verify before committing: Go `go vet ./...` → `go build ./...` → `go test ./...` (CI adds `-race` and `golangci-lint`); frontend `npm run typecheck` → `npm run build`.
- CI only triggers on pushes to `main` + all PRs; the default branch is `master` — direct pushes to `master` don't run CI, PRs do.
- `golangci-lint` is strict (`disable-all` + errcheck/govet/staticcheck/revive/…); keep it clean.

## Architecture

- **Go module `github.com/aaks/server`; `go.mod` lives in `backend/`, not the repo root.** All services and `internal/*` are packages of this one module. Never move go.mod; imports resolve relative to `backend/`.
- **Service topology** — `backend/services/<name>` = one binary, one logical DB, one port: gateway **:8080** (BFF, sole HTTP entrypoint), project :8081, task :8082, agent :8083, catalog :8084, settings :8085, runner :8086, auth :8087, orgs :8088, resources :8089, admin :8090. Each `cmd/main.go` is a thin `svcrun.Run(name, addr, Register)`.
- **Gateway is a path-aware reverse proxy**: `/api/<domain>/...` → owning service, stripping `/api`. Domains come from `UPSTREAM_PROJECT` / `UPSTREAM_TASK` / `UPSTREAM_AGENT` / `UPSTREAM_CATALOG` / `UPSTREAM_SETTINGS` / `UPSTREAM_RUNNER` / `UPSTREAM_AUTH` / `UPSTREAM_ORGS` / `UPSTREAM_RESOURCES` / `UPSTREAM_ADMIN` env vars (fails fast if unset). Session cookie → identity (Auth `/internal/identity`) → memberships (Orgs `/internal/users/{uid}/workspaces`, 60s cache) → injected scoping headers `X-User-ID` / `X-User-Name` / `X-User-Email` / `X-User-Superadmin` / `X-Workspace-ID` / `X-Workspace-IDs`; 401 on protected routes without a session, 403 for non-superadmin `/sysadmin/*`. `/workspaces/{wid}/skills` → catalog; `/workspaces/{wid}/knowledge|plugins|rules|mcp|audit` → resources; `/tasks/{id}/runs|artifacts` → runner; `/tasks/:id/stream` (SSE) → replay from Runner `/internal/tasks/{id}/steps` + tail Kafka step topics.
- **Event bus**: sarama client; topic catalog + event types in `backend/internal/contracts/events.go`. All lifecycle/step topics are **partitioned by task_id** (ordered delivery per task); producer is idempotent; consumers are at-least-once with an idempotency hook. Task service is the **saga coordinator**: emits commands (`task.run-requested`, `task.review-requested`, `task.stop-requested`), consumes facts (`run.completed`, `verdict`, `pr.opened`, `task.status-changed`).
- **Secrets**: Settings service is the sole decryptor of provider keys (master key + mTLS channel to Runner; dev certs in `deploy/certs/`). The credential-less-sandbox invariant carries over from the old design — never put API keys or git credentials in the container path.
- Shared packages: `internal/config` (env config), `internal/contracts` (DTOs + Kafka topics), `internal/db` (pgx + migrations), `internal/httputil`, `internal/kafka` (producer/consumer wrappers), `internal/svcrun` (service runtime: JSON logging, /healthz, graceful shutdown).

## The API contract lives in the frontend

`frontend/src/api/client.ts` (`request<T>`, throws `ApiError`) + `types.ts` + per-entity modules are the **contract of record**. When adding or renaming an endpoint, declare the type + client function in the frontend first; the backend catches up. UI must always render its full layout in error/empty states (via `<AsyncBoundary>`), never crash — that's how it survives unimplemented endpoints. Auth screens boot from a **dev-fallback synthetic session** (`frontend/src/store/auth.ts`) until the Auth service lands.

## Frontend conventions

- Design system = `frontend/src/styles.css`, ported verbatim from `prototype/assets/app.css`; class names are the styling API — mirror the matching `prototype/*.html` DOM, don't add CSS or a framework. `lib/icons.tsx` is the typed `<Icon>` set.
- UI primitives in `src/components/ui/`: `Card`/`Badge`/`Progress` don't accept a `style` prop (only `className`/`flush`); `Badge` requires children. Data: TanStack Query with plain-array invalidation keys (`["tasks"]`, `["task", id]`). Use **relative imports** — the `@/` alias in tsconfig is unused. Kanban uses native HTML5 drag-and-drop calling `tasks.patchStatus`.
- `DESIGN.md` at root = Apple design-system analysis (prototype source); `docs/design.md` = system design. Different documents.

## Workflow

- OpenSpec-driven (`openspec/`, schema: spec-driven); repo-local skills in `.claude/skills/openspec-*` run propose → apply → archive. Active change: `event-driven-microservices-backend` (mid-implementation); `add-remaining-frontend-pages` (complete). `docs/conformance-report.md` tracks code-vs-spec conformance.
- Kafka integration tests skip unless `AAKS_KAFKA_TEST_BROKERS` is set — `go test ./...` stays green without infrastructure.
