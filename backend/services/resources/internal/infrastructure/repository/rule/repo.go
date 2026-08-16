// Package rule implements the rule aggregate repository adapter on Postgres
// (Ports & Adapters: the adapter side of the hexagon).
package rule

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
	"github.com/aaks/server/services/resources/internal/domain"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repo is the pool- or tx-backed adapter for the rule aggregate.
type Repo struct{ q querier }

// New wraps a querier (pool or tx) as the rule adapter.
func New(q querier) *Repo { return &Repo{q: q} }

const ruleCols = `id, name, description, enabled`

func scanRule(row pgx.Row) (resources.Rule, error) {
	var r resources.Rule
	err := row.Scan(&r.ID, &r.Name, &r.Description, &r.Enabled)
	return r, err
}

func (r *Repo) List(ctx context.Context, workspaceID identity.ID) ([]resources.Rule, error) {
	rows, err := r.q.Query(ctx, `
		SELECT `+ruleCols+` FROM rules
		WHERE workspace_id = $1 ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resources.Rule{}
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

func (r *Repo) Create(ctx context.Context, workspaceID identity.ID, name, description string, enabled bool) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO rules (workspace_id, name, description, enabled)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING`, workspaceID, name, description, enabled)
	return err
}

func (r *Repo) SetEnabled(ctx context.Context, workspaceID, id identity.ID, enabled bool) (resources.Rule, error) {
	row := r.q.QueryRow(ctx, `
		UPDATE rules SET enabled = $3 WHERE workspace_id = $1 AND id = $2
		RETURNING `+ruleCols, workspaceID, id, enabled)
	rule, err := scanRule(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.Rule{}, domain.ErrNotFound
	}
	return rule, err
}

func (r *Repo) Enabled(ctx context.Context, workspaceID identity.ID) ([]resources.Rule, error) {
	rows, err := r.q.Query(ctx, `
		SELECT `+ruleCols+` FROM rules
		WHERE workspace_id = $1 AND enabled = true ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resources.Rule{}
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}