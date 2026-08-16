-- Workspace scoping (task 3.6 / design D8).
ALTER TABLE agents ADD COLUMN IF NOT EXISTS workspace_id UUID NOT NULL;
CREATE INDEX IF NOT EXISTS agents_workspace_idx ON agents (workspace_id);
