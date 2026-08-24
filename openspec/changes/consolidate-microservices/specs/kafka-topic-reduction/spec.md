## Purpose

The asynchronous backbone that connects services without synchronous coupling. It defines the
Kafka topic catalog, ordering guarantees, delivery semantics, and consumer grouping that the
saga, the runner, and realtime streaming rely on. This spec documents the reduction from 21
topics to 9 topics, retaining only events that genuinely need cross-service async processing.

## ADDED Requirements

### Requirement: Defined topic catalog (post-consolidation)
The event bus SHALL provide the following topics used by the system:

**Commands** (Workspace service → Executor):
- `task.run-requested`
- `task.review-requested`
- `task.stop-requested`
- `task.pr-open-requested`

**Facts** (Executor → Workspace service):
- `step` — per-step events for SSE fan-out (single topic; messages carry `task_id` + `run_id`)
- `run.completed` — run completion events
- `finding` — findings from agent execution (single topic)
- `verdict` — agent verdict events
- `pr.opened` — PR creation events consumed by the task saga

Each topic SHALL have a documented message schema (producer, consumer, payload).

#### Scenario: Catalog is exactly 9 topics
- **WHEN** the event bus auto-creates or validates its topic catalog
- **THEN** exactly the 9 listed topics exist and each has a documented producer, consumer, and payload schema

#### Scenario: PR open command flows to the Executor
- **WHEN** the task saga approves a task for PR creation and publishes `task.pr-open-requested`
- **THEN** the Executor consumes the command and opens the PR for that task

### Requirement: Per-task ordering via partition key
Messages on the lifecycle and step topics SHALL be partitioned by `task_id` so that all
events for a given task are delivered to a single consumer in order. `task_id` MUST be
present on every such message.

#### Scenario: Events for one task arrive in order
- **WHEN** the Executor publishes steps 1..N for a task in quick succession
- **THEN** every consumer receives them in publish order because all share the task's `task_id` partition

### Requirement: Consumer groups isolate consumers
Each consuming service SHALL use a distinct consumer group so that multiple independent
consumers (e.g. the runner for commands, the gateway for realtime) each receive the relevant
messages independently.

#### Scenario: Gateway and Workspace both receive facts
- **WHEN** the Executor publishes a `step` event
- **THEN** both the Gateway's SSE tailer (its own consumer group) and any Workspace consumer receive the event independently

### Requirement: At-least-once delivery with idempotency
The event bus SHALL deliver messages at least once. Consumers SHALL be idempotent on a
stable message/entity id (e.g. dedup `step.id`) so that redelivery does not produce
duplicates observable to the frontend.

#### Scenario: Redelivered step is deduped
- **WHEN** a `step` message is redelivered after a consumer rebalance
- **THEN** the consumer recognizes the already-seen `step.id` and does not emit a duplicate to the SSE stream or persist a second row

#### Scenario: Saga does not double-advance
- **WHEN** `run.completed` or `verdict` is redelivered to the task saga
- **THEN** the saga's `(task_id, run_id)`-keyed state machine ignores the duplicate transition

### Requirement: Topic pruning — remove unnecessary topics
The following topics SHALL be removed from the Kafka catalog or reduced to in-process
processing, as they no longer cross service boundaries:

| Topic | Reason for Removal |
|:------|:-------------------|
| `signup.requested/approved/declined` | All within Identity Service now — in-process events |
| `invite.created` | All within Identity Service now — in-process events |
| `workspace.created` | Can be handled by direct Identity→Workspace call or polling |
| `mcp.created/deleted` | All within Workspace Service now — in-process projections |
| `skill.created/deleted` | Workspace→Agent can be a synchronous call on write |
| `audit.recorded` | All within Identity Service now — in-process events |
| `run.started` | Executor→Agent projection; currently no consumer — dropped |
| `task.status-changed` | Internal to Workspace Service now — no consumer exists today; status reaches the frontend via REST |

#### Scenario: Pruned topics no longer exist
- **WHEN** the consolidated system's Kafka cluster is inspected
- **THEN** none of the pruned topics exist and no producer attempts to publish to them

#### Scenario: Signup flow completes without Kafka
- **WHEN** an operator approves a signup request in the Identity Service
- **THEN** the user activation and audit recording happen as in-process function calls with no Kafka publish

### Requirement: Minimal Kafka topology
The Kafka deployment SHALL remain in KRaft mode with the reduced topic catalog. The
following topics are the only ones that truly require async, partitioned, at-least-once
delivery:
- `task.run-requested`, `task.review-requested`, `task.stop-requested`, `task.pr-open-requested` (saga coordination)
- `step` (SSE realtime streaming)
- `run.completed`, `finding`, `verdict`, `pr.opened` (execution results)

All other previously-defined topics become in-process events, direct HTTP calls, or
Postgres LISTEN/NOTIFY.

#### Scenario: Cross-service execution stays async
- **WHEN** a task moves to Doing and the Workspace Service requests execution
- **THEN** it publishes `task.run-requested` to Kafka and the Executor picks it up asynchronously — no synchronous Workspace→Executor HTTP call is introduced

### Requirement: Consumer group simplification
Each service SHALL use a reduced set of consumer groups corresponding to the surviving
Kafka topics. Previously, each service had its own consumer group for each topic; post-
consolidation, consumer groups map directly to the 9 surviving topics.

#### Scenario: Only surviving topics have consumer groups
- **WHEN** the consolidated services register their Kafka consumers
- **THEN** consumer groups exist only for the 9 surviving topics; no group subscribes to a pruned topic
