-- Workspace scoping (task 3.6 / design D8): every core entity carries the
-- owning workspace's id; services filter by it on every read and inherit it
-- on create. Fresh-DB migration: the column is NOT NULL from day one.
ALTER TABLE projects ADD COLUMN IF NOT EXISTS workspace_id UUID NOT NULL;
CREATE INDEX IF NOT EXISTS projects_workspace_idx ON projects (workspace_id);
