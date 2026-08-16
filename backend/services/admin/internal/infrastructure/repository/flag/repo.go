// Package flag implements the FeatureFlag aggregate's Postgres adapter.
package flag

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aaks/server/internal/contracts/admin"
	"github.com/aaks/server/services/admin/internal/domain"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repo is the pool-backed adapter for domain.FlagRepository.
type Repo struct{ q querier }

// New wraps the given querier (pool or tx) as a feature flag repository.
func New(q querier) *Repo { return &Repo{q: q} }

const flagCols = `key, label, description, enabled`

func scanFlag(row pgx.Row) (admin.FeatureFlag, error) {
	var f admin.FeatureFlag
	err := row.Scan(&f.Key, &f.Label, &f.Description, &f.Enabled)
	return f, err
}

func (r *Repo) List(ctx context.Context) ([]admin.FeatureFlag, error) {
	rows, err := r.q.Query(ctx, `SELECT `+flagCols+` FROM feature_flags ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []admin.FeatureFlag{}
	for rows.Next() {
		f, err := scanFlag(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r *Repo) SetEnabled(ctx context.Context, key string, enabled bool) (admin.FeatureFlag, error) {
	row := r.q.QueryRow(ctx, `
		UPDATE feature_flags SET enabled = $2, updated_at = $3 WHERE key = $1
		RETURNING `+flagCols, key, enabled, time.Now())
	f, err := scanFlag(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return admin.FeatureFlag{}, domain.ErrNotFound
	}
	return f, err
}
