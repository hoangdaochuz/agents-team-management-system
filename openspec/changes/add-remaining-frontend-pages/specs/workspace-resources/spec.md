## Purpose

Lets workspace admins manage the resources that scope agent behavior within a workspace: knowledge
sources, skills, plugins, MCP servers, and rules.

## ADDED Requirements

### Requirement: Tabbed resource console
The system SHALL present a per-workspace resources screen with five tabs: Knowledge base, Skills,
Plugins, MCP servers, Rules. Each tab SHALL show a searchable list of that resource type scoped to
the active workspace.

#### Scenario: Switching tabs
- **WHEN** the user selects a tab
- **THEN** only that resource type's list is shown, and the search query is scoped to it

#### Scenario: Empty tab
- **WHEN** a resource type has no entries
- **THEN** an empty state with the relevant "add" affordance is shown

### Requirement: Knowledge base management
The system SHALL list knowledge sources with their indexing status and chunk/page counts, and allow
importing from the repo or adding a source.

#### Scenario: Re-indexing source
- **WHEN** a source is mid-reindex
- **THEN** its status badge reads "re-indexing" and a non-interactive state is shown until complete

### Requirement: Skills, plugins, and rules toggling
The system SHALL let admins enable/disable skills, plugins, and rules inline via a toggle, with the
change reflected immediately in the list.

#### Scenario: Disabling a skill
- **WHEN** the admin toggles an enabled skill off
- **THEN** the skill's status updates to disabled and the API is called to persist it; on failure
  the toggle reverts with an error toast

### Requirement: MCP server status and reconnect
The system SHALL list MCP servers with their transport, tool count, tool names, and connection
status, and SHALL offer a reconnect action for offline servers.

#### Scenario: Reconnecting an offline server
- **WHEN** the admin clicks "Reconnect" on an offline MCP server
- **THEN** the system attempts reconnection via the API and updates the status badge accordingly

### Requirement: Add affordances
The system SHALL provide an "add" affordance per resource type (add source, add skill, install
plugin, connect server, add rule) that opens the corresponding create flow.

#### Scenario: Opening an add flow
- **WHEN** the admin clicks an "add" button for a resource type
- **THEN** a modal/form for that resource type opens; if the create flow is not yet implemented, the
  UI indicates so gracefully
