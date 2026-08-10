# Tasks — add-remaining-frontend-pages

Frontend-only. Mirror each prototype file's DOM/class structure; all HTTP via `api/client.ts`
`request<T>`; server state via TanStack Query + `<AsyncBoundary>`; CSS via existing class names in
`styles.css` (add new classes only if a prototype uses one not yet ported). Relative imports,
strict TS. Each page must render its full layout in error/empty states. Run `make web-typecheck`
and `make web-build` after each group.

## 1. Shared foundation: domain types + API modules

- [x] 1.1 Add new types to `frontend/src/api/types.ts`: `User`, `Session`, `Organization`,
      `Workspace` (id, name, repo_source, glyph, description, agent_count, open_task_count,
      created_at), `Role` (`"owner"|"admin"|"member"`), `MemberStatus`
      (`"active"|"invited"|"suspended"`), `Member` (user + role + status + last_active_at),
      `Invite`, `SignupRequest`, `KnowledgeSource` (title, type, chunks/pages, index_status),
      `Plugin` (name, version, capabilities, scopes, enabled), `FeatureFlag` (key, label,
      description, enabled), `AuditEntry` (actor, action, target, time, ip), `ServiceHealth`
      (name, pct, status), `SystemHealth`, `AutonomyMode`, `Guardrails`, `Plan`
      (`"free"|"team"|"pro"|"enterprise"`), `OrgStatus`. Extend `Agent` *additively* with optional
      builder fields: `temperature?`, `max_output_tokens?`, `autonomy_mode?`, `guardrails?`,
      `knowledge_source_ids?`, `provider?`.
- [x] 1.2 Create `frontend/src/api/auth.ts`: `login(email,password,remember)`, `signup(input)`,
      `me()`, `logout()`, `signupStatus()`, `resendSignupNotification()`, `ssoBegin(provider)`.
      Re-export from `client.ts`.
- [x] 1.3 Create `frontend/src/api/workspaces.ts`: `list()`, `get(id)`, `create(input)`,
      `setActive(id)` (client-side only). Re-export.
- [x] 1.4 Create `frontend/src/api/members.ts`: `list(wid)`, `updateRole(wid,memberId,role)`,
      `remove(wid,memberId)`, `resendInvite(wid,memberId)`; `invites.ts`: `listPending(wid)`,
      `approve(wid,id)`, `decline(wid,id)`, `send(wid,{emails,role})`. Re-export.
- [x] 1.5 Create `frontend/src/api/audit.ts`: `list(wid,{filter})`, `exportLog(wid)`. Re-export.
- [x] 1.6 Create resource modules: `knowledgeSources.ts`, `skills.ts` (extend existing if any),
      `plugins.ts`, `workspaceMcp.ts`, `rules.ts` — each `list(wid)` + `setEnabled(wid,id,bool)` /
      `reconnect` / `add`. Re-export from `client.ts`.
- [x] 1.7 Create `frontend/src/api/sysadmin.ts`: `listOrgs()`, `suspendOrg(id)`, `restoreOrg(id)`,
      `createOrg(input)`, `listSignupRequests()`, `approveSignup(id)`, `listFeatureFlags()`,
      `toggleFeatureFlag(key,bool)`, `systemHealth()`, `systemAudit()`, `runMaintenance()`.
      Re-export.
- [x] 1.8 Add a workspace-scoping helper in `client.ts` (or `lib/workspace.ts`) that derives the
      active workspace id from the auth store and prefixes paths for new workspace-scoped calls;
      legacy calls stay un-scoped (design D3).

## 2. Auth: session store, gate, guards, and auth pages

- [x] 2.1 Create `frontend/src/store/auth.ts` (`useAuth`): state `{user, activeWorkspace, status,
      devFallback}`, actions `hydrate()`, `setActiveWorkspace(id)`, `login`, `logout`; persist
      `{userId, activeWorkspaceId}` to localStorage (namespaced key). Selector helpers
      `useActiveWorkspace()`, `hasRole(role)`.
- [x] 2.2 Create `frontend/src/components/auth/AppGate.tsx`: on mount run `me()` via TanStack
      Query; on success hydrate store; on error in DEV/no-auth → synthesize single-operator session
      + `devFallback=true` + visible badge; otherwise leave unauthenticated. Render children only
      after settle (loading splash otherwise). Wrap `<App>` root in it.
- [x] 2.3 Create `frontend/src/components/auth/RequireAuth.tsx` (no session →
      `<Navigate to="/login" state={{from}}>`) and `RequireRole.tsx` (wrong role → `<NoAccess/>`
      without redirect). Create `NoAccess.tsx` empty-state component.
