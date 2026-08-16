// Package projection implements the local catalog-projection repository
// adapter on Postgres (Ports & Adapters: the adapter side of the hexagon): the
// projected skill/MCP definition rows the Agent service keeps for attachment
// validation, plus the projection writes driven by the catalog consumers.
package projection

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/agent/internal/domain"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repo is the pool- or tx-backed adapter for the catalog projections.
type Repo struct{ q querier }

// New wraps a querier (pool or tx) as the catalog-projection adapter.
func New(q querier) *Repo { return &Repo{q: q} }

// SkillWorkspace returns the workspace a skill belongs to, or ErrUnknownDefinition.
func (r *Repo) SkillWorkspace(ctx context.Context, skillID identity.ID) (identity.ID, error) {
	var ws identity.ID
	err := r.q.QueryRow(ctx, `SELECT workspace_id FROM known_skills WHERE skill_id = $1`, skillID).Scan(&ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrUnknownDefinition
	}
	return ws, err
}

// McpWorkspace returns the workspace an MCP definition belongs to, or ErrUnknownDefinition.
func (r *Repo) McpWorkspace(ctx context.Context, mcpID identity.ID) (identity.ID, error) {
	var ws identity.ID
	err := r.q.QueryRow(ctx, `SELECT workspace_id FROM known_mcps WHERE mcp_id = $1`, mcpID).Scan(&ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrUnknownDefinition
	}
	return ws, err
}

// ── Projection/status mutations (run-lifecycle + catalog consumers) ─────────
// These write the local projections and runtime status derived from events;
// the consumers that call them arrive with the messaging conversion.

// UpsertSkillProjection records a catalog skill's workspace.
func (r *Repo) UpsertSkillProjection(ctx context.Context, skillID, workspaceID identity.ID) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO known_skills (skill_id, workspace_id) VALUES ($1, $2)
		ON CONFLICT (skill_id) DO UPDATE SET workspace_id = EXCLUDED.workspace_id`, skillID, workspaceID)
	return err
}

// DeleteSkillProjection forgets a deleted catalog skill.
func (r *Repo) DeleteSkillProjection(ctx context.Context, skillID identity.ID) error {
	_, err := r.q.Exec(ctx, `DELETE FROM known_skills WHERE skill_id = $1`, skillID)
	return err
}

// UpsertMcpProjection records a catalog MCP definition's workspace.
func (r *Repo) UpsertMcpProjection(ctx context.Context, mcpID, workspaceID identity.ID) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO known_mcps (mcp_id, workspace_id) VALUES ($1, $2)
		ON CONFLICT (mcp_id) DO UPDATE SET workspace_id = EXCLUDED.workspace_id`, mcpID, workspaceID)
	return err
}

// DeleteMcpProjection forgets a deleted catalog MCP definition.
func (r *Repo) DeleteMcpProjection(ctx context.Context, mcpID identity.ID) error {
	_, err := r.q.Exec(ctx, `DELETE FROM known_mcps WHERE mcp_id = $1`, mcpID)
	return err
}