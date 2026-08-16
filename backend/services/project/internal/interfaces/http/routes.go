// Package http exposes the Project use cases as thin HTTP handlers: decode →
// call application handler → encode. All business rules live in application.
// Routes are registered without the /api prefix; the Gateway strips /api,
// resolves the workspace context, and forwards it via X-Workspace-ID /
// X-Workspace-IDs (matching frontend/src/api/projects.ts).
package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/aaks/server/internal/contracts/identity"
	httputil "github.com/aaks/server/internal/platform/http"
	"github.com/aaks/server/internal/platform/tenancy"
	"github.com/aaks/server/services/project/internal/application"
	"github.com/aaks/server/services/project/internal/domain"
)

// Server wires the Project routes to the application service.
type Server struct {
	app *application.App
	log *slog.Logger
}

// New builds the HTTP adapter.
func New(app *application.App, log *slog.Logger) *Server { return &Server{app: app, log: log} }

// Register mounts all Project routes on mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /projects", s.list)
	mux.HandleFunc("POST /projects", s.create)
	mux.HandleFunc("GET /projects/{id}", s.get)
	mux.HandleFunc("PUT /projects/{id}", s.update)
	mux.HandleFunc("DELETE /projects/{id}", s.delete)
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	ps, err := s.app.List(r.Context(), workspaceIDs(r))
	if err != nil {
		httputil.ServerError(w, s.log, "project.List", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, ps)
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	p, err := s.app.Get(r.Context(), identity.ID(r.PathValue("id")), workspaceIDs(r))
	if errors.Is(err, domain.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, s.log, "project.Get", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, p)
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	ws := tenancy.WorkspaceID(r)
	if ws == "" {
		httputil.Error(w, http.StatusBadRequest, "no workspace context")
		return
	}
	var in domain.CreateInput
	if httputil.Decode(w, r, &in) {
		return
	}
	if in.Name == "" || in.RepoSource == "" || in.RepoType == "" {
		httputil.Error(w, http.StatusBadRequest, "name, repo_source and repo_type are required")
		return
	}
	p, err := s.app.Create(r.Context(), identity.ID(ws), in)
	if err != nil {
		httputil.ServerError(w, s.log, "project.Create", err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, p)
}

func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	var in domain.UpdateInput
	if httputil.Decode(w, r, &in) {
		return
	}
	p, err := s.app.Update(r.Context(), identity.ID(r.PathValue("id")), workspaceIDs(r), in)
	if errors.Is(err, domain.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, s.log, "project.Update", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, p)
}

func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	err := s.app.Delete(r.Context(), identity.ID(r.PathValue("id")), workspaceIDs(r))
	if errors.Is(err, domain.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, s.log, "project.Delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
