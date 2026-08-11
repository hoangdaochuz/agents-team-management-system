-- Task-saga idempotency: one row per (task_id, run_id) processed by the saga
-- coordinator, so at-least-once Kafka redelivery cannot double-transition.
CREATE TABLE IF NOT EXISTS saga_runs (
    task_id      UUID        NOT NULL,
    run_id       UUID        NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (task_id, run_id)
);
