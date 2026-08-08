## Purpose

Delivers the live agent step-log to the frontend. The Gateway serves a Server-Sent Events
stream per task that replays persisted steps then tails new ones from the event bus, so a
refresh or reconnect resumes the live view without gaps or duplicates.

## ADDED Requirements

### Requirement: SSE endpoint per task
The Gateway SHALL serve `GET /api/tasks/:id/stream` as an SSE connection. It SHALL emit an
event named `step` whose data is the full `Step` shape (`id, run_id, seq, kind, payload,
created_at`), where `kind` ∈ `message | tool_call | tool_result | reasoning`.

#### Scenario: Subscribing to a task
- **WHEN** the frontend opens `GET /api/tasks/:id/stream`
- **THEN** the Gateway holds the connection open and emits `step` events as they occur

### Requirement: Replay then tail
On connection, the Gateway SHALL first replay already-persisted steps for the task (sourced
from the Agent-Runner's history) in `seq` order, then tail live `step.*` events from the
event bus for new steps.

#### Scenario: Reconnecting mid-run
- **WHEN** the frontend reconnects to a stream for a task that already has steps
- **THEN** the Gateway replays the persisted steps first, then continues with live events

### Requirement: Reconnect and dedup
The stream SHALL rely on native EventSource reconnection. Because replay and live tail can
both deliver a step around a reconnect, the consumer-facing stream MUST be deduplicatable by
`step.id` so a reconnect does not double-render steps.

#### Scenario: Duplicate step around reconnect
- **WHEN** a step is delivered both in replay and in the live tail
- **THEN** the frontend can deduplicate it by `step.id` and render it once

### Requirement: Run error signaling
The stream SHALL support an `error` event to communicate a run-level error to the frontend
without clearing already-received steps.

#### Scenario: A run errors
- **WHEN** the underlying run encounters an error
- **THEN** the Gateway emits an `error` event with a message, and previously received steps
  remain visible
