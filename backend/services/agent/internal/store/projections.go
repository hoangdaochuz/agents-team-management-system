package store

import (
	"context"
	"errors"

	"github.com/aaks/server/internal/contracts"
)

// ErrUnknownDefinition reports an attachment referencing a definition the
// catalog has not projected into this workspace.
var ErrUnknownDefinition = errors.New("skill or mcp definition is unknown in this workspace")


// Projections consumed from the Catalog (skill/mcp definitions) and the Runner
// (run lifecycle) so the Agent service can validate attachments within a
// workspace and derive runtime status without service-to-service sync calls.

// skillWorkspace returns the workspace a skill belongs to, or an error.
func (s *Store) skillWorkspace(ctx context.Context, skillID contracts.ID) (contracts.ID, error) {
	var ws contracts.ID
	err := s.pool.QueryRow(ctx, `SELECT workspace_id FROM known_skills WHERE skill_id = $1`, skillID).Scan(&ws)
	return ws, err
}

// mcpWorkspace returns the workspace an MCP definition belongs to, or an error.
func (s *Store) mcpWorkspace(ctx context.Context, mcpID contracts.ID) (contracts.ID, error) {
	var ws contracts.ID
	err := s.pool.QueryRow(ctx, `SELECT workspace_id FROM known_mcps WHERE mcp_id = $1`, mcpID).Scan(&ws)
	return ws, err
}

// UpsertSkillProjection records a catalog skill's workspace.
func (s *Store) UpsertSkillProjection(ctx context.Context, skillID, workspaceID contracts.ID) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO known_skills (skill_id, workspace_id) VALUES ($1, $2)
		ON CONFLICT (skill_id) DO UPDATE SET workspace_id = EXCLUDED.workspace_id`, skillID, workspaceID)
	return err
}

// DeleteSkillProjection forgets a deleted catalog skill.
func (s *Store) DeleteSkillProjection(ctx context.Context, skillID contracts.ID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM known_skills WHERE skill_id = $1`, skillID)
	return err
}

// UpsertMcpProjection records a catalog MCP definition's workspace.
func (s *Store) UpsertMcpProjection(ctx context.Context, mcpID, workspaceID contracts.ID) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO known_mcps (mcp_id, workspace_id) VALUES ($1, $2)
		ON CONFLICT (mcp_id) DO UPDATE SET workspace_id = EXCLUDED.workspace_id`, mcpID, workspaceID)
	return err
}

// DeleteMcpProjection forgets a deleted catalog MCP definition.
func (s *Store) DeleteMcpProjection(ctx context.Context, mcpID contracts.ID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM known_mcps WHERE mcp_id = $1`, mcpID)
	return err
}

// SetAgentRunning marks an agent as executing a task (run.started consumer).
func (s *Store) SetAgentRunning(ctx context.Context, agentID, taskID contracts.ID) error {
	_, err := s.pool.Exec(ctx, `UPDATE agents SET status = 'running', current_task_id = $2 WHERE id = $1`, agentID, taskID)
	return err
}

// SetAgentIdle clears an agent's running state (run.completed consumer).
func (s *Store) SetAgentIdle(ctx context.Context, agentID contracts.ID) error {
	_, err := s.pool.Exec(ctx, `UPDATE agents SET status = 'idle', current_task_id = NULL, load = NULL WHERE id = $1 AND status = 'running'`, agentID)
	return err
}

// SetAgentPaused pauses an agent (admin action).
func (s *Store) SetAgentPaused(ctx context.Context, agentID contracts.ID) error {
	_, err := s.pool.Exec(ctx, `UPDATE agents SET status = 'paused' WHERE id = $1`, agentID)
	return err
}
