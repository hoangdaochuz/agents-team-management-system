## Purpose

Owns the Task entity and its human-feedback thread, and acts as the saga coordinator for
the task lifecycle: it drives status transitions and review rounds by emitting commands and
consuming execution facts over the event bus. Exposes the task surface the frontend uses
(`/tasks`, `/tasks/:id/feedback`, status patch, re-run, stop, open-pr).

## ADDED Requirements

### Requirement: Task CRUD matching the frontend contract
The service SHALL implement `listTasks` (with the `TaskQuery` filters: `project_id, status,
type, priority, assignee, label, q`), `getTask`, `createTask` (accepting `project_id, title,
prompt, description?, type?, priority?, labels?, points?, agent_id?, due_at?`), `updateTask`
(partial), and `deleteTask`, returning the full `Task` shape (`id, project_id, agent_id,
model_override, title, prompt, description, status, type, priority, labels, points, due_at,
progress, branch_name, worktree_path, comments_count, attachments_count, created_at,
updated_at`).

#### Scenario: Creating a task
- **WHEN** the frontend calls `createTask` with required fields
- **THEN** the service persists the task and returns it synchronously with a generated id
  and `status: "backlog"`

#### Scenario: Filtering tasks by status
- **WHEN** the frontend calls `listTasks({ status: "doing" })`
- **THEN** the service returns only tasks whose status is `doing`

#### Scenario: Partial update
- **WHEN** the frontend calls `updateTask(id, partial)`
- **THEN** only supplied fields change and the updated `Task` is returned

### Requirement: Status patch drives the saga
`PATCH /api/tasks/:id/status` SHALL update the task status synchronously and return the
updated `Task`. When the new status is `doing`, the service SHALL additionally publish a
`task.run-requested` command so an agent run begins. The service SHALL own the authoritative
status (`backlog | doing | review | done | blocked | cancelled | stopped`).

#### Scenario: Moving a task to Doing
- **WHEN** the frontend moves a task to `doing` via `patchStatus`
- **THEN** the status is updated synchronously and the updated `Task` is returned, and a
  `task.run-requested` command is published

### Requirement: Review round coordination
The service SHALL consume execution facts (`run.completed`, `verdict`) and advance status
accordingly: on a reviewer verdict of `REQUEST_CHANGES` while the round number is below the
maximum (5), the service SHALL move the task back to `doing` and publish another
`task.run-requested`; on `APPROVE` it SHALL move the task to `review`/`done` as appropriate.
The service SHALL track `round_no` and MUST NOT exceed 5 review rounds.

#### Scenario: Reviewer requests changes within the round limit
- **WHEN** the service consumes a `verdict` of `REQUEST_CHANGES` and `round_no < 5`
- **THEN** the task returns to `doing` and a new `task.run-requested` command is published

#### Scenario: Reviewer approves
- **WHEN** the service consumes a `verdict` of `APPROVE`
- **THEN** the task advances out of the active run loop (to `review`/`done`)

#### Scenario: Round limit reached
- **WHEN** a fifth review round still yields `REQUEST_CHANGES`
- **THEN** the service MUST NOT start another round and SHALL surface the task for human
  action (e.g. mark `blocked`)

### Requirement: Re-run on feedback
`POST /api/tasks/:id/re-run` SHALL request a fresh implementer run using the latest
feedback. Because execution is asynchronous, the endpoint SHALL accept the request and
publish a `task.run-requested` command; it MAY return the current `Run` context rather than
blocking until completion.

#### Scenario: Re-running a task
- **WHEN** the frontend calls `rerunTask(id)`
- **THEN** the service publishes a `task.run-requested` command for that task

### Requirement: Stop an in-flight task
`POST /api/tasks/:id/stop` SHALL set the task status to `stopped` synchronously, return the
updated `Task`, and publish a `task.stop-requested` command that causes any in-flight agent
run for that task to be aborted.

#### Scenario: Stopping a running task
- **WHEN** the frontend calls `stopTask(id)` on a `doing` task
- **THEN** the status becomes `stopped`, the updated `Task` is returned, and a
  `task.stop-requested` command is published

### Requirement: Open a pull request (human-initiated only)
`POST /api/tasks/:id/open-pr` SHALL create a GitHub pull request from the task branch on
demand and return `{ url }`. Pull requests MUST NEVER be created automatically as a side
effect of any other flow.

#### Scenario: Opening a PR
- **WHEN** the frontend calls `openPr(id)`
- **THEN** a PR is created from the task branch and its `url` is returned

#### Scenario: No automatic PR
- **WHEN** a run completes or a verdict is reached through any automated flow
- **THEN** no pull request is created

### Requirement: Feedback thread
The service SHALL expose `GET /api/tasks/:id/feedback` (list) and `POST /api/tasks/:id/feedback`
(add `{ body }`), returning `Feedback` objects (`id, task_id, author: "user", body,
created_at`).

#### Scenario: Adding feedback
- **WHEN** the frontend calls `addFeedback(taskId, body)`
- **THEN** a `Feedback` record is created and returned with a generated id and timestamp
