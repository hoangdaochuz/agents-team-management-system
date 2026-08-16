package application

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/tasks"
	"github.com/aaks/server/services/task/internal/domain"
)

// List returns the tasks matching q (workspace-scoped; an empty workspace set
// yields no rows — fail closed), most recently updated first.
func (a *App) List(ctx context.Context, q domain.Query) ([]tasks.Task, error) {
	out, err := a.repo.Tasks.List(ctx, q)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []tasks.Task{}
	}
	return out, nil
}

// Get returns one task, scoped to the caller's workspace set.
func (a *App) Get(ctx context.Context, id identity.ID, ws []identity.ID) (tasks.Task, error) {
	return a.repo.Tasks.Get(ctx, id, ws)
}

// Create registers a task in backlog for the given workspace.
func (a *App) Create(ctx context.Context, workspaceID identity.ID, in domain.CreateInput) (tasks.Task, error) {
	return a.repo.Tasks.Create(ctx, workspaceID, in)
}

// Update applies a partial update (frontend updateTask sends Partial<Task>).
func (a *App) Update(ctx context.Context, id identity.ID, ws []identity.ID, fields map[string]any) (tasks.Task, error) {
	return a.repo.Tasks.Update(ctx, id, ws, fields)
}

// Delete removes a task, scoped to the caller's workspace set.
func (a *App) Delete(ctx context.Context, id identity.ID, ws []identity.ID) error {
	return a.repo.Tasks.Delete(ctx, id, ws)
}

// ListFeedback returns a task's human comments, oldest first.
func (a *App) ListFeedback(ctx context.Context, taskID identity.ID, ws []identity.ID) ([]tasks.Feedback, error) {
	out, err := a.repo.Feedback.List(ctx, taskID, ws)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []tasks.Feedback{}
	}
	return out, nil
}

// AddFeedback appends a human comment to a task.
func (a *App) AddFeedback(ctx context.Context, taskID identity.ID, ws []identity.ID, body string) (tasks.Feedback, error) {
	return a.repo.Feedback.Add(ctx, taskID, ws, body)
}

// OpenTaskCount counts tasks in open states (doing/review) per workspace for
// the Gateway's workspace-stats composition.
func (a *App) OpenTaskCount(ctx context.Context, workspaceID identity.ID) (int, error) {
	return a.repo.Tasks.CountOpenByWorkspace(ctx, workspaceID)
}

// TaskWorkspace returns the owning workspace of a task so the Gateway can gate
// task sub-routes (runs/artifacts) against the session's workspace union.
func (a *App) TaskWorkspace(ctx context.Context, id identity.ID) (identity.ID, error) {
	t, err := a.repo.Tasks.GetUnscoped(ctx, id)
	if err != nil {
		return "", err
	}
	return t.WorkspaceID, nil
}
