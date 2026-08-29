## Purpose

The backend is over-decomposed into 11 microservices (~20K LOC total) for a single-operator MVP.
Four services (Settings 716 LOC, Project 918 LOC, Admin 812 LOC, Resources 1,342 LOC) are
nano-services with trivial domain logic that don't justify the overhead of their own container,
database, migration pipeline, and Kafka consumer group. The Gateway currently makes 3+ synchronous
fan-out calls per authenticated request (Auth → Orgs → enrichments), and Auth ↔ Orgs are so
tightly coupled that separating them creates distributed transactions for what is fundamentally
a single identity/authorization flow.

The recommended re-decomposition consolidates 11 services into 5 services + Gateway BFF,
aligned to true bounded contexts, reducing operational complexity by ~60%.

## Consolidated Service Map

| Service | Merged From | Bounded Context | Key Entities |
|:--------|:------------|:----------------|:------------|
| **Identity** | Auth + Orgs + Admin | Who you are + what you can access | users, sessions, organizations, workspaces, members, invites, feature_flags, audit_log |
| **Workspace** | Project + Task + Catalog + Resources | What work + tools exist | projects, tasks, feedback, skills, mcp_servers, knowledge_sources, plugins, rules, mcp_connections |
| **Agent** | Agent + Settings | How agents are configured | agents, agent_skills, agent_mcps, provider_keys |
| **Executor** | Runner (unchanged) | Run tasks in sandboxed containers | runs, steps, findings, artifacts |
| **Gateway** | Gateway BFF (simplified) | Route, auth, compose, SSE | routing, session composition, SSE fan-out |

---

## ADDED Requirements

### Requirement: Identity Service consolidation
The service SHALL merge Auth (1,982 LOC) + Orgs (2,408 LOC) + Admin (812 LOC) into a single
Identity Service with a unified database schema (`identity_db`).
- **Merged entities**: users, sessions, organizations, workspaces, members, invites, signup_requests, feature_flags, audit_log
- **Rationale**: Auth and Orgs are tightly coupled (session → user → workspace memberships). Admin is pure reads against the same entities. The Gateway currently makes 3 synchronous calls to compose a session; merging turns these into local function calls.
- **Bounded Context**: "Who are you, what can you access, and what has happened?" — Identity, authorization, tenancy, and audit form a single bounded context.
- **Kafka**: Producer: `signup.*`, `invite.created`, `workspace.created`, `audit.recorded`. Consumer: same (internal projection). Most become in-process events.

#### Scenario: Signup approval without distributed transactions
- **WHEN** an operator approves a signup request
- **THEN** user activation, org/workspace creation, membership assignment, and audit recording all complete inside the Identity Service as local operations with no cross-service calls or Kafka events

#### Scenario: Session composed in one call
- **WHEN** the Gateway asks the Identity Service to resolve a session
- **THEN** one HTTP call returns user, workspaces, role, and superadmin flag — no Auth→Orgs fan-out

### Requirement: Workspace Service consolidation
The service SHALL merge Project (918 LOC) + Task (2,130 LOC) + Catalog (1,526 LOC) + Resources (1,342 LOC) into a single Workspace Service with a unified database schema (`workspace_db`).
- **Merged entities**: projects, tasks, task_thread, feedback, skills, mcp_servers, knowledge_sources, plugins, rules, mcp_connections
- **Rationale**: These are all workspace-scoped entities that users manage together. Projects contain tasks, tasks use agents/skills/MCPs from the catalog, resources are workspace-level configurations. The Catalog→Resources Kafka projection becomes a simple local function call. Task saga coordinator gains direct access to project/skill data.
- **Bounded Context**: "What work exists in this workspace, and what tools/resources are available?" — The workspace's complete operational surface.
- **Kafka**: Producer: `task.run-requested`, `task.review-requested`, `task.stop-requested`, `task.pr-open-requested`. Consumer: `run.completed`, `finding`, `verdict`, `pr.opened`. (These are the only events that truly need Kafka — they cross the Workspace→Executor boundary.)

#### Scenario: Catalog projection is a function call
- **WHEN** an MCP server is created in the catalog
- **THEN** the resource projection updates as an in-process call inside the Workspace Service — no Kafka publish

#### Scenario: Saga reads project data locally
- **WHEN** the task saga coordinator needs the parent project's repository settings to dispatch a run
- **THEN** it reads them directly from `workspace_db` without a cross-service HTTP call

