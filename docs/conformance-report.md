# Code-vs-Spec Conformance Report

**Date:** 2026-08-09
**Scope:** Frontend SPA vs `openspec/changes/add-remaining-frontend-pages`; Go backend vs
`openspec/changes/event-driven-microservices-backend`.
**Method:** Two independent read-only code scans (frontend and backend), each requirement
of every capability spec checked against the implementation with `file:line` evidence.
**Status of the changes audited:** frontend change marked **complete**; backend change
**mid-implementation** (18/46 tasks checked at time of audit).

---

## 1. Frontend conformance (`add-remaining-frontend-pages`)

**Verdict: NOT exactly per spec — 24/30 requirements IMPLEMENTED, 6 PARTIAL, 0 MISSING.**

| Capability | IMPLEMENTED | PARTIAL | MISSING |
|---|---|---|---|
| auth | 3 | 3 | 0 |
| workspaces | 3 | 1 | 0 |
| workspace-resources | 4 | 1 | 0 |
| agent-builder | 7 | 0 | 0 |
| admin-console | 7 | 1 | 0 |
| **Total** | **24** | **6** | **0** |

### 1.1 Two genuinely broken behaviors (security-relevant)

1. **Sysadmin access control is wrong.** `App.tsx:62` gates `/sysadmin` on
   `role="owner"`, and `hasRole("owner")` (`frontend/src/store/auth.ts:96-104`) returns
   true for workspace **admins**, not just superadmins. A non-superadmin workspace admin
   can reach the system console, violating the spec's "Access SHALL require the superadmin
   role" and its "no access" scenario.
2. **Sign-out never calls the API.** `POST /api/auth/logout` is declared
   (`frontend/src/api/auth.ts:44-46`) but has zero callers; `Sidebar.tsx:106` only clears
   the local Zustand store. The server-side session is never invalidated.

### 1.2 Per-capability detail

**auth** (3 partial)
- SSO entry points: SAML button missing on the sign-up screen (`SignupPage.tsx:157-171`
  renders Google only); spec requires both Google Workspace and SAML on sign-in *and*
  sign-up.
