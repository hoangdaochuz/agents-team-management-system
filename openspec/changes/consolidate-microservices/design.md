## Context

The backend is over-decomposed into 11 microservices (~20K LOC total) for a single-operator MVP.
Four services (Settings 716 LOC, Project 918 LOC, Admin 812 LOC, Resources 1,342 LOC) are
nano-services with trivial domain logic that don't justify the overhead of their own container,
database, migration pipeline, and Kafka consumer group. The Gateway currently makes 3+ synchronous
fan-out calls per authenticated request (Auth → Orgs → enrichments), and Auth ↔ Orgs are so
tightly coupled that separating them creates distributed transactions for what is fundamentally
a single identity/authorization flow.

The re-decomposition consolidates 11 services into 5 services + Gateway BFF, aligned to true
bounded contexts, reducing operational complexity by ~60%.

## Goals / Non-Goals

**Goals:**
- Consolidate 11 microservices into 5 (Identity, Workspace, Agent, Executor, Gateway) aligned to true bounded contexts
- Merge Auth + Orgs + Admin into a single Identity Service with unified `identity_db`
- Merge Project + Task + Catalog + Resources into a single Workspace Service with unified `workspace_db`
- Merge Agent + Settings into a single Agent Service with unified `agent_db`
- Keep Executor (Runner) as a standalone service — unchanged, the one service that genuinely deserves isolation
- Keep Gateway BFF — simplified from 10 upstream env vars to 4 (`UPSTREAM_IDENTITY`, `UPSTREAM_WORKSPACE`, `UPSTREAM_AGENT`, `UPSTREAM_EXECUTOR`)
- Consolidate 10 logical databases to 4 (`identity_db`, `workspace_db`, `agent_db`, `runner_db`)
- Reduce Kafka topics from 21 to 9, retaining only execution-boundary events
- Preserve DDD 4-layer architecture within each consolidated service
- Preserve the security model: credential-less sandbox, sole-decryptor pattern, mTLS handoff
- No behavioral changes to the frontend-facing REST/SSE API contract

**Non-Goals:**
- New features — this is a pure architectural consolidation
- Frontend changes — the SPA is untouched; all change is behind the Gateway
- Kafka removal — Kafka is retained for the execution saga and SSE step streaming
- Changing the DDD layering or shared platform packages — `internal/platform/*` and `internal/contracts/*` remain as-is
- Changing the overall API shape — frontend-facing endpoints remain the same

## Decisions

### D1. 5-service consolidation aligned to bounded contexts
**Choice:** Consolidate 11 services into 5:
- **Identity** = Auth + Orgs + Admin (who you are + what you can access)
- **Workspace** = Project + Task + Catalog + Resources (what work + tools exist)
- **Agent** = Agent + Settings (how agents are configured)
- **Executor** = Runner (run tasks in sandboxed containers) — unchanged
- **Gateway** = Gateway BFF (route, auth, compose, SSE) — simplified

**Rationale / alternatives:** This aligns services with genuine bounded contexts rather than arbitrary splits. The original 11-service split created nano-services with less domain logic than a typical monolith controller. *Alternatives rejected:* (a) keeping all 11 services — excessive ops cost for one operator; (b) consolidating to 3 services — too coarse, loses meaningful isolation of the Executor; (c) modular monolith — contradicts the microservices goal but without the operational benefits.

### D2. CRUD synchronous, execution async over Kafka (preserved)
**Choice:** The Gateway handles CRUD synchronously (forward → service → DB → respond). Only execution/lifecycle side-effects travel over Kafka: task commands (`task.run-requested`, `task.review-requested`, `task.stop-requested`, `task.pr-open-requested`) and facts (`step`, `run.completed`, `finding`, `verdict`, `pr.opened`). All other previously-defined topics become in-process events or synchronous calls.

