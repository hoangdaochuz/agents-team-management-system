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
	"github.com/aaks/server/internal/httputil"
	"github.com/aaks/server/internal/kafka"
	"github.com/aaks/server/services/task/internal/store"
)

type App struct {
	store *store.Store
	prod  sarama.SyncProducer // nilable: saga is a no-op when Kafka is unavailable
	log   *slog.Logger
}

func Register(mux *http.ServeMux, log *slog.Logger) error {
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
	// Async actions — full runner-driven flow lands in phase 6.
	mux.HandleFunc("POST /tasks/{id}/re-run", app.notImplemented)
	mux.HandleFunc("POST /tasks/{id}/stop", app.notImplemented)
	mux.HandleFunc("POST /tasks/{id}/open-pr", app.notImplemented)

	log.Info("task routes registered", "endpoints", 11, "saga_enabled", app.prod != nil)
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

func (a *App) notImplemented(w http.ResponseWriter, _ *http.Request) {
	httputil.Error(w, http.StatusNotImplemented, "async action lands with the runner in phase 6")
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
	out, err := a.store.List(r.Context(), q)
	httputil.RespondOK(w, a.log, "task.List", out, err)
}

func (a *App) get(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.Get(r.Context(), r.PathValue("id"))
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
	out, err := a.store.Create(r.Context(), in)
	httputil.RespondCreated(w, a.log, "task.Create", out, err)
}

func (a *App) update(w http.ResponseWriter, r *http.Request) {
	var fields map[string]any
	if httputil.Decode(w, r, &fields) {
		return
	}
	out, err := a.store.Update(r.Context(), r.PathValue("id"), fields)
	httputil.RespondOK(w, a.log, "task.Update", out, err, store.ErrTaskNotFound)
}

func (a *App) delete(w http.ResponseWriter, r *http.Request) {
	err := a.store.Delete(r.Context(), r.PathValue("id"))
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

	prev, err := a.store.Get(r.Context(), id)
	if err != nil {
		httputil.RespondOK(w, a.log, "task.PatchStatus.get", nil, err, store.ErrTaskNotFound)
		return
	}
	out, err := a.store.SetStatus(r.Context(), id, status)
	if err != nil {
		httputil.RespondOK(w, a.log, "task.PatchStatus.set", nil, err, store.ErrTaskNotFound)
		return
	}

	// Saga: emit commands + the status-changed fact on real transitions.
	if prev.Status != status {
		a.publish(r.Context(), contracts.TopicTaskStatusChanged, contracts.TaskStatusChangedData{
			TaskID: out.ID, From: prev.Status, To: status, RoundNo: out.RoundNo,
		}, out.ID)
	}
	switch status {
	case contracts.TaskStatus("doing"):
		a.publish(r.Context(), contracts.TopicTaskRunRequested, contracts.RunRequestedData{
			TaskID:        out.ID,
			AgentID:       derefID(out.AgentID),
			ProjectID:     out.ProjectID,
			RoundNo:       out.RoundNo,
			ModelOverride: derefStr(out.ModelOverride),
		}, out.ID)
	case contracts.TaskStatus("stopped"), contracts.TaskStatus("cancelled"):
		a.publish(r.Context(), contracts.TopicTaskStopRequested, contracts.StopRequestedData{TaskID: out.ID}, out.ID)
	}

	httputil.WriteJSON(w, http.StatusOK, out)
}

func (a *App) listFeedback(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.ListFeedback(r.Context(), r.PathValue("id"))
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
	out, err := a.store.AddFeedback(r.Context(), r.PathValue("id"), body.Body)
	httputil.RespondCreated(w, a.log, "feedback.Add", out, err)
}

func derefID(p *contracts.ID) contracts.ID {
	if p != nil {
		return *p
	}
	return ""
}

func derefStr(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}
