## Purpose

Manages Agent definitions (persona + capabilities) and their associations to Skills and MCP
servers. Exposes the agent surface the frontend uses (`/agents` and the attach/detach
endpoints).

## ADDED Requirements

### Requirement: Agent CRUD matching the frontend contract
The service SHALL implement `listAgents`, `getAgent`, `createAgent` (accepting `name, role,
system_prompt?, default_model?, allowed_tools?`), `updateAgent` (partial), and
`deleteAgent`, returning the full `Agent` shape (`id, name, role, system_prompt,
default_model, allowed_tools, status, load, current_task_id, capabilities, skill_ids,
mcp_ids, created_at`).

#### Scenario: Creating an agent
- **WHEN** the frontend calls `createAgent` with a name and role
- **THEN** the service persists the agent and returns it synchronously with a generated id

#### Scenario: Partial update
- **WHEN** the frontend calls `updateAgent(id, partial)`
- **THEN** only supplied fields change and the updated `Agent` is returned

#### Scenario: Deleting an agent
- **WHEN** the frontend deletes a known agent
- **THEN** the service removes it and responds 204

### Requirement: Attach and detach skills
The service SHALL attach a skill via `POST /api/agents/:id/skills` (`{ skill_id }`) and
detach via `DELETE /api/agents/:id/skills/:skillId`.

#### Scenario: Attaching a skill
- **WHEN** the frontend calls `attachSkill(agentId, skillId)`
- **THEN** the skill is added to the agent's `skill_ids` and the endpoint responds 204

#### Scenario: Detaching a skill
- **WHEN** the frontend calls `detachSkill(agentId, skillId)`
- **THEN** the skill is removed from the agent's `skill_ids` and the endpoint responds 204

### Requirement: Attach and detach MCP servers
The service SHALL attach an MCP server via `POST /api/agents/:id/mcps`
(`{ mcp_server_id }`) and detach via `DELETE /api/agents/:id/mcps/:mcpId`.

#### Scenario: Attaching an MCP server
- **WHEN** the frontend calls `attachMcp(agentId, mcpId)`
- **THEN** the MCP server is added to the agent's `mcp_ids` and the endpoint responds 204

#### Scenario: Detaching an MCP server
- **WHEN** the frontend calls `detachMcp(agentId, mcpId)`
- **THEN** the MCP server is removed from the agent's `mcp_ids` and the endpoint responds 204

### Requirement: Agent runtime status is derived
Agent `status` (`running | paused | idle`), `load`, and `current_task_id` reflect the
agent's live execution state. These MAY be derived from execution facts rather than edited
directly by the frontend.

#### Scenario: Reflecting an active run
- **WHEN** an agent is actively executing a task
- **THEN** listing/getting that agent shows `status: "running"` with the corresponding
  `current_task_id`