**Rationale / alternatives:** The frontend expects synchronous resource returns; an all-async model would break the contract. *Alternatives rejected:* (a) all-async write model — breaks SPA optimistic updates; (b) Kafka only for SSE fan-out — not an "event-driven backbone"; (c) in-process pub/sub only — loses the ordering/guarantees needed for cross-service execution coordination.

### D3. Gateway = sole synchronous caller; session composition simplified
**Choice:** Only the Gateway makes synchronous calls into services. Session composition becomes a single call to the Identity Service (`/internal/identity`) instead of the previous 3-call fan-out (Auth → Orgs → enrichment). The Gateway also owns the SSE connection: on connect it replays persisted steps from the Runner, then tails the Kafka `step` topic filtered by `task_id`.

**Rationale / alternatives:** Centralizing composition in the Gateway keeps services decoupled and gives the frontend one stable endpoint. *Alternatives rejected:* (a) CQRS projections via events — stale reads + too many consumers for MVP; (b) direct service-to-service sync calls — re-couples services and creates cascading-failure risk; (c) Runner serves SSE directly — couples execution to long-lived client connections.

### D4. Database consolidation: 10 logical DBs → 4
**Choice:** Consolidate 10 logical databases to 4:
- `identity_db`: merges auth_db, orgs_db, admin_db
- `workspace_db`: merges project_db, task_db, catalog_db, resources_db
- `agent_db`: merges agent_db, settings_db
- `runner_db`: unchanged

Single migration pipeline creates all tables across the 4 databases. The previous 10 pipelines are reduced to 4.

**Rationale / alternatives:** These databases have no isolation benefit — they share the same Postgres instance, same credentials, same failure domain. The only thing they add is migration complexity and cross-service join impossibility. *Alternatives rejected:* (a) keeping 10 databases — perpetuates the over-decomposed state; (b) 1 shared DB — reintroduces coupling and violates the per-service schema independence that was the original rationale.

### D5. Kafka topic reduction: 21 → 9
**Choice:** Retain only these topics (the only ones that genuinely need cross-service async processing):
- Commands: `task.run-requested`, `task.review-requested`, `task.stop-requested`, `task.pr-open-requested`
- Facts: `step`, `run.completed`, `finding`, `verdict`, `pr.opened`

All other previously-defined topics (`signup.*`, `invite.created`, `workspace.created`, `mcp.created/deleted`, `skill.created/deleted`, `audit.recorded`, `run.started`, `task.status-changed`) become in-process events or synchronous calls, or are dropped outright (`run.started` and `task.status-changed` currently have no consumers).

**Rationale / alternatives:** Of the 21 Kafka topics, only the execution boundary events truly need async, partitioned, at-least-once delivery. The rest are simple projections or CRUD events that add unnecessary cross-service coupling. *Alternatives rejected:* (a) keeping all topics — perpetuates the operational overhead; (b) removing Kafka entirely — loses the ordering/guarantees needed for the execution saga.

### D6. DDD layering preservation
**Choice:** Each consolidated service preserves the DDD 4-layer architecture (domain/application/infrastructure/interfaces). Existing domain/application/infrastructure packages are restructured as internal subpackages within the new service (e.g., `internal/domain/auth/`, `internal/domain/orgs/`, `internal/domain/admin/` → `internal/domain/identity/`). No layer violations are introduced.

**Rationale / alternatives:** The DDD layering is already clean in the existing code. The merge is primarily a composition-root refactoring — internal packages can be preserved as-is within a multi-aggregate service. *Alternatives rejected:* (a) flattening all layers into one — loses the architectural guardrails; (b) introducing a god-package — contrary to the DDD refactoring that was already completed.

### D7. Security model preservation
**Choice:** The following security properties are maintained:
- **Credential-less sandbox**: Provider keys never reach container env/filesystem/logs
- **Sole-decryptor pattern**: Settings (now within Agent Service) is the sole decryptor of provider keys via internal token channel
- **mTLS handoff**: Gateway-to-service mTLS is maintained with the simplified upstream config
- **No provider key/git token leakage**: The credential-less-sandbox invariant carries over

