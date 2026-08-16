-- Resources service schema (resources_db): workspace-level knowledge sources,
-- plugins, rules, and MCP connections (projected from catalog mcp-servers).
CREATE TABLE IF NOT EXISTS knowledge_sources (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID        NOT NULL,
    title        TEXT        NOT NULL,
    kind         TEXT        NOT NULL DEFAULT 'file', -- file | folder | url | upload
    chunks       INTEGER     NOT NULL DEFAULT 0,
    pages        INTEGER     NOT NULL DEFAULT 0,
    status       TEXT        NOT NULL DEFAULT 'pending', -- pending | indexing | indexed | failed
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS knowledge_sources_ws_idx ON knowledge_sources (workspace_id);

CREATE TABLE IF NOT EXISTS plugins (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID        NOT NULL,
    name         TEXT        NOT NULL,
    version      TEXT        NOT NULL DEFAULT '1.0.0',
    capabilities TEXT[]      NOT NULL DEFAULT '{}',
    scopes       TEXT[]      NOT NULL DEFAULT '{}',
    enabled      BOOLEAN     NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, name)
);

CREATE INDEX IF NOT EXISTS plugins_ws_idx ON plugins (workspace_id);

CREATE TABLE IF NOT EXISTS rules (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID        NOT NULL,
    name         TEXT        NOT NULL,
    description  TEXT        NOT NULL DEFAULT '',
    enabled      BOOLEAN     NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, name)
);

CREATE INDEX IF NOT EXISTS rules_ws_idx ON rules (workspace_id);

CREATE TABLE IF NOT EXISTS mcp_connections (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID        NOT NULL,
    mcp_server_id UUID       NOT NULL,
    name         TEXT        NOT NULL,
    transport    TEXT        NOT NULL DEFAULT 'stdio', -- stdio | http
    tool_count   INTEGER     NOT NULL DEFAULT 0,
    tool_names   TEXT[]      NOT NULL DEFAULT '{}',
    status       TEXT        NOT NULL DEFAULT 'connected', -- connected | offline
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, mcp_server_id)
);

CREATE INDEX IF NOT EXISTS mcp_connections_ws_idx ON mcp_connections (workspace_id);
