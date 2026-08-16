-- Runner service schema (runner_db): runs, steps, findings, artifacts. All
-- task-scoped; the Gateway serves them under /tasks/:id/runs and /runs/:id/*.
CREATE TABLE IF NOT EXISTS runs (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id     UUID        NOT NULL,
    role        TEXT        NOT NULL, -- implementer | reviewer
    agent_id    UUID        NOT NULL,
    model       TEXT        NOT NULL DEFAULT '',
    status      TEXT        NOT NULL DEFAULT 'running', -- running | done | aborted | stopped
    round_no    INTEGER     NOT NULL DEFAULT 1,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at    TIMESTAMPTZ,
    token_usage INTEGER     NOT NULL DEFAULT 0,
    error       TEXT        NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS runs_task_idx ON runs (task_id, started_at DESC);

CREATE TABLE IF NOT EXISTS steps (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id     UUID        NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    seq        INTEGER     NOT NULL,
    kind       TEXT        NOT NULL, -- message | tool_call | tool_result | reasoning
    payload    JSONB       NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (run_id, seq)
);

CREATE INDEX IF NOT EXISTS steps_run_idx ON steps (run_id, seq);

CREATE TABLE IF NOT EXISTS findings (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id         UUID        NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    file           TEXT        NOT NULL DEFAULT '',
    line           INTEGER     NOT NULL DEFAULT 0,
    severity       TEXT        NOT NULL DEFAULT 'info', -- info | warning | error | critical
    issue          TEXT        NOT NULL,
    recommendation TEXT        NOT NULL DEFAULT '',
    status         TEXT        NOT NULL DEFAULT 'open', -- open | resolved
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS findings_run_idx ON findings (run_id);

CREATE TABLE IF NOT EXISTS artifacts (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id    UUID        NOT NULL,
    run_id     UUID,
    filename   TEXT        NOT NULL,
    kind       TEXT        NOT NULL, -- patch | document
    summary    TEXT        NOT NULL DEFAULT '',
    additions  INTEGER     NOT NULL DEFAULT 0,
    deletions  INTEGER     NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS artifacts_task_idx ON artifacts (task_id, created_at DESC);
