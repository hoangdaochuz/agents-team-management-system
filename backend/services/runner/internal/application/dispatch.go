package application

import (
	"context"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
)

// Dispatch handles one runner command envelope. Runs execute asynchronously
// (a stop command must be consumable while a run is in flight); a per-task
// in-flight guard prevents overlapping runs, and stop cancels via the task
// context.
func (r *Runner) Dispatch(ctx context.Context, env events.EventEnvelope) error {
	switch env.EventType {
	case events.TopicTaskRunRequested:
		var d events.RunRequestedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		r.startRun(ctx, string(d.TaskID), func(rctx context.Context) { r.StartImplementer(rctx, d) })
	case events.TopicTaskReviewRequested:
		var d events.ReviewRequestedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		r.startRun(ctx, string(d.TaskID), func(rctx context.Context) { r.StartReviewer(rctx, d) })
	case events.TopicTaskStopRequested:
		var d events.StopRequestedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		r.CancelTask(string(d.TaskID))
	case events.TopicPrOpenRequested:
		var d events.PrOpenRequestedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		return r.OpenPr(ctx, d)
	}
	return nil
}

// ListRuns returns a task's runs, newest first.
func (r *Runner) ListRuns(ctx context.Context, taskID identity.ID) ([]agentexec.Run, error) {
	runs, err := r.runs.ListRunsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if runs == nil {
		runs = []agentexec.Run{}
	}
	return runs, nil
}

// ListArtifacts returns a task's artifacts, newest first.
func (r *Runner) ListArtifacts(ctx context.Context, taskID identity.ID) ([]agentexec.Artifact, error) {
	arts, err := r.artifacts.ListArtifactsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if arts == nil {
		arts = []agentexec.Artifact{}
	}
	return arts, nil
}

// ListSteps returns a run's steps in sequence order.
func (r *Runner) ListSteps(ctx context.Context, runID identity.ID) ([]agentexec.Step, error) {
	steps, err := r.steps.ListSteps(ctx, runID)
	if err != nil {
		return nil, err
	}
	if steps == nil {
		steps = []agentexec.Step{}
	}
	return steps, nil
}

// ListFindings returns a run's findings.
func (r *Runner) ListFindings(ctx context.Context, runID identity.ID) ([]agentexec.Finding, error) {
	findings, err := r.findings.ListFindings(ctx, runID)
	if err != nil {
		return nil, err
	}
	if findings == nil {
		findings = []agentexec.Finding{}
	}
	return findings, nil
}

// ListTaskSteps returns all steps for a task's runs, in run/seq order (SSE
// replay).
func (r *Runner) ListTaskSteps(ctx context.Context, taskID identity.ID) ([]agentexec.Step, error) {
	steps, err := r.steps.ListStepsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if steps == nil {
		steps = []agentexec.Step{}
	}
	return steps, nil
}