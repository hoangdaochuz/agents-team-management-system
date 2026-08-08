## Why

The SPA currently implements 7 of 15 prototype pages (Launcher, Dashboard, Kanban, Task detail,
Agents, History, Settings). The remaining 8 — auth (login/signup/pending-approval), workspaces and
workspace resources, the agent builder, and the admin/sysadmin consoles — are the visual source of
truth in `prototype/` but have no frontend code and no declared API contract. This change completes
the frontend SPA against the prototype and declares the full API client for those pages so the
backend can fill in phase by phase, following the repo's "frontend declares the contract now"
convention. The system also expands beyond the current single-operator no-auth MVP to the
multi-workspace, role-based model the prototype portrays.

## What Changes

- **Auth & session (new).** Functional login, signup (join-workspace vs. create-org modes), and a
  post-signup "awaiting approval" page, backed by a typed `auth` API client
  (`POST /api/auth/login`, `/signup`, `GET /api/auth/me`, `POST /api/auth/logout`, signup-status
  poll), a `useAuth` Zustand session store, and a `<RequireAuth>`/`<RequireRole>` route guard.
  Email/password plus stubbed SSO (Google Workspace, SAML) buttons.
- **Workspaces (new).** Workspace switcher grid with per-workspace stats, a "new workspace" modal,
  workspace scope banner, and selection of the active workspace (persisted in the auth/session
  store). Navigation into a workspace goes to the dashboard.
- **Workspace resources (new).** A tabbed resources screen per workspace: Knowledge base, Skills,
  Plugins, MCP servers, Rules — each a searchable list with status badges, toggles, and kebab
  actions. Declares resource-management endpoints scoped to a workspace.
- **Agent builder (new).** A full create/edit agent wizard (Identity, Model, Prompts, Mode &
  autonomy, Skills, Tools & MCP, Rules & guardrails, Knowledge access) with a sticky summary card.
  Reuses the existing `Agent` domain plus builder-only fields (temperature, max tokens, autonomy
  mode, guardrail toggles).
- **Admin console — members & roles (new).** Members table, roles-and-permissions matrix, audit
  log, pending-approvals queue, and an invite modal. Declares member/invite/audit endpoints.
- **Sysadmin console (new).** System-wide view: organizations table, cross-org sign-up requests,
  feature-flag toggles, system-health metrics, and system-audit feed. Declares superadmin endpoints.
- **Domain model expansion (new).** New shared types in `api/types.ts`: `Organization`, `Workspace`,
  `Member`, `Role`, `Invite`, `SignupRequest`, `KnowledgeSource`, `Plugin`, `FeatureFlag`,
  `AuditEntry`, `SystemHealth`, and auth/session types, with matching `api/*.ts` client modules.
- **Routing & shell wiring.** New routes under `/login`, `/signup`, `/pending`, `/workspaces`,
  `/workspaces/:id/resources`, `/agents/builder[/:id]`, `/admin`, `/sysadmin`; sidebar/topbar
  updated for workspace scope and role-gated nav entries. Existing routes become guarded.
- **BREAKING (frontend-only):** `/` Launcher and all app-shell routes now require a session; the app
  redirects unauthenticated users to `/login`. This contradicts the current CLAUDE.md "no auth MVP"
  stance and is an intentional move to the multi-user model the prototype depicts. (Backend remains
  Phase 0; guards degrade gracefully — see design.md "No-auth dev fallback".)

## Capabilities

### New Capabilities
- `auth`: Authentication, signup, approval flow, session, and route access control.
- `workspaces`: Workspace listing, creation, selection/switching, and scope.
- `workspace-resources`: Per-workspace management of knowledge, skills, plugins, MCP servers, rules.
- `agent-builder`: Create/edit agent configuration wizard and its API.
- `admin-console`: Workspace members, roles, invites, audit log; sysadmin organizations, feature
  flags, system health, and system audit.

### Modified Capabilities
<!-- No existing specs exist yet (openspec/specs is empty); all behavior is newly declared. -->

## Impact

- **Frontend code:** new pages under `frontend/src/pages/` (LoginPage, SignupPage,
  PendingApprovalPage, WorkspacesPage, WorkspaceResourcesPage, AgentBuilderPage, AdminPage,
  SysadminPage); new components (RoleMatrix, ResourceList, AgentBuilder sections, InviteModal,
  WorkspaceSwitcher, StepProgress); new `frontend/src/api/` modules (`auth.ts`, `workspaces.ts`,
  `members.ts`, `audit.ts`, `sysadmin.ts`, resource modules) and expanded `types.ts`; new
  `frontend/src/store/auth.ts`; route guards; routing + sidebar/topbar changes in `App.tsx` and
  `components/shell/`.
- **API contract (declared, not implemented):** roughly 25–35 new endpoints under `/api/auth`,
  `/api/workspaces`, `/api/workspaces/:id/{members,resources,...}`, `/api/admin`, `/api/sysadmin`.
  Backend is Phase 0 and 404s all of these today; the UI must render full layout in error/empty
  states per existing convention.
- **Dependencies:** no new runtime libraries — uses existing React Router, TanStack Query, Zustand.
- **Docs:** CLAUDE.md "Current state" and "no auth MVP" notes will need updating once this lands;
  captured as a follow-up note, not part of this change's code.
