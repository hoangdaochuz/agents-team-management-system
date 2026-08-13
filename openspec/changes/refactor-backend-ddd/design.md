## Context

See `proposal.md - Why` for motivation. The current shape this design starts from (verified by code
scan):

- Each of the 11 services is a two-layer `cmd/` + `internal/{httpapi,store}` split. `cmd/main.go` is a
  one-line `svcrun.Run(name, addr, Register)`; **all** wiring (env, db pool, store, producer, consumer
  goroutines, route registration, superadmin seeding) is crammed into `httpapi.Register`.
- There is **no domain layer**. `backend/internal/contracts` is a single ~40-type + ~21-topic god-package
  every service imports wholesale; store row structs embed those DTOs and add persistence fields
  (`PasswordHash`, `Active`).
- Handlers depend on a **concrete `*store.Store`** (no repository interfaces). Business logic
  (last-owner protection, login throttling, signup/multi-step-create orchestration, the runner agent
  loop) lives in `routes.go`. `httpapi.App` is a god-object: composition root + use-case layer + Kafka
  and HTTP adapters fused.
- Multi-step mutations (Orgs' `createWorkspace`, `approveOrgRequest`) run as **separate store calls with
  no transaction**; only `auth.CreateSignupRequest` uses `BEGIN`. Kafka consumers run on
  `context.Background()` and are **never drained** on shutdown.
- Shared infra carries dead/duplicated code: `httpapi.Server` scaffold (duplicates `svcrun`), two
  `Config` types (`db.Config` is dead), two near-identical migrators, `httputil/scope.go` leaking the
  `contracts` dependency into the JSON helpers, and identity-header string literals scattered with no
  `const` block.

Constraints that are **fixed** (not up for design): single Go module `github.com/aaks/server` with
`go.mod` in `backend/`; one logical DB per service; REST and Kafka contracts byte-for-byte preserved;
stdlib + pgx + sarama only (no new deps); the credential-less-sandbox invariant in the runner.

## Goals / Non-Goals

**Goals:**

- Establish a repeatable DDD reference architecture proven on one pilot service, then roll out.
- Make `domain` packages import-free of infrastructure, enforced by an automated check.
- Make application handlers unit-testable without a database (repository ports + mocks).
- Fix the two real correctness gaps: unit-of-work for multi-aggregate mutations; graceful consumer drain.
- Decompose the contracts god-package into a per-domain shared kernel without changing wire shapes.

**Non-Goals:**

- No new user-facing features, no API/event contract changes, no service-topology or DB changes.
- No multi-module split, no new third-party libraries, no ORM/query-builder introduction.
- No rewrite of the runner's driver/sandbox/mcp internals — those leaf packages already have clean ports
  (`Driver`, `Env`, `McpBridge`); only the runner's `httpapi.App` orchestration is refactored.

## Decisions

### D1 — Four-layer DDD per bounded context (chosen architecture)

Each service becomes `internal/{domain, application, infrastructure, interfaces}`:

```
backend/
  internal/
    platform/                 # shared infra kernel (renamed from ad-hoc internal/*)
      db/ kafka/ svcrun/ http/ config/
    contracts/                # shared wire DTO + event kernel, decomposed by domain
      identity/ workspaces/ tasks/ agentexec/ events/ ...
  services/<svc>/
    cmd/main.go               # thin entrypoint; calls an explicit composition root
    internal/
      domain/                 # aggregates, value objects, domain events, repo PORT interfaces
      application/            # command/query handlers, saga coordinators (depend on domain only)
      infrastructure/         # postgres repo impls, kafka adapters, inter-service HTTP clients
      interfaces/             # http handlers + kafka consumers (entrypoints)
```

**Why DDD over alternatives:**
- *Clean Architecture (domain/application/adapter)* — rejected: the user selected DDD; the distinction
  between `infrastructure` (repos, clients) and `interfaces` (entrypoints) maps better to this codebase's
  two kinds of adapters (HTTP + Kafka) and the saga coordinators in `task`/`runner`.
- *Lighter use-case + repo-port extraction only* — rejected as the sole target: it leaves the contracts
  god-package and god-object `App` unaddressed, but it is exactly the **fallback** if a service is too
  trivial (see D8).

### D2 — Pilot service: `orgs` first, `runner` as the complex stress test

The pilot proves the pattern on the service with the richest domain payoff:

- `orgs` has genuine domain logic (memberships, role enforcement, last-owner invariant), the
  **transactional bug** to fix (org→workspace→membership multi-step create), Kafka consumers, and
  inter-service identity boundaries. Converting it exercises every layer and every new abstraction
  (ports, UoW, consumer drain, DTO mapping).

The pattern is then validated against the **runner** (the most complex service: the god-object
`httpapi.App` holds run/review/PR orchestration + Kafka + 3 outbound HTTP clients) before rolling out to
the remaining services. The runner's existing clean leaf ports (`driver.Driver`, `sandbox.Env`,
`mcp.Bridge`) are preserved; only its `httpapi` orchestration moves into `application`.

