## Purpose

Lets admins create and edit agent configurations through a guided builder covering identity, model,
prompts, autonomy, skills, tools/MCP, guardrails, and knowledge access.

## ADDED Requirements

### Requirement: Builder sections
The system SHALL present the agent builder as an ordered set of sections — Identity, Model,
Prompts, Mode & autonomy, Skills, Tools & MCP, Rules & guardrails, Knowledge base access — with a
sticky summary card reflecting the in-progress configuration.

#### Scenario: Live summary
- **WHEN** the user edits any builder field
- **THEN** the sticky summary card updates to reflect workspace, model, skill count, tool count, and
  autonomy level

### Requirement: Identity and model configuration
The system SHALL capture an agent name, role title, provider, model, temperature, and max output
tokens.

#### Scenario: Adjusting temperature
- **WHEN** the user moves the temperature slider
- **THEN** a decimal readout (e.g. 0.30) updates in real time within the 0–1 range

### Requirement: Prompts with variable insertion
The system SHALL accept a system prompt and a user-prompt template, and SHALL offer clickable
variable tags (e.g. task title, repo, skills) that insert placeholders into the prompt body.

#### Scenario: Inserting a variable
- **WHEN** the user clicks a variable tag
- **THEN** the corresponding `{{variable}}` placeholder is inserted into the focused prompt editor

### Requirement: Autonomy mode
The system SHALL let the user choose one of three autonomy modes: assigned-only, picks matching
tasks, or fully autonomous.

#### Scenario: Selecting autonomy
- **WHEN** the user selects an autonomy mode
- **THEN** only that mode is active and it is reflected in the summary card

### Requirement: Skills, tools, MCP, and knowledge selection
The system SHALL let the user toggle skills, built-in tools, MCP servers, and knowledge sources on
or off via chips/checkboxes.

#### Scenario: Toggling a tool
- **WHEN** the user toggles a built-in tool
- **THEN** its selected state flips and the tool count in the summary updates

### Requirement: Guardrails
The system SHALL expose guardrail toggles (auto-pause on test failure, allow direct commits, allow
shell commands, require human approval before merge) plus numeric caps (max steps per run, wall-clock
cap in minutes).

#### Scenario: Setting caps
- **WHEN** the user edits max steps or wall-clock cap
- **THEN** the numeric values are captured for the agent configuration

### Requirement: Save and create actions
The system SHALL provide "Create agent" and "Save draft" actions. On success the system SHALL show a
confirmation toast and navigate back to the agents list.

#### Scenario: Creating an agent
- **WHEN** the user submits a valid configuration via "Create agent"
- **THEN** the agent is created through the API, a success toast names the agent, and the user is
  routed to `/agents`

#### Scenario: Cancel
- **WHEN** the user cancels
- **THEN** unsaved changes are discarded and the user is routed back to `/agents`
