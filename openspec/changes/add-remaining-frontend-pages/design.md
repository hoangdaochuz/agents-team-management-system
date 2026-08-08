## Context

The SPA implements 7 prototype pages today against a declared API contract in `frontend/src/api/`
(see `client.ts` `request<T>`, `types.ts` domain model, one module per entity). The backend is
Phase 0 and only implements `GET /healthz`; every other call 404s, and the UI is required to render
its full layout in the error/empty state and never crash (`<AsyncBoundary>`). See proposal.md for
*why* the remaining 8 pages and the multi-workspace model are being added.

Key existing conventions this design must honor:
- All HTTP goes through `api/client.ts`'s `request<T>` (base `/api`, throws `ApiError`). One thin
  module per entity; types in `api/types.ts`.
- Server state via TanStack Query (`useQuery`/`useMutation`), UI state via Zustand
  (`store/ui.ts` exists). Invalidation keys are plain arrays.
- Design system = ported CSS (`styles.css`); class names (`.card`, `.tabs`, `.badge`, `.switch`,
  `.kpi`, `.matrix`, `.res-card`, …) are the styling API. No CSS framework.
- Relative imports; strict TS (`noUnusedLocals`/`noUnusedParameters`).
- Prototype HTML is the DOM/class source of truth — each new page mirrors its prototype file.

## Goals / Non-Goals

**Goals:**
- Declare a complete, typed API client and domain model for all 8 remaining pages so the backend
  has a contract to fill in.
- Make the SPA fully navigable and functional end-to-end in the **no-backend** state (graceful
  error/empty states) AND in the future implemented state.
- Introduce auth/session + workspace scoping with minimal disruption to existing pages.

**Non-Goals:**
- Implementing any backend endpoint (still Phase 0; this is frontend-only).
- Real SSO/SAML/OAuth wiring — SSO buttons are declared stubs.
- Persisting/validating the roles-and-permissions matrix as editable config (it is display-only,
  derived from role definitions in the contract).
- Updating CLAUDE.md/docs prose (tracked as a follow-up note, not code).

## Decisions

### D1. Auth/session lives in a Zustand store, not React context
A new `store/auth.ts` (`useAuth`) holds `{ user, activeWorkspace, status, login, logout, hydrate }`,
persisted to `localStorage` (user id + active workspace id) so reloads keep context. `hydrate()`
calls `me()` on boot via a top-level `<AppGate>` that gates rendering on the query settling.

*Why over context:* Zustand is already the project's UI-state choice (`store/ui.ts`), selectors
avoid re-render storms, and non-component code (route guards, query-key derivation) can read it via
`useAuth.getState()` without prop-drilling.

*Alternatives:* React context (rejected — no hook access outside components); TanStack Query as the
sole source (rejected — session is mutable client state with side effects like redirect, not just
cached server data; we still use a query internally for `me()`).

### D2. No-auth "dev fallback" keeps the existing SPA usable
Because the backend is unimplemented, a hard redirect-to-`/login` would brick the entire app today.
`<AppGate>` treats a failed/404 `me()` as: if `import.meta.env.DEV` (or a `VITE_DEV_NOAUTH` flag)
→ synthesize a single-operator session (one workspace, owner role) and show a persistent
"dev-fallback — no backend" badge; otherwise → real redirect to `/login`. This satisfies both the
auth spec (real redirect when backend exists) and the project's "UI must always render" rule today.

### D3. Workspace scoping via query keys + route params, not a global filter
The active workspace id (from `useAuth`) becomes part of TanStack Query keys
(`["tasks", workspaceId]`, `["agents", workspaceId]`) and is injected into API paths
(`/api/workspaces/:wid/...`). Switching workspaces changes the key → automatic refetch; no manual
cache invalidation. Existing single-tenant endpoints (`/api/tasks`, `/api/agents`) are *re-interpreted*
as implicitly scoped to the active workspace in the client layer during the transition (the backend
contract may later move them under `/workspaces/:wid`); a thin wrapper centralizes this so the
existing pages do not all need rewriting.

