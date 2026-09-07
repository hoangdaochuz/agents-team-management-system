package application

import (
	"context"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/tasks"
	"github.com/aaks/server/services/workspace/internal/domain/task"
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
	switch status {
	case tasks.TaskDoing:
		// Skip the run request when the task has no assigned agent (task 6.8):
		// surface the un-runnable task as blocked instead. Return the blocked
		// task, not the stale doing DTO from the SetStatus above.
		if out.AgentID == nil || *out.AgentID == "" {
			out, err = a.repo.Tasks.SetStatus(ctx, id, tasks.TaskBlocked)
			if err != nil {
				return tasks.Task{}, err
			}
			return out, nil
		}
		a.pub.Publish(ctx, events.TopicTaskRunRequested, events.RunRequestedData{
			TaskID:        out.ID,
			AgentID:       *out.AgentID,
			ProjectID:     out.ProjectID,
			WorkspaceID:   out.WorkspaceID,
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
	// Status + round commit in ONE statement: a crash between two writes must
	// not bump the round without returning the task to doing.
	out, err := a.repo.Tasks.SetStatusAndRound(ctx, id, tasks.TaskDoing, next)
	if err != nil {
		return tasks.Task{}, err
	}
	a.pub.Publish(ctx, events.TopicTaskRunRequested, events.RunRequestedData{
		TaskID: id, AgentID: *t.AgentID, ProjectID: t.ProjectID,
		WorkspaceID: t.WorkspaceID, RoundNo: next, Prompt: t.Prompt, ModelOverride: derefStr(t.ModelOverride),
	}, id)
	_ = ws
	return out, nil
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
	// Idempotent: an already-stopped task still asks the Executor to abort,
	// but no transition happened, so no event beyond stop-requested is emitted.
	_ = prev
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
func (a *App) Dispatch(ctx context.Context, msg events.EventEnvelope) error {
	switch msg.EventType {
	case events.TopicRunCompleted:
		var d events.RunCompletedData
		if err := msg.DecodeData(&d); err != nil {
			return err
		}
		if d.Role != agentexec.RunRoleImplementer {
			return nil // reviewer runs do not advance the task
		}
		if d.Status != agentexec.RunDone {
			// A failed/stopped/aborted implementer run leaves nothing in
			// flight; surface the task as blocked (the system's
			// "needs attention" signal) instead of stranding it in doing.
			return a.onImplementerFailed(ctx, d)
		}
		return a.onImplementerDone(ctx, d)
	case events.TopicVerdict:
		var d events.VerdictData
		if err := msg.DecodeData(&d); err != nil {
			return err
		}
		return a.onVerdict(ctx, d)
	case events.TopicPrOpened:
		var d events.PrOpenedData
		if err := msg.DecodeData(&d); err != nil {
			return err
		}
		a.log.Info("pr opened for task", "task_id", d.TaskID, "run_id", d.RunID, "url", d.URL)
	}
	return nil
}

// onImplementerFailed moves the task to blocked when an implementer run ended
// without success (wall-clock/step cap, provider failure, stop). Idempotent
// via the same SagaAdvance dedup as the happy path; blocked is terminal
// enough for a human to re-run consciously.
func (a *App) onImplementerFailed(ctx context.Context, d events.RunCompletedData) error {
	ok, err := a.repo.Tasks.SagaAdvance(ctx, d.TaskID, d.RunID, tasks.TaskDoing, tasks.TaskBlocked, 0, false)
	if err != nil {
		return err
	}
	if ok {
		a.log.Warn("implementer run failed; task blocked", "task_id", d.TaskID, "run_id", d.RunID, "status", d.Status)
	}
	return nil
}

// onImplementerDone moves the task to review and requests a reviewer run.
// The task is read BEFORE the atomic advance so a read failure retries on
// redelivery (the mark is not yet made); the publish itself is necessarily
// post-commit, the same publish-after-commit pattern PatchStatus uses.
func (a *App) onImplementerDone(ctx context.Context, d events.RunCompletedData) error {
	t, err := a.repo.Tasks.GetUnscoped(ctx, d.TaskID)
	if err != nil {
		return err
	}
	ok, err := a.repo.Tasks.SagaAdvance(ctx, d.TaskID, d.RunID, tasks.TaskDoing, tasks.TaskReview, 0, false)
	if err != nil {
		return err
	}
	if !ok {
		// Already processed (the mark exists) or the task was not in doing.
		// The mark existing means the advance committed — but the post-commit
		// publish may have been lost (crash between commit and publish). If
		// the task sits at the transition's target, re-emit the command so the
		// saga cannot strand; a duplicate is absorbed by the Executor's
		// per-round run dedup.
		return a.republishIfAt(ctx, d.TaskID, tasks.TaskReview, func(t tasks.Task) {
			if t.AgentID == nil {
				return
			}
			a.pub.Publish(ctx, events.TopicTaskReviewRequested, events.ReviewRequestedData{
				TaskID: d.TaskID, AgentID: *t.AgentID, RunID: d.RunID, WorkspaceID: t.WorkspaceID,
				RoundNo: d.RoundNo, Prompt: t.Prompt,
			}, d.TaskID)
		})
	}
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

// republishIfAt re-reads the task and invokes emit when its status matches
// target — the redelivery recovery leg for a lost post-commit publish. When
// the status has moved on, the command's moment has passed and it is skipped.
func (a *App) republishIfAt(ctx context.Context, taskID identity.ID, target tasks.TaskStatus, emit func(tasks.Task)) error {
	t, err := a.repo.Tasks.GetUnscoped(ctx, taskID)
	if err != nil {
		return err
	}
	if t.Status == target {
		emit(t)
	}
	return nil
}

// onVerdict advances the task on the reviewer's decision. The dedup mark and
// the status transition (including the round bump) commit atomically via
// SagaAdvance, so a redelivered verdict can neither double-advance the task
// nor — on a partially failed first delivery — leave it stranded in review.
func (a *App) onVerdict(ctx context.Context, d events.VerdictData) error {
	t, err := a.repo.Tasks.GetUnscoped(ctx, d.TaskID)
	if err != nil {
		return err
	}
	var to tasks.TaskStatus
	var next int
	var setRound bool
	switch d.Decision {
	case agentexec.VerdictApprove:
		to = tasks.TaskDone
	case agentexec.VerdictRequestChanges:
		next = d.RoundNo + 1
		if next > maxReviewRounds {
			to = tasks.TaskBlocked
		} else {
			to = tasks.TaskDoing
			setRound = true
		}
	}
	ok, err := a.repo.Tasks.SagaAdvance(ctx, d.TaskID, d.RunID, tasks.TaskReview, to, next, setRound)
	if err != nil {
		return err
	}
	if !ok {
		// Already processed (the mark exists) or the task was not in review.
		// A committed advance with a lost post-commit publish is recovered on
		// redelivery: if the task sits at the transition's target, re-emit
		// the round's run command; a duplicate is absorbed by the Executor's
		// per-round run dedup.
		if d.Decision == agentexec.VerdictRequestChanges {
			return a.republishIfAt(ctx, d.TaskID, tasks.TaskDoing, func(t tasks.Task) {
				// The committed advance already bumped the round (setRound),
				// so the re-read task carries the round this command wants.
				a.pub.Publish(ctx, events.TopicTaskRunRequested, events.RunRequestedData{
					TaskID: d.TaskID, AgentID: derefAgentID(t.AgentID), ProjectID: t.ProjectID,
					WorkspaceID: t.WorkspaceID, RoundNo: t.RoundNo, Prompt: t.Prompt, ModelOverride: derefStr(t.ModelOverride),
				}, d.TaskID)
			})
		}
		return nil
	}
	if d.Decision == agentexec.VerdictRequestChanges && to == tasks.TaskBlocked {
		a.log.Warn("review rounds exhausted; task blocked", "task_id", d.TaskID, "rounds", next)
		return nil
	}
	if d.Decision == agentexec.VerdictRequestChanges {
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
