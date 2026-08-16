// Package httpapi registers the Task service routes: Task CRUD + feedback +
// status patch (matching frontend/src/api/tasks.ts + feedback.ts).
//
// The task-lifecycle saga is now live: PATCHing a task's status emits the
// corresponding command on Kafka — transitioning to "doing" publishes
// task.run-requested (so a run can begin once the runner exists), to
// "stopped"/"cancelled" publishes task.stop-requested, and every authoritative
// change publishes the task.status-changed fact. The producer is best-effort
// (no-op when KAFKA_BROKERS is unset), so the service still runs without Kafka.
// re-run/stop/open-pr remain 501 until the runner consumer lands (phase 6).
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/IBM/sarama"

	"github.com/aaks/server/internal/contracts"
	"github.com/aaks/server/internal/platform/http"
	"github.com/aaks/server/internal/platform/tenancy"
	"github.com/aaks/server/internal/platform/kafka"
	"github.com/aaks/server/services/task/internal/store"
)

type App struct {
	store *store.Store
	prod  sarama.SyncProducer // nilable: saga is a no-op when Kafka is unavailable
	log   *slog.Logger
}

func Register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("TASK_DB_DSN")
	if dsn == "" {
		return errors.New("TASK_DB_DSN is not set")
	}
	st, err := store.New(context.Background(), dsn, log)
	if err != nil {
		return err
	}
	app := &App{store: st, log: log}
	// Best-effort Kafka producer for the task saga. Absent/unreachable Kafka is
	// non-fatal: status patches still persist; the event emission is skipped.
	// (Graceful producer close on shutdown is deferred — svcrun has no hook yet.)
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		p, err := kafka.NewProducer(kafka.Brokers(strings.Split(brokers, ",")), log)
		if err != nil {
			log.Warn("kafka producer unavailable; task saga emits no events", "error", err)
		} else {
			app.prod = p
		}
	}

	mux.HandleFunc("GET /tasks", app.list)
	mux.HandleFunc("POST /tasks", app.create)
	mux.HandleFunc("GET /tasks/{id}", app.get)
	mux.HandleFunc("PUT /tasks/{id}", app.update)
	mux.HandleFunc("DELETE /tasks/{id}", app.delete)
	mux.HandleFunc("PATCH /tasks/{id}/status", app.patchStatus)
	mux.HandleFunc("GET /tasks/{id}/feedback", app.listFeedback)
	mux.HandleFunc("POST /tasks/{id}/feedback", app.addFeedback)
	// Async actions driving the saga (phase 6).
	mux.HandleFunc("POST /tasks/{id}/re-run", app.reRun)
	mux.HandleFunc("POST /tasks/{id}/stop", app.stop)
	mux.HandleFunc("POST /tasks/{id}/open-pr", app.openPr)

	// Internal surface used only by the Gateway (workspace stats composition).
	mux.HandleFunc("GET /internal/workspace/{wid}/open-task-count", app.openTaskCount)
	mux.HandleFunc("GET /internal/tasks/{id}/workspace", app.taskWorkspace)

	app.startConsumers()

	log.Info("task routes registered", "endpoints", 12, "saga_enabled", app.prod != nil)
	return nil
}

// startConsumers subscribes the saga coordinator to execution facts
// (best-effort). Idempotency: each (task_id, run_id) is processed once.
func (a *App) startConsumers() {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		return
	}
	bs := kafka.Brokers(strings.Split(brokers, ","))
	cg, err := kafka.NewConsumerGroup(bs, "task-saga", a.log)
	if err != nil {
		a.log.Warn("task saga consumers unavailable", "error", err)
		return
	}
	go func() {
		if err := cg.Run(context.Background(),
			[]string{contracts.TopicRunCompleted, contracts.TopicVerdict, contracts.TopicPrOpened},
			a.consume); err != nil {
			a.log.Error("task saga consumer stopped", "error", err)
		}
	}()
}

