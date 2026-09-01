// Package http exposes the Workspace service's internal provisioning endpoint:
// the Identity service calls it when a workspace is created (the former
// workspace.created Kafka event, now a direct call). It establishes the
// default project repo binding and seeds the default rules — both idempotent,
// so redelivery is a no-op.
package http

import (
	"log/slog"
	"net/http"

	"github.com/aaks/server/internal/contracts/events"
	httputil "github.com/aaks/server/internal/platform/http"
	projectapp "github.com/aaks/server/services/workspace/internal/application/project"
	resourcesapp "github.com/aaks/server/services/workspace/internal/application/resources"
)

// Server wires the provisioning route to the project and resources handlers.
type Server struct {
	project   *projectapp.App
	resources *resourcesapp.App
	log       *slog.Logger
}

// New builds the HTTP adapter.
func New(project *projectapp.App, resources *resourcesapp.App, log *slog.Logger) *Server {
	return &Server{project: project, resources: resources, log: log}
}

// Register mounts the internal provisioning route on mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /internal/workspaces/provision", s.provision)
}

// provision applies one workspace-creation fact: default repo binding + seed
// rules. Failures are logged and reported so the caller can retry.
func (s *Server) provision(w http.ResponseWriter, r *http.Request) {
	var d events.WorkspaceCreatedData
	if httputil.Decode(w, r, &d) {
		return
	}
	if d.WorkspaceID == "" {
		httputil.Error(w, http.StatusBadRequest, "workspace_id is required")
		return
	}
	if err := s.project.BindWorkspace(r.Context(), d); err != nil {
		httputil.ServerError(w, s.log, "provision.BindWorkspace", err)
		return
	}
	if err := s.resources.BootstrapWorkspace(r.Context(), d); err != nil {
		httputil.ServerError(w, s.log, "provision.BootstrapWorkspace", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
