-- Agent service schema (agent_db): agents + skill/mcp link tables.
CREATE TABLE IF NOT EXISTS agents (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT        NOT NULL,
    role            TEXT        NOT NULL,
    system_prompt   TEXT        NOT NULL DEFAULT '',
    default_model   TEXT        NOT NULL DEFAULT '',
    allowed_tools   TEXT[]      NOT NULL DEFAULT '{}',
    status          TEXT        NOT NULL DEFAULT 'idle',     -- running | paused | idle
    load            INTEGER,
    current_task_id UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS agent_skills (
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    skill_id UUID NOT NULL,
    PRIMARY KEY (agent_id, skill_id)
);

CREATE TABLE IF NOT EXISTS agent_mcps (
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    mcp_id   UUID NOT NULL,
    PRIMARY KEY (agent_id, mcp_id)
);

CREATE INDEX IF NOT EXISTS agents_status_idx ON agents (status);