### D3 — Shared kernel decomposition

- `internal/contracts` is split into per-domain subpackages (`contracts/identity`, `contracts/workspaces`,
  `contracts/tasks`, `contracts/agentexec`, …) plus `contracts/events` for the envelope + topic/payload
  catalog. Wire JSON tags and field names are unchanged.
- Cross-cutting infra moves under `internal/platform` (`platform/db`, `platform/kafka`, `platform/svcrun`,
  `platform/http`, `platform/config`). This is a rename/re-parent only.
- `httputil/scope.go` becomes `internal/platform/tenancy` (or `platform/http/scope`) so the JSON helpers
  no longer import `contracts`; identity-header literals become a shared `const` block used by both the
  gateway injector and downstream readers.
- **Migration aid:** temporary type re-exports (e.g. `package contracts // import "…/contracts/identity"
  alias`) can bridge old import sites during rollout, removed when a service is converted. This avoids a
  flag-day across all 11 services.

*Alternative considered:* per-service private domain types with an anti-corruption mapping layer —
rejected for now (more boilerplate than this stage needs); the decomposed shared kernel keeps interop
simple. A service MAY still keep private domain types in its own `domain` package distinct from the wire
DTOs.

### D4 — Ports, dependency inversion & unit of work (SOLID: DIP + ISP)

- Each service declares repository **port interfaces** in `domain`, **segregated per aggregate**
  (`WorkspaceRepository`, `MemberRepository`, `OrganizationRepository`, …) — not one fat data-access
  interface (ISP). They carry domain-shaped methods only (no `pgx.Tx`, no SQL leaking).
- `application` handlers depend on these ports. The concrete postgres implementations live in
  `infrastructure/store` and are injected at the composition root (DIP).
- **Dependency inversion extends to all infrastructure, not just the DB.** The sarama producer is wrapped
  behind an `EventPublisher` port (so `application` never imports sarama); each inter-service HTTP
  dependency is wrapped behind its own focused anti-corruption-layer client port (`SettingsKeyClient`,
  `ResourcesRulesClient`, `AgentMcpClient` — not one mega-client, ISP). The database transaction is
  exposed only through the application-layer `UnitOfWork`, never as a raw `pgx.Tx` in `domain`.
- **Unit of Work:** an `application`-layer `UnitOfWork` interface (infra-shaped tx concerns kept out of
  `domain`) exposes `Do(ctx, func(work *Tx) error)`. The infrastructure implementation wraps a `pgx.Tx`
  and constructs transactional repository instances scoped to that tx. Multi-aggregate use cases
  (`createWorkspace`, `approveOrgRequest`) call through the UoW; single-aggregate mutations may call a
  repository directly. This satisfies the spec's unit-of-work requirement without forcing every read
  through a transaction.

*Alternative considered:* putting a `Querier` interface in `domain` (Exec/Query/QueryRow) — rejected: it
imports SQL semantics into the domain. The UoW stays in `application`; `domain` ports stay pure.

### D5 — Explicit composition root; lifecycle context propagation

- Each service gains an explicit composition root (a `wire`/`app` function in `cmd` or `interfaces`) that
  constructs config → platform deps → repositories → application handlers → HTTP/Kafka adapters, and hands
  the built `http.Handler` to `svcrun.Run`. `svcrun.Registrar` is retained but now receives
  already-constructed dependencies instead of building them from `os.Getenv` inline.
