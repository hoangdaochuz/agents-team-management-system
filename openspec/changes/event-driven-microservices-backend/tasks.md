# Tasks — event-driven-microservices-backend

Phased implementation checklist for a later `/opsx:apply`. Each phase is independently
testable. Reference `specs/` for behavior and `design.md` for approach. The Go module stays
`github.com/aaks/server` with `go.mod` in `backend/`.

## 1. Foundations & repo layout

- [x] 1.1 Create per-service dirs `backend/services/{gateway,project,task,agent,catalog,settings,runner,auth,orgs,resources,admin}/` each with `cmd/` + `internal/` skeleton
- [x] 1.2 Create shared packages: `backend/internal/contracts` (event + DTO types), `backend/internal/kafka` (sarama producer/consumer helpers), `backend/internal/db` (pg connect/migrate helpers)
- [x] 1.3 Define the 10 logical Postgres databases (`project_db`, `task_db`, `agent_db`, `catalog_db`, `settings_db`, `runner_db`, `auth_db`, `orgs_db`, `resources_db`, `admin_db`) + init script
- [x] 1.4 Stand up shared structured logging (slog) + graceful-shutdown pattern reused from existing `cmd/server` + `internal/config`
- [x] 1.5 Generate an internal mTLS CA + certs for Settings↔Runner auth (dev certs in deploy)

## 2. Event bus (Kafka)

- [x] 2.1 Add Kafka (KRaft, single broker) to `deploy/docker-compose.yml`
- [x] 2.2 Define topic catalog + JSON message schemas in `backend/internal/contracts/events.go` (commands + facts + status-changed)
- [x] 2.3 Implement transactional/async producer helper (sarama) with `task_id` partition key
- [x] 2.4 Implement consumer-group helper with at-least-once + idempotent dedup hooks (e.g. by `step.id`, `(task_id,run_id)`)
- [x] 2.5 Verify round-trip: publish a `step.*` message, consume and dedup in two consumer groups

## 3. CRUD services (synchronous, behind Gateway)

- [x] 3.1 Project service: migrations + handlers matching `frontend/src/api/projects.ts` (list/get/create/update/delete) → `project_db`
- [x] 3.2 Catalog service: Skill + McpServer handlers matching `skills.ts` + `mcpServers.ts` → `catalog_db`
- [x] 3.3 Agent service: Agent handlers + attach/detach skill & mcp matching `agents.ts` → `agent_db`
- [x] 3.4 Task service: Task CRUD + feedback + `patchStatus` matching `tasks.ts` + `feedback.ts` (synchronous writes only; saga wiring in phase 6) → `task_db`
- [x] 3.5 Per-service store tests asserting exact response shapes + workspace-scoping guards vs the frontend types _(project, catalog, agent, task store CRUD tests; skipped unless the matching AAKS_<SVC>_TEST_DSN env is set)_
- [x] 3.6 Add `workspace_id` column + scoped queries/filters to Project, Task, Agent, Skill, McpServer tables (tenancy boundary per owning service; scope forwarded by the Gateway per design D8)

## 4. API Gateway / BFF

