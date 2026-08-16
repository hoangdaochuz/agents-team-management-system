# Tasks

Ordered by dependency. Each task is gated by: `go build ./...`, `go vet ./...`, `go test ./...`, and
`golangci-lint run` green, plus external-contract preservation (REST + Kafka shapes unchanged). A task is
not done until its gate passes.

## 1. Phase 0 — Shared kernel, infra cleanup & tooling

- [x] 1.1 Re-parent shared infra under `backend/internal/platform` (`db`, `kafka`, `svcrun`, `httputil`→`platform/http`, `config`→`platform/config`); update all import paths repo-wide.
- [x] 1.2 Split the identity-header string literals into a shared `const` block (used by the gateway injector and downstream readers); extract `scope.go` into `platform/tenancy` so JSON helpers no longer import `contracts`.
- [x] 1.3 Delete dead code: `internal/httpapi.Server` scaffold, unused `db.Config` struct.
- [x] 1.4 Collapse the two near-identical migrators (`MigrateDir`/`MigrateFS`) into one; keep embedded-FS behavior.
- [x] 1.5 Decompose `internal/contracts` into per-domain subpackages (`identity`, `workspaces`, `tasks`, `agentexec`, `events`, …); add temporary re-export aliases at the old `contracts` path so unconverted services still compile.
- [ ] 1.6 Add the import-direction lint: a `go test`-based check asserting no `domain` package imports `application`/`infrastructure`/`interfaces`/platform-infra, plus a `golangci-lint` `depguard` config.
- [ ] 1.7 Add a REST + Kafka contract-conformance check (golden snapshot of endpoint paths/methods/JSON shapes and topic/payload shapes) as a CI test; baseline it against the current system.
- [ ] 1.8 Gate: `make build test vet lint` green; contract-conformance check unchanged; mixed layout interoperates.

## 2. Phase 1 — Pilot: `orgs` service DDD conversion

- [ ] 2.1 Create the `services/orgs/internal/{domain,application,infrastructure,interfaces}` package skeleton.
- [ ] 2.2 Define domain entities/aggregates (Workspace, Member, Org, Invite, JoinRequest) and repository **port interfaces** in `domain` (move logic, not SQL).
- [ ] 2.3 Implement postgres repository adapters in `infrastructure/store` satisfying the domain ports; migrate inline SQL from the old `store.Store`.
- [ ] 2.4 Extract business logic from `routes.go` into `application` handlers: membership/role enforcement, last-owner invariant, workspace create, org-request approval.
- [ ] 2.5 Introduce the application-layer `UnitOfWork` and wrap the multi-aggregate mutations (`createWorkspace`, `approveOrgRequest`) so they commit atomically or roll back. Introduce an `EventPublisher` port (SOLID DIP) and wrap the sarama producer behind it in `infrastructure`; inject it into the application handlers that publish events (no sarama import in `application`/`domain`).
- [ ] 2.6 Add an `interfaces/http` layer of thin handlers (decode → call application handler → encode) and `interfaces/messaging` Kafka consumers delegating to application handlers.
- [ ] 2.7 Wire an explicit composition root in `cmd`/`interfaces` that injects repositories and producers into application handlers; pass the lifecycle context into the Kafka consumer group.
- [ ] 2.8 Write mock-based `application` unit tests (no live Postgres) for last-owner protection, role enforcement, and the create/approve flows.
- [ ] 2.9 Add a rollback test: inject a mid-transaction failure in a multi-aggregate mutation and assert no partial state persists.
- [ ] 2.10 Remove the temporary `contracts` re-export usage from `orgs`; import per-domain subpackages directly.
- [ ] 2.11 Gate: parity (all pre-existing `orgs` tests pass), new unit/rollback tests pass, import-lint green, contract-conformance unchanged.

## 3. Phase 2a — Complex validation: `runner` service

