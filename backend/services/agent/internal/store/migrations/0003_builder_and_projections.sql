-- Agent-builder fields (agent-management capability) + catalog projections.
ALTER TABLE agents ADD COLUMN IF NOT EXISTS role_title TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS temperature DOUBLE PRECISION;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS max_output_tokens INTEGER;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS autonomy_mode TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS user_prompt_template TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS knowledge_source_ids TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS guardrails JSONB NOT NULL DEFAULT '{}';

-- Catalog projections (skill/mcp -> workspace) for attachment validation.
CREATE TABLE IF NOT EXISTS known_skills (
    skill_id     UUID PRIMARY KEY,
    workspace_id UUID NOT NULL
);
CREATE TABLE IF NOT EXISTS known_mcps (
    mcp_id       UUID PRIMARY KEY,
    workspace_id UUID NOT NULL
);
