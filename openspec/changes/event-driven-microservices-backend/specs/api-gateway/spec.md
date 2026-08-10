## Purpose

The single HTTP entrypoint the frontend talks to. It exposes the exact REST + SSE surface
declared in `frontend/src/api/*.ts`, composes cross-service reads via synchronous fan-out,
and serves the realtime stream — so the frontend never knows services exist behind it.

## ADDED Requirements

### Requirement: Single REST entrypoint matching the frontend contract
The Gateway SHALL expose every route the frontend declares under `/api`, with the same
method, path, query parameters, and response body shapes defined in the full set of
`frontend/src/api/*.ts` modules: `projects, tasks, agents, skills, mcpServers,
providerKeys, feedback, runs, auth, workspaces, members, invites, audit, sysadmin,
knowledgeSources, plugins, rules, workspaceMcp` — including the workspace-scoped
`/workspaces/:id/...` paths and the `/sysadmin/...` paths. The Gateway SHALL keep
`/healthz` reachable.

#### Scenario: Frontend hits any declared endpoint
- **WHEN** the frontend calls any function in `frontend/src/api/*.ts`
- **THEN** the Gateway accepts the request and returns the declared response shape (or a
  documented error) without the frontend needing to address any other host or path

#### Scenario: Scoped resource paths
- **WHEN** the frontend calls a `/workspaces/:id/{skills,knowledge,plugins,rules,mcp,
  members,invites,requests,audit}` endpoint
- **THEN** the Gateway routes it to the owning service with the workspace id resolved
  from the path

### Requirement: Session-aware access control
The Gateway SHALL require a valid session for every route except `/healthz`,
`/auth/login`, `/auth/signup`, `/auth/signup-status`, and `/auth/signup-status/resend`,
responding 401 when the session is absent or invalid. It SHALL validate role/superadmin
requirements for `/sysadmin/*` (403 otherwise) and pass the authenticated user's identity
and membership context to downstream services so they can enforce workspace scoping.

#### Scenario: Unauthenticated request
- **WHEN** a request without a valid session reaches a protected route
- **THEN** the Gateway responds 401 without forwarding the request to any service

#### Scenario: Non-superadmin on sysadmin
- **WHEN** a valid session without `is_superadmin` reaches a `/sysadmin/*` route
- **THEN** the Gateway responds 403 and no system data is returned

### Requirement: Session-aware workspace resolution
Because the SPA never sends a workspace parameter, the Gateway SHALL resolve the
workspace context for unscoped endpoints (`/tasks`, `/agents`, `/skills`, `/projects`)
per the workspaces capability contract: an explicit `X-Workspace-ID` header when present,
else the session's single workspace, else the union of the session's workspaces. For
explicitly scoped paths (`/workspaces/:id/...`) the path id SHALL be authoritative, and
the Gateway SHALL reject requests whose workspace is not in the session's membership.

#### Scenario: Single-workspace session
- **WHEN** a user with one workspace calls `/tasks`
- **THEN** the Gateway forwards the session workspace context and the Task service returns
  only that workspace's tasks

#### Scenario: Explicit path overrides context
- **WHEN** the frontend calls `/workspaces/:id/skills` for a workspace the user belongs to
- **THEN** the Gateway forwards exactly that workspace id, regardless of the session context

### Requirement: Synchronous fan-out composition for cross-service reads
For responses that join data from more than one service (e.g. a task list enriched with
agent names, an agent shown with its skill/mcp labels, the `Session` assembled from user +
workspace memberships, workspace cards with `agent_count`/`open_task_count`, or
`SystemHealth` composed from per-service probes), the Gateway SHALL issue synchronous
internal calls to the owning services and assemble the combined response.

#### Scenario: Listing tasks with assignee enrichment
- **WHEN** the frontend calls `listTasks`
- **THEN** the Gateway obtains tasks from the Task service and, for any referenced
  `agent_id`, fetches agent identity from the Agent service to compose the response

#### Scenario: Hydrating a session
- **WHEN** the frontend calls `me()`
- **THEN** the Gateway assembles the `Session` from the Auth service (user) and the
  Orgs/Workspaces service (memberships + roles)

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
