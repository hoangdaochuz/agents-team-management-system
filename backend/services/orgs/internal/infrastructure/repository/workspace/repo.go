// Package workspace adapts the Workspace aggregate repository port to
// Postgres (Ports & Adapters: the adapter side of the hexagon). Instances
// serve both plain-pool and tx-scoped access via the shared querier.
package workspace

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/workspaces"
	"github.com/aaks/server/services/orgs/internal/domain"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repo is the Postgres adapter for the Workspace aggregate.
type Repo struct{ q querier }

// New builds the adapter on a pool or transaction querier.
func New(q querier) *Repo { return &Repo{q: q} }

const wsCols = `id, organization_id, name, repo_source, default_branch, glyph, description, created_at`

func scanWorkspace(row pgx.Row) (workspaces.Workspace, error) {
	var w workspaces.Workspace
	var org identity.ID
	err := row.Scan(&w.ID, &org, &w.Name, &w.RepoSource, &w.DefaultBranch, &w.Glyph, &w.Description, &w.CreatedAt)
	if err != nil {
		return workspaces.Workspace{}, err
	}
	return w, err
}

func (r *Repo) Create(ctx context.Context, orgID identity.ID, name, repoSource, defaultBranch, glyph, description string) (workspaces.Workspace, error) {
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	row := r.q.QueryRow(ctx, `
		INSERT INTO workspaces (organization_id, name, repo_source, default_branch, glyph, description)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+wsCols, orgID, name, repoSource, defaultBranch, glyph, description)
	return scanWorkspace(row)
}

func (r *Repo) ByID(ctx context.Context, id identity.ID) (workspaces.Workspace, error) {
	row := r.q.QueryRow(ctx, `SELECT `+wsCols+` FROM workspaces WHERE id = $1`, id)
	w, err := scanWorkspace(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return workspaces.Workspace{}, domain.ErrNotFound
	}
	return w, err
}

func (r *Repo) ListByUser(ctx context.Context, userID identity.ID) ([]workspaces.Workspace, error) {
	rows, err := r.q.Query(ctx, `
		SELECT w.id, w.organization_id, w.name, w.repo_source, w.default_branch, w.glyph, w.description, w.created_at, m.role
		FROM memberships m JOIN workspaces w ON w.id = m.workspace_id
		WHERE m.user_id = $1 AND m.status = 'active'
		ORDER BY w.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []workspaces.Workspace{}
	for rows.Next() {
		var w workspaces.Workspace
		var org, role string
		if err := rows.Scan(&w.ID, &org, &w.Name, &w.RepoSource, &w.DefaultBranch, &w.Glyph, &w.Description, &w.CreatedAt, &role); err != nil {
			return nil, err
		}
		w.Role = identity.Role(role)
		out = append(out, w)
	}
	return out, rows.Err()
}

func (r *Repo) GetByUser(ctx context.Context, userID, workspaceID identity.ID) (workspaces.Workspace, error) {
	row := r.q.QueryRow(ctx, `
		SELECT w.id, w.organization_id, w.name, w.repo_source, w.default_branch, w.glyph, w.description, w.created_at, m.role
		FROM memberships m JOIN workspaces w ON w.id = m.workspace_id
		WHERE m.user_id = $1 AND w.id = $2 AND m.status = 'active'`, userID, workspaceID)
	var w workspaces.Workspace
	var org, role string
	err := row.Scan(&w.ID, &org, &w.Name, &w.RepoSource, &w.DefaultBranch, &w.Glyph, &w.Description, &w.CreatedAt, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return workspaces.Workspace{}, domain.ErrNotFound
	}
	if err != nil {
		return workspaces.Workspace{}, err
	}
	w.Role = identity.Role(role)
	return w, nil
}