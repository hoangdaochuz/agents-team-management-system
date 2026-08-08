## Purpose

The asynchronous backbone that connects services without synchronous coupling. It defines
the Kafka topic catalog, ordering guarantees, delivery semantics, and consumer grouping
that the saga, the runner, and realtime streaming rely on.

## ADDED Requirements

### Requirement: Defined topic catalog
The event bus SHALL provide the topics used by the system:
- Commands (Task service → Agent-Runner): `task.run-requested`,
  `task.review-requested`, `task.stop-requested`
- Facts (Agent-Runner → consumers): `step.*`, `run.completed`, `finding.*`, `verdict`,
  `pr.opened`
- Task state (Task service → any consumer): `task.status-changed`

Each topic SHALL have a documented message schema (producer, consumer, payload).

#### Scenario: A run is requested
- **WHEN** the Task service wants a run to start
- **THEN** it publishes `task.run-requested` and the Agent-Runner consumes it

#### Scenario: A step is produced
- **WHEN** the runner produces a step
- **THEN** it publishes a `step.*` message consumable by the realtime stream

### Requirement: Per-task ordering via partition key
Messages on the lifecycle and step topics SHALL be partitioned by `task_id` so that all
events for a given task are delivered to a single consumer in order. `task_id` MUST be
present on every such message.

#### Scenario: Ordering within a task
- **WHEN** multiple events for the same task are produced in quick succession
- **THEN** they are delivered to the same partition in publish order

### Requirement: Consumer groups isolate consumers
Each consuming service SHALL use a distinct consumer group so that multiple independent
consumers (e.g. the runner for commands, the gateway for realtime) each receive the relevant
messages independently.

#### Scenario: Independent consumers
- **WHEN** both the runner and the gateway subscribe to overlapping topics
- **THEN** each receives the messages in its own consumer group without excluding the other

### Requirement: At-least-once delivery with idempotency
The event bus SHALL deliver messages at least once. Consumers SHALL be idempotent on a
stable message/entity id (e.g. dedup `step.id`) so that redelivery does not produce
duplicates observable to the frontend.

#### Scenario: Redelivery of a step
- **WHEN** a `step.*` message is delivered more than once
- **THEN** the consumer deduplicates by `step.id` and the frontend observes a single step

### Requirement: Operational deployment
The event bus SHALL run in Kafka KRaft mode (no Zookeeper dependency) and SHALL be
deployable as part of the single-operator docker-compose topology.

#### Scenario: Local startup
- **WHEN** the operator runs the deployment
- **THEN** Kafka is available without a separate Zookeeper process
