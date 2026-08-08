## Purpose

The Catalog service stores reusable Skills (markdown capabilities injected into agent
prompts) and MCP server definitions (external tool bridges), exposing their CRUD surfaces.

## ADDED Requirements

### Requirement: Skill CRUD matching the frontend contract
The service SHALL implement `listSkills`, `getSkill`, `createSkill` (accepting `name,
description?, body_md`), `updateSkill` (partial), and `deleteSkill`, returning the full
`Skill` shape (`id, name, description, body_md, resources_path?, created_at`).

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
the full `McpServer` shape (`id, name, command, args, env, created_at`).

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
