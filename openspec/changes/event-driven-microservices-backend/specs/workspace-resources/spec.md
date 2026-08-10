## Purpose

Owns the per-workspace resources that scope agent behavior: knowledge sources (RAG
indexing), plugins, rules (guardrail enforcement), and MCP connection state. These are
distinct from the Catalog's Skill/McpServer *definitions*; this capability manages their
per-workspace lifecycle and status. The backend-side behavior behind
`frontend/src/api/{knowledgeSources,plugins,rules,workspaceMcp}.ts`.

## ADDED Requirements

### Requirement: Workspace-scoped resource lists
The service SHALL serve `GET /workspaces/:id/knowledge`, `GET /workspaces/:id/plugins`,
`GET /workspaces/:id/rules`, and `GET /workspaces/:id/mcp`, each returning only resources
belonging to that workspace and only to members of it (404/403 otherwise). Resources are
referenced by their owning workspace; no cross-workspace leakage SHALL occur.

#### Scenario: Listing a workspace's plugins
- **WHEN** the frontend calls `plugins.list(workspaceId)`
- **THEN** only that workspace's plugins are returned

#### Scenario: Non-member access
- **WHEN** a non-member calls any scoped resource endpoint
- **THEN** the request is rejected and no resources are returned

### Requirement: Knowledge source management and indexing status
The service SHALL serve `GET /workspaces/:id/knowledge` and
`POST /workspaces/:id/knowledge` (accepting `kind: file | folder | url | upload` plus the
source details), returning `KnowledgeSource` objects (`id, title, kind, chunks?, pages?,
status`). Creating or re-creating a source SHALL trigger asynchronous indexing; the
`status` SHALL follow the lifecycle `pending → indexed | failed`, with `reindexing`
reported while an existing source is being re-indexed. `chunks`/`pages` SHALL be populated
when indexing completes.

#### Scenario: Adding a source
- **WHEN** the frontend calls `knowledgeSources.create(workspaceId, input)`
- **THEN** the source is created with `status: "pending"`, indexing starts asynchronously,
  and subsequent reads report `indexed` (with chunk/page counts) or `failed`

#### Scenario: Re-indexing source
- **WHEN** an existing source is being re-indexed
- **THEN** its status reads `reindexing` and updates to `indexed` or `failed` when the
  re-index completes

### Requirement: Plugin enable/disable
The service SHALL serve `PATCH /workspaces/:id/plugins/:id` with `{ enabled }`, persisting
the toggle and returning the updated `Plugin` (`id, name, version, capabilities?, scopes?,
enabled`).

#### Scenario: Disabling a plugin
- **WHEN** the frontend calls `plugins.setEnabled(workspaceId, id, false)`
- **THEN** the plugin's `enabled` becomes `false` and the updated `Plugin` is returned

### Requirement: Rule enable/disable
The service SHALL serve `PATCH /workspaces/:id/rules/:id` with `{ enabled }`, returning the
updated `Rule` (`id, name, description?, enabled`). An enabled rule SHALL be enforced by
the agent-execution-runner as a guardrail constraint on runs in that workspace.

#### Scenario: Enforcing a rule
- **WHEN** the frontend sets a rule's `enabled: true`
- **THEN** the rule is persisted as enforced and the updated `Rule` is returned
- **AND** the runner SHALL apply the rule's constraint to subsequent runs in that workspace

### Requirement: MCP connection state and reconnect
The service SHALL serve `GET /workspaces/:id/mcp` returning `McpConnection` objects
(`id, name, transport: stdio | http, tool_count, tool_names?, status: connected | offline`),
and `POST /workspaces/:id/mcp/:id/reconnect` SHALL attempt to re-establish the connection
and return the updated connection with its current status.

#### Scenario: Listing connections
- **WHEN** the frontend calls `workspaceMcp.list(workspaceId)`
- **THEN** each MCP connection shows its transport, tool counts/names, and connection status

#### Scenario: Reconnecting an offline server
- **WHEN** the frontend calls `workspaceMcp.reconnect(workspaceId, id)` on an offline connection
- **THEN** the service attempts reconnection and returns the connection with `status:
  "connected"` on success or `"offline"` if it still fails

### Requirement: Relationship to the Catalog
MCP connections and Skill enable state reference the Catalog's `McpServer` and `Skill`
definitions: a connection SHALL correspond to an McpServer definition (the definition's
`command`/`args`/`env` launch the connection), and a workspace's skill enable state is the
`Skill.enabled` field managed via `/workspaces/:id/skills` (see skill-mcp-catalog).

#### Scenario: Connection references a definition
- **WHEN** a workspace connects an MCP server
- **THEN** the connection is tied to a Catalog `McpServer` definition and reflects its
  launch configuration
