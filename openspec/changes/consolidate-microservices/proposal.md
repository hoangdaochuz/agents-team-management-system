## Why

The backend is over-decomposed into 11 microservices (~20K LOC total) for a single-operator
MVP. Four services (Settings 716 LOC, Project 918 LOC, Admin 812 LOC, Resources 1,342 LOC)
are nano-services with trivial domain logic that don't justify the overhead of their own
container, database, migration pipeline, and Kafka consumer group. The Gateway currently
makes 3+ synchronous fan-out calls per authenticated request (Auth → Orgs → enrichments),
and Auth ↔ Orgs are so tightly coupled that separating them creates distributed transactions
for what is fundamentally a single identity/authorization flow. The system requires 13
containers, 10 logical databases, and ~22 Kafka topics — excessive operational complexity
for the system's actual scale.

## What Changes

- **Merge Auth + Orgs + Admin into a single Identity Service.** Users, sessions, signup,
  organizations, workspaces, memberships, invites, feature flags, and audit log are unified
  under one bounded context ("who you are, what you can access, what has happened").
  **BREAKING** vs. the current 3-service split — container topology, database layout, and
  gateway upstream configuration all change.
- **Merge Project + Task + Catalog + Resources into a single Workspace Service.** Projects,
  tasks, feedback, skills, MCP servers, knowledge sources, plugins, rules, and MCP
  connections are unified under one bounded context ("what work and tools exist in this
  workspace"). The Task saga coordinator gains direct access to project/skill data.
  **BREAKING** vs. the current 4-service split.
- **Merge Agent + Settings into a single Agent Service.** Agent configuration (persona,
  model, tools, attached skills/MCPs) and provider key encryption are combined. The Runner
  calls one service for "agent config + decrypted credentials" instead of two. The
  encryption boundary is maintained within the service. **BREAKING** vs. the current
  2-service split.
- **Keep Runner (Executor) as a standalone service** — unchanged. It manages Docker
  containers, long-running agent loops, sandbox security, and step streaming. Its failure
  modes and resource profile are fundamentally different from CRUD services.
- **Keep Gateway BFF as a standalone service** — simplified from 10 upstream env vars to 4
  (`UPSTREAM_IDENTITY`, `UPSTREAM_WORKSPACE`, `UPSTREAM_AGENT`, `UPSTREAM_EXECUTOR`).
  Session composition becomes a single call to the Identity Service.
- **Consolidate 10 logical databases to 4** (`identity_db`, `workspace_db`, `agent_db`,
  `runner_db`). Same Postgres instance, fewer migration pipelines.
- **Reduce Kafka topics from ~22 to ~8.** Only execution-boundary events survive (task
  commands, run facts, step streaming). Intra-service events (signup flow, catalog
  projections, audit recording) become in-process function calls.
- **Preserve DDD 4-layer architecture** within each consolidated service. Existing
  domain/application/infrastructure/interfaces packages are restructured as internal
  subpackages (e.g., `internal/domain/auth/`, `internal/domain/orgs/`) — no layer violations.
- **Preserve the security model.** Credential-less sandbox, sole-decryptor pattern, and
  mTLS handoff are all maintained. The encryption module remains isolated within the Agent
  Service's infrastructure layer.

## Out of Scope

- **Frontend changes.** The SPA is untouched; all change is behind the Gateway. The REST/SSE
  contract is unchanged.
- **New features.** This is a pure architectural consolidation — no new capabilities, no new
  endpoints, no behavioral changes.
- **Kafka removal.** Kafka is retained for the execution saga (Task↔Runner) and SSE step
  streaming where it genuinely earns its keep.
- **Changing the DDD layering or shared platform packages.** `internal/platform/*` and
  `internal/contracts/*` remain as-is, with contracts potentially simplified.

## Capabilities

### New Capabilities
- `consolidated-identity`: Unified identity, authorization, tenancy, and audit service
  (merges Auth + Orgs + Admin). Covers user lifecycle, sessions, organizations, workspaces,
  memberships, invites, signup approval, feature flags, and audit logging.
- `consolidated-workspace`: Unified workspace management service (merges Project + Task +
  Catalog + Resources). Covers projects, tasks, feedback, task lifecycle saga, skills, MCP
  servers, knowledge sources, plugins, rules, and MCP connections.
- `consolidated-agent`: Unified agent configuration and credential service (merges Agent +
  Settings). Covers agent definitions, skill/MCP attachments, and encrypted provider key
  management.
- `consolidated-gateway`: Simplified gateway routing with 4 upstream services instead of 10.
  Covers reduced fan-out composition, simplified session resolution, and streamlined SSE.
- `consolidated-event-bus`: Reduced Kafka topic catalog (~8 topics) retaining only
  execution-boundary events. Covers topic pruning, consumer group simplification, and
  in-process event replacement for intra-service flows.

### Modified Capabilities
- None. No `openspec/specs/` exist yet; the existing `event-driven-microservices-backend`
  change's specs were never synced to the main spec directory.

## Impact

- **Code (refactor):** Go services under `backend/services/` restructured: `auth/`, `orgs/`,
  `admin/` → `identity/`; `project/`, `task/`, `catalog/`, `resources/` → `workspace/`;
  `agent/`, `settings/` → `agent/` (enhanced); `runner/` → `executor/` (renamed). Old
  service directories deleted. `cmd/main.go` composition roots rewritten for each
  consolidated service. Internal packages become subpackages within the new service structure.
- **APIs:** The frontend-facing REST/SSE contract is **unchanged**. Internal service-to-service
  API surfaces are simplified (fewer upstream services, fewer Kafka topics).
- **Dependencies:** No new Go dependencies. Kafka and Postgres versions unchanged.
- **Deploy:** `deploy/docker-compose.yml` simplified from 13 containers to 7 (4 services +
  Gateway + Postgres + Kafka). `deploy/postgres/01-create-databases.sql` reduced to 4
  databases. Environment variable count reduced substantially.
- **Docs:** `AGENTS.md`, `CLAUDE.md`, `docs/design.md`, and `docs/tasks.md` updated to
  reflect the new 5-service topology.
