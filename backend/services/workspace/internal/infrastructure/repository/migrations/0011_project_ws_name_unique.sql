-- Project names are unique per workspace. This is what makes the provisioning
-- binding idempotent: the Identity sweeper re-POSTs workspaces on an interval
-- and BindWorkspace upserts on this conflict target.
CREATE UNIQUE INDEX IF NOT EXISTS projects_ws_name_unique ON projects (workspace_id, name);
