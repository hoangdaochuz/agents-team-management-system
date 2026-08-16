-- Catalog service schema (catalog_db): Skills + MCP servers.
CREATE TABLE IF NOT EXISTS skills (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name           TEXT        NOT NULL,
    description    TEXT        NOT NULL DEFAULT '',
    body_md        TEXT        NOT NULL,
    resources_path TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS mcp_servers (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    command    TEXT        NOT NULL,
    args       TEXT[]      NOT NULL DEFAULT '{}',
    env        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
