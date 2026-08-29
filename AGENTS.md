# AGENTS.md

AI Agent Kanban System — a kanban board where each ticket is a task executed by an AI agent inside a registered repo, plus management layers for agents/skills/MCP servers. Docs: `docs/spec.md` (requirements), `docs/design.md` (architecture), `docs/tasks.md` (backend phases; both carry supersede banners — the OpenSpec change is authoritative). `CLAUDE.md` has the long-form agent guide.

## Current state (verified)

- **Frontend SPA is complete** (React 18 + Vite + TS strict): all 15 prototype pages; the full API contract is declared in `frontend/src/api/*.ts` (`types.ts` + 17 entity modules, ~60 endpoints + 1 SSE stream). Typecheck/build pass.
- **Backend is implemented** as a consolidated event-driven backend (OpenSpec changes `event-driven-microservices-backend` then `consolidate-microservices`): **5 services** — gateway, identity (auth+orgs+admin), workspace (project+task+catalog+resources), agent (agent+settings), executor (runner, renamed) — all implemented, wired into `deploy/docker-compose.yml`; `go build ./...`, `go vet ./...`, `go test ./...` pass. `docs/design.md`/`docs/tasks.md` carry supersede banners pointing at the OpenSpec changes (which are authoritative).
- **`docker compose up` runs the whole stack**: Postgres (4 logical DBs — `identity_db`, `workspace_db`, `agent_db`, `runner_db` — via `deploy/postgres/01-create-databases.sql`) + Kafka KRaft (single node, auto-create topics, 6 partitions) + the 4 consolidated service containers + the Gateway on :8080. Frontend is served separately.
- **Sandbox/agent execution is unchanged through the consolidation**: the Executor ships `simulated`/`llm` drivers; `EXECUTOR_SANDBOX=docker|local` exec sandbox with worktree bind-mounted at /workspace; the sandbox secret-leak test (`services/executor/internal/sandbox/secret_leak_test.go`, opt-in via `AAKS_SANDBOX_TEST_DOCKER=1`) verifies no provider key/git token reaches container env/filesystem/logs. Container E2E suite `deploy/e2e/e2e.sh` (`make e2e`) needs a docker daemon — it has not been executed in CI yet.
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
- **DDD four-layer layout per service** (OpenSpec change `refactor-backend-ddd`, all 44 tasks done): `internal/domain` (aggregate value types + per-aggregate repo port interfaces + sentinel errors; imports no infra, no pgx/sarama/platform transport — enforced by `internal/archlint` as a failing test), `internal/application` (use-case logic; `EventPublisher`/`UnitOfWork`/ACL ports only — no pgx/sarama/net-http), `internal/infrastructure` (pgx repo adapters as per-aggregate subpackages over a shared `querier` serving plain + tx paths, `bus` sarama publisher adapter, ACL HTTP clients, crypto, tool provisioning; migrations live in `infrastructure/repository/migrations/`), `internal/interfaces` (thin `http` handlers + `messaging` consumers on the lifecycle ctx). `cmd/main.go` is the explicit composition root. `internal/contracts` is decomposed into per-domain subpackages (`{identity,workspaces,events,agentexec,resources,tasks,admin}`) — no god-package re-exports. Application tests use hand-rolled fakes; UnitOfWork only where multi-aggregate mutations exist (D8 proportional layering).
- **Service topology** — `backend/services/<name>` = one binary, one logical DB, one port: gateway **:8080** (BFF, sole HTTP entrypoint), identity :8085, workspace :8081, agent :8083, executor :8086. Each consolidated service keeps the DDD layers as per-plane subpackages (`internal/domain/{auth,orgs,admin}` inside identity, etc.). Each `cmd/main.go` is a thin `svcrun.Run(name, addr, Register)`.
- **Gateway is a path-aware reverse proxy**: `/api/<domain>/...` → owning service, stripping `/api`. Four upstream env vars (fails fast if unset): `UPSTREAM_IDENTITY` / `UPSTREAM_WORKSPACE` / `UPSTREAM_AGENT` / `UPSTREAM_EXECUTOR`. Session composition is a **single call** to Identity `/internal/identity` (user + workspace union, 60s cache) → injected scoping headers `X-User-ID` / `X-User-Name` / `X-User-Email` / `X-User-Superadmin` / `X-Workspace-ID` / `X-Workspace-IDs`; 401 on protected routes without a session, 403 for non-superadmin `/sysadmin/*`. `/workspaces/{wid}/skills` → workspace; `/workspaces/{wid}/knowledge|plugins|rules|mcp` → workspace; `/workspaces/{wid}/audit` → identity; `/tasks/{id}/runs|artifacts` → executor; `/tasks/:id/stream` (SSE) → replay from Executor `/internal/tasks/{id}/steps` + tail the Kafka `step` topic.
- **Event bus**: sarama client; topic catalog + event types in `backend/internal/contracts/events/` — **9 topics**, all execution-boundary and **partitioned by task_id**. Workspace is the **saga coordinator**: emits commands (`task.run-requested`, `task.review-requested`, `task.stop-requested`, `task.pr-open-requested`), consumes facts (`run.completed`, `verdict`, `pr.opened`). Executor emits facts (`step`, `run.completed`, `finding`, `verdict`, `pr.opened`). Intra-service flows (signup/invite/audit inside identity, MCP projections inside workspace) dispatch over an **in-process event bus** — never Kafka. Workspace provisioning on org approval is a direct Identity→Workspace HTTP call.
- **Secrets**: the Agent service is the sole decryptor of provider keys (master key `AGENT_MASTER_KEY` + internal token `AGENT_INTERNAL_TOKEN` channel to the Executor; dev certs in `deploy/certs/`). The credential-less-sandbox invariant carries over from the old design — never put API keys or git credentials in the container path.
- Shared packages under `internal/platform/`: `config` (env config), `db` (pgx + shared migrator), `http` (httputil helpers), `kafka` (producer/consumer wrappers), `svcrun` (service runtime: JSON logging, /healthz, graceful shutdown), `tenancy` (injected identity-header constants).

