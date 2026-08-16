// Package store implements the Task domain repository ports on Postgres
// (Ports & Adapters: the adapter side of the hexagon). Each aggregate has its
// own adapter satisfying the domain port; pool-backed instances serve the
// use cases and the saga coordinator.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/tasks"
	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/task/internal/domain"
)

//go:embed migrations/*.sql
var migrations embed.FS

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store owns the task Postgres pool and exposes pool-backed adapters for the
// domain ports. It is the composition-root entrypoint of this package.
type Store struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	Tasks    domain.TaskRepository
	Feedback domain.FeedbackRepository
}

// New opens the task database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Store, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	s := &Store{pool: pool, log: log}
	s.Tasks = &taskRepo{q: pool}
	s.Feedback = &feedbackRepo{q: pool}
	return s, nil
}

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

const taskCols = `id, workspace_id, project_id, agent_id, model_override, title, prompt, description,
	status, type, priority, labels, points, due_at, progress, branch_name, worktree_path,
	round_no, created_at, updated_at`

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

// whereScoped is whereScopedAt with a fresh parameter index.
func whereScoped(ws []identity.ID) (string, []any) {
	return whereScopedAt(1, ws)
}

func scanTask(row pgx.Row) (tasks.Task, error) {
	var t tasks.Task
	var agentID, modelOverride *string
	var taskType, priority string
	var points, progress *int
	var dueAt *time.Time
	err := row.Scan(&t.ID, &t.WorkspaceID, &t.ProjectID, &agentID, &modelOverride, &t.Title, &t.Prompt, &t.Description,
		&t.Status, &taskType, &priority, &t.Labels, &points, &dueAt, &progress, &t.BranchName, &t.WorktreePath,
		&t.RoundNo, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return tasks.Task{}, err
	}
	t.Type = tasks.TaskType(taskType)
	t.Priority = tasks.Priority(priority)
	t.Points = points
	t.Progress = progress
	t.ModelOverride = modelOverride
	t.DueAt = dueAt
	if agentID != nil {
		id := identity.ID(*agentID)
		t.AgentID = &id
	}
	if t.Labels == nil {
		t.Labels = []string{}
	}
	return t, nil
}

type taskRepo struct{ q querier }

// queryClauses builds the WHERE clause and args for a task list query (the
// SQL-shaping half of domain.Query; the query itself is a domain value object).
func queryClauses(q domain.Query) (string, []any) {
	where := []string{}
	args := []any{}
	idx := 1
	add := func(pred string, v any) { where = append(where, fmt.Sprintf(pred, idx)); args = append(args, v); idx++ }
	if len(q.Workspaces) > 0 {
		ids := make([]string, len(q.Workspaces))
		for i, id := range q.Workspaces {
			ids[i] = string(id)
		}
		where = append(where, fmt.Sprintf("workspace_id = ANY($%d::uuid[])", idx))
		args = append(args, ids)
		idx++
	} else {
		// Fail closed: without a workspace context nothing is returned.
		where = append(where, "false")
	}
	if q.ProjectID != "" {
		add("project_id = $%d", q.ProjectID)
	}
	if q.Status != "" {
		add("status = $%d", q.Status)
	}
	if q.Type != "" {
		add("type = $%d", q.Type)
	}
	if q.Priority != "" {
		add("priority = $%d", q.Priority)
	}
	if q.Assignee != "" {
		add("agent_id = $%d", q.Assignee)
	}
	if q.Label != "" {
		add("$%d = ANY(labels)", q.Label)
	}
	if q.Q != "" {
		add("title ILIKE $%d", "%"+q.Q+"%")
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}
	return clause, args
}