// consume drives the saga state machine (tasks 6.2–6.6):
//
//	run.completed (implementer, done) → review + task.review-requested
//	verdict APPROVE              → done
//	verdict REQUEST_CHANGES      → round < 5 → doing + task.run-requested(round+1)
//	                              → round ≥ 5 → blocked (review rounds exhausted)
func (a *App) consume(ctx context.Context, env contracts.EventEnvelope) error {
	switch env.EventType {
	case contracts.TopicRunCompleted:
		var d contracts.RunCompletedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		if d.Role != contracts.RunRoleImplementer || d.Status != contracts.RunDone {
			return nil // reviewer/aborted runs do not advance the task
		}
		return a.onImplementerDone(ctx, d)
	case contracts.TopicVerdict:
		var d contracts.VerdictData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		return a.onVerdict(ctx, d)
	case contracts.TopicPrOpened:
		var d contracts.PrOpenedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		a.log.Info("pr opened for task", "task_id", d.TaskID, "run_id", d.RunID, "url", d.URL)
	}
	return nil
}

// onImplementerDone moves the task to review and requests a reviewer run.
func (a *App) onImplementerDone(ctx context.Context, d contracts.RunCompletedData) error {
	ok, err := a.store.SagaNew(ctx, d.TaskID, d.RunID)
	if err != nil {
		return err
	}
	if !ok {
		return nil // already processed (redelivery)
	}
	t, err := a.store.GetUnscoped(ctx, d.TaskID)
	if err != nil {
		return err
	}
	if t.Status != contracts.TaskDoing {
		a.log.Info("implementer done ignored: task not in doing", "task_id", d.TaskID, "status", t.Status)
		return nil
	}
	if _, err := a.store.SetStatus(ctx, d.TaskID, contracts.TaskReview); err != nil {
		return err
	}
	a.publish(ctx, contracts.TopicTaskStatusChanged, contracts.TaskStatusChangedData{
		TaskID: d.TaskID, From: contracts.TaskDoing, To: contracts.TaskReview, RoundNo: d.RoundNo,
	}, d.TaskID)
	if t.AgentID == nil {
		a.log.Warn("no agent to review; task stuck in review", "task_id", d.TaskID)
		return nil
	}
	a.publish(ctx, contracts.TopicTaskReviewRequested, contracts.ReviewRequestedData{
		TaskID: d.TaskID, AgentID: *t.AgentID, RunID: d.RunID, WorkspaceID: t.WorkspaceID,
		RoundNo: d.RoundNo, Prompt: t.Prompt,
	}, d.TaskID)
	return nil
}

