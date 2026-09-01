-- Admin service schema (admin_db): audit entries and feature flags. The orgs
-- snapshot for the sysadmin list stays owned by the Orgs service (read via
-- Gateway fan-out); this service owns the workspace-agnostic admin surface.
CREATE TABLE IF NOT EXISTS audit_entries (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID        NOT NULL,
    actor_name   TEXT        NOT NULL DEFAULT '',
    actor_id     UUID,
    action       TEXT        NOT NULL,
    action_kind  TEXT        NOT NULL DEFAULT '',
    target       TEXT        NOT NULL DEFAULT '',
    ip           TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS audit_entries_ws_idx ON audit_entries (workspace_id, created_at DESC);
CREATE INDEX IF NOT EXISTS audit_entries_kind_idx ON audit_entries (action_kind);

CREATE TABLE IF NOT EXISTS feature_flags (
    key         TEXT PRIMARY KEY,
    label       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    enabled     BOOLEAN NOT NULL DEFAULT false,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO feature_flags (key, label, description, enabled) VALUES
    ('parallel_runs',       'Parallel runs',       'allow multiple agents to run in parallel', true),
    ('autonomous_mode',     'Autonomous mode',     'let agents pick tasks themselves',          false),
    ('knowledge_indexing',  'Knowledge indexing',  'enable document indexing for workspaces',   true)
ON CONFLICT (key) DO NOTHING;
