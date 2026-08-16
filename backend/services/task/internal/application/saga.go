package application

import (
	"context"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/tasks"
	"github.com/aaks/server/services/task/internal/domain"
)

// maxReviewRounds caps the review loop (task 6.3): beyond it a REQUEST_CHANGES
// verdict blocks the task instead of starting another implementer round.
const maxReviewRounds = 5

// PatchStatus applies a status change and emits the saga events for the
// transition. "doing" requests an implementer run; "stopped"/"cancelled"
// request an abort; any real change publishes the status-changed fact.
// Commands are published strictly AFTER the mutations commit, so a failed
// transition never emits events.
func (a *App) PatchStatus(ctx context.Context, id identity.ID, ws []identity.ID, status tasks.TaskStatus) (tasks.Task, error) {
	prev, err := a.repo.Tasks.Get(ctx, id, ws)
	if err != nil {
		return tasks.Task{}, err
	}
	out, err := a.repo.Tasks.SetStatus(ctx, id, status)
	if err != nil {
		return tasks.Task{}, err
	}
	// Idempotent PATCH: a no-op transition must not re-emit events.
	if prev.Status == status {
		return out, nil
	}
	a.pub.Publish(ctx, events.TopicTaskStatusChanged, events.TaskStatusChangedData{
		TaskID: out.ID, From: prev.Status, To: status, RoundNo: out.RoundNo,
	}, out.ID)
	switch status {
	case tasks.TaskDoing:
		// Skip the run request when the task has no assigned agent (task 6.8):
		// surface the un-runnable task as blocked instead.
		if out.AgentID == nil || *out.AgentID == "" {
			if _, err := a.repo.Tasks.SetStatus(ctx, id, tasks.TaskBlocked); err != nil {
				return tasks.Task{}, err
			}
			a.pub.Publish(ctx, events.TopicTaskStatusChanged, events.TaskStatusChangedData{
				TaskID: id, From: status, To: tasks.TaskBlocked, RoundNo: out.RoundNo,
			}, id)
			return out, nil
		}
		a.pub.Publish(ctx, events.TopicTaskRunRequested, events.RunRequestedData{
			TaskID:        out.ID,
			AgentID:       *out.AgentID,
			ProjectID:     out.ProjectID,
			RoundNo:       out.RoundNo,
			Prompt:        out.Prompt,
			ModelOverride: derefStr(out.ModelOverride),
		}, out.ID)
	case tasks.TaskStopped, tasks.TaskCancelled:
		a.pub.Publish(ctx, events.TopicTaskStopRequested, events.StopRequestedData{TaskID: out.ID}, out.ID)
	}
	return out, nil
}

// ReRun requests a fresh implementer run (saga action; task 6.4): the round
// counter advances and the task returns to doing.
func (a *App) ReRun(ctx context.Context, id identity.ID, ws []identity.ID) (tasks.Task, error) {
	t, err := a.repo.Tasks.Get(ctx, id, ws)
	if err != nil {
		return tasks.Task{}, err
	}
	if t.AgentID == nil || *t.AgentID == "" {
		return tasks.Task{}, domain.ErrNoAgent
	}
	next := t.RoundNo + 1
	if err := a.repo.Tasks.SetRoundNo(ctx, id, next); err != nil {
		return tasks.Task{}, err
	}
	if _, err := a.repo.Tasks.SetStatus(ctx, id, tasks.TaskDoing); err != nil {
		return tasks.Task{}, err
	}
	a.pub.Publish(ctx, events.TopicTaskStatusChanged, events.TaskStatusChangedData{
		TaskID: id, From: t.Status, To: tasks.TaskDoing, RoundNo: next,
	}, id)
	a.pub.Publish(ctx, events.TopicTaskRunRequested, events.RunRequestedData{
		TaskID: id, AgentID: *t.AgentID, ProjectID: t.ProjectID,
		WorkspaceID: t.WorkspaceID, RoundNo: next, Prompt: t.Prompt, ModelOverride: derefStr(t.ModelOverride),
	}, id)
	return a.repo.Tasks.Get(ctx, id, ws)
}

// Stop sets the task stopped synchronously and requests an abort (task 6.4).
func (a *App) Stop(ctx context.Context, id identity.ID, ws []identity.ID) (tasks.Task, error) {
	prev, err := a.repo.Tasks.Get(ctx, id, ws)
	if err != nil {
		return tasks.Task{}, err
	}
	out, err := a.repo.Tasks.SetStatus(ctx, id, tasks.TaskStopped)
	if err != nil {
		return tasks.Task{}, err
	}
	if prev.Status != out.Status {
		a.pub.Publish(ctx, events.TopicTaskStatusChanged, events.TaskStatusChangedData{
			TaskID: out.ID, From: prev.Status, To: out.Status, RoundNo: out.RoundNo,
		}, out.ID)
	}
	a.pub.Publish(ctx, events.TopicTaskStopRequested, events.StopRequestedData{TaskID: id}, id)
	return out, nil
}

// OpenPr requests PR creation from the Runner (task 6.5); the PR is never
// auto-created anywhere else.
func (a *App) OpenPr(ctx context.Context, id identity.ID, ws []identity.ID) error {
	t, err := a.repo.Tasks.Get(ctx, id, ws)
	if err != nil {
		return err
	}
	if t.Status != tasks.TaskDone {
		return domain.ErrNotDone
	}
	a.pub.Publish(ctx, events.TopicPrOpenRequested, events.PrOpenRequestedData{TaskID: id}, id)
	return nil
}

