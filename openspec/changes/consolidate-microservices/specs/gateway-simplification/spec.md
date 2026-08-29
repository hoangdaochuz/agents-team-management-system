## Purpose

The Gateway BFF (Backend For Frontend) is the sole HTTP entrypoint into the system. This spec
documents its simplification from 10 upstream service env vars to 4, reflecting the reduced
service boundaries after microservice consolidation.

## ADDED Requirements

### Requirement: Reduced upstream environment variables
The Gateway SHALL be configured with exactly 4 upstream environment variables, down from 10:

| Env Var | Target Service | Previously Covered |
|:--------|:---------------|:-------------------|
| `UPSTREAM_IDENTITY` | Identity Service | `UPSTREAM_AUTH`, `UPSTREAM_ORGS`, `UPSTREAM_ADMIN` |
| `UPSTREAM_WORKSPACE` | Workspace Service | `UPSTREAM_PROJECT`, `UPSTREAM_TASK`, `UPSTREAM_CATALOG`, `UPSTREAM_RESOURCES` |
| `UPSTREAM_AGENT` | Agent Service | `UPSTREAM_AGENT` (unchanged) |
| `UPSTREAM_EXECUTOR` | Executor Service | `UPSTREAM_RUNNER` (renamed) |

#### Scenario: Gateway boots with 4 upstreams
- **WHEN** the Gateway starts with only `UPSTREAM_IDENTITY`, `UPSTREAM_WORKSPACE`, `UPSTREAM_AGENT`, and `UPSTREAM_EXECUTOR` set
- **THEN** all frontend-facing routes resolve to one of the 4 upstream services without the old per-service variables

#### Scenario: Missing upstream fails fast
- **WHEN** the Gateway starts with one of the 4 upstream variables unset
- **THEN** startup reports a configuration error naming the missing upstream

### Requirement: Session composition simplification
The Gateway SHALL compose a user session with a single HTTP call to the Identity Service
(`/internal/identity`) instead of the previous 3 synchronous fan-out calls (Auth → Orgs → enrichment).
- **Before**: Gateway → Auth → Orgs → (enrichment services), with additive latency.
- **After**: Gateway → Identity (single call), with local composition of session data.
- **Response**: The Identity service returns `{ user, workspaces, active_workspace_id? }`
  shape, matching the `Session` type used by the frontend (role and the superadmin flag
  live on the `User` object, per `frontend/src/api/types.ts` — the contract of record).

#### Scenario: Session resolved in one call
- **WHEN** an authenticated request arrives at the Gateway
- **THEN** the Gateway issues exactly one call to the Identity Service to resolve the session (user + workspace union)

#### Scenario: Session shape matches the frontend contract
- **WHEN** the Identity Service responds to the session composition call
- **THEN** the response contains `{ user, workspaces, active_workspace_id? }` matching the frontend `Session` type

### Requirement: SSE composition unchanged
The Gateway's SSE replay stream (`/tasks/{id}/stream`) SHALL continue to replay steps
from the Runner's internal task steps endpoint and tail the Kafka `step` topic. The reduced upstream
config does not affect the SSE pathway.

#### Scenario: SSE stream still works end to end
- **WHEN** a client opens `GET /api/tasks/{id}/stream` for a running task
- **THEN** the Gateway replays persisted steps from the Executor and then streams new steps from the `step` topic, deduped by `step.id`

### Requirement: 401/403 authorization flow
The Gateway SHALL maintain the following authorization checks based on the session issued by
Identity:
- **401** on protected routes without a valid session cookie.
- **403** for non-superadmin `/sysadmin/*` routes.
- Session data (`is_superadmin`, `role`) SHALL be injected as request headers
  (`X-User-ID`, `X-User-Name`, `X-User-Email`, `X-User-Superadmin`, `X-Workspace-ID`,
  `X-Workspace-IDs`) by the Gateway middleware.

#### Scenario: Missing session is rejected
- **WHEN** a request to a protected route arrives without a valid session cookie
- **THEN** the Gateway responds 401 without proxying upstream

#### Scenario: Non-superadmin blocked from sysadmin
- **WHEN** a session with `is_superadmin = false` calls a `/sysadmin/*` route
- **THEN** the Gateway responds 403

#### Scenario: Identity headers injected downstream
- **WHEN** an authenticated request is proxied to an upstream service
- **THEN** the upstream receives `X-User-ID`, `X-User-Name`, `X-User-Email`, `X-User-Superadmin`, `X-Workspace-ID`, and `X-Workspace-IDs` headers set by the Gateway, and any inbound client-supplied `X-User-*`/`X-Workspace-*` values are stripped

### Requirement: Workspace skills endpoint
The Gateway SHALL forward `/workspaces/{wid}/skills` requests to the Workspace Service
(skills move from Catalog into the Workspace Service's bounded context after consolidation).

#### Scenario: Skills request routed to Workspace
- **WHEN** a client calls `GET /api/workspaces/{wid}/skills`
- **THEN** the Gateway proxies the request to the Workspace Service upstream

### Requirement: Gateway health probes
The Gateway SHALL continue to probe all downstream services at `/healthz` endpoint, but with
only 4 upstream services instead of 10. The health check response SHALL reflect the
connectivity of these 4 services.

#### Scenario: Healthz reflects 4 upstreams
- **WHEN** `GET /healthz` is called on the Gateway
- **THEN** the response reports connectivity for the gateway itself plus exactly the 4 upstream services (identity, workspace, agent, executor)

### Requirement: No behavioral API changes
The frontend-facing REST/SSE API contract SHALL remain unchanged. All endpoint paths,
request shapes, and response shapes are identical to the pre-consolidation API. The only
internal difference is that the Gateway resolves these endpoints against fewer upstream
services.

#### Scenario: Frontend contract untouched
- **WHEN** the SPA build runs against the consolidated backend
- **THEN** every `frontend/src/api/*.ts` client function works without modification — same paths, same request/response shapes
