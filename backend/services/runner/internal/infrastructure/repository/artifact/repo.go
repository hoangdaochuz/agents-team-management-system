// Package artifact implements the Artifacts aggregate repository on Postgres.
package artifact

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/identity"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repo is the pool-backed adapter satisfying domain.ArtifactRepository.
type Repo struct{ q querier }

// New builds the adapter over a pool or transaction.
func New(q querier) *Repo { return &Repo{q: q} }

func (r *Repo) AddArtifact(ctx context.Context, taskID identity.ID, runID *identity.ID, filename, kind, summary string, additions, deletions int) (agentexec.Artifact, error) {
	var a agentexec.Artifact
	err := r.q.QueryRow(ctx, `
		INSERT INTO artifacts (task_id, run_id, filename, kind, summary, additions, deletions)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, task_id, run_id, filename, kind, summary, additions, deletions, created_at`,
		taskID, runID, filename, kind, summary, additions, deletions).
		Scan(&a.ID, &a.TaskID, &a.RunID, &a.Filename, &a.Kind, &a.Summary, &a.Additions, &a.Deletions, &a.CreatedAt)
	return a, err
}

func (r *Repo) ListArtifactsByTask(ctx context.Context, taskID identity.ID) ([]agentexec.Artifact, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, task_id, run_id, filename, kind, summary, additions, deletions, created_at
		FROM artifacts WHERE task_id = $1 ORDER BY created_at DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []agentexec.Artifact{}
	for rows.Next() {
		var a agentexec.Artifact
		if err := rows.Scan(&a.ID, &a.TaskID, &a.RunID, &a.Filename, &a.Kind, &a.Summary, &a.Additions, &a.Deletions, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}