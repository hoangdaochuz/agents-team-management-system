## Purpose

Defines the Domain-Driven Design layering, SOLID, Clean Code, and design-pattern contract that every
backend service must satisfy — inward dependency direction, dependency inversion through segregated
ports, unit-of-work transactional boundaries, and a decomposed shared kernel — so the codebase is
testable in isolation, extensible, and structurally maintainable, without changing the external REST or
Kafka contracts.

## ADDED Requirements

### Requirement: Layered package structure per bounded context

Each service MUST be organized into four layers under its `internal/` directory: `domain`,
`application`, `infrastructure`, and `interfaces`. The `domain` layer holds aggregates, value objects,
domain events, and repository port interfaces. The `application` layer holds command/query handlers and
saga coordinators. The `infrastructure` layer holds postgres repository implementations, Kafka adapters,
and inter-service HTTP clients. The `interfaces` layer holds HTTP handlers and Kafka consumer entrypoints.

#### Scenario: Service directory layout conforms to layers

- **WHEN** a service's `internal/` directory is inspected after conversion
- **THEN** it MUST contain `domain/`, `application/`, `infrastructure/`, and `interfaces/` packages, and MUST NOT retain a flat `httpapi/` package that holds business logic or a `store/` package consumed directly by handlers

### Requirement: Inward dependency direction

Dependencies MUST point strictly inward: `interfaces` and `infrastructure` MAY depend on `application`
and `domain`; `application` MAY depend only on `domain`; `domain` MUST NOT depend on `application`,
`infrastructure`, `interfaces`, or any shared infrastructure package (database drivers, the Kafka client,
or the HTTP server runtime).

#### Scenario: Domain layer imports no infrastructure

- **WHEN** the import graph of any service's `domain` packages is analyzed
- **THEN** it MUST NOT contain an import of that service's `application`, `infrastructure`, or `interfaces` packages, nor any shared infra package such as the pgx/pgxpool, sarama, or server-runtime packages; an automated import-lint test SHALL enforce this and fail the build on violation

### Requirement: Repository ports with dependency injection

Data access from the `application` and `interfaces` layers MUST go through repository interfaces
declared in the `domain` layer. Concrete postgres repository implementations (in `infrastructure`) MUST
be supplied only at the service composition root and injected into application handlers; application and
interface code MUST NOT reference concrete repository structs or the database pool directly.

#### Scenario: Application handlers are unit-testable without a database

- **WHEN** an application handler's unit test is run without a live Postgres connection
- **THEN** the handler MUST be exercisable against an in-memory/mock repository that implements the domain port, because the handler depends only on the port interface

### Requirement: Unit-of-work boundaries for multi-aggregate mutations

When an application use case mutates more than one aggregate in a single operation, all writes MUST
execute within one transactional unit of work that commits atomically or rolls back entirely.

#### Scenario: Partial failure rolls back multi-step creation

- **WHEN** an application use case performs a multi-step creation (for example creating an organization, a workspace, and an owner membership together) and one of the writes fails
- **THEN** all prior writes in that use case MUST be rolled back, leaving no partial aggregate persisted, and the caller MUST receive a single error result

### Requirement: Kafka consumers tied to service lifecycle

Kafka consumers MUST run on the service lifecycle context and MUST drain in-flight work on shutdown.
A shutdown signal during processing MUST NOT drop a partially-processed message.

#### Scenario: Graceful drain on shutdown

- **WHEN** the service receives a shutdown signal (SIGTERM/SIGINT) while a consumer is processing an in-flight message
- **THEN** the consumer MUST finish processing that message and commit its offset only on success, within a bounded drain window, before the process exits

### Requirement: Shared kernel decomposed by bounded context

Shared wire DTOs and event schemas MUST be organized into per-domain shared-kernel packages (for example
identity, workspaces, tasks, agent-execution, events) rather than a single monolithic contracts package.
A service MUST be able to import one domain's types without importing the entire unrelated type catalog.

#### Scenario: Scoped imports per domain

- **WHEN** a service needs types belonging to a single bounded context
- **THEN** those types MUST be importable from a domain-scoped shared-kernel package, and the service's import set MUST NOT transitively pull in unrelated domains' type definitions; the JSON wire shapes of all shared types MUST remain byte-for-byte unchanged

### Requirement: External contract preservation

The refactor MUST preserve the external interface contracts exactly. Every REST endpoint's path, HTTP
method, and JSON request/response shape, and every Kafka topic name, partitioning key, and event payload
JSON shape, MUST remain identical to the pre-refactor system.

#### Scenario: REST and Kafka contracts unchanged after conversion

- **WHEN** any service is converted to the new layering
- **THEN** its REST endpoints and Kafka event schemas MUST match the pre-refactor contract, verified by existing API tests and a contract-conformance check, and the frontend SPA (`frontend/src/api/*.ts`) MUST continue to interoperate without modification

### Requirement: Behavioral parity and build green per converted service

Each service conversion MUST preserve all pre-existing behavior. After a service is converted, the
build, vet, lint, and test gates MUST pass and all of that service's pre-existing tests MUST remain green.

#### Scenario: Converted service passes full gates

- **WHEN** a service conversion is complete
- **THEN** `go build ./...`, `go vet ./...`, `go test ./...` (Kafka integration tests gated by `AAKS_KAFKA_TEST_BROKERS`), and `golangci-lint run` MUST succeed, and every test that passed before the conversion MUST still pass

### Requirement: Full dependency inversion to all infrastructure

The dependency-inversion principle MUST apply to **all** infrastructure, not only database access. The
`domain` and `application` layers MUST NOT reference concrete infrastructure types — they MUST NOT import
the Kafka client library (sarama), an HTTP client, or the database driver/transaction types directly.
The Kafka producer, inter-service HTTP clients, and database access MUST each be consumed through a port
interface (for example an `EventPublisher` port, anti-corruption-layer client ports, and repository
ports) whose concrete implementations live in `infrastructure` and are injected at the composition root.

#### Scenario: Application layer imports no concrete infra

- **WHEN** the import graph of any service's `application` and `domain` packages is analyzed
- **THEN** it MUST NOT contain a direct import of the sarama Kafka client, `net/http` client types, or the pgx driver/transaction types; the import-lint test SHALL fail the build on any such import

### Requirement: Interface segregation of ports

Ports MUST be segregated so a consumer depends only on the methods it uses. The monolithic concrete
data-access struct MUST be replaced by per-aggregate repository ports (for example separate
`WorkspaceRepository`, `MemberRepository`, `OrganizationRepository` rather than one combined interface),
and cross-cutting infrastructure MUST be split into focused ports (a producer behind an `EventPublisher`
port; each inter-service dependency behind its own anti-corruption-layer client port) rather than one
large interface.

#### Scenario: Handler depends only on the ports it uses

- **WHEN** an application handler or HTTP adapter declares its dependencies
- **THEN** it MUST depend on the specific per-aggregate or focused port(s) it actually uses, and MUST NOT depend on a single combined repository or mega-client interface that exposes unrelated aggregates' or concerns' methods
