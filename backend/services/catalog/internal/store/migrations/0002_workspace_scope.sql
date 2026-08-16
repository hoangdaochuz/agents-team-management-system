-- Workspace scoping (task 3.6 / design D8).
ALTER TABLE skills ADD COLUMN IF NOT EXISTS workspace_id UUID NOT NULL;
CREATE INDEX IF NOT EXISTS skills_workspace_idx ON skills (workspace_id);

ALTER TABLE mcp_servers ADD COLUMN IF NOT EXISTS workspace_id UUID NOT NULL;
CREATE INDEX IF NOT EXISTS mcp_servers_workspace_idx ON mcp_servers (workspace_id);