// onVerdict advances the task on the reviewer's decision.
func (a *App) onVerdict(ctx context.Context, d contracts.VerdictData) error {
	ok, err := a.store.SagaNew(ctx, d.TaskID, d.RunID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	t, err := a.store.GetUnscoped(ctx, d.TaskID)
	if err != nil {
		return err
	}
	if t.Status != contracts.TaskReview {
		a.log.Info("verdict ignored: task not in review", "task_id", d.TaskID, "status", t.Status)
		return nil
	}
	switch d.Decision {
	case contracts.VerdictApprove:
		if _, err := a.store.SetStatus(ctx, d.TaskID, contracts.TaskDone); err != nil {
			return err
		}
		return a.publishErr(ctx, contracts.TopicTaskStatusChanged, contracts.TaskStatusChangedData{
			TaskID: d.TaskID, From: contracts.TaskReview, To: contracts.TaskDone, RoundNo: d.RoundNo,
		}, d.TaskID)
	case contracts.VerdictRequestChanges:
		next := d.RoundNo + 1
		if next > maxReviewRounds {
			if _, err := a.store.SetStatus(ctx, d.TaskID, contracts.TaskBlocked); err != nil {
				return err
			}
			a.log.Warn("review rounds exhausted; task blocked", "task_id", d.TaskID, "rounds", next)
			return a.publishErr(ctx, contracts.TopicTaskStatusChanged, contracts.TaskStatusChangedData{
				TaskID: d.TaskID, From: contracts.TaskReview, To: contracts.TaskBlocked, RoundNo: d.RoundNo,
			}, d.TaskID)
		}
		if _, err := a.store.SetStatus(ctx, d.TaskID, contracts.TaskDoing); err != nil {
			return err
		}
		if err := a.store.SetRoundNo(ctx, d.TaskID, next); err != nil {
			return err
		}
		a.publish(ctx, contracts.TopicTaskStatusChanged, contracts.TaskStatusChangedData{
			TaskID: d.TaskID, From: contracts.TaskReview, To: contracts.TaskDoing, RoundNo: next,
		}, d.TaskID)
		a.publish(ctx, contracts.TopicTaskRunRequested, contracts.RunRequestedData{
			TaskID: d.TaskID, AgentID: derefAgentID(t.AgentID), ProjectID: t.ProjectID,
			WorkspaceID: t.WorkspaceID, RoundNo: next, Prompt: t.Prompt, ModelOverride: derefStr(t.ModelOverride),
		}, d.TaskID)
	}
	return nil
}

// derefAgentID flattens the agent pointer; the caller already validated it.
func derefAgentID(p *contracts.ID) contracts.ID {
	if p != nil {
		return *p
	}
	return ""
}

// maxReviewRounds caps the review loop (task 6.3).
const maxReviewRounds = 5

// publishErr publishes and returns the error (for tail-call convenience).
func (a *App) publishErr(ctx context.Context, topic string, data any, taskID contracts.ID) error {
	a.publish(ctx, topic, data, taskID)
	return nil
}

// publish emits env to topic, keyed by taskID. No-op (and non-fatal) when the
// producer is nil or publishing fails — the status change is already persisted.
func (a *App) publish(ctx context.Context, topic string, data any, taskID contracts.ID) {
	if a.prod == nil {
		return
	}
	env := contracts.EventEnvelope{TaskID: taskID, Data: data}
	if err := kafka.Publish(ctx, a.prod, topic, env, a.log); err != nil {
		a.log.Error("publish event failed", "topic", topic, "task_id", taskID, "error", err)
	}
}

// reRun requests a fresh implementer run (saga action; task 6.4).
func (a *App) reRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := a.store.Get(r.Context(), id, tenancy.WorkspaceIDs(r))
	if err != nil {
		httputil.RespondOK(w, a.log, "task.ReRun.get", nil, err, store.ErrTaskNotFound)
		return
	}
	if t.AgentID == nil || *t.AgentID == "" {
		httputil.Error(w, http.StatusBadRequest, "task has no assigned agent")
		return
	}
	next := t.RoundNo + 1
	if err := a.store.SetRoundNo(r.Context(), id, next); err != nil {
		httputil.ServerError(w, a.log, "task.ReRun.round", err)
		return
	}
	if _, err := a.store.SetStatus(r.Context(), id, contracts.TaskDoing); err != nil {
		httputil.ServerError(w, a.log, "task.ReRun.status", err)
		return
	}
	a.publish(r.Context(), contracts.TopicTaskStatusChanged, contracts.TaskStatusChangedData{
		TaskID: id, From: t.Status, To: contracts.TaskDoing, RoundNo: next,
	}, id)
	a.publish(r.Context(), contracts.TopicTaskRunRequested, contracts.RunRequestedData{
		TaskID: id, AgentID: *t.AgentID, ProjectID: t.ProjectID,
		WorkspaceID: t.WorkspaceID, RoundNo: next, Prompt: t.Prompt, ModelOverride: derefStr(t.ModelOverride),
	}, id)
	out, err := a.store.Get(r.Context(), id, tenancy.WorkspaceIDs(r))
	httputil.RespondOK(w, a.log, "task.ReRun", out, err)
}