### D4. Route guards as two layout components
- `<RequireAuth>` wraps `<AppLayout>`: no session → `<Navigate to="/login">`.
- `<RequireRole role=...>` (used inside admin/sysadmin routes): wrong role → `<NoAccess/>` empty
  state (no redirect, so users see they lack permission rather than bouncing).
Auth routes (`/login`, `/signup`, `/pending`) sit *outside* `<AppLayout>` (standalone full-page, like
the Launcher today) and redirect to `/dashboard` if a valid session already exists.

### D5. Domain model expansion (additive, in `api/types.ts`)
New types — see tasks.md for the full enumeration; the load-bearing ones:
`User`, `Session`, `Organization`, `Workspace`, `Membership`/`Member`, `Role` (union),
`Invite`, `SignupRequest`, `KnowledgeSource`, `Plugin`, `FeatureFlag`, `AuditEntry`,
`SystemHealth`/`ServiceHealth`. `Workspace` is referenced by the existing `Project` (a workspace
owns projects/repos) but existing types are **not** mutated to avoid breaking the current 7 pages.
New `api/*.ts` modules: `auth.ts`, `workspaces.ts`, `members.ts`, `invites.ts`, `audit.ts`,
`knowledgeSources.ts`, `plugins.ts`, `featureFlags.ts`, `sysadmin.ts`. Re-exported from `client.ts`.

### D6. Agent builder reuses `Agent` + a builder input type
The builder posts via existing `agents.createAgent`/`updateAgent`, extended with optional builder
fields (temperature, max output tokens, autonomy mode, guardrails, knowledge ids) on the `Agent`
type as optional fields — additive, so the Agents list/form keep working. The builder is a
controlled-form component assembling the create payload from section state; no separate "draft"
endpoint — "Save draft" persists to local state only (matches prototype, which toasts without a
network call).

### D7. Resource lists share one `<ResourceList>` component
Knowledge/Skills/Plugins/MCP/Rules tabs differ only in columns, status badge tone, and toggle-vs-
kebab trailing action. A single table-ish component (mirroring `.res-card`/`.res-item` classes)
parameterized by resource type prevents five near-duplicate implementations.

## Risks / Trade-offs

- **[Forced auth bricks the app in dev]** → Mitigated by D2 dev-fallback; clearly badged so it is
  never mistaken for a real session.
- **[Contract drift: client declares endpoints backend may never implement exactly as named]**
  → Acceptable and intentional per repo convention; mitigated by keeping path shapes consistent
  with existing modules and documenting them in `client.ts`. Backend phase plan in `docs/tasks.md`
  will catch up.
- **[Workspace scoping change touches existing query keys]** → D3 keeps existing endpoints working
  via a wrapper; existing pages are not rewritten, only their invalidation keys gain a workspace
  segment where trivial. Risk of missed key → caught by typecheck + manual nav smoke test.
- **[Large surface → long task list]** → Mitigated by domain-grouped tasks (D-sections below) so
  work can parallelize across disjoint files.
- **[Roles matrix shown as static config could mislead users into expecting edits]** → Labeled
  read-only; spec marks it display-only.

## Migration Plan

Frontend-only; no data migration. Rollout is just a merged branch:
1. Merge domain types + API modules (additive, no behavior change).
2. Merge auth store + `<AppGate>` + dev-fallback (existing routes keep working under synthetic
   session).
3. Merge new pages + routes + sidebar wiring.
4. Update CLAUDE.md "Current state" / "no auth MVP" wording as a separate docs commit.

**Rollback:** revert the merge commit; no schema or persisted state to undo (localStorage keys are
namespaced and ignorable).

## Open Questions

- Exact path shape for workspace-scoping existing entities (`/api/workspaces/:wid/tasks` vs keeping
  `/api/tasks` and passing a header) — deferrable; the client wrapper (D3) isolates this, so it can
  be flipped without touching pages. Defaults to path-scoping for new endpoints, header-free for
  legacy endpoints until backend decides.
- Whether "Save draft" should ever persist server-side — deferred; current design keeps it
  client-local per prototype behavior.
