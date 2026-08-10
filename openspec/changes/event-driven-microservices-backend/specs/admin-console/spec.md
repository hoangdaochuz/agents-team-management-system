## Purpose

Provides workspace-level administration (members, roles, invites, pending approvals, audit
log) and system-level superadministration (organizations, feature flags, system health,
system audit) for authorized operators. The backend-side behavior behind
`frontend/src/api/{members,invites,audit,sysadmin}.ts`. Membership data is owned by the
Orgs/Workspaces service; this capability is its admin-facing contract.

## ADDED Requirements

### Requirement: Members list and role management
The service SHALL serve `GET /workspaces/:id/members` returning `Member` objects
(`id, user: { id, name, email }, role, status: active | invited | suspended,
last_active_at?, is_service_account?`), `PATCH /workspaces/:id/members/:id` with
`{ role }` to change a member's role, `DELETE /workspaces/:id/members/:id` to remove a
member, and `POST /workspaces/:id/members/:id/resend` to re-send a pending invitation.
Role changes SHALL be restricted to `owner | admin` actors, and an owner SHALL NOT be
able to demote the last remaining owner.

#### Scenario: Changing a role
- **WHEN** an admin calls `members.updateRole(workspaceId, memberId, role)`
- **THEN** the member's role is updated and the updated `Member` is returned

#### Scenario: Removing a member
- **WHEN** an admin calls `members.remove(workspaceId, memberId)`
- **THEN** the member is removed from the workspace and the service responds 204

#### Scenario: Last owner protection
- **WHEN** a role change would demote the workspace's last owner
- **THEN** the change is rejected with an error

### Requirement: Pending approvals and invites
The service SHALL serve `GET /workspaces/:id/requests` returning pending `SignupRequest`
objects (`id, name, email, workspace_name?, workspace_id?, requested_role, requested_at`),
`POST /workspaces/:id/requests/:id/approve` and `POST /workspaces/:id/requests/:id/decline`
(which activate or reject the signup — see the auth capability), and
`POST /workspaces/:id/invites` with `{ emails, role }` creating `Invite` records
(`id, email, name?, role, requested_at`) and returning them.

#### Scenario: Approving a request
- **WHEN** an admin calls `invites.approve(workspaceId, requestId)`
- **THEN** the request leaves the pending queue, the user is activated with the requested
  role, and the service responds 204

#### Scenario: Sending invites
- **WHEN** an admin calls `invites.send(workspaceId, { emails, role })`
- **THEN** invite records are created (status `invited` for the members) and returned,
  and signup with each invite code resolves to this workspace

### Requirement: Audit log
The service SHALL record workspace-level admin actions and serve
`GET /workspaces/:id/audit` with an optional `kind` filter, returning `AuditEntry` objects
(`id, actor: { name }, action, action_kind?, target?, created_at, ip?`), and
`POST /workspaces/:id/audit/export` SHALL trigger an export (e.g. emailed CSV or signed
URL) and respond `{ ok: true }`.

#### Scenario: Filtering the audit log
- **WHEN** the frontend calls `audit.list(workspaceId, { kind })`
- **THEN** only entries matching the kind are returned

#### Scenario: Exporting the audit log
- **WHEN** an admin calls `audit.exportLog(workspaceId)`
- **THEN** the service triggers the export and responds `{ ok: true }`

### Requirement: Sysadmin organizations
The service SHALL serve `GET /sysadmin/orgs`, `POST /sysadmin/orgs`
(`{ name, plan }`), `POST /sysadmin/orgs/:id/suspend`, and `POST /sysadmin/orgs/:id/restore`,
returning `Organization` objects (`id, name, subdomain?, plan, workspace_count,
seats_used, seats_total, status: active | trial | suspended, created_at`). These endpoints
SHALL be restricted to superadmin sessions.

#### Scenario: Suspending an org
- **WHEN** a superadmin calls `sysadmin.suspendOrg(id)`
- **THEN** the org's status becomes `suspended` and the updated `Organization` is returned

#### Scenario: Unauthorized access
- **WHEN** a non-superadmin calls any `/sysadmin/*` endpoint
- **THEN** the request is rejected (403) and no system data is returned

### Requirement: Cross-org sign-up requests
The service SHALL serve `GET /sysadmin/requests` returning pending `SignupRequest`s across
organizations, and `POST /sysadmin/requests/:id/approve` to approve a cross-org signup
(activating the user and their organization per the auth capability).

#### Scenario: Approving a cross-org request
- **WHEN** a superadmin approves a cross-org request
- **THEN** the request leaves the pending list and the user's organization becomes active

### Requirement: Feature flags
The service SHALL serve `GET /sysadmin/flags` returning `FeatureFlag` objects
(`key, label, description?, enabled`) and `PATCH /sysadmin/flags/:key` with
`{ enabled }` to toggle a flag, returning the updated `FeatureFlag`. Flags SHALL be
consulted by services at runtime to enable/disable system features.

#### Scenario: Toggling a flag
- **WHEN** a superadmin calls `sysadmin.toggleFeatureFlag(key, enabled)`
- **THEN** the flag's state is persisted and the updated `FeatureFlag` is returned

### Requirement: System KPIs, health, and system audit
The service SHALL serve `GET /sysadmin/kpis` returning `SystemKpis` (`organizations,
orgs_delta?, workspaces, active_users_24h, active_users_delta?, open_seats,
open_seats_delta?`), `GET /sysadmin/health` returning `SystemHealth` with one
`ServiceHealth` (`name, pct: 0..100, status: ok | warn | down`) per tracked service
(composed by the Gateway from per-service health probes, preserving the Gateway as the
only synchronous caller), and `GET /sysadmin/audit` returning a `SystemHealth`-independent
system-audit feed of recent superadmin actions as `AuditEntry` objects.

#### Scenario: Viewing system health
- **WHEN** a superadmin calls `sysadmin.systemHealth()`
- **THEN** each tracked service shows a health percentage and a color-coded status

### Requirement: Maintenance
The service SHALL serve `POST /sysadmin/maintenance`, triggering a maintenance pass (e.g.
cleanup, flag re-evaluation) and responding `{ ok: true }`.

#### Scenario: Running maintenance
- **WHEN** a superadmin calls `sysadmin.runMaintenance()`
- **THEN** the service performs the maintenance pass and responds `{ ok: true }`