// stop sets the task stopped synchronously and requests an abort (task 6.4).
func (a *App) stop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	prev, err := a.store.Get(r.Context(), id, tenancy.WorkspaceIDs(r))
	if err != nil {
		httputil.RespondOK(w, a.log, "task.Stop.get", nil, err, store.ErrTaskNotFound)
		return
	}
	out, err := a.store.SetStatus(r.Context(), id, contracts.TaskStopped)
	if err != nil {
		httputil.RespondOK(w, a.log, "task.Stop.set", nil, err, store.ErrTaskNotFound)
		return
	}
	if prev.Status != out.Status {
		a.publish(r.Context(), contracts.TopicTaskStatusChanged, contracts.TaskStatusChangedData{
			TaskID: out.ID, From: prev.Status, To: out.Status, RoundNo: out.RoundNo,
		}, out.ID)
	}
	a.publish(r.Context(), contracts.TopicTaskStopRequested, contracts.StopRequestedData{TaskID: id}, id)
	httputil.WriteJSON(w, http.StatusOK, out)
}

// openPr requests PR creation from the Runner (task 6.5); the PR is never
// auto-created anywhere else.
func (a *App) openPr(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := a.store.Get(r.Context(), id, tenancy.WorkspaceIDs(r))
	if err != nil {
		httputil.RespondOK(w, a.log, "task.OpenPr.get", nil, err, store.ErrTaskNotFound)
		return
	}
	if t.Status != contracts.TaskDone {
		httputil.Error(w, http.StatusConflict, "open-pr is only allowed on done tasks")
		return
	}
	a.publish(r.Context(), contracts.TopicPrOpenRequested, contracts.PrOpenRequestedData{TaskID: id}, id)
	w.WriteHeader(http.StatusAccepted)
}

// openTaskCount serves the open-task count for the Gateway's workspace list.
func (a *App) openTaskCount(w http.ResponseWriter, r *http.Request) {
	n, err := a.store.CountOpenByWorkspace(r.Context(), r.PathValue("wid"))
	if err != nil {
		httputil.ServerError(w, a.log, "task.OpenTaskCount", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]int{"open_task_count": n})
}

// taskWorkspace returns the owning workspace of a task so the Gateway can gate
// task sub-routes (runs/artifacts) against the session's workspace union.
func (a *App) taskWorkspace(w http.ResponseWriter, r *http.Request) {
	t, err := a.store.GetUnscoped(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrTaskNotFound) {
		httputil.Error(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, a.log, "task.TaskWorkspace", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"workspace_id": string(t.WorkspaceID)})
}

func (a *App) list(w http.ResponseWriter, r *http.Request) {
	q := store.Query{
		ProjectID: r.URL.Query().Get("project_id"),
		Status:    contracts.TaskStatus(r.URL.Query().Get("status")),
		Type:      contracts.TaskType(r.URL.Query().Get("type")),
		Priority:  contracts.Priority(r.URL.Query().Get("priority")),
		Assignee:  r.URL.Query().Get("assignee"),
		Label:     r.URL.Query().Get("label"),
		Q:         r.URL.Query().Get("q"),
	}
	q.Workspaces = tenancy.WorkspaceIDs(r)
	out, err := a.store.List(r.Context(), q)
	httputil.RespondOK(w, a.log, "task.List", out, err)
}

func (a *App) get(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.Get(r.Context(), r.PathValue("id"), tenancy.WorkspaceIDs(r))
	httputil.RespondOK(w, a.log, "task.Get", out, err, store.ErrTaskNotFound)
}

func (a *App) create(w http.ResponseWriter, r *http.Request) {
	var in store.CreateInput
	if httputil.Decode(w, r, &in) {
		return
	}
	if in.ProjectID == "" || in.Title == "" || in.Prompt == "" {
		httputil.Error(w, http.StatusBadRequest, "project_id, title and prompt are required")
		return
	}
	ws := tenancy.WorkspaceID(r)
	if ws == "" {
		httputil.Error(w, http.StatusBadRequest, "no workspace context")
		return
	}
	out, err := a.store.Create(r.Context(), ws, in)
	httputil.RespondCreated(w, a.log, "task.Create", out, err)
}

