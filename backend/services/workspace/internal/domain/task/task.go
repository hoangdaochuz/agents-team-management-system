package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/tasks"
)

// Task is the task aggregate (wire DTO as domain type, D7).
type Task = tasks.Task

// Feedback is the human-comment aggregate (wire DTO as domain type, D7).
type Feedback = tasks.Feedback

// Query mirrors frontend TaskQuery (the filters the SPA sends).
type Query struct {
	Workspaces []identity.ID    // workspace context (empty = no results, fail closed)
	ProjectID  identity.ID      // project_id filter
	Status     tasks.TaskStatus // status filter
	Type       tasks.TaskType   // type filter
	Priority   tasks.Priority   // priority filter
	Assignee   identity.ID      // agent_id filter
	Label      string           // labels filter
	Q          string           // title ILIKE filter
}

// CreateInput is the body of POST /tasks (matches frontend createTask).
type CreateInput struct {
	ProjectID   identity.ID    `json:"project_id"`
	Title       string         `json:"title"`
	Prompt      string         `json:"prompt"`
	Description string         `json:"description,omitempty"`
	Type        tasks.TaskType `json:"type,omitempty"`
	Priority    tasks.Priority `json:"priority,omitempty"`
	Labels      []string       `json:"labels,omitempty"`
	Points      *int           `json:"points,omitempty"`
	AgentID     *identity.ID   `json:"agent_id,omitempty"`
	DueAt       *string        `json:"due_at,omitempty"`
}

// TaskRepository is the task aggregate port. Reads are scoped to the caller's
// workspace set (mutations reject rows outside it); GetUnscoped is the saga
// consumer's internal read, which carries its own task context.
type TaskRepository interface {
	List(ctx context.Context, q Query) ([]Task, error)
	Get(ctx context.Context, id identity.ID, ws []identity.ID) (Task, error)
	GetUnscoped(ctx context.Context, id identity.ID) (Task, error)
	Create(ctx context.Context, workspaceID identity.ID, in CreateInput) (Task, error)
	Update(ctx context.Context, id identity.ID, ws []identity.ID, fields map[string]any) (Task, error)
	// SetStatus transitions a task's status. The WHERE status <> $2 guard makes
	// a no-op PATCH atomic against concurrent PATCHes: two racing callers both
	// see "changed", but only one UPDATE matches rows and emits events.
	SetStatus(ctx context.Context, id identity.ID, status tasks.TaskStatus) (Task, error)
	// SetStatusAndRound transitions status and round in ONE statement (ReRun's
	// two-write mutation — a crash between two statements must not bump the
	// round without the transition).
	SetStatusAndRound(ctx context.Context, id identity.ID, status tasks.TaskStatus, roundNo int) (Task, error)
	Delete(ctx context.Context, id identity.ID, ws []identity.ID) error
	CountOpenByWorkspace(ctx context.Context, workspaceID identity.ID) (int, error)
	// SagaAdvance atomically records (task_id, run_id) as processed by the
	// saga coordinator AND moves the task from `from` to `to` (optionally
	// setting the review round) in a single statement. It returns false when
	// the run was already seen or the task is not in `from` — both are
	// idempotent no-ops under at-least-once redelivery. Keeping the dedup mark
	// and the transition in one statement is what makes the idempotency hook
	// safe: a failure after the mark can never strand the task un-transitioned.
	SagaAdvance(ctx context.Context, taskID, runID identity.ID, from, to tasks.TaskStatus, roundNo int, setRound bool) (bool, error)
}

// FeedbackRepository is the human-comment aggregate port.
type FeedbackRepository interface {
	List(ctx context.Context, taskID identity.ID, ws []identity.ID) ([]Feedback, error)
	Add(ctx context.Context, taskID identity.ID, ws []identity.ID, body string) (Feedback, error)
}
