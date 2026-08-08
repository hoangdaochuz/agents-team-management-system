## Purpose

The single HTTP entrypoint the frontend talks to. It exposes the exact REST + SSE surface
declared in `frontend/src/api/*.ts`, composes cross-service reads via synchronous fan-out,
and serves the realtime stream — so the frontend never knows services exist behind it.

## ADDED Requirements

### Requirement: Single REST entrypoint matching the frontend contract
The Gateway SHALL expose every route the frontend declares under `/api`, with the same
method, path, query parameters, and response body shapes defined in
`frontend/src/api/{projects,tasks,agents,skills,mcpServers,providerKeys,feedback,runs}.ts`.
The Gateway SHALL keep `/healthz` reachable.

#### Scenario: Frontend hits any declared endpoint
- **WHEN** the frontend calls any function in `frontend/src/api/*.ts`
- **THEN** the Gateway accepts the request and returns the declared response shape (or a
  documented error) without the frontend needing to address any other host or path

### Requirement: Synchronous fan-out composition for cross-service reads
For responses that join data from more than one service (e.g. a task list enriched with
agent names, an agent shown with its skill/mcp labels), the Gateway SHALL issue
synchronous internal calls to the owning services and assemble the combined response.

#### Scenario: Listing tasks with assignee enrichment
- **WHEN** the frontend calls `listTasks`
- **THEN** the Gateway obtains tasks from the Task service and, for any referenced
  `agent_id`, fetches agent identity from the Agent service to compose the response

### Requirement: Transparent passthrough for single-owner resources
For requests whose data is owned by exactly one service (e.g. Project CRUD, Skill CRUD),
the Gateway SHALL forward the request to that service and return its response unchanged.

#### Scenario: Single-owner mutation
- **WHEN** the frontend creates or updates a Project, Skill, or MCP server
- **THEN** the Gateway forwards to the owning service and returns its response verbatim

### Requirement: Normalized error contract
The Gateway SHALL translate internal service failures into the frontend's error contract
(`ApiError`: HTTP status + textual body) and MUST NOT leak internal stack traces, host
names, or service topology to the frontend.

#### Scenario: An owning service is unavailable
- **WHEN** a backing service returns an error or is unreachable
- **THEN** the Gateway responds with an appropriate HTTP status and a generic message,
  and does not expose internal details

### Requirement: No direct service-to-service synchronous coupling
The Gateway SHALL be the only component permitted to make synchronous calls into services.
Services MUST NOT make synchronous calls into one another; cross-service interaction beyond
the Gateway SHALL occur only via the event bus.

#### Scenario: A service needs another service's data
- **WHEN** a service requires data owned by another service that the Gateway has not
  already composed
- **THEN** the service obtains it indirectly (via events/projection or by surfacing the
  need to the Gateway), not by calling the other service directly

### Requirement: Request identity and resilience
The Gateway SHALL attach a request id to every inbound request, propagate it to downstream
internal calls, and recover from panics in handlers without dropping the process.

#### Scenario: A handler panics
- **WHEN** a handler raises a panic
- **THEN** the Gateway recovers, returns a 5xx to the caller, and continues serving other
  requests
