## Why

The frontend SPA is fully built against a declared API contract (`frontend/src/api/*.ts`:
~40 endpoints across 8 domains plus one SSE stream), but the backend is still Phase 0
(only `GET /healthz`). The current `docs/design.md` prescribes a single Go binary with
in-process pub/sub for realtime. We want the backend that fulfills the frontend contract
to be **microservices**, **event-driven**, with **Kafka** as the message-broker backbone —
while keeping the frontend↔backend boundary strictly **synchronous REST** so the existing
frontend contract is preserved unchanged.

## What Changes

- **Introduce a service topology of 6 bounded-context services + a Gateway/BFF**, all in
  Go, each owning its own logical database: Project, Task, Agent, Catalog (Skills + MCP),
  Settings (provider keys + secrets), and Agent-Runner (the agent loop, LLM/MCP/git,
  worktree + sandbox; owns Run/Step/Finding/Artifact). **BREAKING** vs. the current
  single-binary design.
- **Introduce an API Gateway/BFF** as the frontend's sole entrypoint: it serves the exact
  REST surface the frontend declares, performs synchronous fan-out to compose
  cross-service reads, and serves the SSE stream (replay persisted steps, then tail Kafka).
- **Adopt Kafka (KRaft mode) as the event-driven backbone** using the `sarama` client.
  Commands (`task.run-requested`, `task.review-requested`, `task.stop-requested`) and facts
  (`step.*`, `run.completed`, `finding.*`, `verdict`, `pr.opened`, `task.status-changed`)
  flow over Kafka. CRUD reads/writes remain synchronous.
- **Make the Task service the saga coordinator** for task lifecycle via choreography: it
  owns task status + review `round_no` (≤5), emits run/review/stop commands, and consumes
  run/verdict/pr facts to advance state. Stop aborts an in-flight Runner loop.
- **Re-architect the secret/credential boundary for multi-process**: Settings service is
  the sole decryptor (holds the master key); plaintext provider keys + git token are
  handed to Agent-Runner over mTLS, in-memory only, and **never** placed in the sandbox
  container. The credential-less-sandbox invariant is preserved.
- **Realtime (SSE)** moves from in-process pub/sub to Kafka-backed: the Gateway subscribes
  to `step.*` (keyed by `task_id`), replays persisted steps, then tails live events; the
  SSE event is named `step` carrying the full `Step` shape, with reconnect/dedup by `step.id`.
- **Deployment topology**: one Postgres container with 6 logical databases; the frontend
  served as a separate static build (not embedded in the Gateway); Kafka in KRaft mode;
  no auth (single-operator MVP).
- **Supersedes** the single-binary / in-process-pubsub portions of `docs/design.md`
  (architecture, realtime, and secret-handling ADRs). All load-bearing invariants are
  preserved: PRs never auto-merged (ADR-05), worktree-per-task, credential-less sandbox,
  reviewer ≤5 rounds.

## Capabilities

### New Capabilities
- `api-gateway`: Frontend's sole REST+SSE entrypoint — route table matching
  `frontend/src/api/*.ts`, synchronous fan-out composition for cross-service reads, SSE
  serving (replay + Kafka tail), request-id/recovery middleware.
- `project-management`: Project entity CRUD (`/projects`) backed by the Project service DB.
- `task-management`: Task CRUD, feedback, status patch, and the task-lifecycle saga
  (run/review/stop/open-pr) coordinated by the Task service.
- `agent-management`: Agent CRUD plus attach/detach Skill and MCP references.
- `skill-mcp-catalog`: Skill and McpServer CRUD (the Catalog service).
- `provider-key-settings`: ProviderKey set/list/update/delete and the sole-decryptor
  secret flow (Settings service decrypts; mTLS handoff to Runner).
- `agent-execution-runner`: The agent loop (LLM + MCP client + git), worktree-per-task,
  Docker-exec sandbox, ownership of Run/Step/Finding/Artifact, event emission, reviewer
  variant, and stop/context-cancel handling.
- `event-bus`: Kafka topic catalog, partitioning (key = `task_id`), consumer groups,
  at-least-once delivery with idempotency, KRaft deployment.
- `realtime-streaming`: SSE `step` event contract, replay-then-tail semantics, reconnect,
  and dedup by `step.id`.

### Modified Capabilities
- None. No `openspec/specs/` exist yet; this change establishes the baseline specs.

## Impact

- **Code (new):** Go services under `backend/services/{gateway,project,task,agent,catalog,
  settings,runner}/`; shared `backend/internal/{contracts,kafka,db}`; the existing
  `backend/cmd/server` + `internal/{config,httpapi}` become the Gateway base; the existing
  `backend/runner/Dockerfile` (credential-less sandbox) is reused by Agent-Runner.
- **APIs:** the frontend-facing REST/SSE contract is **unchanged**; all change is behind
  the Gateway. New internal REST/JSON service APIs and Kafka topic contracts are introduced.
- **Dependencies (new):** Kafka (KRaft), `sarama` Go client, mTLS CA for Settings↔Runner,
  per-service Postgres logical DBs + migrations.
- **Deploy:** `deploy/docker-compose.yml` and `deploy/.env.example` grow to include Kafka
  and the service fleet (frontend served separately).
- **Docs:** `docs/design.md` architecture/realtime/secret ADRs are superseded by this
  change's `design.md`; `docs/tasks.md` phased scope is replaced by this change's `tasks.md`.
- **Security:** the core property ("backend owns everything sensitive; container is a pure
  sandbox") is preserved and sharpened — credentials now cross process boundaries only via
  the Settings→Runner mTLS decrypt handoff, never entering the container.
