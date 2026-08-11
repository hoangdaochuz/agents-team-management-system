-- Dedup guard for seeded default rules. CreateRule upserts with
-- ON CONFLICT DO NOTHING, so a unique index on (workspace_id, name) prevents
-- duplicate rule rows when workspace.created is redelivered (at-least-once).
-- Idempotent: this file re-runs on every boot.
CREATE UNIQUE INDEX IF NOT EXISTS rules_ws_name_unique ON rules (workspace_id, name);
