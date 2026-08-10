## Context

The frontend SPA is complete against a declared REST+SSE contract (`frontend/src/api/*.ts`),
but the backend is Phase 0 (`GET /healthz` only). The current `docs/design.md` prescribes a
single Go binary with in-process pub/sub for realtime and a monolithic agent loop. See
`proposal.md` for *why* we are moving to microservices + Kafka; this document covers *how*.

Hard constraints that shape the approach:
- The frontend contract (`frontend/src/api/types.ts` + the `*.ts` modules) is fixed and
  synchronous; we must not break it.
- The security property is load-bearing: the per-task sandbox container holds no API keys
  and no git credentials; all sensitive work happens in backend processes.
- Single-operator MVP, deployable via docker-compose — but with real multi-user auth,
  workspace membership, and roles, since the SPA gates every route behind a session.

## Goals / Non-Goals

**Goals:**
- Decompose the backend into 10 bounded-context Go services + a Gateway, each owning its DB.
- Make execution (agent loop, review rounds, stop, PR) event-driven over Kafka, while CRUD
  stays synchronous so the frontend contract is unchanged.
- Preserve and sharpen the credential boundary across process boundaries (Settings is the
  sole decryptor; mTLS handoff to the runner; container stays pure).
- Serve realtime SSE from the Gateway backed by replay + Kafka tail.
- Provide auth (sessions), multi-tenancy (orgs/workspaces/members, roles), workspace-scoped
  resources, and the sysadmin surface, all behind the Gateway, so the entire SPA contract
  is fulfilled.
- Keep everything runnable by one operator in docker-compose (Kafka KRaft, one Postgres).

**Non-Goals:**
- SSO provider integration — `/auth/sso/begin` returns a configured `redirect_url`; the
  Google/SAML exchange is stubbed.
- RAG indexing engine — knowledge sources are managed and index-statused; the
  embedding/chunking pipeline is deferred.
- Billing / plan enforcement — seats/plans are stored and displayed; billing is not.
- Polyglot services (all Go) or Zookeeper (KRaft only).
- Auto-merging pull requests — PRs are human-initiated only (ADR-05 preserved).
- Frontend changes — the SPA is untouched; all change is behind the Gateway.
- A separate Orchestrator service — the Task service coordinates the saga via choreography.

## Decisions

### D1. Ten bounded-context services + Gateway, DB-per-service
**Choice:** Project, Task, Agent, Catalog (Skills+MCP), Settings (keys+secrets), Agent-Runner
(loop/LLM/MCP/git/worktree/sandbox; owns Run/Step/Finding/Artifact), Auth (users, sessions,
signup/approval), Orgs/Workspaces (organizations, workspaces, membership, roles, invites),
Resources (knowledge, plugins, rules, MCP connection state), and Admin/Sysadmin (audit,
feature flags, system KPIs/health), plus an API Gateway/BFF. Each service owns one logical
Postgres database.

**Rationale / alternatives:** This isolates the credential-sensitive runtime (Runner +
Settings) from the CRUD-heavy domains, keeps services small, and matches the frontend's
natural domains. *Alternatives rejected:* (a) 2-service "execution vs catalog" split —
too coarse, re-couples unrelated CRUD; (b) one-service-per-entity (~10+) — excessive ops
cost for one operator; (c) modular monolith with internal Kafka — not actually
microservices, contradicts the stated goal.

Entity ownership mirrors the frontend types 1:1 (`Project→Project`,
`Task,Feedback→Task`, `Agent→Agent`, `Skill,McpServer→Catalog`,
`ProviderKey`+git token→`Settings`, `Run,Step,Finding,Artifact→Runner`,
`User,Session→Auth`, `Organization,Workspace,Member,Invite,SignupRequest→Orgs`,
`KnowledgeSource,Plugin,Rule,McpConnection→Resources`,
`AuditEntry,FeatureFlag→Admin`). Feedback lives in Task because
`/tasks/:id/feedback` is task-coupled. Every core entity carries `workspace_id`; the
workspace is the tenancy boundary enforced by every owning service.

### D2. CRUD synchronous, execution async over Kafka
**Choice:** The Gateway handles CRUD synchronously (forward or fan-out → service → DB →
respond with the resource). Only execution/lifecycle side-effects travel over Kafka:
`task.run-requested`, `task.review-requested`, `task.stop-requested` (commands);
`step.*`, `run.completed`, `finding.*`, `verdict`, `pr.opened`, `task.status-changed`
(facts). Actions like `re-run`/`stop` return 202 + publish, then facts update state.

