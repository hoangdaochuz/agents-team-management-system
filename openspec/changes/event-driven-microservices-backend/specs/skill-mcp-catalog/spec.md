## Purpose

The Catalog service stores reusable Skills (markdown capabilities injected into agent
prompts) and MCP server definitions (external tool bridges), exposing their CRUD surfaces.

## ADDED Requirements

### Requirement: Skill CRUD matching the frontend contract
The service SHALL implement `listSkills`, `getSkill`, `createSkill` (accepting `name,
description?, body_md`), `updateSkill` (partial), and `deleteSkill`, returning the full
`Skill` shape (`id, workspace_id, name, description, body_md, resources_path?,
created_at`).

#### Scenario: Creating a skill
- **WHEN** the frontend calls `createSkill` with a name and `body_md`
- **THEN** the service persists the skill and returns it synchronously with a generated id

#### Scenario: Partial update
- **WHEN** the frontend calls `updateSkill(id, partial)`
- **THEN** only supplied fields change and the updated `Skill` is returned

#### Scenario: Deleting a skill
- **WHEN** the frontend deletes a known skill
- **THEN** the service removes it and responds 204

### Requirement: MCP server CRUD matching the frontend contract
The service SHALL implement `listMcpServers`, `getMcpServer`, `createMcpServer` (accepting
`name, command, args?, env?`), `updateMcpServer` (partial), and `deleteMcpServer`, returning
the full `McpServer` shape (`id, workspace_id, name, command, args, env, created_at`).

#### Scenario: Registering an MCP server
- **WHEN** the frontend calls `createMcpServer` with a name and command
- **THEN** the service persists the definition and returns it synchronously with a
  generated id

#### Scenario: Partial update
- **WHEN** the frontend calls `updateMcpServer(id, partial)`
- **THEN** only supplied fields change and the updated `McpServer` is returned

#### Scenario: Deleting an MCP server
- **WHEN** the frontend deletes a known MCP server
- **THEN** the service removes it and responds 204

### Requirement: Skill enable state and metadata
The `Skill` resource SHALL carry the additive fields the frontend declares: `enabled`
(per-context enable state), `trigger` (natural-language trigger description), and
`step_count` (derived number of steps). These are optional and additive to the core
`Skill` shape; `createSkill`/`updateSkill` accept them and list/get return them.

#### Scenario: Toggling a skill's enable state
- **WHEN** the frontend updates a skill with `enabled: false`
- **THEN** the persisted `Skill` reflects `enabled: false` on subsequent reads

#### Scenario: Skill trigger metadata
- **WHEN** a skill is saved with a `trigger` and `step_count`
- **THEN** those fields are returned by `listSkills` / `getSkill`

### Requirement: Workspace-scoped skill listing and enable state
The service SHALL serve `GET /workspaces/:id/skills` returning the skills of exactly that
workspace (members only), and `PATCH /workspaces/:id/skills/:id` with `{ enabled }` to
toggle the workspace-level enable state, returning the updated `Skill`. Skills are scoped
to their owning workspace (`workspace_id`); cross-workspace reads and toggles SHALL be
rejected.

#### Scenario: Listing a workspace's skills
- **WHEN** the frontend calls `skills.listForWorkspace(workspaceId)`
- **THEN** only that workspace's skills are returned

#### Scenario: Disabling a skill
- **WHEN** the frontend calls `skills.setEnabled(workspaceId, id, false)`
- **THEN** the skill's `enabled` becomes `false` and the updated `Skill` is returned

### Requirement: Workspace scoping of the catalog
Every skill and MCP server SHALL carry a `workspace_id`; list/get/create SHALL be scoped
to the session's workspace context, and `createSkill`/`createMcpServer` SHALL inherit the
workspace from it.

#### Scenario: Listing scoped skills
- **WHEN** the frontend calls `listSkills` in a single-workspace session
- **THEN** only that workspace's skills are returned, each carrying its `workspace_id`
