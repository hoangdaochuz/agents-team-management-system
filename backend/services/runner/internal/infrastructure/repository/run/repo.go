// Package run implements the Runs aggregate repository on Postgres.
package run

import (
	"context"
	"time"

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

// Repo is the pool-backed adapter satisfying domain.RunRepository.
type Repo struct{ q querier }

// New builds the adapter over a pool or transaction.
func New(q querier) *Repo { return &Repo{q: q} }

func scanRun(row pgx.Row) (agentexec.Run, error) {
	var r agentexec.Run
	var ended *time.Time
	var agent identity.ID
	var errMsg string
	err := row.Scan(&r.ID, &r.TaskID, &r.Role, &agent, &r.Model, &r.Status, &r.RoundNo, &r.StartedAt, &ended, &r.TokenUsage, &errMsg)
	if err != nil {
		return agentexec.Run{}, err
	}
	r.AgentID = agent
	r.EndedAt = ended
	r.Error = errMsg
	return r, nil
}

func (r *Repo) CreateRun(ctx context.Context, taskID identity.ID, role agentexec.RunRole, agentID identity.ID, model string, roundNo int) (identity.ID, error) {
	var id identity.ID
	err := r.q.QueryRow(ctx, `
		INSERT INTO runs (task_id, role, agent_id, model, round_no)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		taskID, role, agentID, model, roundNo).Scan(&id)
	return id, err
}

func (r *Repo) ListRunsByTask(ctx context.Context, taskID identity.ID) ([]agentexec.Run, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, task_id, role, agent_id, model, status, round_no, started_at, ended_at, token_usage, error
		FROM runs WHERE task_id = $1 ORDER BY started_at DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []agentexec.Run{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (r *Repo) GetRun(ctx context.Context, runID identity.ID) (agentexec.Run, error) {
	row := r.q.QueryRow(ctx, `
		SELECT id, task_id, role, agent_id, model, status, round_no, started_at, ended_at, token_usage, error
		FROM runs WHERE id = $1`, runID)
	return scanRun(row)
}

func (r *Repo) LatestRun(ctx context.Context, taskID identity.ID) (agentexec.Run, error) {
	row := r.q.QueryRow(ctx, `
		SELECT id, task_id, role, agent_id, model, status, round_no, started_at, ended_at, token_usage, error
		FROM runs WHERE task_id = $1 ORDER BY started_at DESC LIMIT 1`, taskID)
	return scanRun(row)
}

func (r *Repo) FinishRun(ctx context.Context, runID identity.ID, status agentexec.RunStatus, tokenUsage int, errMsg string) error {
	_, err := r.q.Exec(ctx, `
		UPDATE runs SET status = $2, token_usage = $3, error = $4, ended_at = now()
		WHERE id = $1`, runID, status, tokenUsage, errMsg)
	return err
}