### Requirement: Agent Service consolidation
The service SHALL merge Agent (1,389 LOC) + Settings (716 LOC) into a single Agent Service with a unified database schema (`agent_db`).
- **Merged entities**: agents, agent_skills, agent_mcps, provider_keys
- **Rationale**: Agent configuration (persona, model, tools, attached skills/MCPs) is deeply linked to provider keys (which model to call with which key). The Runner already calls Settings internally to fetch decrypted keys — merging means the Executor calls one service for "give me agent config + decrypted credentials" instead of two. The encryption boundary is maintained within the service.
- **Bounded Context**: "How are agents configured and credentialed?" — Agent identity and the secrets that power them.
- **Kafka**: Consumer: `skill.created`, `skill.deleted` (catalog projections) — could become a direct HTTP call from Workspace Service on write.

#### Scenario: Executor fetches config and credentials from one service
- **WHEN** the Executor starts a run and needs the agent's persona/model plus a decrypted provider key
- **THEN** both requests go to the Agent Service (its `/internal/keys/{provider}` and `/internal/agents/{id}/mcp-servers` endpoints) instead of the former separate Settings and Agent services — one upstream, two calls

### Requirement: Executor Service preservation
The Executor Service (Runner, 4,064 LOC) SHALL remain as a standalone service — unchanged.
- **Rationale**: The Runner is the one service that genuinely deserves isolation: it manages Docker containers, long-running agent loops, sandbox security, step streaming, and LLM provider calls. Its failure modes (container crashes, OOM, timeouts) are fundamentally different from CRUD services.
- **Kafka**: Producer: `step`, `run.completed`, `finding`, `verdict`, `pr.opened`. Consumer: `task.run-requested`, `task.review-requested`, `task.stop-requested`, `task.pr-open-requested`. (`run.started` is dropped — it has no consumer.) (This is where Kafka earns its keep — async, ordered, at-least-once delivery for long-running operations.)

#### Scenario: Runner behavior preserved through rename
- **WHEN** `services/runner/` is renamed to `services/executor/`
- **THEN** the Docker sandbox driver, agent loop, worktree-per-task management, and step persistence/streaming behave identically

#### Scenario: Executor failure does not take down CRUD
- **WHEN** the Executor crashes or its container pool is exhausted
- **THEN** the Identity, Workspace, Agent, and Gateway services keep serving CRUD and session requests

### Requirement: Gateway BFF simplification
The Gateway BFF SHALL be simplified from 10 upstream env vars to 4 (`UPSTREAM_IDENTITY`, `UPSTREAM_WORKSPACE`, `UPSTREAM_AGENT`, `UPSTREAM_EXECUTOR`).
- **Session composition**: becomes a single call to Identity Service instead of Auth + Orgs.
- **SSE composition**: unchanged pattern but with reduced upstream dependencies.
- **Upstream reduction**: 10 → 4 environment variables.

#### Scenario: All frontend routes resolve against 4 upstreams
- **WHEN** every route in the gateway route table is resolved post-consolidation
- **THEN** each maps to one of identity, workspace, agent, or executor — no route references a removed upstream

### Requirement: Database consolidation
The system SHALL consolidate 10 logical databases to 4 (`identity_db`, `workspace_db`, `agent_db`, `runner_db`).
- **Current**: 10 logical databases each with their own migration pipeline, sharing the same Postgres instance and credentials.
- **Target**: 4 databases, single migration pipeline, same Postgres instance.
- **Databases merged**:
  - `identity_db`: merges auth_db, orgs_db, admin_db
  - `workspace_db`: merges project_db, task_db, catalog_db, resources_db
  - `agent_db`: merges agent_db, settings_db
  - `runner_db`: unchanged
- **Rationale**: These databases have no isolation benefit — they share the same Postgres instance, same credentials, same failure domain. The only thing they add is migration complexity and cross-service join impossibility.

#### Scenario: Database init script creates 4 databases
- **WHEN** `deploy/postgres/01-create-databases.sql` runs on a fresh Postgres
- **THEN** exactly `identity_db`, `workspace_db`, `agent_db`, and `runner_db` are created

### Requirement: Kafka topic reduction
The event bus SHALL reduce Kafka topics from 21 to 9, retaining only execution-boundary events.
**Keep (execution boundary — genuinely async)**:
| Topic | Producer → Consumer |
|:------|:-------------------|
| `task.run-requested` | Workspace → Executor |
| `task.review-requested` | Workspace → Executor |
| `task.stop-requested` | Workspace → Executor |
| `task.pr-open-requested` | Workspace → Executor |
| `step` | Executor → Gateway (SSE) |
| `run.completed` | Executor → Workspace |
| `finding` | Executor → Workspace |
| `verdict` | Executor → Workspace |
| `pr.opened` | Executor → Workspace |

