## Purpose

Manages the lifecycle of a registered repository (a "project") that tasks run against, and
exposes the Project resource the frontend manipulates via `/projects`.

## ADDED Requirements

### Requirement: List projects
The Project service SHALL return all registered projects via `GET /api/projects`, each
matching the `Project` shape (`id, workspace_id, name, repo_source, repo_type, cloned_path,
default_branch, created_at`).

#### Scenario: Fetching the project list
- **WHEN** the frontend calls `listProjects`
- **THEN** the service returns a JSON array of `Project` objects scoped to the session's
  workspace context, each carrying its `workspace_id`

### Requirement: Get a single project
The service SHALL return one project via `GET /api/projects/:id`, returning 404 when the id
does not exist.

#### Scenario: Existing project
- **WHEN** the frontend calls `getProject(id)` for a known id
- **THEN** the service returns that `Project`

#### Scenario: Unknown project
- **WHEN** the id does not exist
- **THEN** the service responds 404

### Requirement: Create a project
The service SHALL create a project from `POST /api/projects` accepting
`{ name, repo_source, repo_type, default_branch? }`, defaulting `default_branch`
sensibly, and returning the created `Project` (including generated `id`, `cloned_path`,
and `created_at`).

#### Scenario: Creating from a URL
- **WHEN** the frontend creates a project with `repo_type: "url"`
- **THEN** the service records the source and returns the created `Project` with a
  `cloned_path`

### Requirement: Update a project
The service SHALL apply a partial update via `PUT /api/projects/:id` and return the updated
`Project`.

#### Scenario: Renaming a project
- **WHEN** the frontend sends a partial update changing `name`
- **THEN** only the supplied fields change and the updated `Project` is returned

### Requirement: Delete a project
The service SHALL delete a project via `DELETE /api/projects/:id`, returning 204 on success
and 404 when absent.

#### Scenario: Deleting an existing project
- **WHEN** the frontend deletes a known project
- **THEN** the service removes it and responds 204

### Requirement: Workspace scoping of projects
Every project SHALL carry a `workspace_id` and SHALL bind one workspace to its repository
(a workspace's `repo_source` and its project's `repo_source` are the same repository, per
the workspaces capability). `listProjects`, `getProject`, `createProject`, and updates
SHALL be scoped to the session's workspace context; `createProject` SHALL inherit the
workspace from it, and access to projects outside the context SHALL be rejected (404).

#### Scenario: Creating a scoped project
- **WHEN** the frontend creates a project in a single-workspace session
- **THEN** the created `Project` carries the session workspace's `workspace_id`
