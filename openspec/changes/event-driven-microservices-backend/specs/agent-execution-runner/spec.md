## Purpose

Executes agent runs: it consumes run/review/stop commands, drives the agent loop (LLM
provider calls, MCP client, git operations), manages per-task worktrees and credential-less
sandbox containers, persists Run/Step/Finding/Artifact records, and emits execution facts.
It is the only place provider keys and git credentials are used in plaintext.

## ADDED Requirements

### Requirement: Consume lifecycle commands and emit facts
The runner SHALL consume `task.run-requested`, `task.review-requested`, and
`task.stop-requested` commands from the event bus and SHALL emit `step.*`, `run.completed`,
`finding.*`, `verdict`, and `pr.opened` facts as execution proceeds.

#### Scenario: A run is requested
- **WHEN** the runner consumes a `task.run-requested` command
- **THEN** it begins an implementer run for that task and emits `step.*` events as it works

#### Scenario: A stop is requested
- **WHEN** the runner consumes a `task.stop-requested` command for an in-flight run
- **THEN** it aborts the run (cancelling in-flight work) and emits a terminal `run.completed`

### Requirement: Agent loop with injected persona and tools
For each run the runner SHALL construct the system prompt from the assigned agent's persona
plus attached skills, assemble the tool set (built-in `read_file`/`write_file`/`list_files`/
`run_command` plus bridged MCP tools), reconstruct context, and loop provider calls → tool
dispatch → step emission. Each run SHALL be capped by a step limit (~50), a token budget,
and a wall-clock limit (~30 min).

#### Scenario: Step emission
- **WHEN** the loop produces a message, tool call, tool result, or reasoning
- **THEN** the runner persists a `Step` (with `kind` ∈ `message|tool_call|tool_result|
  reasoning`) and emits a corresponding `step.*` event

#### Scenario: Run caps
- **WHEN** a run reaches the step, token, or wall-clock limit
- **THEN** the run terminates and a `run.completed` fact is emitted

### Requirement: Reviewer variant and verdict
For `task.review-requested`, the runner SHALL execute a reviewer variant that inspects the
work and emits a `verdict` with `decision` ∈ `APPROVE | REQUEST_CHANGES`. The number of
review rounds coordinated by the Task service MUST NOT exceed 5.

#### Scenario: Reviewer requests changes
- **WHEN** the reviewer finds issues requiring changes
- **THEN** it emits `verdict { decision: REQUEST_CHANGES }`

#### Scenario: Reviewer approves
- **WHEN** the reviewer finds the work acceptable
- **THEN** it emits `verdict { decision: APPROVE }`

### Requirement: Worktree-per-task and credential-less sandbox
Each task moving to execution SHALL get its own git worktree on branch
`agent/<task-id>-<slug>`, bind-mounted read-write into a sandbox container. The runner
SHALL drive build/test/edit commands in the container via the container exec API.
Concurrent doing tasks SHALL use disjoint worktrees. The sandbox container MUST hold no API
keys and no git credentials.

#### Scenario: Worktree creation
- **WHEN** a run starts for a task
- **THEN** a dedicated worktree on `agent/<task-id>-<slug>` is created and mounted into the
  container

#### Scenario: Credential isolation
- **WHEN** the container runs a command
- **THEN** it has no access to provider keys or git credentials

### Requirement: Own Run, Step, Finding, and Artifact records
The runner SHALL own and persist `Run` (`id, task_id, role, agent_id, model, status,
round_no, started_at, ended_at, token_usage, error`), `Step`, `Finding` (`file, line,
severity, issue, recommendation, status`), and `Artifact` (`patch|document`, summary,
additions/deletions) records, and the corresponding query endpoints
(`/tasks/:id/runs`, `/runs/:id/steps`, `/runs/:id/findings`, `/tasks/:id/artifacts`) SHALL
be served (directly or via the Gateway) from this ownership.

#### Scenario: Querying a run's steps
- **WHEN** the frontend calls `listRunSteps(runId)`
- **THEN** the runner-owned steps for that run are returned in sequence order

#### Scenario: Reviewer findings
- **WHEN** a reviewer run records findings
- **THEN** they are retrievable via `listRunFindings(runId)`

### Requirement: Git operations and PR creation stay on the host
All git operations (commit, push) and `gh pr create` SHALL be performed by the runner on the
host, never inside the sandbox container. Pull requests SHALL be created only on explicit
`open-pr` request and MUST NEVER be auto-merged.

#### Scenario: Opening a PR
- **WHEN** an `open-pr` action is processed
- **THEN** the runner commits/pushes on the host and creates a PR; it does not auto-merge it

### Requirement: Plaintext secrets used in-process only
When the runner needs a provider key or git credential, it SHALL obtain the plaintext from
the Settings service over the authenticated internal channel, use it in process memory for
the duration of the run, and MUST NOT persist or log it.

#### Scenario: Using a provider key
- **WHEN** the runner calls an LLM provider
- **THEN** it uses the plaintext key obtained from Settings in memory only, and discards it
  when the run ends
