-- Provisioning reconcile state: set when the Workspace service confirms the
-- provisioning POST, NULL while it still needs to be (re)issued. The sweeper
-- only sweeps unprovisioned rows instead of re-POSTing every workspace forever.
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS provisioned_at TIMESTAMPTZ;