- **Consumer drain:** the service lifecycle/shutdown context is passed into `kafka.ConsumerGroup.Run`.
  `svcrun.Run` already owns the signal context; it is threaded into the consumer goroutines so SIGTERM
  triggers a bounded drain (new `Run(ctx, ...)` contract; the consumer commits offsets only on success).

### D6 — Import-direction enforcement

A repository-local import-lint test (a `go test` under `backend/` that walks `go list -deps`/`go/packages`
and asserts no `domain` package imports `application`/`infrastructure`/`interfaces`/platform-infra) makes
the dependency-direction spec requirement a **failing build** on violation. A `golangci-lint` `depguard`
config is added as a complementary fast check.

### D7 — Mapping at the interface boundary

HTTP/Kafka adapters in `interfaces` map between shared-kernel wire DTOs and the service's domain types
(where they differ). Where a service's domain type is identical to the wire DTO (common for the simpler
CRUD services), the mapping is a no-op/alias and no extra type is introduced — avoiding forced
boilerplate. This keeps the refactor proportional to each service's complexity.

### D8 — Proportional layering (anti-cargo-cult)

The four directories exist in every service to satisfy the layering requirement, but they are **not**
required to be equally fleshed out. For services with little domain logic (e.g. `catalog`, `resources`,
`admin` CRUD, the `gateway` proxy), `domain` may be thin (ports + value types) and `application` a
pass-through; the full aggregate/value-object/saga machinery is reserved for services that warrant it
(`orgs`, `task`, `runner`, `auth`). This is the explicit fallback referenced by D1 and keeps the refactor
proportional — applying every pattern everywhere would be its own anti-pattern.

### D9 — SOLID, Clean Code & Design Patterns applied explicitly

The refactor is engineered around SOLID, Clean Code, and named design patterns — not just "layers."

**SOLID**
- **Single Responsibility:** each layer and package has one reason to change (domain = business rules,
  application = use-case orchestration, infrastructure = technical mechanisms, interfaces = transport).
  The god-object `httpapi.App` is decomposed into single-purpose application handlers, one concern each.
- **Open/Closed:** new capability is added by new ports/adapters (a new driver, sandbox, repository
  implementation, or ACL client) without modifying `domain`/`application`. The existing `Strategy`
  choices (`Simulated`/`LLM` driver, `docker`/`local` sandbox) already exhibit this; the refactor
  preserves and extends it.
- **Liskov Substitution:** every port implementation is substitutable — postgres repos stand in for mock
  repos in tests, `Simulated` for `LLM`, `localEnv` for `dockerEnv`. Go interface satisfaction enforces
  this at compile time; the new mock-based unit tests prove behavioral substitutability.
- **Interface Segregation:** the monolithic `*store.Store` is replaced by per-aggregate repository ports
  (D4/ISP); the producer by a focused `EventPublisher`; inter-service clients by focused ACL ports.
- **Dependency Inversion:** `domain`/`application` depend only on abstractions; concretions (pgx repos,
  sarama producer, HTTP clients) are injected at the composition root. No `sarama.`/`http.Client`/`pgx.`
  symbols appear in `domain` or `application` — enforced by the import-lint (D6).

**Clean Code**
- Remove dead/duplicated code (delete `httpapi.Server`, `db.Config`; collapse the duplicate migrators);
  DRY up the identity-header literals into one `const` block; small, named functions; consistent error
  handling (sentinel errors + the shared `httputil.Respond*` helpers); explicit DTO↔domain mapping at the
  boundary (D7); value-object immutability where feasible; no god-objects.

**Design Patterns**
- **Repository** — domain ports + infrastructure implementations.
- **Unit of Work** — transactional multi-aggregate mutation boundaries (D4).
- **Ports & Adapters / Hexagonal** — the four-layer layout itself; `interfaces`/`infrastructure` are the
  adapters around the `application`/`domain` hexagon.
