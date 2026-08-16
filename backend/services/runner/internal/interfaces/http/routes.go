// Package http exposes the Runner query surface as thin HTTP handlers.
package http

import (
	"log/slog"
	"net/http"

	"github.com/aaks/server/internal/contracts/identity"
	httputil "github.com/aaks/server/internal/platform/http"
	"github.com/aaks/server/services/runner/internal/application"
)

// Server wires the Runner query routes to the application service.
type Server struct {
	app *application.Runner
	log *slog.Logger
}

// New builds the HTTP adapter.
func New(app *application.Runner, log *slog.Logger) *Server { return &Server{app: app, log: log} }

// Register mounts all Runner routes on mux.
func (s *Server) Register(mux *http.ServeMux) {
	// Query surface (served through the Gateway under /tasks/:id/* and /runs/:id/*).
	mux.HandleFunc("GET /tasks/{id}/runs", s.listRuns)
	mux.HandleFunc("GET /tasks/{id}/artifacts", s.listArtifacts)
	mux.HandleFunc("GET /runs/{id}/steps", s.listSteps)
	mux.HandleFunc("GET /runs/{id}/findings", s.listFindings)

	// Internal surface used only by the Gateway (SSE replay).
	mux.HandleFunc("GET /internal/tasks/{id}/steps", s.listTaskSteps)
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.app.ListRuns(r.Context(), identity.ID(r.PathValue("id")))
	if err != nil {
		httputil.ServerError(w, s.log, "runner.ListRuns", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, runs)
}

func (s *Server) listArtifacts(w http.ResponseWriter, r *http.Request) {
	arts, err := s.app.ListArtifacts(r.Context(), identity.ID(r.PathValue("id")))
	if err != nil {
		httputil.ServerError(w, s.log, "runner.ListArtifacts", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, arts)
}

func (s *Server) listSteps(w http.ResponseWriter, r *http.Request) {
	steps, err := s.app.ListSteps(r.Context(), identity.ID(r.PathValue("id")))
	if err != nil {
		httputil.ServerError(w, s.log, "runner.ListSteps", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, steps)
}

func (s *Server) listFindings(w http.ResponseWriter, r *http.Request) {
	findings, err := s.app.ListFindings(r.Context(), identity.ID(r.PathValue("id")))
	if err != nil {
		httputil.ServerError(w, s.log, "runner.ListFindings", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, findings)
}

// listTaskSteps serves the SSE replay: all steps for a task, in run/seq order.
func (s *Server) listTaskSteps(w http.ResponseWriter, r *http.Request) {
	steps, err := s.app.ListTaskSteps(r.Context(), identity.ID(r.PathValue("id")))
	if err != nil {
		httputil.ServerError(w, s.log, "runner.ListTaskSteps", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, steps)
}