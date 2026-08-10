## Purpose

Owns organizations, workspaces, membership, and the workspace-scoping contract that scopes
every core entity (`/tasks`, `/agents`, `/skills`, `/projects`) and the resource/admin
surfaces to the session's workspace context. The backend-side behavior behind
`frontend/src/api/workspaces.ts`.

## ADDED Requirements

### Requirement: Workspace list with derived stats
The service SHALL serve `GET /api/workspaces`, returning the workspaces the current user
belongs to, each matching the `Workspace` shape (`id, name, repo_source?, glyph?,
description?, agent_count?, open_task_count?, role, created_at`). `agent_count` and
`open_task_count` are derived counts (composed by the Gateway from the Agent and Task
services), and `role` is the current user's role in that workspace.

#### Scenario: Viewing workspaces
- **WHEN** the frontend calls `workspaces.list`
- **THEN** the service returns the user's workspaces with their derived stats and the
  user's role in each
- **AND** a workspace the user does not belong to is never returned

### Requirement: Get a single workspace
The service SHALL serve `GET /api/workspaces/:id`, returning the `Workspace` when the
current user is a member, and 404 otherwise (not exposing the workspace's existence).

#### Scenario: Member fetches a workspace
- **WHEN** the frontend calls `workspaces.get(id)` for a workspace the user belongs to
- **THEN** the service returns that workspace

#### Scenario: Non-member access
- **WHEN** the id belongs to a workspace the user is not a member of
- **THEN** the service responds 404

### Requirement: Create a workspace
The service SHALL accept `POST /api/workspaces` with
`{ name, repo_source, default_branch?, role: owner | admin }`, create the workspace under
the user's organization, make the creator a member with the requested role, bind the
workspace to its repository (`repo_source` + `default_branch`), and return the created
`Workspace`. The created workspace SHALL become the scope for the creator's subsequent
unscoped calls (see the scoping contract below).

#### Scenario: Creating a workspace
- **WHEN** the frontend calls `workspaces.create` with valid details
- **THEN** the workspace is created, the creator becomes a member with the requested
  role, and the created `Workspace` is returned

### Requirement: Active-workspace scoping contract
Core list/mutation endpoints that lack an explicit workspace path segment (`/tasks`,
`/agents`, `/skills`, `/projects`, `/provider-keys` excepted) SHALL be scoped to the
session's workspace context. The context SHALL resolve in this order:
1. an explicit `X-Workspace-ID` request header when present (forward-compatible extension;
   the SPA does not send it today);
2. the session's single workspace when the user belongs to exactly one;
3. otherwise the union of all workspaces the session can access.

Every resource returned by a scoped endpoint SHALL carry its `workspace_id`. Explicitly
scoped paths (`/workspaces/:id/...`) SHALL override the context and SHALL be authorized
against membership in that workspace. The SPA's `active_workspace_id` is a client-side
preference and is NOT sent to the backend; scoping must therefore never require it.

#### Scenario: Single-workspace user
- **WHEN** a user belonging to one workspace calls `listTasks`
- **THEN** only that workspace's tasks are returned, each with its `workspace_id`

#### Scenario: Explicit scoped path
- **WHEN** the frontend calls `skills.listForWorkspace(workspaceId)`
- **THEN** skills are listed for exactly that workspace, subject to membership

#### Scenario: Cross-workspace request denied
- **WHEN** a user calls an endpoint whose explicit `workspaceId` they are not a member of
- **THEN** the request is rejected (404/403) and no data is returned

### Requirement: Workspace ↔ repository binding
A workspace SHALL reference exactly one repository: `Workspace.repo_source` /
`Project.repo_source` are the same repository, and the Project entity owned by the
workspace SHALL carry the same `workspace_id`. Creating a workspace with a `repo_source`
SHALL establish (or reuse) the workspace's Project.

#### Scenario: Workspace with a repo
- **WHEN** a workspace is created with `repo_source`
- **THEN** the workspace's Project exists with the same `workspace_id` and repository source
