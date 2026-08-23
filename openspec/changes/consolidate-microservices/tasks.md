# Tasks — consolidate-microservices

Phased implementation checklist for `/opsx:apply`. Each phase is independently testable.
Reference `specs/` for behavior and `design.md` for approach. The Go module stays
`github.com/aaks/server` with `go.mod` in `backend/`.

## Phase 1: Merge Auth + Orgs + Admin → Identity Service

- [ ] 1.1 Create `services/identity/` directory with `cmd/main.go` composition root
- [ ] 1.2 Move domain packages from Auth, Orgs, Admin as internal subpackages:
    - `internal/domain/identity/` combining auth, org, admin domains
    - Preserve existing domain/application/infrastructure layering
- [ ] 1.3 Consolidate databases: single migration pipeline creating all tables in `identity_db`
    - Merge: users, sessions, organizations, workspaces, members, invites, signup_requests,
      feature_flags, audit_log from auth_db, orgs_db, admin_db
- [ ] 1.4 Replace inter-service Kafka events with in-process event bus
    - `signup.*`, `invite.created`, `workspace.created`, `audit.recorded` become local events
- [ ] 1.5 Update Gateway: `UPSTREAM_AUTH` + `UPSTREAM_ORGS` + `UPSTREAM_ADMIN` → `UPSTREAM_IDENTITY`
- [ ] 1.6 Run `go vet ./... && go build ./...` — ensure no layer violations, compilation passes
- [ ] 1.7 Run existing service tests with hand-rolled fakes to confirm no regressions

## Phase 2: Merge Project + Task + Catalog + Resources → Workspace Service

- [ ] 2.1 Create `services/workspace/` directory with `cmd/main.go` composition root
- [ ] 2.2 Move domain packages from Project, Task, Catalog, Resources as internal subpackages:
    - `internal/domain/workspace/` combining project, task, catalog, resource domains
- [ ] 2.3 Consolidate databases: single migration pipeline creating all tables in `workspace_db`
    - Merge: projects, tasks, task_thread, feedback, skills, mcp_servers,
      knowledge_sources, plugins, rules, mcp_connections from project_db, task_db, catalog_db, resources_db
- [ ] 2.4 Catalog→Resources projections become local function calls (no Kafka needed)
- [ ] 2.5 Task saga coordinator gains direct access to project/skill data (no cross-service calls)
- [ ] 2.6 Update Gateway routing for workspace endpoints
- [ ] 2.7 Run `go vet ./... && go build ./...` — ensure compilation and layer integrity
- [ ] 2.8 Run existing service tests to confirm no regressions

## Phase 3: Merge Agent + Settings → Agent Service

- [ ] 3.1 Create `services/agent/` directory with enhanced `cmd/main.go` composition root
- [ ] 3.2 Move domain packages from Agent, Settings as internal subpackages:
    - `internal/domain/agent/` combining agent config + provider key encryption
    - Provider key encryption module stays isolated within `internal/infrastructure/crypto/`
- [ ] 3.3 Consolidate databases: single migration pipeline creating all tables in `agent_db`
    - Merge: agents, agent_skills, agent_mcps, provider_keys from agent_db, settings_db
- [ ] 3.4 Update Runner's credential fetch to point at Agent Service (single call instead of two)
- [ ] 3.5 Preserve encryption boundary: provider keys encrypted at rest, master key in memory only
- [ ] 3.6 Run `go vet ./... && go build ./...` — ensure compilation and layer integrity
- [ ] 3.7 Run existing service tests to confirm no regressions

## Phase 4: Executor Service (unchanged, renaming Runner → Executor)

- [ ] 4.1 Rename `services/runner/` → `services/executor/` (preserving all internal structure)
- [ ] 4.2 Update `cmd/main.go` composition root if needed for new package path
- [ ] 4.3 Kafka topics unchanged: `step`, `run.completed`, `finding`, `verdict`, `pr.opened`,
  `run.started` as producer; `task.run-requested`, `task.review-requested`, `task.stop-requested` as consumer
- [ ] 4.4 Run `go vet ./... && go build ./...` — verify unchanged service compiles
- [ ] 4.5 Run existing runner/executor tests

## Phase 5: Gateway BFF simplification

- [ ] 5.1 Reduce upstream env vars from 10 to 4:
    - `UPSTREAM_IDENTITY` (replaces `UPSTREAM_AUTH`, `UPSTREAM_ORGS`, `UPSTREAM_ADMIN`)
    - `UPSTREAM_WORKSPACE` (replaces `UPSTREAM_PROJECT`, `UPSTREAM_TASK`, `UPSTREAM_CATALOG`, `UPSTREAM_RESOURCES`)
    - `UPSTREAM_AGENT` (unchanged)
    - `UPSTREAM_EXECUTOR` (replaces `UPSTREAM_RUNNER`)
- [ ] 5.2 Simplify session composition: single call to Identity Service instead of Auth→Orgs fan-out
- [ ] 5.3 Update Gateway health probes to check 4 downstream services instead of 11
- [ ] 5.4 Run `go vet ./... && go build ./...` — verify Gateway compiles with reduced config
- [ ] 5.5 Verify frontend API contract unchanged — run `make web-typecheck && make web-build`

## Phase 6: Database and Kafka consolidation

- [ ] 6.1 Update `deploy/postgres/01-create-databases.sql` from 10 databases to 4:
    - `identity_db`, `workspace_db`, `agent_db`, `runner_db`
- [ ] 6.2 Reduce Kafka topic catalog from ~22 to ~8 surviving topics:
    - Keep: `task.run-requested`, `task.review-requested`, `task.stop-requested`,
      `step.*`, `run.completed`, `finding.*`, `verdict`, `task.status-changed`
    - Remove: `signup.*`, `invite.created`, `workspace.created`, `mcp.*`, `skill.*`,
      `audit.recorded`, `run.started`, `pr.opened`
- [ ] 6.3 Update Kafka consumer groups to match surviving topics only
- [ ] 6.4 Run `go vet ./... && go build ./...` — verify compilation
- [ ] 6.5 Run Kafka integration tests if `AAKS_KAFKA_TEST_BROKERS` is set

## Phase 7: Clean up and verify

- [ ] 7.1 Remove 6 legacy Dockerfiles and DB init scripts for consolidated services
- [ ] 7.2 Simplify `deploy/docker-compose.yml` from 13 containers to 7 (4 services + Gateway + Postgres + Kafka)
- [ ] 7.3 Update `AGENTS.md`, `CLAUDE.md`, `docs/design.md`, and `docs/tasks.md` to reflect the new 5-service topology
- [ ] 7.4 Run full verification: `go vet ./... && go build ./... && go test ./...`
- [ ] 7.5 Run frontend: `make web-typecheck && make web-build`
- [ ] 7.6 Verify no critical bugs: confirm the credential-less-sandbox invariant holds (no provider keys or git tokens reach container env/filesystem/logs)