- [x] 2.4 Port `prototype/login.html` → `frontend/src/pages/LoginPage.tsx`: split auth layout,
      email/password form (keep-signed-in checkbox), SSO buttons (call `auth.ssoBegin`, stubbed),
      links to signup. `useMutation(auth.login)` → on success set session + navigate to `from` or
      `/dashboard`; inline error on `ApiError`. Render full layout on backend error.
- [x] 2.5 Port `prototype/signup.html` → `SignupPage.tsx`: name/email/password, segmented
      "Join a workspace" vs "Create new org" (toggle invite-code vs org-name field), terms checkbox
      (block submit until checked), "Request access" → `auth.signup` → navigate `/pending`; SSO
      button.
- [x] 2.6 Port `prototype/pending-approval.html` → `PendingApprovalPage.tsx`: centered layout,
      4-step progress (admin review current), show requesting email + workspace name from store/
      signupStatus, "Resend notification" → `auth.resendSignupNotification` + toast, "Back to sign
      in" → `/login`.
- [x] 2.7 Add standalone routes `/login`, `/signup`, `/pending` *outside* `<AppLayout>` in
      `App.tsx`; redirect to `/dashboard` if already authenticated.

## 3. Workspaces + workspace resources

- [x] 3.1 Build `WorkspaceScopeBanner` component (active workspace name + switch control) for use
      across workspace-scoped screens; add "Switch workspace" affordance to Topbar.
- [x] 3.2 Port `prototype/workspaces.html` → `WorkspacesPage.tsx`: workspace grid (card per
      `workspaces.list` via `useQuery(["workspaces"])` + `<AsyncBoundary>`), each card shows
      name/repo/role/agents/open-tasks and selects active workspace on click → navigate
      `/dashboard`; isolation-guidance panel; "New workspace" → modal.
- [x] 3.3 Build `NewWorkspaceModal` (name, repository select with "connect new repo", default
      branch, your-role select) → `workspaces.create` mutation + toast + set active + nav
      `/dashboard`.
- [x] 3.4 Build shared `<ResourceList>` component (mirrors `.res-card`/`.res-item`): props for
      columns, status-badge tone mapping, trailing toggle vs kebab, search filter.
- [x] 3.5 Port `prototype/workspace-resources.html` → `WorkspaceResourcesPage.tsx`: breadcrumb
      (Workspaces), scope banner, 5 tabs (Knowledge base | Skills | Plugins | MCP servers | Rules),
      each tab a `<ResourceList>` fed by its resource module's `list(activeWorkspaceId)`, per-tab
      search, enable/disable toggles (`setEnabled` mutations with optimistic update + revert on
      error), MCP "Reconnect" action, and `+ Add …` affordances (modals may be stubs that indicate
      "not implemented" gracefully).

## 4. Agent builder

- [x] 4.1 Build builder section components under `frontend/src/components/agents/builder/`:
      `IdentitySection`, `ModelSection` (provider/model selects, temperature slider with decimal
      readout, max tokens), `PromptsSection` (system + user-prompt textareas with clickable
      `{{variable}}` insertion tags), `AutonomySection` (3 radio modes), `SkillsSection` /
      `ToolsMcpSection` / `KnowledgeSection` (toggle chips/checkboxes), `GuardrailsSection`
      (switches + max-steps/wall-clock number inputs).
- [x] 4.2 Build `AgentBuilderSummary` sticky card (workspace, model, skill count, tool count,
      autonomy) driven by shared builder form state.
- [x] 4.3 Port `prototype/agent-builder.html` → `AgentBuilderPage.tsx` (`/agents/builder` new,
      `/agents/builder/:id` edit): compose sections + summary, breadcrumb Agents, Cancel →
      `/agents`, "Create agent" → assemble payload → `agents.createAgent`/`updateAgent` mutation →
      toast + nav `/agents`; "Save draft" → persist to component state only + toast. If editing,
      seed form from `agents.getAgent(id)`.

## 5. Admin console (members, roles, audit)

- [x] 5.1 Port `prototype/admin.html` → `AdminPage.tsx` (inside `<RequireRole role="admin">`):
      breadcrumb, owner-only notice banner, tabs (Members | Roles & permissions | Audit log).
- [x] 5.2 Members tab: table from `members.list(activeWorkspaceId)` (Member | Role | Status | Last
      active | kebab), inline role `<select>` → `members.updateRole`; seat-usage footer note;
      pending-approvals card with approve/decline (`invites.approve/decline`) removing the row +
      toast; "+ Invite people" → InviteModal.
- [x] 5.3 Build `InviteModal` (emails textarea, role segmented Member/Admin) →
      `invites.send` + toast + close.
- [x] 5.4 Roles & permissions tab: read-only capability matrix (Owner/Admin/Member × 9
      capabilities) using `.matrix`; mark display-only.
- [x] 5.5 Audit log tab: table from `audit.list(activeWorkspaceId)` (Actor | Action | Target |
      Time | IP) + "Export audit log" → `audit.exportLog` + toast.

