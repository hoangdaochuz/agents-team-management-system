// Package feedback implements the Feedback aggregate repository on Postgres.
package feedback

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/tasks"
	"github.com/aaks/server/services/task/internal/domain"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repo is the pool-backed adapter satisfying domain.FeedbackRepository.
type Repo struct{ q querier }

// New builds the adapter over a pool or transaction.
func New(q querier) *Repo { return &Repo{q: q} }

// whereScopedAt returns `AND workspace_id = ANY($start)` scoping plus its args.
// start is the 1-based index of the next statement parameter.
func whereScopedAt(start int, ws []identity.ID) (string, []any) {
	if len(ws) == 0 {
		return " AND false", nil
	}
	ids := make([]string, len(ws))
	for i, id := range ws {
		ids[i] = string(id)
	}
	return fmt.Sprintf(" AND workspace_id = ANY($%d::uuid[])", start), []any{ids}
}

func (r *Repo) List(ctx context.Context, taskID identity.ID, ws []identity.ID) ([]tasks.Feedback, error) {
	where, args := whereScopedAt(2, ws) // $1=taskID, $2=ws ids
	if len(args) == 0 {
		return []tasks.Feedback{}, nil
	}
	rows, err := r.q.Query(ctx, `SELECT id, task_id, author, body, created_at FROM feedback WHERE task_id = $1`+where+` ORDER BY created_at ASC`, append([]any{taskID}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []tasks.Feedback{}
	for rows.Next() {
		var f tasks.Feedback
		if err := rows.Scan(&f.ID, &f.TaskID, &f.Author, &f.Body, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r *Repo) Add(ctx context.Context, taskID identity.ID, ws []identity.ID, body string) (tasks.Feedback, error) {
	var f tasks.Feedback
	scopeClause, scopeArgs := whereScopedAt(3, ws) // $1=taskID, $2=body, $3=ws ids
	if len(scopeArgs) == 0 {
		return tasks.Feedback{}, domain.ErrNotFound
	}
	err := r.q.QueryRow(ctx, `
		INSERT INTO feedback (task_id, author, body)
		SELECT $1, 'user', $2 FROM tasks WHERE id = $1`+scopeClause+`
		RETURNING id, task_id, author, body, created_at`,
		taskID, body, scopeArgs[0]).Scan(&f.ID, &f.TaskID, &f.Author, &f.Body, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return tasks.Feedback{}, domain.ErrNotFound
	}
	return f, err
}