- **Strategy** — driver (`Simulated`/`LLM`) and sandbox (`docker`/`local`) selection.
- **Adapter** — `sandboxExec` (`sandbox.Env` → `driver.ToolExec`), ACL HTTP adapters, and DTO mappers.
- **Factory** — `New()` constructors and the composition-root wiring.
- **Dependency Injection** — constructor injection at the composition root (no service locator, no global
  state).
- **Saga** — `task` as the command/fact saga coordinator; `runner` run/review/PR orchestration.
- **Anti-Corruption Layer** — the ACL client ports isolate each service from siblings' internal shapes.
- **Publish/Subscribe (Observer)** — the Kafka bus behind the `EventPublisher` port.

*Trade-off:* applying all of these indiscriminately is over-engineering. D8 (proportional layering) and
D7 (alias-where-identical mapping) are the explicit guards that keep pattern application proportional to
each service's real complexity.

## Risks / Trade-offs

- **[Large blast radius across 11 services]** → Pilot-first; each service lands behind a behavioral-parity
  gate (`build`/`vet`/`test`/`lint` green + pre-existing tests pass). Mixed layering across services is
  runtime-safe because external contracts are unchanged, so a partial rollout is not an outage risk.
- **[Transactional UoW changes failure semantics]** → Intended and spec'd (atomic rollback). Documented as
  a behavior change; existing tests cover the happy path; a new test asserts rollback on injected
  mid-transaction failure.
- **[Mapping boilerplate / over-engineering]** → Mitigated by D7: domain types alias wire DTOs where they
  don't diverge; only `orgs`/`runner`-scale services get rich private domain types.
- **[Import cycles during contracts decomposition]** → Mitigated by the temporary re-export bridge (D3)
  and by converting one service at a time so the old monolithic `contracts` can coexist with new
  subpackages temporarily.
- **[Behavioral drift in the runner god-object extraction]** → The runner keeps its existing leaf ports
  untouched; its `httpapi` logic is moved, not rewritten; the sandbox secret-leak test and the opt-in
  container E2E suite are the acceptance gate.
- **[Consumer drain could delay shutdown]** → Bounded drain window with a hard timeout; in-flight work is
  finished but the process still exits within the bound.

## Migration Plan

1. **Phase 0 — Shared kernel & tooling (no service logic changes):** re-parent infra to
   `internal/platform`; split `internal/contracts` into per-domain subpackages with temporary re-export
   aliases; add the `tenancy` split + header `const` block; delete dead `httpapi.Server` and `db.Config`;
   collapse the duplicate migrators; add the import-lint test + `depguard`. Gate: `build`/`vet`/`test`/
   `lint` green, contracts unchanged.
2. **Phase 1 — Pilot (`orgs`):** full DDD conversion, repository ports, UoW for the multi-step create
   flows, consumer-lifecycle drain, mock-based `application` unit tests. Gate: parity + new rollback test.
3. **Phase 2a — Complex validation (`runner`):** extract `httpapi.App` orchestration into `application`;
   preserve driver/sandbox/mcp ports; add ACL ports for the Settings/Resources/Agent HTTP clients. Gate:
   parity + sandbox secret-leak test + container E2E.
4. **Phase 2b — Roll-out waves** (by similarity, each independently gated): `auth`, `project`/`task`
  (saga coordinators), `catalog`/`settings`/`agent`/`resources`/`admin`, then `gateway` (lightest — proxy,
  see Open Questions). Each service is one reviewable increment; a failing gate reverts that service only.

**Rollback:** every phase/service is a discrete commit set; revert the conversion commit to restore the
prior two-layer shape. Because external contracts are preserved, a reverted or not-yet-converted service
interoperates with converted ones at runtime.

## Open Questions

- **Gateway depth:** the gateway is a path-aware reverse proxy with near-zero domain logic. It may warrant
  a *lighter* touch (D8 fallback: use-case + client-port extraction) rather than the full four-layer
  treatment. Deferrable — decided when its wave is reached; does not change specs, approach, or task
  breakdown.
- **UoW scope beyond orgs/runner:** some services are single-aggregate and may not need a transactional
  UoW at all. Whether to introduce the UoW abstraction universally or only where multi-aggregate
  mutations exist is decided per service during roll-out; the spec requires UoW only where multiple
  aggregates are mutated, so this is safely deferrable.
