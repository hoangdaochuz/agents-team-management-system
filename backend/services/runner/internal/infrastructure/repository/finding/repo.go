// Package finding implements the Findings aggregate repository on Postgres.
package finding

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

// Repo is the pool-backed adapter satisfying domain.FindingRepository.
type Repo struct{ q querier }

// New builds the adapter over a pool or transaction.
func New(q querier) *Repo { return &Repo{q: q} }

func (r *Repo) AddFinding(ctx context.Context, runID identity.ID, f agentexec.Finding) (agentexec.Finding, error) {
	var out agentexec.Finding
	err := r.q.QueryRow(ctx, `
		INSERT INTO findings (run_id, file, line, severity, issue, recommendation, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, run_id, file, line, severity, issue, recommendation, status`,
		runID, f.File, f.Line, f.Severity, f.Issue, f.Recommendation, f.Status).
		Scan(&out.ID, &out.RunID, &out.File, &out.Line, &out.Severity, &out.Issue, &out.Recommendation, &out.Status)
	return out, err
}

func (r *Repo) ListFindings(ctx context.Context, runID identity.ID) ([]agentexec.Finding, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, run_id, file, line, severity, issue, recommendation, status FROM findings
		WHERE run_id = $1 ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []agentexec.Finding{}
	for rows.Next() {
		var f agentexec.Finding
		if err := rows.Scan(&f.ID, &f.RunID, &f.File, &f.Line, &f.Severity, &f.Issue, &f.Recommendation, &f.Status); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}