**Rationale / alternatives:** These properties are load-bearing across process boundaries. *Alternatives rejected:* (a) spreading decryption capability — spreads the secret surface; (b) env/mounted secrets — defeats the credential-less-sandbox invariant.

### D8. API contract compatibility
**Choice:** The frontend-facing REST/SSE API contract is unchanged. All endpoint paths, request shapes, and response shapes are identical to the pre-consolidation API. The only internal difference is that the Gateway resolves these endpoints against fewer upstream services.

**Rationale / alternatives:** The change is a pure architectural consolidation behind the Gateway. *Alternatives rejected:* (a) changing endpoint paths — would require frontend updates and break compatibility; (b) adding/removing endpoints — not part of the scope.

## Risks / Trade-offs

- **[Saga correctness under redelivery]** duplicate facts could double-advance status or rounds. → Mitigation: idempotent saga keyed by `(task_id, run_id)`; consumers dedup `step.id`; at-least-once + idempotency is a first-class event-bus requirement.
- **[Kafka operational weight reduction]** fewer topics means simpler deployment but fewer cross-service event observation points. → Mitigation: surviving topics still provide ordering/guarantees for execution; in-process events are well-defined within service boundaries.
- **[Gateway fan-out reduction]** fewer upstream services means simpler composition but less per-service granularity. → Mitigation: the Identity/Workspace/Agent services each own their bounded context completely; the Gateway's single-call composition is faster and more reliable than multi-hop fan-out.
- **[Database consolidation risk]** merging databases could lose perceived isolation. → Mitigation: all 4 databases share the same Postgres instance anyway; the consolidation actually simplifies migration pipelines and enables intra-database joins where previously cross-database joins were impossible.
- **[Security model preservation risk]** merging Auth+Orgs+Admin could expand the secret surface. → Mitigation: the encryption boundary is maintained within the Identity Service; provider keys still never reach container env/filesystem/logs; the sole-decryptor pattern is preserved.
- **[DDD layer migration risk]** restructuring internal packages could introduce layer violations. → Mitigation: existing layering is preserved; the archlint test from the refactor-backend-ddd change continues to enforce no-infra-imports-from-domain.

## Migration Plan

Phase 1: Merge Auth + Orgs + Admin → Identity Service
1. Create `services/identity/` with combined `cmd/main.go` composition root
2. Move domain/application/infrastructure packages from all three services as internal subpackages (e.g., `internal/domain/identity/`)
3. Consolidate databases: single migration pipeline creating all tables in `identity_db`
4. Replace inter-service Kafka events with in-process event bus
5. Update Gateway: `UPSTREAM_AUTH` + `UPSTREAM_ORGS` + `UPSTREAM_ADMIN` → `UPSTREAM_IDENTITY`

Phase 2: Merge Project + Task + Catalog + Resources → Workspace Service
1. Same pattern: combined composition root, subpackage domains
2. Catalog→Resources projections become function calls
3. Task saga coordinator gains direct access to project/skill data
4. Update Gateway routing

Phase 3: Merge Agent + Settings → Agent Service
1. Combine composition roots
2. Provider key encryption module stays isolated within `internal/infrastructure/crypto/`
3. Update Runner's credential fetch to point at Agent Service

Phase 4: Clean Up
1. Remove 6 Dockerfiles, 6 DB init scripts, 12 Kafka topics
2. Simplify `deploy/docker-compose.yml` from 13 containers to 7
3. Update E2E tests

## Open Questions

- **MCP client lifecycle**: whether the MCP client is launched per-run or pooled in the Executor is an implementation detail that does not change any spec; defer to apply time.
- **Provider call retries/timeouts**: exact per-provider backoff is tunable at apply time without affecting the contract or saga.
- **Observability stack**: logging/metrics/tracing choice (e.g. slog is already in use) can be finalized during implementation without changing specs.