- Session & current user: hydration, persistence, and logout-store clearing work
  (`store/auth.ts:65-94`), but the logout API call is never made (see 1.1 #2).
- Route access control: `RequireAuth` preserves return-to path (`RequireAuth.tsx:19-21`),
  `RequireRole`/`NoAccess` exist; gaps are (a) `/` Launcher is not guarded (`App.tsx:31`)
  despite the proposal's BREAKING "all routes require a session", (b) sysadmin role gate
  bug (see 1.1 #1).
- Minor: `PendingApprovalPage.tsx:14-18` fetches signup status once — no `refetchInterval`
  polling despite the "signup-status poll" requirement.

**workspaces** (1 partial)
- Create workspace: modal captures name/repo/branch/role and makes it active
  (`NewWorkspaceModal.tsx:23-33`), but does **not** navigate to `/dashboard` after
  success (spec scenario); "Connect a new repo…" option is inert (submits
  `repo_source: ""`).

**workspace-resources** (1 partial)
- MCP status & reconnect: transport/tool_count/status + Reconnect with optimistic update
  implemented (`WorkspaceResourcesPage.tsx:163-201`), but `tool_names` is never rendered
  (`types.ts:138` declares it).

**agent-builder** (7/7) — fully conformant: 8 sections + sticky summary, identity/model,
variable-tag prompt insertion, autonomy modes, chips toggling, guardrails, create/save/
cancel with toasts and navigation (`AgentBuilderPage.tsx`).

**admin-console** (1 partial)
- Sysadmin organizations view: table + suspend/restore implemented
  (`SysadminPage.tsx:172-242`), but the superadmin-only access control is broken (1.1 #1).

### 1.3 Cross-cutting gaps

- **Workspace scoping (design D3) not applied to pre-existing pages.** Dashboard/Kanban/
  Agents/History query keys (`["tasks"]`, `["agents"]`, … at `DashboardPage.tsx:19`,
  `KanbanPage.tsx:24`) carry no workspace id; `requireWorkspaceId()` (`lib/workspace.ts:13`)
  was built but unused. Switching the active workspace does not refetch scoped data.
- `/workspaces/:id/resources` ignores its `:id` route param (`WorkspaceResourcesPage.tsx:20`
  uses the session's active workspace instead) — deep links to a specific workspace's
  resources cannot work.
- ~20 declared API functions never called by any page/component: `projects.*` (entire
  module), `runs.listRuns/listRunSteps/listRunFindings`, `tasks.openPr`, `auth.logout`,
  `members.remove/resendInvite`, `knowledgeSources.create`, `sysadmin.createOrg`,
  `agents.deleteAgent/attachSkill/detachSkill/attachMcp/detachMcp`,
  `skills.listSkills/getSkill/createSkill/updateSkill/deleteSkill`,
  `mcpServers.updateMcpServer/deleteMcpServer`, `providerKeys.updateProviderKey`,
  `workspaces.get`. Dead contract surface that typecheck cannot flag.
- Minor: admin seat total hardcoded (`AdminPage.tsx:240`); `Agent` payload maps
  `role_title` into both `role` and `role_title` (`AgentBuilderPage.tsx:409-427`).

---

## 2. Backend conformance (`event-driven-microservices-backend`)

**Verdict: 15/82 requirements IMPLEMENTED, 9/82 PARTIAL, 58/82 MISSING.** Every missing
item corresponds to an **unchecked** task in `tasks.md` (expected mid-implementation); no
untracked gaps. No contradictions with the spec in the implemented subset — endpoint
paths and JSON shapes match the frontend contract for the 7 routed domains.

### 2.1 What is implemented

- **event-bus 5/5** — topic catalog + payload schemas (`backend/internal/contracts/events.go:11-125`),
  per-task partitioning (`kafka.go:56-80`), consumer groups (`kafka.go:89-119`, verified
  by two-group round-trip test `kafka_test.go:109-110`), at-least-once + `EventID` dedup
  (`kafka.go:130-146`), KRaft compose (`deploy/docker-compose.yml:27-52`).
- **api-gateway (4/8)** — transparent passthrough proxy (`proxy.go:18-39`), normalized
  error contract without internals (`httputil.go:35-44`), no service-to-service sync
  coupling, request-id + panic recovery (`svcrun/middleware.go:18-46`).

### 2.2 Partial requirements

| Requirement | What exists | What's missing |
|---|---|---|
| Gateway route table | Reverse proxy for 7 domains (`routes.go:26-140`): projects, tasks, agents, skills, mcp-servers, provider-keys, runner sub-routes | 10 of 17 `frontend/src/api/*.ts` modules → hard-coded 501 (`routes.go:39,107-113`): all `/workspaces/:id/...`, `/sysadmin/...`, `/auth/...`, and `/tasks/:id/stream` |
| Project CRUD | list/get/create/update/delete + 404s (`handlers.go`, `store.go`) | `workspace_id` absent from shape (`dto.go:62-70`); `cloned_path` returns `''` (no clone op) |
| Task CRUD + feedback | All `TaskQuery` filters (`store.go:78-137`), create defaults `backlog`, partial update, feedback list/add (`store.go:243-266`) | `workspace_id`; `comments_count`/`attachments_count` never populated; **`patchStatus→doing` does not publish `task.run-requested`** (`store.go:219-228`, saga deferred) |
| Agent CRUD + attach/detach | CRUD + skill/mcp join tables with 204s (`store.go:73-196`) | `workspace_id` + all builder fields (`role_title, provider, temperature, max_output_tokens, autonomy_mode, user_prompt_template, knowledge_source_ids, guardrails`) absent from `contracts.Agent` (`dto.go:99-113`); `status`/`load`/`current_task_id` editable but never derived from facts |
| Catalog CRUD | Skill + McpServer CRUD (`store.go:56-248`) | `workspace_id` and `enabled/trigger/step_count` absent (`dto.go:116-133`); `/workspaces/:id/skills` + `setEnabled` → 501 |

### 2.3 Fully missing capabilities (all expected — tasks unchecked)

`provider-key-settings` (5), `agent-execution-runner` (7), `realtime-streaming` (4),
`auth` (8), `workspaces` (5), `workspace-resources` (6), `admin-console` (8), plus the
Gateway's session/workspace-resolution requirements (3). Notably: Settings and Runner are
route-less stubs (`settings/.../routes.go:11-15`, `runner/.../routes.go:13-17`); no
service imports `internal/kafka`; mTLS certs exist in `deploy/certs/` but no code consumes
them; `ENCRYPTION_KEY` (`config.go:27`) is unused.

### 2.4 Spec-vs-tasks drift (checked `[x]` tasks that are not actually done)

| Task | Claim | Reality |
|---|---|---|
| 1.1 | Per-service dirs incl. `auth,orgs,resources,admin` | Only 7/11 exist; the 4 multi-tenant dirs have no skeleton |
| 1.3 | 10 logical Postgres DBs + init script | Only 6 DBs (`deploy/postgres/01-create-databases.sql:10-15`); `.env.example` still says 6 |
| 4.1 | Route table exposes the full `/api` surface | 7/17 modules; 10 domains hard-code 501 |
| 4.5 | Contract test: every `*.ts` function resolves | `gateway_test.go:43-66` asserts routing for in-scope domains and asserts 501s for the rest |
| 2.3–2.5 | Producer/consumer helpers + round-trip | Helpers + test exist but are imported by **no service** — the event-driven backbone is inert |

No false claims found among the *unchecked* tasks — their code is genuinely absent.

### 2.5 Biggest deviations

1. **`workspace_id` is absent everywhere** — no DTO field, no migration column, no scoped
   query in project/task/agent/catalog. The single most load-bearing addition of the
   consolidated backend spec (design D8) has zero implementation.
2. **The saga never starts** — `patchStatus→doing` persists the status but never emits
   `task.run-requested`, so no run can begin even when the runner exists.
3. **Settings and Runner are inert stubs** — the secret boundary and the agent loop exist
   only as directory skeletons.

---

## 3. Bottom line

- **Frontend: ~80% exact.** 0 missing requirements, 6 partials. Two violations matter:
  the sysadmin role gate (non-superadmin access) and sign-out never calling the API.
- **Backend: far from complete, as expected** (18/46 tasks), and 4 checked tasks
  overstate what exists (service skeletons, DB count, route coverage, Kafka wiring).
- **No spec contradictions** in the code that exists — the gaps are missing
  implementation, not wrong implementation.