**Rationale / alternatives:** The frontend expects the resource back synchronously
(`createTask` returns a `Task`); an all-async write model would break the contract and the
SPA's optimistic updates. *Alternatives rejected:* (a) reads-sync/writes-async — breaks
`createTask`/`patchStatus` return values; (b) Kafka only for SSE fan-out — not an
"event-driven backbone"; the chosen split gives both a stable REST contract and genuine
event-driven execution.

### D3. Gateway = sole synchronous caller; fan-out composition; SSE owner
**Choice:** Only the Gateway makes synchronous calls into services. Cross-service reads are
composed by synchronous fan-out (e.g. list tasks → batch-fetch agent names; `me()` →
assemble user + memberships into a `Session`; workspace cards → agent/task counts;
`/sysadmin/health` → per-service probes). The Gateway also owns the SSE connection: on
connect it replays persisted steps from the Runner, then tails the Kafka `step.*` topic
filtered by `task_id`.

**Rationale / alternatives:** Centralizing composition in the Gateway keeps services
decoupled (no service-to-service sync calls — a core microservices goal) and gives the
frontend one stable endpoint. *Alternatives rejected:* (a) CQRS projections via events —
stale reads + N projection consumers, more than an MVP needs; (b) direct service-to-service
calls — synchronous coupling and cascading-failure risk; (c) Runner serves SSE directly —
couples the execution process to long-lived client connections and complicates scaling.

### D4. Task service = saga coordinator via choreography
**Choice:** The Task service owns task status + `round_no` and coordinates the lifecycle by
emitting commands and consuming facts: doing → `task.run-requested` → runner →
`run.completed`/`verdict`; on `REQUEST_CHANGES` with `round_no < 5` it returns the task to
`doing` and re-emits `task.run-requested`; on `APPROVE` it advances to review/done. Stop =
set `stopped` + `task.stop-requested`, which the runner consumes to cancel its in-flight
context.

**Rationale / alternatives:** Choreography avoids a separate orchestrator process and keeps
the source of truth for "where is this task" in the Task service (queryable). *Alternatives
rejected:* (a) dedicated Orchestrator service — a 7th service + DB for no MVP gain;
(b) runner self-coordinates rounds — Task status becomes a stale derived view and the state
machine is implicit/hard to query.

Idempotency: the Task service keys saga advances by `(task_id, run_id)` so redelivered
facts cannot double-advance status or double-count rounds.

### D5. Settings is the sole decryptor; mTLS handoff; container stays pure
**Choice:** Secrets are encrypted at rest with a master key held only by Settings. The
Runner obtains plaintext provider keys and the git credential over an authenticated,
encrypted internal channel (mTLS) at run start, uses them in process memory only, and never
persists or logs them; they are never placed in the sandbox container env or filesystem.

**Rationale / alternatives:** This preserves the load-bearing invariant across process
boundaries — one tight decryptor, one consumer, nothing in the container. *Alternatives
rejected:* (a) Runner holds the master key and decrypts locally — spreads decryption
capability into the execution process; (b) external secret manager (Vault/KMS) —
heavyweight for a single-operator MVP; (c) env/mounted secrets — kills the Settings
`/provider-keys` UI workflow and bakes keys into deployment.

### D6. Stack: Go everywhere, internal REST/JSON, sarama, Kafka KRaft
**Choice:** All services + Gateway in Go (reuse the existing `github.com/aaks/server`
module and toolchain). Internal transport is REST/JSON. Kafka client is `sarama`. Kafka
runs in KRaft mode (no Zookeeper) inside docker-compose.

**Rationale / alternatives:** One language + one toolchain + one Kafka client minimizes
operational surface and matches the existing code. *Alternatives rejected:* (a) gRPC
internally — proto codegen + a second transport for no MVP gain; (b) polyglot per service —
multiplies CI/tooling for a CRUD+execution system; (c) Zookeeper + sarama-legacy —
Zookeeper is Kafka-deprecated. (User-specified preference: `sarama` over `franz-go`.)

### D7. Topology: 1 Postgres / 10 logical DBs, separate frontend, session auth
**Choice:** One Postgres container with ten logical databases (`project_db`, `task_db`,
`agent_db`, `catalog_db`, `settings_db`, `runner_db`, `auth_db`, `orgs_db`, `resources_db`,
`admin_db`) for isolation without ten DB processes. The frontend is served as a separate
static build (not embedded in the Gateway). Auth is session-cookie based (httpOnly cookie,
no auth header exists in the SPA client); `/healthz`, `/auth/login`, `/auth/signup`, and
`/auth/signup-status` are unauthenticated, everything else requires a session.

