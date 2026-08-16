// Package step implements the Steps aggregate repository on Postgres.
package step

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

// Repo is the pool-backed adapter satisfying domain.StepRepository.
type Repo struct{ q querier }

// New builds the adapter over a pool or transaction.
func New(q querier) *Repo { return &Repo{q: q} }

func (r *Repo) AppendStep(ctx context.Context, runID identity.ID, seq int, kind agentexec.StepKind, payload []byte) (agentexec.Step, error) {
	var st agentexec.Step
	err := r.q.QueryRow(ctx, `
		INSERT INTO steps (run_id, seq, kind, payload) VALUES ($1, $2, $3, $4)
		ON CONFLICT (run_id, seq) DO NOTHING
		RETURNING id, run_id, seq, kind, payload, created_at`,
		runID, seq, kind, payload).
		Scan(&st.ID, &st.RunID, &st.Seq, &st.Kind, &st.Payload, &st.CreatedAt)
	return st, err
}

func (r *Repo) ListSteps(ctx context.Context, runID identity.ID) ([]agentexec.Step, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, run_id, seq, kind, payload, created_at FROM steps
		WHERE run_id = $1 ORDER BY seq`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []agentexec.Step{}
	for rows.Next() {
		var st agentexec.Step
		if err := rows.Scan(&st.ID, &st.RunID, &st.Seq, &st.Kind, &st.Payload, &st.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (r *Repo) ListStepsByTask(ctx context.Context, taskID identity.ID) ([]agentexec.Step, error) {
	rows, err := r.q.Query(ctx, `
		SELECT st.id, st.run_id, st.seq, st.kind, st.payload, st.created_at
		FROM steps st JOIN runs r ON r.id = st.run_id
		WHERE r.task_id = $1 ORDER BY r.started_at, st.seq`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []agentexec.Step{}
	for rows.Next() {
		var st agentexec.Step
		if err := rows.Scan(&st.ID, &st.RunID, &st.Seq, &st.Kind, &st.Payload, &st.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (r *Repo) MaxStepSeq(ctx context.Context, runID identity.ID) (int, error) {
	var n int
	err := r.q.QueryRow(ctx, `SELECT coalesce(max(seq), 0) FROM steps WHERE run_id = $1`, runID).Scan(&n)
	return n, err
}
