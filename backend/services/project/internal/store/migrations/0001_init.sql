-- Project service schema (project_db).
CREATE TABLE IF NOT EXISTS projects (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT        NOT NULL,
    repo_source   TEXT        NOT NULL,
    repo_type     TEXT        NOT NULL,            -- 'path' | 'url'
    cloned_path   TEXT        NOT NULL DEFAULT '',
    default_branch TEXT       NOT NULL DEFAULT 'main',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS projects_created_at_idx ON projects (created_at DESC);
