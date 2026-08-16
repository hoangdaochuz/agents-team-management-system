// Package http exposes the Task use cases as thin HTTP handlers: decode →
// call application handler → encode. All business rules and the saga live in
// application. Routes are registered without the /api prefix; the Gateway
// strips /api and forwards the workspace context (frontend/src/api/tasks.ts +
// feedback.ts).
package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/tasks"
	httputil "github.com/aaks/server/internal/platform/http"
	"github.com/aaks/server/internal/platform/tenancy"
	"github.com/aaks/server/services/task/internal/application"
	"github.com/aaks/server/services/task/internal/domain"
)

// Server wires the Task routes to the application service.
type Server struct {
	app *application.App
	log *slog.Logger
}

// New builds the HTTP adapter.
func New(app *application.App, log *slog.Logger) *Server { return &Server{app: app, log: log} }

// Register mounts all Task routes on mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /tasks", s.list)
	mux.HandleFunc("POST /tasks", s.create)
	mux.HandleFunc("GET /tasks/{id}", s.get)
	mux.HandleFunc("PUT /tasks/{id}", s.update)
	mux.HandleFunc("DELETE /tasks/{id}", s.delete)
	mux.HandleFunc("PATCH /tasks/{id}/status", s.patchStatus)
	mux.HandleFunc("GET /tasks/{id}/feedback", s.listFeedback)
	mux.HandleFunc("POST /tasks/{id}/feedback", s.addFeedback)

	// Async actions driving the saga.
	mux.HandleFunc("POST /tasks/{id}/re-run", s.reRun)
	mux.HandleFunc("POST /tasks/{id}/stop", s.stop)
	mux.HandleFunc("POST /tasks/{id}/open-pr", s.openPr)

	// Internal surface used only by the Gateway (workspace stats composition).
	mux.HandleFunc("GET /internal/workspace/{wid}/open-task-count", s.openTaskCount)
	mux.HandleFunc("GET /internal/tasks/{id}/workspace", s.taskWorkspace)
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	q := domain.Query{
		ProjectID: identity.ID(r.URL.Query().Get("project_id")),
		Status:    tasks.TaskStatus(r.URL.Query().Get("status")),
		Type:      tasks.TaskType(r.URL.Query().Get("type")),
		Priority:  tasks.Priority(r.URL.Query().Get("priority")),
		Assignee:  identity.ID(r.URL.Query().Get("assignee")),
		Label:     r.URL.Query().Get("label"),
		Q:         r.URL.Query().Get("q"),
	}
	q.Workspaces = workspaceIDs(r)
	out, err := s.app.List(r.Context(), q)
	httputil.RespondOK(w, s.log, "task.List", out, err)
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.Get(r.Context(), identity.ID(r.PathValue("id")), workspaceIDs(r))
	httputil.RespondOK(w, s.log, "task.Get", out, err, domain.ErrNotFound)
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	var in domain.CreateInput
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
	out, err := s.app.Create(r.Context(), identity.ID(ws), in)
	httputil.RespondCreated(w, s.log, "task.Create", out, err)
}

func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	var fields map[string]any
	if httputil.Decode(w, r, &fields) {
		return
	}
	out, err := s.app.Update(r.Context(), identity.ID(r.PathValue("id")), workspaceIDs(r), fields)
	httputil.RespondOK(w, s.log, "task.Update", out, err, domain.ErrNotFound)
}

func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	err := s.app.Delete(r.Context(), identity.ID(r.PathValue("id")), workspaceIDs(r))
	httputil.RespondDelete(w, s.log, "task.Delete", err, domain.ErrNotFound)
}

// patchStatus applies a status change; the application emits the saga events
// for the transition ("doing" requests an implementer run, "stopped"/
// "cancelled" request an abort, any real change publishes status-changed).
func (s *Server) patchStatus(w http.ResponseWriter, r *http.Request) {
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
	out, err := s.app.PatchStatus(r.Context(), identity.ID(r.PathValue("id")), workspaceIDs(r), tasks.TaskStatus(body.Status))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httputil.Error(w, http.StatusNotFound, "task not found")
	case err != nil:
		httputil.ServerError(w, s.log, "task.PatchStatus", err)
	default:
		httputil.WriteJSON(w, http.StatusOK, out)
	}
}

func (s *Server) listFeedback(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.ListFeedback(r.Context(), identity.ID(r.PathValue("id")), workspaceIDs(r))
	httputil.RespondOK(w, s.log, "feedback.List", out, err)
}

func (s *Server) addFeedback(w http.ResponseWriter, r *http.Request) {
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
	out, err := s.app.AddFeedback(r.Context(), identity.ID(r.PathValue("id")), workspaceIDs(r), body.Body)
	httputil.RespondCreated(w, s.log, "feedback.Add", out, err, domain.ErrNotFound)
}

// reRun requests a fresh implementer run (saga action).
func (s *Server) reRun(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.ReRun(r.Context(), identity.ID(r.PathValue("id")), workspaceIDs(r))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httputil.Error(w, http.StatusNotFound, "task not found")
	case errors.Is(err, domain.ErrNoAgent):
		httputil.Error(w, http.StatusBadRequest, "task has no assigned agent")
	case err != nil:
		httputil.ServerError(w, s.log, "task.ReRun", err)
	default:
		httputil.WriteJSON(w, http.StatusOK, out)
	}
}

// stop sets the task stopped synchronously and requests an abort (saga action).
func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.Stop(r.Context(), identity.ID(r.PathValue("id")), workspaceIDs(r))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httputil.Error(w, http.StatusNotFound, "task not found")
	case err != nil:
		httputil.ServerError(w, s.log, "task.Stop", err)
	default:
		httputil.WriteJSON(w, http.StatusOK, out)
	}
}

// openPr requests PR creation from the Runner; the PR is never auto-created
// anywhere else.
func (s *Server) openPr(w http.ResponseWriter, r *http.Request) {
	err := s.app.OpenPr(r.Context(), identity.ID(r.PathValue("id")), workspaceIDs(r))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httputil.Error(w, http.StatusNotFound, "task not found")
	case errors.Is(err, domain.ErrNotDone):
		httputil.Error(w, http.StatusConflict, "open-pr is only allowed on done tasks")
	case err != nil:
		httputil.ServerError(w, s.log, "task.OpenPr", err)
	default:
		w.WriteHeader(http.StatusAccepted)
	}
}

// openTaskCount serves the open-task count for the Gateway's workspace list.
func (s *Server) openTaskCount(w http.ResponseWriter, r *http.Request) {
	n, err := s.app.OpenTaskCount(r.Context(), identity.ID(r.PathValue("wid")))
	if err != nil {
		httputil.ServerError(w, s.log, "task.OpenTaskCount", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]int{"open_task_count": n})
}

// taskWorkspace returns the owning workspace of a task so the Gateway can gate
// task sub-routes (runs/artifacts) against the session's workspace union.
func (s *Server) taskWorkspace(w http.ResponseWriter, r *http.Request) {
	ws, err := s.app.TaskWorkspace(r.Context(), identity.ID(r.PathValue("id")))
	if errors.Is(err, domain.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, s.log, "task.TaskWorkspace", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"workspace_id": string(ws)})
}

// workspaceIDs returns the session's workspace set from the Gateway-injected
// scoping headers.
func workspaceIDs(r *http.Request) []identity.ID {
	raw := tenancy.WorkspaceIDs(r)
	ids := make([]identity.ID, 0, len(raw))
	for _, id := range raw {
		ids = append(ids, identity.ID(id))
	}
	return ids
}