func (a *App) update(w http.ResponseWriter, r *http.Request) {
	var fields map[string]any
	if httputil.Decode(w, r, &fields) {
		return
	}
	out, err := a.store.Update(r.Context(), r.PathValue("id"), tenancy.WorkspaceIDs(r), fields)
	httputil.RespondOK(w, a.log, "task.Update", out, err, store.ErrTaskNotFound)
}

func (a *App) delete(w http.ResponseWriter, r *http.Request) {
	err := a.store.Delete(r.Context(), r.PathValue("id"), tenancy.WorkspaceIDs(r))
	httputil.RespondDelete(w, a.log, "task.Delete", err, store.ErrTaskNotFound)
}

// patchStatus applies a status change and emits the saga events for the
// transition. "doing" requests an implementer run; "stopped"/"cancelled"
// request an abort; any real change publishes the status-changed fact.
func (a *App) patchStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status string `json:"status"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	if body.Status == "" {
		httputil.Error(w, http.StatusBadRequest, "status is required")
		return
	}
	id := r.PathValue("id")
	status := contracts.TaskStatus(body.Status)

	prev, err := a.store.Get(r.Context(), id, tenancy.WorkspaceIDs(r))
	if err != nil {
		httputil.RespondOK(w, a.log, "task.PatchStatus.get", nil, err, store.ErrTaskNotFound)
		return
	}
	out, err := a.store.SetStatus(r.Context(), id, status)
	if err != nil {
		httputil.RespondOK(w, a.log, "task.PatchStatus.set", nil, err, store.ErrTaskNotFound)
		return
	}

	// Saga: emit commands + the status-changed fact only on real transitions
	// (task 6.7 — an idempotent PATCH must not re-emit).
	if prev.Status != status {
		a.publish(r.Context(), contracts.TopicTaskStatusChanged, contracts.TaskStatusChangedData{
			TaskID: out.ID, From: prev.Status, To: status, RoundNo: out.RoundNo,
		}, out.ID)
		switch status {
		case contracts.TaskDoing:
			// Skip the run request when the task has no assigned agent
			// (task 6.8): surface the un-runnable task as blocked instead.
			if out.AgentID == nil || *out.AgentID == "" {
				if _, err := a.store.SetStatus(r.Context(), id, contracts.TaskBlocked); err != nil {
					httputil.ServerError(w, a.log, "task.PatchStatus.blocked", err)
					return
				}
				a.publish(r.Context(), contracts.TopicTaskStatusChanged, contracts.TaskStatusChangedData{
					TaskID: id, From: status, To: contracts.TaskBlocked, RoundNo: out.RoundNo,
				}, id)
				httputil.WriteJSON(w, http.StatusOK, out)
				return
			}
			a.publish(r.Context(), contracts.TopicTaskRunRequested, contracts.RunRequestedData{
				TaskID:        out.ID,
				AgentID:       *out.AgentID,
				ProjectID:     out.ProjectID,
				RoundNo:       out.RoundNo,
				Prompt:        out.Prompt,
				ModelOverride: derefStr(out.ModelOverride),
			}, out.ID)
		case contracts.TaskStopped, contracts.TaskCancelled:
			a.publish(r.Context(), contracts.TopicTaskStopRequested, contracts.StopRequestedData{TaskID: out.ID}, out.ID)
		}
	}

	httputil.WriteJSON(w, http.StatusOK, out)
}

func (a *App) listFeedback(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.ListFeedback(r.Context(), r.PathValue("id"), tenancy.WorkspaceIDs(r))
	httputil.RespondOK(w, a.log, "feedback.List", out, err)
}

func (a *App) addFeedback(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Body string `json:"body"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	if body.Body == "" {
		httputil.Error(w, http.StatusBadRequest, "body is required")
		return
	}
	out, err := a.store.AddFeedback(r.Context(), r.PathValue("id"), tenancy.WorkspaceIDs(r), body.Body)
	httputil.RespondCreated(w, a.log, "feedback.Add", out, err)
}

func derefStr(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}
