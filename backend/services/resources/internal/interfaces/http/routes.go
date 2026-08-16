// Package http exposes the Resources use cases as thin HTTP handlers: decode →
// call application handler → encode. All business rules live in application.
// Every route is scoped by workspace_id and gated on membership by the Gateway.
package http

import (
	"log/slog"
	"net/http"

	"github.com/aaks/server/internal/contracts/identity"
	httputil "github.com/aaks/server/internal/platform/http"
	"github.com/aaks/server/services/resources/internal/application"
	"github.com/aaks/server/services/resources/internal/domain"
)

// Server wires the Resources routes to the application service.
type Server struct {
	app *application.App
	log *slog.Logger
}

// New builds the HTTP adapter.
func New(app *application.App, log *slog.Logger) *Server { return &Server{app: app, log: log} }

// Register mounts all Resources routes on mux, matching
// frontend/src/api/{knowledgeSources,plugins,rules,workspaceMcp}.ts.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /workspaces/{id}/knowledge", s.listKnowledge)
	mux.HandleFunc("POST /workspaces/{id}/knowledge", s.createKnowledge)
	mux.HandleFunc("GET /workspaces/{id}/plugins", s.listPlugins)
	mux.HandleFunc("PATCH /workspaces/{id}/plugins/{rid}", s.setPluginEnabled)
	mux.HandleFunc("GET /workspaces/{id}/rules", s.listRules)
	mux.HandleFunc("PATCH /workspaces/{id}/rules/{rid}", s.setRuleEnabled)
	mux.HandleFunc("GET /workspaces/{id}/mcp", s.listMcp)
	mux.HandleFunc("POST /workspaces/{id}/mcp/{rid}/reconnect", s.reconnectMcp)

	// Internal surface used by the Runner (guardrails).
	mux.HandleFunc("GET /internal/workspaces/{id}/enabled-rules", s.internalEnabledRules)
}

// ── Knowledge sources ───────────────────────────────────────────────────────

func (s *Server) listKnowledge(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.ListKnowledge(r.Context(), identity.ID(r.PathValue("id")))
	httputil.RespondOK(w, s.log, "resources.ListKnowledge", out, err)
}

func (s *Server) createKnowledge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
		Kind  string `json:"kind"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	if body.Title == "" {
		httputil.Error(w, http.StatusBadRequest, "title is required")
		return
	}
	out, err := s.app.CreateKnowledge(r.Context(), identity.ID(r.PathValue("id")), body.Title, body.Kind)
	httputil.RespondCreated(w, s.log, "resources.CreateKnowledge", out, err)
}

// ── Plugins ─────────────────────────────────────────────────────────────────

func (s *Server) listPlugins(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.ListPlugins(r.Context(), identity.ID(r.PathValue("id")))
	httputil.RespondOK(w, s.log, "resources.ListPlugins", out, err)
}

func (s *Server) setPluginEnabled(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	out, err := s.app.SetPluginEnabled(r.Context(), identity.ID(r.PathValue("id")), identity.ID(r.PathValue("rid")), body.Enabled)
	httputil.RespondOK(w, s.log, "resources.SetPluginEnabled", out, err, domain.ErrNotFound)
}

// ── Rules ───────────────────────────────────────────────────────────────────

func (s *Server) listRules(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.ListRules(r.Context(), identity.ID(r.PathValue("id")))
	httputil.RespondOK(w, s.log, "resources.ListRules", out, err)
}

func (s *Server) setRuleEnabled(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	out, err := s.app.SetRuleEnabled(r.Context(), identity.ID(r.PathValue("id")), identity.ID(r.PathValue("rid")), body.Enabled)
	httputil.RespondOK(w, s.log, "resources.SetRuleEnabled", out, err, domain.ErrNotFound)
}

// ── MCP connections ─────────────────────────────────────────────────────────

func (s *Server) listMcp(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.ListMcpConnections(r.Context(), identity.ID(r.PathValue("id")))
	httputil.RespondOK(w, s.log, "resources.ListMcp", out, err)
}

func (s *Server) reconnectMcp(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.ReconnectMcpConnection(r.Context(), identity.ID(r.PathValue("id")), identity.ID(r.PathValue("rid")))
	httputil.RespondOK(w, s.log, "resources.ReconnectMcp", out, err, domain.ErrNotFound)
}

// ── Internal (Runner guardrails) ────────────────────────────────────────────

func (s *Server) internalEnabledRules(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.EnabledRules(r.Context(), identity.ID(r.PathValue("id")))
	httputil.RespondOK(w, s.log, "resources.EnabledRules", out, err)
}