-- Workspace scoping (task 3.6 / design D8).
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS workspace_id UUID NOT NULL;
CREATE INDEX IF NOT EXISTS tasks_workspace_idx ON tasks (workspace_id);
