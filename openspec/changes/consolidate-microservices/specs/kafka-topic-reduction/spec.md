## Purpose

The asynchronous backbone that connects services without synchronous coupling. It defines the
Kafka topic catalog, ordering guarantees, delivery semantics, and consumer grouping that the
saga, the runner, and realtime streaming rely on. This spec documents the reduction from ~22
topics to ~8 topics, retaining only events that genuinely need cross-service async processing.

## ADDED Requirements

### Requirement: Defined topic catalog (post-consolidation)
The event bus SHALL provide the following topics used by the system:

**Commands** (Workspace service → Executor):
- `task.run-requested`
- `task.review-requested`
- `task.stop-requested`

**Facts** (Executor → Workspace service):
- `step.*` — per-step events for SSE fan-out
- `run.completed` — run completion events
- `finding.*` — findings from agent execution
- `verdict` — agent verdict events

**Task state** (Workspace service → any consumer):
- `task.status-changed` — task status updates

Each topic SHALL have a documented message schema (producer, consumer, payload).

### Requirement: Per-task ordering via partition key
Messages on the lifecycle and step topics SHALL be partitioned by `task_id` so that all
events for a given task are delivered to a single consumer in order. `task_id` MUST be
present on every such message.

### Requirement: Consumer groups isolate consumers
Each consuming service SHALL use a distinct consumer group so that multiple independent
consumers (e.g. the runner for commands, the gateway for realtime) each receive the relevant
messages independently.

### Requirement: At-least-once delivery with idempotency
The event bus SHALL deliver messages at least once. Consumers SHALL be idempotent on a
stable message/entity id (e.g. dedup `step.id`) so that redelivery does not produce
duplicates observable to the frontend.

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
| `run.started` | Executor→Agent projection; can be a sync callback |
| `pr.opened` | Executor→Workspace; can be made synchronous or kept as-is |
| `task.status-changed` | Internal to Workspace Service now — no longer cross-service |

### Requirement: Minimal Kafka topology
The Kafka deployment SHALL remain in KRaft mode with the reduced topic catalog. The
following topics are the only ones that truly require async, partitioned, at-least-once
delivery:
- `task.run-requested`, `task.review-requested`, `task.stop-requested` (saga coordination)
- `step.*` (SSE realtime streaming)
- `run.completed`, `finding.*`, `verdict` (execution results)

All other previously-defined topics become in-process events, direct HTTP calls, or
Postgres LISTEN/NOTIFY.

### Requirement: Consumer group simplification
Each service SHALL use a reduced set of consumer groups corresponding to the surviving
Kafka topics. Previously, each service had its own consumer group for each topic; post-
consolidation, consumer groups map directly to the 8 surviving topics.