// Package store is the Task service persistence layer (tasks + feedback).
//
// Phase 3 scope: synchronous CRUD + feedback + status patch only. The saga
// (run/review/stop/open-pr over Kafka) is wired in phase 6; for now re-run/stop
// are no-ops returning 501 from the handlers, and patchStatus just persists.
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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/contracts"
	"github.com/aaks/server/internal/db"
)

//go:embed migrations/*.sql
var migrations embed.FS

var ErrTaskNotFound = errors.New("task not found")

type Store struct{ pool *pgxpool.Pool }

func New(ctx context.Context, dsn string, log *slog.Logger) (*Store, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.MigrateFS(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

const taskCols = `id, workspace_id, project_id, agent_id, model_override, title, prompt, description,
	status, type, priority, labels, points, due_at, progress, branch_name, worktree_path,
	round_no, created_at, updated_at`

// whereScoped returns `AND workspace_id = ANY($start)` scoping plus its args.
// start is the 1-based index of the next statement parameter.
func whereScopedAt(start int, ws []contracts.ID) (string, []any) {
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
func whereScoped(ws []contracts.ID) (string, []any) {
	return whereScopedAt(1, ws)
}

func scanTask(row pgx.Row) (contracts.Task, error) {
	var t contracts.Task
	var agentID, modelOverride *string
	var taskType, priority string
	var points, progress *int
	var dueAt *time.Time
	err := row.Scan(&t.ID, &t.WorkspaceID, &t.ProjectID, &agentID, &modelOverride, &t.Title, &t.Prompt, &t.Description,
		&t.Status, &taskType, &priority, &t.Labels, &points, &dueAt, &progress, &t.BranchName, &t.WorktreePath,
		&t.RoundNo, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return contracts.Task{}, err
	}
	t.Type = contracts.TaskType(taskType)
	t.Priority = contracts.Priority(priority)
	t.Points = points
	t.Progress = progress
	t.ModelOverride = modelOverride
	t.DueAt = dueAt
	if agentID != nil {
		id := contracts.ID(*agentID)
		t.AgentID = &id
	}
	if t.Labels == nil {
		t.Labels = []string{}
	}
	return t, nil
}

// Query mirrors frontend TaskQuery (the filters the SPA sends).
type Query struct {
	Workspaces []contracts.ID // workspace context (empty = no results, fail closed)
	ProjectID  contracts.ID
	Status     contracts.TaskStatus
	Type       contracts.TaskType
	Priority   contracts.Priority
	Assignee   contracts.ID
	Label      string
	Q          string
}

func (q Query) clauses() (string, []any) {
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

func (s *Store) List(ctx context.Context, q Query) ([]contracts.Task, error) {
	clause, args := q.clauses()
	rows, err := s.pool.Query(ctx, `SELECT `+taskCols+` FROM tasks`+clause+` ORDER BY updated_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, id contracts.ID, ws []contracts.ID) (contracts.Task, error) {
	where, args := whereScopedAt(2, ws) // $1=id, $2=ws ids
	row := s.pool.QueryRow(ctx, `SELECT `+taskCols+` FROM tasks WHERE id = $1`+where, append([]any{id}, args...)...)
	t, err := scanTask(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Task{}, ErrTaskNotFound
	}
	return t, err
}

// GetUnscoped returns a task regardless of workspace context (saga consumer).
func (s *Store) GetUnscoped(ctx context.Context, id contracts.ID) (contracts.Task, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+taskCols+` FROM tasks WHERE id = $1`, id)
	t, err := scanTask(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Task{}, ErrTaskNotFound
	}
	return t, err
}

// CreateInput is the body of POST /tasks (matches frontend createTask).
type CreateInput struct {
	ProjectID    contracts.ID      `json:"project_id"`
	Title        string            `json:"title"`
	Prompt       string            `json:"prompt"`
	Description  string            `json:"description,omitempty"`
	Type         contracts.TaskType `json:"type,omitempty"`
	Priority     contracts.Priority `json:"priority,omitempty"`
	Labels       []string          `json:"labels,omitempty"`
	Points       *int              `json:"points,omitempty"`
	AgentID      *contracts.ID     `json:"agent_id,omitempty"`
	DueAt        *string           `json:"due_at,omitempty"`
}

func (s *Store) Create(ctx context.Context, workspaceID contracts.ID, in CreateInput) (contracts.Task, error) {
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
	row := s.pool.QueryRow(ctx, `
		INSERT INTO tasks (workspace_id, project_id, agent_id, title, prompt, description, type, priority, labels, points, due_at, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'backlog')
		RETURNING id`, workspaceID, in.ProjectID, in.AgentID, in.Title, in.Prompt, in.Description,
		in.Type, in.Priority, in.Labels, in.Points, dueAt)
	var id contracts.ID
	if err := row.Scan(&id); err != nil {
		return contracts.Task{}, err
	}
	return s.Get(ctx, id, []contracts.ID{workspaceID})
}

// Update applies a partial update (frontend updateTask sends Partial<Task>).
// It accepts a raw map to mirror the frontend's "any subset of fields" semantics.
func (s *Store) Update(ctx context.Context, id contracts.ID, ws []contracts.ID, fields map[string]any) (contracts.Task, error) {
	allowed := map[string]string{
		"title": "title", "prompt": "prompt", "description": "description",
		"type": "type", "priority": "priority", "points": "points",
		"due_at": "due_at", "progress": "progress", "branch_name": "branch_name",
		"worktree_path": "worktree_path", "agent_id": "agent_id", "model_override": "model_override",
		"labels": "labels",
	}
	where, scopeArgs := whereScoped(ws)
	if len(scopeArgs) == 0 {
		return contracts.Task{}, ErrTaskNotFound
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
	row := s.pool.QueryRow(ctx, q, args...)
	t, err := scanTask(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Task{}, ErrTaskNotFound
	}
	return t, err
}

// SetStatus updates a task's status and returns the updated task. The saga
// (phase 6) wraps this to publish commands when transitioning to doing.
func (s *Store) SetStatus(ctx context.Context, id contracts.ID, status contracts.TaskStatus) (contracts.Task, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE tasks SET status = $2, updated_at = now() WHERE id = $1
		RETURNING `+taskCols, id, status)
	t, err := scanTask(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Task{}, ErrTaskNotFound
	}
	return t, err
}

// SetRoundNo advances the review round counter (saga phase 6).
func (s *Store) SetRoundNo(ctx context.Context, id contracts.ID, roundNo int) error {
	_, err := s.pool.Exec(ctx, `UPDATE tasks SET round_no = $2, updated_at = now() WHERE id = $1`, id, roundNo)
	return err
}

// CountOpenByWorkspace counts tasks in open states (doing/review) per workspace
// for the Gateway's workspace-stats composition.
func (s *Store) CountOpenByWorkspace(ctx context.Context, workspaceID contracts.ID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM tasks WHERE workspace_id = $1 AND status IN ('doing', 'review')`,
		workspaceID).Scan(&n)
	return n, err
}

// SagaNew records (task_id, run_id) as processed; false when already seen.
// This is the saga's idempotency hook for at-least-once Kafka redelivery.
func (s *Store) SagaNew(ctx context.Context, taskID, runID contracts.ID) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO saga_runs (task_id, run_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, taskID, runID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) Delete(ctx context.Context, id contracts.ID, ws []contracts.ID) error {
	where, args := whereScopedAt(2, ws) // $1=id, $2=ws ids
	if len(args) == 0 {
		return ErrTaskNotFound
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM tasks WHERE id = $1`+where, append([]any{id}, args...)...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// ── Feedback ────────────────────────────────────────────────────────────────

func (s *Store) ListFeedback(ctx context.Context, taskID contracts.ID, ws []contracts.ID) ([]contracts.Feedback, error) {
	where, args := whereScopedAt(2, ws) // $1=taskID, $2=ws ids
	if len(args) == 0 {
		return []contracts.Feedback{}, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT id, task_id, author, body, created_at FROM feedback WHERE task_id = $1`+where+` ORDER BY created_at ASC`, append([]any{taskID}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.Feedback{}
	for rows.Next() {
		var f contracts.Feedback
		if err := rows.Scan(&f.ID, &f.TaskID, &f.Author, &f.Body, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) AddFeedback(ctx context.Context, taskID contracts.ID, ws []contracts.ID, body string) (contracts.Feedback, error) {
	var f contracts.Feedback
	scopeClause, scopeArgs := whereScopedAt(3, ws) // $1=taskID, $2=body, $3=ws ids
	if len(scopeArgs) == 0 {
		return contracts.Feedback{}, ErrTaskNotFound
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO feedback (task_id, author, body)
		SELECT $1, 'user', $2 FROM tasks WHERE id = $1`+scopeClause+`
		RETURNING id, task_id, author, body, created_at`,
		taskID, body, scopeArgs[0]).Scan(&f.ID, &f.TaskID, &f.Author, &f.Body, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Feedback{}, ErrTaskNotFound
	}
	return f, err
}
