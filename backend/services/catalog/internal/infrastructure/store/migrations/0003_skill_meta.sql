-- Skill enable state + metadata (skill-mcp-catalog capability).
ALTER TABLE skills ADD COLUMN IF NOT EXISTS enabled BOOLEAN;
ALTER TABLE skills ADD COLUMN IF NOT EXISTS trigger TEXT NOT NULL DEFAULT '';
ALTER TABLE skills ADD COLUMN IF NOT EXISTS step_count INTEGER;
