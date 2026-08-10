## Purpose

Manages Agent definitions (persona + capabilities) and their associations to Skills and MCP
servers. Exposes the agent surface the frontend uses (`/agents` and the attach/detach
endpoints).

## ADDED Requirements

### Requirement: Agent CRUD matching the frontend contract
The service SHALL implement `listAgents`, `getAgent`, `createAgent` (accepting `name, role,
system_prompt?, default_model?, allowed_tools?`), `updateAgent` (partial), and
`deleteAgent`, returning the full `Agent` shape (`id, workspace_id, name, role,
system_prompt, default_model, allowed_tools, status, load, current_task_id, capabilities,
skill_ids, mcp_ids, created_at`).

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

### Requirement: Agent builder fields
The `Agent` resource SHALL carry the optional builder fields the Agent Builder screen
declares: `role_title`, `provider` (`openai | anthropic | gemini | glm`),
`temperature` (0..1), `max_output_tokens`, `autonomy_mode`
(`assigned | matching | autonomous`), `user_prompt_template`, `knowledge_source_ids`,
and `guardrails`. `createAgent`/`updateAgent` SHALL accept and persist these fields, and
list/get SHALL return them. They are additive to the core `Agent` shape and all optional.

#### Scenario: Building an agent with provider and autonomy
- **WHEN** the frontend creates an agent with `provider: "anthropic"`,
  `temperature: 0.2`, and `autonomy_mode: "assigned"`
- **THEN** the persisted `Agent` echoes those fields on subsequent reads

#### Scenario: Knowledge sources attached to an agent
- **WHEN** an agent is saved with `knowledge_source_ids`
- **THEN** the runner can later inject those sources into the agent's context

### Requirement: Agent guardrails (execution policy)
An agent's `guardrails` SHALL express execution policy: `auto_pause_on_test_fail`,
`allow_direct_commits`, `allow_shell_commands`, `require_approval_before_merge`,
`max_steps_per_run`, and `wall_clock_cap_min`. The Agent service persists them as part of
the `Agent`; the Agent-Runner enforces them at run time (see the agent-execution-runner
capability). When `max_steps_per_run` / `wall_clock_cap_min` are set, they override the
runner defaults for that agent's runs.

#### Scenario: Approval required before merge
- **WHEN** an agent has `guardrails.require_approval_before_merge: true`
- **THEN** the runner MUST NOT open a PR automatically; a human must initiate `open-pr`

#### Scenario: Per-agent step cap
- **WHEN** an agent has `guardrails.max_steps_per_run: 20`
- **THEN** its runs terminate at 20 steps even though the runner default is ~50

### Requirement: Workspace scoping of agents
Every agent SHALL carry a `workspace_id`, and its attached skills/MCP references SHALL be
restricted to that same workspace (an agent in workspace A MUST NOT be attached to a skill
or MCP server from workspace B). `listAgents`/`getAgent`/`createAgent` SHALL be scoped to
the session's workspace context; `createAgent` SHALL inherit the workspace from it.

#### Scenario: Listing scoped agents
- **WHEN** the frontend calls `listAgents` in a single-workspace session
- **THEN** only that workspace's agents are returned, each carrying its `workspace_id`

#### Scenario: Cross-workspace attachment rejected
- **WHEN** a caller attaches a skill or MCP server from another workspace to an agent
- **THEN** the attachment is rejected with an error