- [ ] 3.1 Create the `services/runner/internal/{domain,application,infrastructure,interfaces}` skeleton; preserve existing `driver`, `sandbox`, `mcp` leaf packages and their ports untouched.
- [ ] 3.2 Define domain ports: `RunRepository`/`StepRepository`/`FindingRepository`/`ArtifactRepository`; move `store.Store` SQL into `infrastructure/store` adapters.
- [ ] 3.3 Extract the `httpapi.App` orchestration (`runImplementer`, `runReviewer`, `executeAndFinish`, `openPr`, `cancelTask`) into `application` handlers depending only on domain ports.
- [ ] 3.4 Introduce ACL port interfaces (`SettingsKeyClient`, `ResourcesRulesClient`, `AgentMcpClient`) in `domain`/`application`; implement HTTP adapters in `infrastructure`, replacing inline `http.DefaultClient` calls. Also introduce an `EventPublisher` port (DIP) wrapping the sarama producer, so the orchestration handlers publish through the abstraction.
- [ ] 3.5 Add `interfaces/http` (query endpoints + SSE replay) and `interfaces/messaging` (command consumers) delegating to application handlers; wire the lifecycle context for consumer drain.
- [ ] 3.6 Wire the explicit composition root: driver (`Simulated`/`LLM`), sandbox manager, MCP bridge, repositories, publisher, ACL clients → application handlers.
- [ ] 3.7 Add mock-based `application` unit tests for run orchestration and PR-open flow (no Docker/Kafka required).
- [ ] 3.8 Remove temporary `contracts` re-export usage from `runner`; import per-domain subpackages directly.
- [ ] 3.9 Gate: parity (incl. `sandbox/secret_leak_test.go` and the opt-in container E2E suite `make e2e`), import-lint green, contract-conformance unchanged.

## 4. Phase 2b — Roll-out waves (each service independently gated)

- [ ] 4.1 `auth`: convert to four layers; extract login throttling + signup orchestration into `application`; add ports + mock tests.
- [ ] 4.2 `project`: convert; extract workspace-scoped project logic into `application`; ports + mock tests.
- [ ] 4.3 `task` (saga coordinator): convert; model command/fact handlers as `application` saga handlers; ports + mock tests.
- [ ] 4.4 `catalog`: convert (skills + MCP servers); ports + mock tests.
- [ ] 4.5 `settings` (provider-key decryptor): convert; isolate crypto + mTLS client in `infrastructure`; ports + mock tests.
- [ ] 4.6 `agent`: convert; ports + mock tests.
- [ ] 4.7 `resources`: convert (per-workspace knowledge/plugins/rules/MCP/audit); ports + mock tests.
- [ ] 4.8 `admin`: convert (sysadmin oversight); ports + mock tests.
- [ ] 4.9 `gateway`: apply the decided depth (full DDD **or** lighter use-case + client-port extraction per design Open Question); preserve the path-aware proxy and identity-injection behavior.
- [ ] 4.10 Remove the now-unused temporary `contracts` re-export aliases once all services import per-domain subpackages.

## 5. Cross-cutting acceptance & finalization

- [ ] 5.1 Run full gate repo-wide: `make build test vet lint`, frontend `npm run typecheck && npm run build`, contract-conformance check, import-lint — all green.
- [ ] 5.2 Confirm `docker compose up` brings the whole stack up and the gateway serves the frontend contract end-to-end.
- [ ] 5.3 Update `AGENTS.md` / `CLAUDE.md` architecture section to describe the DDD layering and `internal/platform` + decomposed `contracts` layout.
- [ ] 5.4 Confirm every `domain` package across all services passes the import-direction lint (no infra imports).
- [ ] 5.5 Clean-Code sweep: verify no dead/duplicated infrastructure (no `httpapi.Server`, `db.Config`, single migrator), one shared identity-header `const` block, consistent sentinel-error + `httputil.Respond*` error handling, small named functions, and no remaining god-objects.
- [ ] 5.6 Pattern & SOLID conformance review: confirm Repository, Unit of Work, Ports & Adapters, Strategy, Adapter, Factory, Dependency Injection, Saga, Anti-Corruption Layer, and Publish/Subscribe are applied where warranted; confirm SRP (one concern per package), OCP/LSP (ports swappable, mocks pass), ISP (segregated per-aggregate ports), and DIP (no concrete infra imports in `domain`/`application`) hold across all services.