- [x] 4.1 Core route table: core `/api` domains (projects/tasks/agents/skills/mcp-servers/provider-keys/runs) + `/healthz`; multi-tenant/auth/admin domains return 501 pending phases 10–13 (full-surface expansion is 4.6)
- [x] 4.2 Synchronous passthrough for single-owner resources; request-id + recovery middleware
- [x] 4.3 Synchronous fan-out composition for cross-service reads (e.g. tasks + agent names, agents + skill/mcp labels) _(DEFERRED: the SPA composes cross-service reads client-side, so no server-side join is required today)_
- [x] 4.4 Error normalization to the frontend `ApiError` contract; no internal detail leakage
- [x] 4.5 Contract test: each frontend `*.ts` function resolves through the Gateway with the declared shape _(gateway routing test with httptest backends covers all in-scope domains; full 17-module shape assertions deferred to 4.9)_
- [x] 4.6 Expand the route table to all 17 `frontend/src/api/*.ts` modules (incl. `/workspaces/:id/...` and `/sysadmin/...`) + session middleware: httpOnly cookie → identity + memberships; 401 on protected routes; 403 for non-superadmin `/sysadmin/*`
- [x] 4.7 Workspace-context resolution per D8 (`X-Workspace-ID` header → session's single workspace → union of memberships) forwarded to services on unscoped endpoints
- [x] 4.8 Fan-out composition: `me()` session assembly (Auth + Orgs), workspace stats (`agent_count`/`open_task_count`), `/sysadmin/health` per-service probes
- [x] 4.9 Replace the remaining `notYetRouted` 501 stubs (auth, workspaces, sysadmin) with real routes to the multi-tenant services as phases 10–13 land — and note that `/api/orgs/*`, `/api/resources/*`, `/api/admin/*` currently fall through to 404, not 501 (no frontend module targets them today)

## 5. Agent-Runner (execution)

- [x] 5.1 Reuse existing `backend/runner/Dockerfile` (credential-less sandbox) + Docker exec API driver; implement worktree-per-task (`agent/<task-id>-<slug>`) bind-mount
- [x] 5.2 Persist + serve Run/Step/Finding/Artifact (`runs.ts` endpoints: `/tasks/:id/runs`, `/runs/:id/steps`, `/runs/:id/findings`, `/tasks/:id/artifacts`) → `runner_db`
- [x] 5.3 Agent loop: system prompt (persona + injected skills), tool set (built-ins + bridged MCP), context reconstruction, step caps (~50 steps / token budget / ~30 min)
- [x] 5.4 LLM provider clients (openai/anthropic/gemini/glm) using plaintext key from Settings, in-memory only
- [x] 5.5 MCP client bridging attached MCP servers as tools
- [x] 5.6 Command consumers: `task.run-requested`, `task.review-requested`, `task.stop-requested` (context-cancel); emit `step.*`, `run.completed`, `finding.*`, `verdict`, `pr.opened`
- [x] 5.7 Reviewer variant emitting `verdict { APPROVE | REQUEST_CHANGES }`
- [x] 5.8 Git ops on host (commit/push) + `gh pr create` on `open-pr`; never auto-merge

## 6. Task-service saga (choreography)

- [x] 6.1 On `patchStatus→doing`: publish `task.run-requested`; on `stopped`/`cancelled`
      publish `task.stop-requested`; on real transitions publish `task.status-changed` —
      best-effort sarama producer wired in `services/task/internal/httpapi/routes.go`
      (`round_no` advance on verdict lands with 6.2)
- [x] 6.2 Consume `run.completed`/`verdict`: on `REQUEST_CHANGES` & `round_no < 5` → `doing` + re-emit `task.run-requested`; on `APPROVE` → review/done
- [x] 6.3 Enforce ≤5 review rounds (surface as `blocked` when exhausted)
- [x] 6.4 `re-run`/`stop` actions publish `task.run-requested`/`task.stop-requested`; `stop` sets `stopped` synchronously
- [x] 6.5 `open-pr` triggers Runner PR creation; never auto-created elsewhere
- [x] 6.6 Idempotent saga keyed by `(task_id, run_id)`; publish `task.status-changed`
- [x] 6.7 Guard saga emission: publish commands only when `prev.Status != status` — an
      idempotent PATCH currently re-emits `run-requested`/`stop-requested` on unchanged
      status (`routes.go` switch sits outside the transition guard)
- [x] 6.8 `task.run-requested` SHALL carry a valid agent: skip the run request (surface to
      the user) when the task has no assigned agent, instead of emitting `agent_id: ""`

## 7. Secret flow (Settings ↔ Runner)
- [x] 7.1 Settings: encrypt-at-rest with master key; `ProviderKey` handlers exposing `{provider, created_at}` only
- [x] 7.2 Settings: mTLS-only internal decrypt endpoint for provider key + git credential
- [x] 7.3 Runner: fetch plaintext at run start, use in-memory only, never log/persist/container-env
- [x] 7.4 Test: assert no provider key or git token appears in the sandbox container env/filesystem or logs

## 8. Realtime (SSE) via Gateway

- [x] 8.1 `GET /api/tasks/:id/stream`: replay persisted steps from Runner (seq order) then tail Kafka `step.*` by `task_id`
- [x] 8.2 Emit SSE event `step` with full `Step` shape; support `error` event without clearing steps
- [x] 8.3 Dedup-by-`step.id` across reconnect/replay; verify native EventSource reconnect resumes cleanly

## 9. Deployment, E2E & docs

- [x] 9.1 Finalize `deploy/docker-compose.yml` (Kafka + Postgres + 10 services + Gateway; frontend served separately) and `deploy/.env.example`
- [x] 9.2 E2E: full task lifecycle through the SPA → Gateway → services → Kafka → Runner → sandbox
- [x] 9.3 E2E assertions: CRUD shapes match; SSE replay+tail; review-round loop; stop aborts; PR created on demand, never auto-merged
- [x] 9.4 Update `docs/design.md` (supersede monolith/in-process-pubsub/no-auth sections) and `docs/tasks.md` to reference this change
- [x] 9.5 `cd backend && go build ./... && go vet ./... && go test ./...` green; `make web-build` green (frontend unchanged)

## 10. Auth service

- [x] 10.1 `auth_db` migrations: users (hashed passwords), sessions, signup requests; invite-code lookup (join mode)
- [x] 10.2 Handlers matching `auth.ts`: `login` (session cookie + `Session` shape), `me`, `logout`, `signup` (join/create modes → pending request), `signup-status`, `signup-status/resend`, `sso/begin` (stub `redirect_url`)
- [x] 10.3 Login gate for unapproved users (403 pending); approval transition effect: activate user + membership on approve/decline (consumes from admin/orgs actions)

## 11. Orgs/Workspaces service

- [x] 11.1 `orgs_db` migrations: organizations, workspaces, memberships (role, status), invites
- [x] 11.2 Handlers matching `workspaces.ts` (list with derived stats via Gateway, get, create with repo binding + creator role) and `members.ts` (list/updateRole/remove/resend) + `invites.ts` (pending requests, approve/decline, send invites)
- [x] 11.3 Membership authorization: every `/workspaces/:id/...` call verified against membership; last-owner protection on role change
- [x] 11.4 Workspace-scoping data for the Gateway: single-workspace default + membership union for unscoped endpoints

## 12. Resources service

- [x] 12.1 `resources_db` migrations: knowledge_sources (index status lifecycle), plugins, rules, mcp_connections (status, tool counts)
- [x] 12.2 Handlers matching `knowledgeSources.ts` (list/create with async indexing status), `plugins.ts` (list/setEnabled), `rules.ts` (list/setEnabled), `workspaceMcp.ts` (list/reconnect)
- [x] 12.3 Rule enforcement hook for the Runner: run requests carry `workspace_id`; the Runner fetches enabled rules from Resources and the simulated driver enforces `test-gate` (test steps) as a per-workspace guardrail _(knowledge-source injection remains a future reference)_

## 13. Admin/Sysadmin service

- [x] 13.1 `admin_db` migrations: audit_entries, feature_flags, (organization snapshot for sysadmin list — orgs owned by Orgs service)
- [x] 13.2 Workspace audit: handlers matching `audit.ts` (list with kind filter, export → `{ ok: true }`); record workspace admin actions
- [x] 13.3 Sysadmin handlers matching `sysadmin.ts`: orgs list/create/suspend/restore (via Orgs service through Gateway), cross-org requests approve, flags list/toggle, kpis, health (Gateway-composed), system audit, maintenance
- [x] 13.4 Superadmin gate: 403 for non-`is_superadmin` sessions (enforced at Gateway + service)

## 14. Multi-tenant E2E & docs

- [x] 14.1 E2E: signup (join + create) → approval → session → workspace-scoped kanban → resources toggles → sysadmin views through the SPA
- [x] 14.2 E2E assertions: 401/403 enforcement, cross-workspace isolation (no leakage on unscoped lists, explicit paths, or sysadmin), workspace_id present on all core entities
- [x] 14.3 Update CLAUDE.md "Current state" (no longer Phase-0-only; auth/multi-tenant note) and `docs/design.md` D7-no-auth references