**Rationale / alternatives:** Logical-DB-per-service keeps schema/migration independence
cheaply. *Alternatives rejected:* (a) ten Postgres containers — heavy for local dev;
(b) one shared DB/schema — reintroduces coupling and violates DB-per-service; (c) embedded
frontend in Gateway — user explicitly chose a separate static frontend; (d) bearer-token
auth — the SPA sends no Authorization header, so a cookie is the only contract-compatible
mechanism.

### D8. Session-derived workspace scoping (contract-compatible)
**Choice:** The SPA never sends a workspace parameter — `active_workspace_id` is a
client-side preference in its session store. The Gateway therefore resolves the workspace
context for unscoped endpoints (`/tasks`, `/agents`, `/skills`, `/projects`) as:
(1) an optional `X-Workspace-ID` header (forward-compatible extension; the SPA does not
send it today), (2) the session's single workspace, (3) the union of the session's
workspaces. Explicit `/workspaces/:id/...` paths are authoritative and membership-checked.
The resolved context is forwarded to services as part of the internal call (header/param),
and every entity carries `workspace_id` so services enforce the boundary independently of
the Gateway.

**Rationale / alternatives:** Honoring the fixed contract: the SPA is already built and
cannot be asked to send scope. *Alternatives rejected:* (a) requiring the client to send
`active_workspace_id` — breaks the unchanged-contract constraint; (b) server-side "last
active workspace" persisted at login — the SPA switches workspaces without any backend
call, so the server would be stale; (c) no scoping at all — multi-tenant isolation
requires it.

Consequences: single-workspace users get precise scoping for free; multi-workspace users
see the union on unscoped lists (their explicit scoped paths are always precise). This
matches today's SPA behavior (it renders whatever the endpoint returns) and is recorded
in the workspaces capability.

## Risks / Trade-offs

- **[Saga correctness under redelivery]** duplicate facts could double-advance status or
  rounds. → Mitigation: idempotent saga keyed by `(task_id, run_id)`; consumers dedup
  `step.id`; at-least-once + idempotency is a first-class event-bus requirement.
- **[Kafka operational weight for an MVP]** Kafka adds moving parts vs. in-process pub/sub.
  → Mitigation: KRaft mode (no Zookeeper), single broker for MVP, all in one compose file.
- **[Gateway fan-out latency]** cross-service reads add internal round-trips. → Mitigation:
  batch internal fetches by id; only compose where the frontend actually joins data.
- **[Credential boundary now spans processes]** more surface than a single binary.
  → Mitigation: sole-decryptor model + mTLS + in-memory-only use + container stays pure;
  spec scenarios assert no secret reaches the container or logs.
- **[Stop is best-effort against an in-flight provider call]** a mid-LLM-call stop cannot
  interrupt instantly. → Mitigation: runner cancels context at the next boundary and emits
  a terminal `run.completed`; stop status is set synchronously regardless.
- **[Auth is a real security surface]** session cookies, passwords, and approval flows
  replace the no-auth assumption. → Mitigation: httpOnly session cookies, server-side
  session store, 401/403 enforced at the Gateway and re-validated per service; no
  plaintext passwords stored (hashed); secrets still never reach the sandbox.
- **[Multi-workspace unscoped lists mix workspaces]** the SPA sends no scope, so
  multi-workspace users get the union on unscoped lists. → Mitigation: documented in D8
  and the workspaces capability; every entity carries `workspace_id`, explicit
  `/workspaces/:id/...` paths stay precise, and a forward-compatible `X-Workspace-ID`
  header exists for clients that adopt it.
- **[Sysadmin health needs cross-service data]** `SystemHealth` reports per-service
  status. → Mitigation: the Gateway (the only permitted synchronous caller) fans out
  health probes and composes the response; no service-to-service health coupling.
- **[Supersedes current design doc]** `docs/design.md` (monolith, in-process pub/sub, no
  auth) will diverge. → Mitigation: this change's design is the new source; a docs-sync
  task is in `tasks.md`.

## Migration Plan

Greenfield behind the Gateway — no live data to migrate (Phase 0). Rollout is phased (see
`tasks.md`): foundations → event bus → CRUD services → Gateway → runner → saga → secrets →
compose/E2E. Each phase is independently testable. Rollback within MVP = stop the new
fleet; the Phase-0 `healthz` binary remains the known-good fallback until the first
end-to-end slice passes.

## Open Questions

- **MCP client lifecycle:** whether the MCP client is launched per-run or pooled in the
  Runner is an implementation detail that does not change any spec; defer to apply time.
- **Provider call retries/timeouts:** exact per-provider backoff is tunable at apply time
  without affecting the contract or saga.
- **Observability stack:** logging/metrics/tracing choice (e.g. slog is already in use) can
  be finalized during implementation without changing specs.