## 6. Sysadmin console

- [x] 6.1 Port `prototype/sysadmin.html` → `SysadminPage.tsx` (inside `<RequireRole
      role="owner">` treating superadmin as owner, or a `superadmin` flag): SUPERADMIN badge header
      + 4 KPI tiles (orgs, workspaces, active users 24h, open seats) from `sysadmin` aggregates.
- [x] 6.2 System health card (per-service progress bars, color-coded) from `sysadmin.systemHealth`;
      feature-flags card with `Switch` toggles → `sysadmin.toggleFeatureFlag`; system-audit feed
      from `sysadmin.systemAudit`; "Run maintenance" button → `sysadmin.runMaintenance` + toast.
- [x] 6.3 Organizations table from `sysadmin.listOrgs` (Org | Plan | Workspaces | Seats | Status |
      actions) with suspend/restore (`sysadmin.suspendOrg/restoreOrg`) updating the row.
- [x] 6.4 Cross-org sign-up requests table from `sysadmin.listSignupRequests` with pending-count
      badge and approve action (`sysadmin.approveSignup`) decrementing the badge.

## 7. Routing, shell wiring, polish

- [x] 7.1 Update `frontend/src/App.tsx`: wrap app-shell routes in `<RequireAuth>`; add
      `/workspaces`, `/workspaces/:id/resources`, `/agents/builder`, `/agents/builder/:id`,
      `/admin` (under `<RequireRole role="admin">`), `/sysadmin` (under superadmin/owner guard).
- [x] 7.2 Update `Sidebar.tsx`: add nav entries (Workspaces, Resources, Agent builder, Admin) with
      role-gated visibility from `useAuth`; replace hardcoded footer user (Dang Anh) with the
      session user; add workspace switcher in brand/topbar area.
- [x] 7.3 Add any missing CSS classes to `frontend/src/styles.css` by porting from
      `prototype/assets/app.css` (e.g. `.auth-*` split layout, `.matrix`, `.res-card`, `.res-item`,
      `.step-progress`) — verbatim port, do not restyle.
- [x] 7.4 Smoke-test navigation across all 15 pages in dev (dev-fallback session): every page
      renders without crashing in the 404/empty state; guards redirect/no-access correctly; SSO
      and "add" stubs degrade gracefully.
- [x] 7.5 Run `make web-typecheck` and `make web-build` clean; fix all strict-TS errors
      (`noUnusedLocals`/`noUnusedParameters`).
- [x] 7.6 Add a follow-up note (commit message or `docs/tasks.md`) that CLAUDE.md "no auth MVP"
      wording needs updating once backend implements auth.

## 8. Follow-ups (noted from conformance verification pass — not yet implemented)

Items found by the code-vs-spec audit (`docs/conformance-report.md` §1 + verification pass)
that were not part of the original build. Implement or explicitly close each one.

- [ ] 8.1 Task detail workspace scoping: key `TaskDetailPage` queries by the active
      workspace (`["task", wid, id]`, `["artifacts", wid, id]`) per the new workspaces spec
      requirement "Task detail workspace scoping".
- [ ] 8.2 Dead API surface: wire up or intentionally retire the declared-but-unused client
      functions (`projects.*`, `runs.listRuns/listRunSteps/listRunFindings`, `tasks.openPr`,
      `members.remove/resendInvite`, `knowledgeSources.create`, `sysadmin.createOrg`,
      `agents.deleteAgent/attachSkill/detachSkill/attachMcp/detachMcp`,
      `skills.listSkills/getSkill/createSkill/updateSkill/deleteSkill`,
      `mcpServers.updateMcpServer/deleteMcpServer`, `providerKeys.updateProviderKey`,
      `workspaces.get`). If kept as declared-contract-only, annotate them as such.
- [ ] 8.3 Admin seats cap: source `seats_total` from the organization/plan data instead of the
      hardcoded `/10` literal (`AdminPage.tsx`).
- [ ] 8.4 AgentBuilderPage payload: stop double-mapping — `role_title` should not also be
      written into `role` (`AgentBuilderPage.tsx` `toPayload`).
- [ ] 8.5 Optimistic rollback on failure for the sysadmin suspend/restore rows and the MCP
      "Reconnect" badge (mirror the FeatureFlags revert-on-error pattern).
- [ ] 8.6 `Sidebar` sign-out: bound `auth.logout()` (timeout/guard) so a hung POST cannot
      delay the local logout + redirect.
- [ ] 8.7 Align `CreateTaskModal`/`AgentFormModal` cache keys with the scoped keys
      (`["agents", wid]`, `["tasks", wid]`) to avoid duplicate fetches.
- [ ] 8.8 `NewWorkspaceModal`: drop the redundant `setActiveWorkspace` after `hydrate`
      (hydrate already sets the new workspace active).
