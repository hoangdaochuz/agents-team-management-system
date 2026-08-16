// Package plugin implements the plugin aggregate repository adapter on Postgres
// (Ports & Adapters: the adapter side of the hexagon).
package plugin

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

// Repo is the pool- or tx-backed adapter for the plugin aggregate.
type Repo struct{ q querier }

// New wraps a querier (pool or tx) as the plugin adapter.
func New(q querier) *Repo { return &Repo{q: q} }

const pluginCols = `id, name, version, capabilities, scopes, enabled`

func scanPlugin(row pgx.Row) (resources.Plugin, error) {
	var p resources.Plugin
	err := row.Scan(&p.ID, &p.Name, &p.Version, &p.Capabilities, &p.Scopes, &p.Enabled)
	return p, err
}

func (r *Repo) List(ctx context.Context, workspaceID identity.ID) ([]resources.Plugin, error) {
	rows, err := r.q.Query(ctx, `
		SELECT `+pluginCols+` FROM plugins
		WHERE workspace_id = $1 ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resources.Plugin{}
	for rows.Next() {
		p, err := scanPlugin(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repo) SetEnabled(ctx context.Context, workspaceID, id identity.ID, enabled bool) (resources.Plugin, error) {
	row := r.q.QueryRow(ctx, `
		UPDATE plugins SET enabled = $3 WHERE workspace_id = $1 AND id = $2
		RETURNING `+pluginCols, workspaceID, id, enabled)
	p, err := scanPlugin(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.Plugin{}, domain.ErrNotFound
	}
	return p, err
}