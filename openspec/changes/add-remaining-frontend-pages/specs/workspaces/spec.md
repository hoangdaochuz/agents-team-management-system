## Purpose

Lets users see, create, and switch between the workspaces they belong to, and establishes the
"active workspace" that scopes every other screen (tasks, agents, resources, members).

## ADDED Requirements

### Requirement: Workspace list
The system SHALL list the workspaces the current user belongs to as cards, each showing the
workspace name, its repository, the user's role, agent count, and open-task count.

#### Scenario: Viewing workspaces
- **WHEN** the user opens `/workspaces`
- **THEN** the system fetches the user's workspaces and renders one card per workspace with the
  stats above, plus a "create workspace" affordance

#### Scenario: No workspaces
- **WHEN** the user belongs to no workspaces
- **THEN** the system shows an empty state with a clear path to create one, never a blank screen

### Requirement: Workspace selection
The system SHALL let the user choose a workspace as active. Selecting a workspace SHALL store it as
the active workspace in the session store and navigate to that workspace's dashboard.

#### Scenario: Selecting a workspace
- **WHEN** the user clicks a workspace card
- **THEN** that workspace becomes the active workspace (persisted across reloads) and the user is
  routed to `/dashboard`

#### Scenario: Workspace scope banner
- **WHEN** any workspace-scoped screen is shown
- **THEN** a scope banner/indicator displays the active workspace name and a control to switch

### Requirement: Create workspace
The system SHALL provide a modal to create a workspace capturing name, repository, default branch,
and the creator's role (Owner/Admin). On success the new workspace becomes active.

#### Scenario: Creating a workspace
- **WHEN** the user submits valid workspace details
- **THEN** the system creates the workspace via the API, shows a success toast, makes it active, and
  navigates to `/dashboard`

#### Scenario: Repository selection
- **WHEN** the user opens the repository field
- **THEN** they may pick a connected repo or choose to connect a new one

### Requirement: Workspace isolation guidance
The system SHALL surface an informational panel explaining workspace isolation (separate repos,
scoped agents & skills, per-workspace members).

#### Scenario: First-time view
- **WHEN** the workspace grid renders
- **THEN** the isolation guidance panel is visible so users understand scoping

### Requirement: Task detail workspace scoping
The task detail view SHALL resolve its data within the session's workspace context:
detail queries SHALL be keyed by the active workspace so switching workspaces invalidates
stale detail state, and opening a task id outside the context SHALL surface the
not-found/error state rather than rendering another workspace's data.

#### Scenario: Opening detail from a scoped list
- **WHEN** the user opens a task from the kanban board
- **THEN** the detail queries are scoped by the active workspace (e.g. `["task", wid, id]`-style keys), so switching the active workspace does not keep stale detail state

#### Scenario: Cross-workspace deep link
- **WHEN** a task id outside the active workspace context is opened directly
- **THEN** the detail view shows the not-found/error state instead of leaking another workspace's data