// Dispatch handles one saga fact envelope and advances the task state machine
// (tasks 6.2–6.6). The saga coordinator is the Task service's consumer entry:
//
//	run.completed (implementer, done) → review + task.review-requested
//	verdict APPROVE              → done
//	verdict REQUEST_CHANGES      → round < 5 → doing + task.run-requested(round+1)
//	                              → round ≥ 5 → blocked (review rounds exhausted)
//	pr.opened                    → logged (PR info is surfaced via the Runner)
func (a *App) Dispatch(ctx context.Context, env events.EventEnvelope) error {
	switch env.EventType {
	case events.TopicRunCompleted:
		var d events.RunCompletedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		if d.Role != agentexec.RunRoleImplementer || d.Status != agentexec.RunDone {
			return nil // reviewer/aborted runs do not advance the task
		}
		return a.onImplementerDone(ctx, d)
	case events.TopicVerdict:
		var d events.VerdictData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		return a.onVerdict(ctx, d)
	case events.TopicPrOpened:
		var d events.PrOpenedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		a.log.Info("pr opened for task", "task_id", d.TaskID, "run_id", d.RunID, "url", d.URL)
	}
	return nil
}

// onImplementerDone moves the task to review and requests a reviewer run.
func (a *App) onImplementerDone(ctx context.Context, d events.RunCompletedData) error {
	ok, err := a.repo.Tasks.SagaNew(ctx, d.TaskID, d.RunID)
	if err != nil {
		return err
	}
	if !ok {
		return nil // already processed (redelivery)
	}
	t, err := a.repo.Tasks.GetUnscoped(ctx, d.TaskID)
	if err != nil {
		return err
	}
	if t.Status != tasks.TaskDoing {
		a.log.Info("implementer done ignored: task not in doing", "task_id", d.TaskID, "status", t.Status)
		return nil
	}
	if _, err := a.repo.Tasks.SetStatus(ctx, d.TaskID, tasks.TaskReview); err != nil {
		return err
	}
	a.pub.Publish(ctx, events.TopicTaskStatusChanged, events.TaskStatusChangedData{
		TaskID: d.TaskID, From: tasks.TaskDoing, To: tasks.TaskReview, RoundNo: d.RoundNo,
	}, d.TaskID)
	if t.AgentID == nil {
		a.log.Warn("no agent to review; task stuck in review", "task_id", d.TaskID)
		return nil
	}
	a.pub.Publish(ctx, events.TopicTaskReviewRequested, events.ReviewRequestedData{
		TaskID: d.TaskID, AgentID: *t.AgentID, RunID: d.RunID, WorkspaceID: t.WorkspaceID,
		RoundNo: d.RoundNo, Prompt: t.Prompt,
	}, d.TaskID)
	return nil
}

// onVerdict advances the task on the reviewer's decision.
func (a *App) onVerdict(ctx context.Context, d events.VerdictData) error {
	ok, err := a.repo.Tasks.SagaNew(ctx, d.TaskID, d.RunID)
	if err != nil {
		return err
	}
	if !ok {
		return nil // already processed (redelivery)
	}
	t, err := a.repo.Tasks.GetUnscoped(ctx, d.TaskID)
	if err != nil {
		return err
	}
	if t.Status != tasks.TaskReview {
		a.log.Info("verdict ignored: task not in review", "task_id", d.TaskID, "status", t.Status)
		return nil
	}
	switch d.Decision {
	case agentexec.VerdictApprove:
		if _, err := a.repo.Tasks.SetStatus(ctx, d.TaskID, tasks.TaskDone); err != nil {
			return err
		}
		a.pub.Publish(ctx, events.TopicTaskStatusChanged, events.TaskStatusChangedData{
			TaskID: d.TaskID, From: tasks.TaskReview, To: tasks.TaskDone, RoundNo: d.RoundNo,
		}, d.TaskID)
	case agentexec.VerdictRequestChanges:
		next := d.RoundNo + 1
		if next > maxReviewRounds {
			if _, err := a.repo.Tasks.SetStatus(ctx, d.TaskID, tasks.TaskBlocked); err != nil {
				return err
			}
			a.log.Warn("review rounds exhausted; task blocked", "task_id", d.TaskID, "rounds", next)
			a.pub.Publish(ctx, events.TopicTaskStatusChanged, events.TaskStatusChangedData{
				TaskID: d.TaskID, From: tasks.TaskReview, To: tasks.TaskBlocked, RoundNo: d.RoundNo,
			}, d.TaskID)
			return nil
		}
		if _, err := a.repo.Tasks.SetStatus(ctx, d.TaskID, tasks.TaskDoing); err != nil {
			return err
		}
		if err := a.repo.Tasks.SetRoundNo(ctx, d.TaskID, next); err != nil {
			return err
		}
		a.pub.Publish(ctx, events.TopicTaskStatusChanged, events.TaskStatusChangedData{
			TaskID: d.TaskID, From: tasks.TaskReview, To: tasks.TaskDoing, RoundNo: next,
		}, d.TaskID)
		a.pub.Publish(ctx, events.TopicTaskRunRequested, events.RunRequestedData{
			TaskID: d.TaskID, AgentID: derefAgentID(t.AgentID), ProjectID: t.ProjectID,
			WorkspaceID: t.WorkspaceID, RoundNo: next, Prompt: t.Prompt, ModelOverride: derefStr(t.ModelOverride),
		}, d.TaskID)
	}
	return nil
}

// derefAgentID flattens the agent pointer; the callers already validated it.
func derefAgentID(p *identity.ID) identity.ID {
	if p != nil {
		return *p
	}
	return ""
}

// derefStr flattens the model-override pointer.
func derefStr(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}