package application

import (
	"context"
	"errors"
	"time"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/runner/internal/driver"
)

// StartImplementer runs the implementer agent over a task, persists the
// result and emits run.completed (+ finding.* / artifact facts).
func (r *Runner) StartImplementer(ctx context.Context, d events.RunRequestedData) {
	if d.AgentID == "" || d.TaskID == "" {
		r.log.Warn("ignoring run request without agent or task", "event", d)
		return
	}
	model := d.ModelOverride
	if model == "" {
		model = "default"
	}
	runID, err := r.runs.CreateRun(ctx, d.TaskID, agentexec.RunRoleImplementer, d.AgentID, model, d.RoundNo)
	if err != nil {
		r.log.Error("create implementer run failed", "error", err)
		return
	}
	r.publish(ctx, events.TopicRunStarted, map[string]any{"run_id": runID, "task_id": d.TaskID}, d.TaskID)

	rc := r.runContext(ctx, d.TaskID, runID, d.AgentID, agentexec.RunRoleImplementer, d.RoundNo, model, d.Prompt, d.WorkspaceID)
	tools, cleanup := r.setupToolsForRun(ctx, d.TaskID, d.Prompt, d.AgentID, d.WorkspaceID)
	if cleanup != nil {
		defer cleanup()
	}
	rc.Tools = tools
	r.executeAndFinish(ctx, d.TaskID, runID, rc)
}

// StartReviewer runs the reviewer agent over the implementer run and emits the
// verdict fact.
func (r *Runner) StartReviewer(ctx context.Context, d events.ReviewRequestedData) {
	if d.AgentID == "" || d.RunID == "" {
		r.log.Warn("ignoring review request without agent or run", "event", d)
		return
	}
	imp, err := r.runs.GetRun(ctx, d.RunID)
	if err != nil {
		r.log.Warn("review run skipped: implementer run not found", "run", d.RunID, "error", err)
		return
	}
	runID, err := r.runs.CreateRun(ctx, d.TaskID, agentexec.RunRoleReviewer, d.AgentID, imp.Model, d.RoundNo)
	if err != nil {
		r.log.Error("create reviewer run failed", "error", err)
		return
	}
	r.publish(ctx, events.TopicRunStarted, map[string]any{"run_id": runID, "task_id": d.TaskID}, d.TaskID)

	rc := r.runContext(ctx, d.TaskID, runID, d.AgentID, agentexec.RunRoleReviewer, d.RoundNo, imp.Model, d.Prompt, d.WorkspaceID)
	tools, cleanup := r.setupToolsForRun(ctx, d.TaskID, d.Prompt, d.AgentID, d.WorkspaceID)
	if cleanup != nil {
		defer cleanup()
	}
	rc.Tools = tools
	res := r.executeAndFinish(ctx, d.TaskID, runID, rc)
	if res.Verdict != "" {
		r.publish(ctx, events.TopicVerdict, events.VerdictData{
			TaskID: d.TaskID, RunID: runID, RoundNo: d.RoundNo,
			Decision: res.Verdict, Summary: res.VerdictSummary,
		}, d.TaskID)
	}
}