## The API contract lives in the frontend

`frontend/src/api/client.ts` (`request<T>`, throws `ApiError`) + `types.ts` + per-entity modules are the **contract of record**. When adding or renaming an endpoint, declare the type + client function in the frontend first; the backend catches up. UI must always render its full layout in error/empty states (via `<AsyncBoundary>`), never crash — that's how it survives unimplemented endpoints. Auth screens boot from a **dev-fallback synthetic session** (`frontend/src/store/auth.ts`) until the Auth service lands.

## Frontend conventions

- Design system = `frontend/src/styles.css`, ported verbatim from `prototype/assets/app.css`; class names are the styling API — mirror the matching `prototype/*.html` DOM, don't add CSS or a framework. `lib/icons.tsx` is the typed `<Icon>` set.
- UI primitives in `src/components/ui/`: `Card`/`Badge`/`Progress` don't accept a `style` prop (only `className`/`flush`); `Badge` requires children. Data: TanStack Query with plain-array invalidation keys (`["tasks"]`, `["task", id]`). Use **relative imports** — the `@/` alias in tsconfig is unused. Kanban uses native HTML5 drag-and-drop calling `tasks.patchStatus`.
- `DESIGN.md` at root = Apple design-system analysis (prototype source); `docs/design.md` = system design. Different documents.

## Workflow

- OpenSpec-driven (`openspec/`, schema: spec-driven); repo-local skills in `.claude/skills/openspec-*` run propose → apply → archive. Changes: `event-driven-microservices-backend` (done), `consolidate-microservices` (implemented — 11 services → 5); `add-remaining-frontend-pages` (complete). `docs/conformance-report.md` tracks code-vs-spec conformance.
- Kafka integration tests skip unless `AAKS_KAFKA_TEST_BROKERS` is set — `go test ./...` stays green without infrastructure.