func (r *taskRepo) List(ctx context.Context, q domain.Query) ([]tasks.Task, error) {
	clause, args := queryClauses(q)
	rows, err := r.q.Query(ctx, `SELECT `+taskCols+` FROM tasks`+clause+` ORDER BY updated_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []tasks.Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *taskRepo) Get(ctx context.Context, id identity.ID, ws []identity.ID) (tasks.Task, error) {
	where, args := whereScopedAt(2, ws) // $1=id, $2=ws ids
	row := r.q.QueryRow(ctx, `SELECT `+taskCols+` FROM tasks WHERE id = $1`+where, append([]any{id}, args...)...)
	t, err := scanTask(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return tasks.Task{}, domain.ErrNotFound
	}
	return t, err
}

// GetUnscoped returns a task regardless of workspace context (saga consumer).
func (r *taskRepo) GetUnscoped(ctx context.Context, id identity.ID) (tasks.Task, error) {
	row := r.q.QueryRow(ctx, `SELECT `+taskCols+` FROM tasks WHERE id = $1`, id)
	t, err := scanTask(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return tasks.Task{}, domain.ErrNotFound
	}
	return t, err
}

func (r *taskRepo) Create(ctx context.Context, workspaceID identity.ID, in domain.CreateInput) (tasks.Task, error) {
	if in.Labels == nil {
		in.Labels = []string{}
	}
	var dueAt any
	if in.DueAt != nil {
		if t, err := time.Parse(time.RFC3339, *in.DueAt); err == nil {
			dueAt = t
		} else {
			dueAt = *in.DueAt // let Postgres reject if unparseable
		}
	}
	row := r.q.QueryRow(ctx, `
		INSERT INTO tasks (workspace_id, project_id, agent_id, title, prompt, description, type, priority, labels, points, due_at, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'backlog')
		RETURNING id`, workspaceID, in.ProjectID, in.AgentID, in.Title, in.Prompt, in.Description,
		in.Type, in.Priority, in.Labels, in.Points, dueAt)
	var id identity.ID
	if err := row.Scan(&id); err != nil {
		return tasks.Task{}, err
	}
	return r.Get(ctx, id, []identity.ID{workspaceID})
}

// Update applies a partial update (frontend updateTask sends Partial<Task>).
// It accepts a raw map to mirror the frontend's "any subset of fields" semantics.
func (r *taskRepo) Update(ctx context.Context, id identity.ID, ws []identity.ID, fields map[string]any) (tasks.Task, error) {
	allowed := map[string]string{
		"title": "title", "prompt": "prompt", "description": "description",
		"type": "type", "priority": "priority", "points": "points",
		"due_at": "due_at", "progress": "progress", "branch_name": "branch_name",
		"worktree_path": "worktree_path", "agent_id": "agent_id", "model_override": "model_override",
		"labels": "labels",
	}
	where, scopeArgs := whereScoped(ws)
	if len(scopeArgs) == 0 {
		return tasks.Task{}, domain.ErrNotFound
	}
	sets := []string{"updated_at = now()"}
	args := append(scopeArgs, id) // [$1..$n = ws ids, then id]
	idx := len(args) + 1
	for jsonKey, col := range allowed {
		v, ok := fields[jsonKey]
		if !ok {
			continue
		}
		sets = append(sets, fmt.Sprintf("%s = $%d", col, idx))
		args = append(args, v)
		idx++
	}
	q := `UPDATE tasks SET ` + strings.Join(sets, ", ") + ` WHERE id = $` + fmt.Sprint(len(scopeArgs)+1) + where + ` RETURNING ` + taskCols
	row := r.q.QueryRow(ctx, q, args...)
	t, err := scanTask(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return tasks.Task{}, domain.ErrNotFound
	}
	return t, err
}

// SetStatus updates a task's status and returns the updated task.
func (r *taskRepo) SetStatus(ctx context.Context, id identity.ID, status tasks.TaskStatus) (tasks.Task, error) {
	row := r.q.QueryRow(ctx, `
		UPDATE tasks SET status = $2, updated_at = now() WHERE id = $1
		RETURNING `+taskCols, id, status)
	t, err := scanTask(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return tasks.Task{}, domain.ErrNotFound
	}
	return t, err
}

// SetRoundNo advances the review round counter (saga phase 6).
func (r *taskRepo) SetRoundNo(ctx context.Context, id identity.ID, roundNo int) error {
	_, err := r.q.Exec(ctx, `UPDATE tasks SET round_no = $2, updated_at = now() WHERE id = $1`, id, roundNo)
	return err
}

// CountOpenByWorkspace counts tasks in open states (doing/review) per workspace
// for the Gateway's workspace-stats composition.
func (r *taskRepo) CountOpenByWorkspace(ctx context.Context, workspaceID identity.ID) (int, error) {
	var n int
	err := r.q.QueryRow(ctx,
		`SELECT count(*) FROM tasks WHERE workspace_id = $1 AND status IN ('doing', 'review')`,
		workspaceID).Scan(&n)
	return n, err
}

// SagaNew records (task_id, run_id) as processed; false when already seen.
// This is the saga's idempotency hook for at-least-once Kafka redelivery.
func (r *taskRepo) SagaNew(ctx context.Context, taskID, runID identity.ID) (bool, error) {
	tag, err := r.q.Exec(ctx, `
		INSERT INTO saga_runs (task_id, run_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, taskID, runID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *taskRepo) Delete(ctx context.Context, id identity.ID, ws []identity.ID) error {
	where, args := whereScopedAt(2, ws) // $1=id, $2=ws ids
	if len(args) == 0 {
		return domain.ErrNotFound
	}
	tag, err := r.q.Exec(ctx, `DELETE FROM tasks WHERE id = $1`+where, append([]any{id}, args...)...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ── Feedback ────────────────────────────────────────────────────────────────

type feedbackRepo struct{ q querier }

func (r *feedbackRepo) List(ctx context.Context, taskID identity.ID, ws []identity.ID) ([]tasks.Feedback, error) {
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

func (r *feedbackRepo) Add(ctx context.Context, taskID identity.ID, ws []identity.ID, body string) (tasks.Feedback, error) {
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