**Remove (become in-process, synchronous, or dropped)**:
| Topic | Why Unnecessary |
|:------|:---------------|
| `signup.requested/approved/declined` | All within Identity Service now |
| `invite.created` | All within Identity Service now |
| `workspace.created` | Identity can call Workspace directly, or Workspace polls |
| `mcp.created/deleted` | All within Workspace Service now |
| `skill.created/deleted` | Workspace→Agent can be a sync call on write |
| `audit.recorded` | All within Identity Service now |
| `run.started` | Executor→Agent projection; no consumer exists — dropped |
| `task.status-changed` | Internal to Workspace Service now; no consumer exists — dropped |

#### Scenario: Only execution-boundary topics remain
- **WHEN** the consolidated system's Kafka catalog is inspected
- **THEN** exactly the 9 execution-boundary topics exist; every other former topic is handled in-process, synchronously, or not at all

### Requirement: DDD layering preservation
Each consolidated service SHALL preserve the DDD 4-layer architecture (domain/application/infrastructure/interfaces).
- Existing domain/application/infrastructure packages are restructured as internal subpackages within the new service (e.g., `internal/domain/auth/`, `internal/domain/orgs/`, `internal/domain/admin/` → `internal/domain/identity/`).
- No layer violations are introduced during the merge.
- Application tests use hand-rolled fakes; UnitOfWork only where multi-aggregate mutations exist.

#### Scenario: archlint still enforces dependencies
- **WHEN** the `archlint` test runs against the consolidated services
- **THEN** it fails if any domain or application package imports infrastructure, pgx, sarama, or platform transport packages — same rule as before the merge

### Requirement: Security model preservation
The following security properties SHALL be maintained:
- **Credential-less sandbox**: Provider keys never reach container env/filesystem/logs.
- **Sole-decryptor pattern**: Settings (now within Agent Service) is the sole decryptor of provider keys via internal token channel.
- **Internal-channel hardening**: the Agent Service's plaintext-key endpoint is gated by the shared internal token and, when `AGENT_MTLS=on`, by a mutually authenticated TLS listener (the same env-gated pipeline the pre-consolidation Settings service exposed as `SETTINGS_MTLS`). Gateway→upstream traffic runs plain HTTP on the trusted compose network in the MVP deployment — there was no gateway mTLS pipeline before consolidation to maintain.
- **No provider key/git token leakage**: The credential-less-sandbox invariant carries over from the old design.

#### Scenario: Sandbox secret-leak test still passes
- **WHEN** the sandbox secret-leak test inspects a running task container's environment, filesystem, and logs
- **THEN** it finds no provider keys and no git credentials — all LLM calls and git operations ran on the host backend

#### Scenario: Key decryption stays token-gated
- **WHEN** the Executor fetches a decrypted provider key from the Agent Service
- **THEN** the request carries the shared internal token, and with `AGENT_MTLS=on` the TLS peer is verified — no other caller can obtain plaintext key material

---

## Out of Scope

- **Frontend changes**: The SPA is untouched; all change is behind the Gateway. The REST/SSE contract is unchanged.
- **New features**: This is a pure architectural consolidation — no new capabilities, no new endpoints, no behavioral changes.
- **Kafka removal**: Kafka is retained for the execution saga (Task↔Runner) and SSE step streaming where it genuinely earns its keep.
- **Changing the DDD layering or shared platform packages**: `internal/platform/*` and `internal/contracts/*` remain as-is, with contracts potentially simplified.
- **Changing the overall API shape**: Frontend-facing REST/SSE endpoints remain the same; only internal service boundaries change.

---

## Impact

- **Code (refactor)**: Go services under `backend/services/` restructured: `auth/`, `orgs/`, `admin/` → `identity/`; `project/`, `task/`, `catalog/`, `resources/` → `workspace/`; `agent/`, `settings/` → `agent/` (enhanced); `runner/` → `executor/` (renamed). Old service directories deleted. `cmd/main.go` composition roots rewritten for each consolidated service. Internal packages become subpackages within the new service structure.
- **APIs**: The frontend-facing REST/SSE contract is **unchanged**. Internal service-to-service API surfaces are simplified (fewer upstream services, fewer Kafka topics).
- **Dependencies**: No new Go dependencies. Kafka and Postgres versions unchanged.
- **Deploy**: `deploy/docker-compose.yml` simplified from 13 containers to 7 (4 services + Gateway + Postgres + Kafka, plus a one-shot kafka-init helper that pre-creates __consumer_offsets). `deploy/postgres/01-create-databases.sql` reduced to 4 databases. Environment variable count reduced substantially.
- **Docs**: `AGENTS.md`, `CLAUDE.md`, `docs/design.md`, and `docs/tasks.md` updated to reflect the new 5-service topology.
