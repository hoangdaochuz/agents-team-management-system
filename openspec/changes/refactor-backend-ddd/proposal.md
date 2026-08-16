## Why

The backend's 11 event-driven services work, but their internal structure resists scaling and
maintenance. Every service is a two-layer `httpapi` + `store` split: there is **no domain layer**,
HTTP handlers depend on a **concrete `*store.Store`** (no repository interfaces, no mock seam),
business logic (last-owner protection, login throttling, signup/multi-step-create orchestration, the
entire agent loop) is embedded in `routes.go`, and `httpapi.App` is a god-object that is
simultaneously the composition root, the use-case layer, and the Kafka/HTTP adapters. The shared
`internal/contracts` package is a ~40-type god-package every service imports wholesale, and
shared infra carries dead/duplicated code (`httpapi.Server` scaffold, two `Config` types, two
migrators). This makes the code hard to test in isolation, hard to extend, and hard to reason about
for correctness (e.g. multi-step mutations in Orgs run with **no transaction boundary**; Kafka
consumers run on `context.Background()` and are **never drained** on shutdown).

We refactor toward **Domain-Driven Design (DDD)**: each service a bounded context with
`domain / application / infrastructure / interfaces` layers, dependency inversion via repository
ports, unit-of-work boundaries, a decomposed shared kernel, and pilot-first sequencing so a working
reference architecture is proven on one service before rolling out to the rest.

## What Changes

- **Adopt a per-service DDD layering contract.** Each service restructures from
  `internal/{httpapi,store}` into `internal/{domain,application,infrastructure,interfaces}`.
  `domain` holds aggregates, value objects, domain events, and **repository port interfaces**
  (no infra imports). `application` holds command/query handlers and saga coordinators that depend
  only on domain ports. `infrastructure` holds the postgres repository implementations, Kafka
  adapters, and inter-service HTTP clients. `interfaces` holds HTTP handlers and Kafka consumers
  (entrypoints). **Dependency direction is strictly inward** (`interfaces`/`infrastructure` →
  `application` → `domain`; `domain` imports nothing infrastructural).
- **Decompose the `contracts` god-package into per-domain shared-kernel packages**
  (e.g. `contracts/identity`, `contracts/workspaces`, `contracts/tasks`, `contracts/agentexec`,
  `contracts/events`), each owning its wire DTOs and event schemas. Wire DTOs/event schemas remain
  the shared kernel and their **JSON shapes are unchanged**.
- **Introduce repository ports + dependency injection.** Every service declares repository
  interfaces in `domain`, satisfied by postgres implementations in `infrastructure`, injected into
  `application` handlers by a thin composition root — eliminating the concrete `*store.Store` coupling
  in handlers and enabling mock-based unit tests.
- **Move business logic out of HTTP handlers** into `application` handlers. HTTP handlers
  (`interfaces/http`) become thin: decode → call application handler → encode. Kafka consumers
  (`interfaces/messaging`) likewise delegate to application handlers.
- **Establish unit-of-work boundaries.** Multi-aggregate mutations (e.g. Orgs'
  org→workspace→membership create flows) run inside a single transaction; `application` handlers own
  these boundaries. **Behavior change:** partial failures now roll back instead of leaving the DB in
  an inconsistent state.
- **Wire Kafka consumers to graceful shutdown.** Consumers run on the service lifecycle context
  (not `context.Background()`) and drain in-flight work on SIGTERM. **Behavior change:** clean
  shutdown instead of dropped-in-flight messages.
- **Clean up shared infra:** delete dead `httpapi.Server` scaffold, dead `db.Config`, collapse the
  two near-identical migrators into one, split `httputil`'s `scope.go` into its own tenancy package
  so the JSON helpers no longer depend on `contracts`, and centralize the identity-header string
  literals as constants.

**Non-goals (preserved):**
- The frontend↔backend **REST API contract** (`frontend/src/api/*.ts` shapes, paths, ~60 endpoints
  + SSE) is unchanged.
- The **Kafka event contract** (topics, `EventEnvelope`, payload JSON shapes, partitioning,
  at-least-once + DLQ semantics) is unchanged.
- Service topology, ports, logical-DB-per-service, the Gateway/BFF path-aware proxy, the
  credential-less-sandbox invariant, and the single Go module `github.com/aaks/server` (with
  `go.mod` in `backend/`) are all unchanged.
- No new user-facing features. This is a structural and internal-correctness refactor.

## Capabilities

### New Capabilities

- `backend-service-architecture`: The DDD layering, dependency-direction, and shared-kernel contract
  that every backend service must satisfy — per-service `domain/application/infrastructure/interfaces`
  layers, repository port interfaces with inverted dependencies, unit-of-work transaction boundaries
  for multi-aggregate mutations, Kafka-consumer lifecycle tied to graceful shutdown, and the
  decomposed shared kernel. Introduced and proven on a pilot service, then rolled out.

### Modified Capabilities

_None at the domain behavior level._ REST and Kafka contracts are preserved unchanged. The two
genuine behavior changes (atomic multi-step mutations; clean consumer shutdown) are encoded as new
requirements under the `backend-service-architecture` capability rather than domain spec edits,
because they are cross-cutting robustness invariants, not domain-logic changes.

## Impact

- **Code:** All 11 services (`gateway`, `project`, `task`, `agent`, `catalog`, `settings`, `runner`,
  `auth`, `orgs`, `resources`, `admin`) and the shared `backend/internal/*` packages are restructured.
  Pilot-first: the shared kernel + one reference service land first, then the remaining services are
  converted in phases.
- **APIs:** None externally. REST and Kafka contracts are byte-for-byte preserved; this is verified
  by existing tests + a contract-conformance check.
- **Dependencies:** No new third-party libraries (pgx, sarama, stdlib only). No `go.mod` module split.
- **Tests:** Existing `go test ./...`, `go vet`, `golangci-lint`, and the runner sandbox secret-leak
  test must stay green throughout. New mock-based unit tests become possible at the
  `application` layer (repository ports). Behavioral parity is the acceptance gate per service.
- **Risk:** Large blast radius. Mitigated by pilot-first sequencing, per-service behavioral-parity
  verification, and preserving all external contracts.