// runContext assembles the driver context: provider key from Settings
// (in-memory only) and the workspace's enabled rules from Resources. Both are
// non-fatal (a run proceeds without keys/rules).
func (r *Runner) runContext(ctx context.Context, taskID, runID, agentID identity.ID, role agentexec.RunRole, roundNo int, model, prompt string, workspaceID identity.ID) driver.RunContext {
	rc := driver.RunContext{
		TaskID: taskID, RunID: runID, AgentID: agentID, Role: role, RoundNo: roundNo,
		Prompt: prompt, Model: model, Provider: "openai", Caps: r.caps, Log: r.log,
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if key, err := r.settings.FetchKey(ctx, string(rc.Provider)); err != nil {
		if !errors.Is(err, ErrNotConfigured) {
			r.log.Warn("no provider key from settings", "error", err)
		}
	} else {
		rc.APIKey = key
	}
	if workspaceID != "" {
		if rules, err := r.resources.FetchEnabledRules(ctx, workspaceID); err != nil {
			if !errors.Is(err, ErrNotConfigured) {
				r.log.Warn("no rules from resources", "workspace", workspaceID, "error", err)
			}
		} else {
			rc.Rules = rules
		}
	}
	return rc
}

// setupToolsForRun fetches the agent's attached MCP servers (best-effort) and
// builds the per-run tool set (sandbox + MCP bridge). Failures degrade to
// stubbed tools.
func (r *Runner) setupToolsForRun(ctx context.Context, taskID, agentID, workspaceID identity.ID, prompt string) (driver.ToolSet, func()) {
	return r.tools.SetupTools(ctx, taskID, agentID, workspaceID, prompt)
}

// executeAndFinish runs the driver, persists steps/artifacts/findings, and
// emits the completion facts. Returns the result for the caller (verdict
// emission). A stop-cancelled context must not prevent the final persist +
// run.completed: the saga would never learn the run ended.
func (r *Runner) executeAndFinish(ctx context.Context, taskID, runID identity.ID, rc driver.RunContext) driver.Result {
	sink := func(st agentexec.Step) error {
		if _, err := r.steps.AppendStep(ctx, runID, st.Seq, st.Kind, st.Payload); err != nil {
			return err
		}
		r.publish(ctx, events.TopicStep, events.StepData{Step: st}, taskID)
		return nil
	}
	res, err := r.driver.Execute(ctx, rc, sink)
	if err != nil {
		res.Status = agentexec.RunAborted
		res.Error = err.Error()
	}
	finishCtx := context.WithoutCancel(ctx)
	for _, f := range res.Findings {
		f.RunID = runID
		if _, err := r.findings.AddFinding(finishCtx, runID, f); err != nil {
			r.log.Warn("finding persist failed", "error", err)
		}
		fd := events.FindingData{Finding: f}
		fd.Finding.RunID = runID
		r.publish(finishCtx, events.TopicFinding, fd, taskID)
	}
	for _, ar := range res.Artifacts {
		run := runID
		if _, err := r.artifacts.AddArtifact(finishCtx, taskID, &run, ar.Filename, ar.Kind, ar.Summary, ar.Additions, ar.Deletions); err != nil {
			r.log.Warn("artifact persist failed", "error", err)
		}
	}
	if err := r.runs.FinishRun(finishCtx, runID, res.Status, res.TokenUsage, res.Error); err != nil {
		r.log.Error("finish run failed", "run", runID, "error", err)
	}
	r.publish(finishCtx, events.TopicRunCompleted, events.RunCompletedData{
		TaskID: taskID, RunID: runID, AgentID: rc.AgentID, Role: rc.Role,
		Status: res.Status, RoundNo: rc.RoundNo, TokenUsage: res.TokenUsage, Error: res.Error,
	}, taskID)
	return res
}

// OpenPr emits pr.opened for the task's latest run (dev PR URL).
func (r *Runner) OpenPr(ctx context.Context, d events.PrOpenRequestedData) error {
	run, err := r.runs.LatestRun(ctx, d.TaskID)
	if err != nil {
		r.log.Warn("open-pr skipped: no run for task", "task", d.TaskID, "error", err)
		return nil
	}
	base := r.prBaseURL
	if base == "" {
		base = "https://github.com/example/repo"
	}
	r.publish(ctx, events.TopicPrOpened, events.PrOpenedData{
		TaskID: d.TaskID, RunID: run.ID, URL: base + "/pull/" + shortID(d.TaskID),
	}, d.TaskID)
	r.log.Info("pr opened (dev)", "task", d.TaskID, "run", run.ID)
	return nil
}

func shortID(id identity.ID) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
