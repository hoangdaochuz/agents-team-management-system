## Purpose

The Gateway BFF (Backend For Frontend) is the sole HTTP entrypoint into the system. This spec
documents its simplification from 10 upstream service env vars to 4, reflecting the reduced
service boundaries after microservice consolidation.

## ADDED Requirements

### Requirement: Reduced upstream environment variables
The Gateway SHALL be configured with exactly 4 upstream environment variables, down from 10:

| Env Var | Target Service | Previously Covered |
|:--------|:---------------|-------------------|
| `UPSTREAM_IDENTITY` | Identity Service | `UPSTREAM_AUTH`, `UPSTREAM_ORGS`, `UPSTREAM_ADMIN` |
| `UPSTREAM_WORKSPACE` | Workspace Service | `UPSTREAM_PROJECT`, `UPSTREAM_TASK`, `UPSTREAM_CATALOG`, `UPSTREAM_RESOURCES` |
| `UPSTREAM_AGENT` | Agent Service | `UPSTREAM_AGENT` (unchanged) |
| `UPSTREAM_EXECUTOR` | Executor Service | `UPSTREAM_RUNNER` (renamed) |

### Requirement: Session composition simplification
The Gateway SHALL compose a user session with a single HTTP call to the Identity Service
(`/internal/identity`) instead of the previous 3 synchronous fan-out calls (Auth → Orgs → enrichment).
- **Before**: Gateway → Auth → Orgs → (enrichment services), with additive latency.
- **After**: Gateway → Identity (single call), with local composition of session data.
- **Response**: The Identity service returns `{ user, workspaces, active_workspace_id?, role, is_superadmin }`
  shape, matching the `Session` type used by the frontend.

### Requirement: SSE composition unchanged
The Gateway's SSE replay stream (`/workspaces/{wid}/stream`) SHALL continue to replay steps
from the Runner's internal task steps endpoint and tail Kafka step topics. The reduced upstream
config does not affect the SSE pathway.

### Requirement: 401/403 authorization flow
The Gateway SHALL maintain the following authorization checks based on the session issued by
Identity:
- **401** on protected routes without a valid session cookie.
- **403** for non-superadmin `/sysadmin/*` routes.
- Session data (`is_superadmin`, `role`) SHALL be injected as request headers
  (`X-User-ID`, `X-User-Name`, `X-User-Email`, `X-User-Superadmin`, `X-Workspace-ID`,
  `X-Workspace-IDs`) by the Gateway middleware.

### Requirement: Workspace skills endpoint
The Gateway SHALL forward `/workspaces/{wid}/skills` requests to the Identity Service (or
Workspace Service, depending on the final routing decision). The skills surface is unified
within the Workspace Service's bounded context after consolidation.

### Requirement: Gateway health probes
The Gateway SHALL continue to probe all downstream services at `/healthz` endpoint, but with
only 4 upstream services instead of 11. The health check response SHALL reflect the
connectivity of these 4 services.

### Requirement: No behavioral API changes
The frontend-facing REST/SSE API contract SHALL remain unchanged. All endpoint paths,
request shapes, and response shapes are identical to the pre-consolidation API. The only
internal difference is that the Gateway resolves these endpoints against fewer upstream
services.