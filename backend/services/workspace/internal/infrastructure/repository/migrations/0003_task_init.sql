-- Task service schema (task_db): tasks + human feedback thread.
CREATE TABLE IF NOT EXISTS tasks (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID        NOT NULL,
    agent_id        UUID,
    model_override  TEXT,
    title           TEXT        NOT NULL,
    prompt          TEXT        NOT NULL,
    description     TEXT        NOT NULL DEFAULT '',
    status          TEXT        NOT NULL DEFAULT 'backlog',
    type            TEXT        NOT NULL DEFAULT '',
    priority        TEXT        NOT NULL DEFAULT '',
    labels          TEXT[]      NOT NULL DEFAULT '{}',
    points          INTEGER,
    due_at          TIMESTAMPTZ,
    progress        INTEGER,
    branch_name     TEXT        NOT NULL DEFAULT '',
    worktree_path   TEXT        NOT NULL DEFAULT '',
    round_no        INTEGER     NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS tasks_status_idx     ON tasks (status);
CREATE INDEX IF NOT EXISTS tasks_project_idx    ON tasks (project_id);
CREATE INDEX IF NOT EXISTS tasks_agent_idx      ON tasks (agent_id);
CREATE INDEX IF NOT EXISTS tasks_updated_idx    ON tasks (updated_at DESC);

CREATE TABLE IF NOT EXISTS feedback (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id    UUID        NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    author     TEXT        NOT NULL DEFAULT 'user',
    body       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS feedback_task_idx ON feedback (task_id, created_at);
