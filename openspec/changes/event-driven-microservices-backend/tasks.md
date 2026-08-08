# Tasks — event-driven-microservices-backend

Phased implementation checklist for a later `/opsx:apply`. Each phase is independently
testable. Reference `specs/` for behavior and `design.md` for approach. The Go module stays
`github.com/aaks/server` with `go.mod` in `backend/`.

## 1. Foundations & repo layout

- [ ] 1.1 Create per-service dirs `backend/services/{gateway,project,task,agent,catalog,settings,runner}/` each with `cmd/` + `internal/` skeleton
- [ ] 1.2 Create shared packages: `backend/internal/contracts` (event + DTO types), `backend/internal/kafka` (sarama producer/consumer helpers), `backend/internal/db` (pg connect/migrate helpers)
- [ ] 1.3 Define the 6 logical Postgres databases (`project_db`, `task_db`, `agent_db`, `catalog_db`, `settings_db`, `runner_db`) + init script
- [ ] 1.4 Stand up shared structured logging (slog) + graceful-shutdown pattern reused from existing `cmd/server` + `internal/config`
- [ ] 1.5 Generate an internal mTLS CA + certs for Settings↔Runner auth (dev certs in deploy)

## 2. Event bus (Kafka)

- [ ] 2.1 Add Kafka (KRaft, single broker) to `deploy/docker-compose.yml`
- [ ] 2.2 Define topic catalog + JSON message schemas in `backend/internal/contracts/events.go` (commands + facts + status-changed)
- [ ] 2.3 Implement transactional/async producer helper (sarama) with `task_id` partition key
- [ ] 2.4 Implement consumer-group helper with at-least-once + idempotent dedup hooks (e.g. by `step.id`, `(task_id,run_id)`)
- [ ] 2.5 Verify round-trip: publish a `step.*` message, consume and dedup in two consumer groups

## 3. CRUD services (synchronous, behind Gateway)

- [ ] 3.1 Project service: migrations + handlers matching `frontend/src/api/projects.ts` (list/get/create/update/delete) → `project_db`
- [ ] 3.2 Catalog service: Skill + McpServer handlers matching `skills.ts` + `mcpServers.ts` → `catalog_db`
- [ ] 3.3 Agent service: Agent handlers + attach/detach skill & mcp matching `agents.ts` → `agent_db`
- [ ] 3.4 Task service: Task CRUD + feedback + `patchStatus` matching `tasks.ts` + `feedback.ts` (synchronous writes only; saga wiring in phase 6) → `task_db`
- [ ] 3.5 Per-service handler tests asserting exact response shapes vs the frontend types

## 4. API Gateway / BFF

- [ ] 4.1 Route table exposing the full `/api` surface from `frontend/src/api/*.ts` + `/healthz`
- [ ] 4.2 Synchronous passthrough for single-owner resources; request-id + recovery middleware
- [ ] 4.3 Synchronous fan-out composition for cross-service reads (e.g. tasks + agent names, agents + skill/mcp labels)
- [ ] 4.4 Error normalization to the frontend `ApiError` contract; no internal detail leakage
- [ ] 4.5 Contract test: each frontend `*.ts` function resolves through the Gateway with the declared shape

## 5. Agent-Runner (execution)

- [ ] 5.1 Reuse existing `backend/runner/Dockerfile` (credential-less sandbox) + Docker exec API driver; implement worktree-per-task (`agent/<task-id>-<slug>`) bind-mount
- [ ] 5.2 Persist + serve Run/Step/Finding/Artifact (`runs.ts` endpoints: `/tasks/:id/runs`, `/runs/:id/steps`, `/runs/:id/findings`, `/tasks/:id/artifacts`) → `runner_db`
- [ ] 5.3 Agent loop: system prompt (persona + injected skills), tool set (built-ins + bridged MCP), context reconstruction, step caps (~50 steps / token budget / ~30 min)
- [ ] 5.4 LLM provider clients (openai/anthropic/gemini/glm) using plaintext key from Settings, in-memory only
- [ ] 5.5 MCP client bridging attached MCP servers as tools
- [ ] 5.6 Command consumers: `task.run-requested`, `task.review-requested`, `task.stop-requested` (context-cancel); emit `step.*`, `run.completed`, `finding.*`, `verdict`, `pr.opened`
- [ ] 5.7 Reviewer variant emitting `verdict { APPROVE | REQUEST_CHANGES }`
- [ ] 5.8 Git ops on host (commit/push) + `gh pr create` on `open-pr`; never auto-merge

## 6. Task-service saga (choreography)

- [ ] 6.1 On `patchStatus→doing`: publish `task.run-requested`; track `round_no`
- [ ] 6.2 Consume `run.completed`/`verdict`: on `REQUEST_CHANGES` & `round_no < 5` → `doing` + re-emit `task.run-requested`; on `APPROVE` → review/done
- [ ] 6.3 Enforce ≤5 review rounds (surface as `blocked` when exhausted)
- [ ] 6.4 `re-run`/`stop` actions publish `task.run-requested`/`task.stop-requested`; `stop` sets `stopped` synchronously
- [ ] 6.5 `open-pr` triggers Runner PR creation; never auto-created elsewhere
- [ ] 6.6 Idempotent saga keyed by `(task_id, run_id)`; publish `task.status-changed`

## 7. Secret flow (Settings ↔ Runner)

- [ ] 7.1 Settings: encrypt-at-rest with master key; `ProviderKey` handlers exposing `{provider, created_at}` only
- [ ] 7.2 Settings: mTLS-only internal decrypt endpoint for provider key + git credential
- [ ] 7.3 Runner: fetch plaintext at run start, use in-memory only, never log/persist/container-env
- [ ] 7.4 Test: assert no provider key or git token appears in the sandbox container env/filesystem or logs

## 8. Realtime (SSE) via Gateway

- [ ] 8.1 `GET /api/tasks/:id/stream`: replay persisted steps from Runner (seq order) then tail Kafka `step.*` by `task_id`
- [ ] 8.2 Emit SSE event `step` with full `Step` shape; support `error` event without clearing steps
- [ ] 8.3 Dedup-by-`step.id` across reconnect/replay; verify native EventSource reconnect resumes cleanly

## 9. Deployment, E2E & docs

- [ ] 9.1 Finalize `deploy/docker-compose.yml` (Kafka + Postgres + 6 services + Gateway; frontend served separately) and `deploy/.env.example`
- [ ] 9.2 E2E: full task lifecycle through the SPA → Gateway → services → Kafka → Runner → sandbox
- [ ] 9.3 E2E assertions: CRUD shapes match; SSE replay+tail; review-round loop; stop aborts; PR created on demand, never auto-merged
- [ ] 9.4 Update `docs/design.md` (supersede monolith/in-process-pubsub sections) and `docs/tasks.md` to reference this change
- [ ] 9.5 `cd backend && go build ./... && go vet ./... && go test ./...` green; `make web-build` green (frontend unchanged)
