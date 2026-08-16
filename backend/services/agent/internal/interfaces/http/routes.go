// Package http exposes the Agent use cases as thin HTTP handlers: decode →
// call application handler → encode. All business rules live in application.
package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/identity"
	httputil "github.com/aaks/server/internal/platform/http"
	"github.com/aaks/server/internal/platform/tenancy"
	"github.com/aaks/server/services/agent/internal/application"
	"github.com/aaks/server/services/agent/internal/domain"
)

// Server wires the Agent routes to the application service.
type Server struct {
	app *application.App
	log *slog.Logger
}

// New builds the HTTP adapter.
func New(app *application.App, log *slog.Logger) *Server { return &Server{app: app, log: log} }

// Register mounts all Agent routes on mux, matching frontend/src/api/agents.ts.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /agents", s.list)
	mux.HandleFunc("POST /agents", s.create)
	mux.HandleFunc("GET /agents/{id}", s.get)
	mux.HandleFunc("PUT /agents/{id}", s.update)
	mux.HandleFunc("DELETE /agents/{id}", s.delete)
	mux.HandleFunc("POST /agents/{id}/skills", s.attachSkill)
	mux.HandleFunc("DELETE /agents/{id}/skills/{skillId}", s.detachSkill)
	mux.HandleFunc("POST /agents/{id}/mcps", s.attachMcp)
	mux.HandleFunc("DELETE /agents/{id}/mcps/{mcpId}", s.detachMcp)

	// Internal surface used only by the Gateway (workspace stats composition).
	mux.HandleFunc("GET /internal/workspace/{wid}/agent-count", s.agentCount)
	// Internal: the agent's attached MCP server definitions, hydrated from
	// Catalog. Consumed by the Runner to bridge MCP tools (task 5.5).
	mux.HandleFunc("GET /internal/agents/{id}/mcp-servers", s.agentMcpServers)
}

// agentCount serves the workspace agent count for the Gateway's workspace list.
func (s *Server) agentCount(w http.ResponseWriter, r *http.Request) {
	n, err := s.app.CountByWorkspace(r.Context(), identity.ID(r.PathValue("wid")))
	if err != nil {
		httputil.ServerError(w, s.log, "agent.AgentCount", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]int{"agent_count": n})
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.List(r.Context(), tenancy.WorkspaceIDs(r))
	httputil.RespondOK(w, s.log, "agent.List", out, err)
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.Get(r.Context(), identity.ID(r.PathValue("id")), tenancy.WorkspaceIDs(r))
	httputil.RespondOK(w, s.log, "agent.Get", out, err, domain.ErrAgentNotFound)
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name               string                 `json:"name"`
		Role               string                 `json:"role"`
		SystemPrompt       string                 `json:"system_prompt,omitempty"`
		DefaultModel       string                 `json:"default_model,omitempty"`
		AllowedTools       []string               `json:"allowed_tools,omitempty"`
		RoleTitle          string                 `json:"role_title,omitempty"`
		Provider           identity.Provider      `json:"provider,omitempty"`
		Temperature        *float64               `json:"temperature,omitempty"`
		MaxOutputTokens    *int                   `json:"max_output_tokens,omitempty"`
		AutonomyMode       agentexec.AutonomyMode `json:"autonomy_mode,omitempty"`
		UserPromptTemplate string                 `json:"user_prompt_template,omitempty"`
		KnowledgeSourceIDs []identity.ID          `json:"knowledge_source_ids,omitempty"`
		Guardrails         *agentexec.Guardrails  `json:"guardrails,omitempty"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	if body.Name == "" || body.Role == "" {
		httputil.Error(w, http.StatusBadRequest, "name and role are required")
		return
	}
	ws := tenancy.WorkspaceID(r)
	if ws == "" {
		httputil.Error(w, http.StatusBadRequest, "no workspace context")
		return
	}
	out, err := s.app.Create(r.Context(), identity.ID(ws), domain.AgentCreate{
		Name: body.Name, Role: body.Role, SystemPrompt: body.SystemPrompt, DefaultModel: body.DefaultModel,
		AllowedTools: body.AllowedTools, RoleTitle: body.RoleTitle, Provider: body.Provider,
		Temperature: body.Temperature, MaxOutputTokens: body.MaxOutputTokens, AutonomyMode: body.AutonomyMode,
		UserPromptTemplate: body.UserPromptTemplate, KnowledgeSourceIDs: body.KnowledgeSourceIDs,
		Guardrails: body.Guardrails,
	})
	httputil.RespondCreated(w, s.log, "agent.Create", out, err)
}

func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name               *string                 `json:"name,omitempty"`
		Role               *string                 `json:"role,omitempty"`
		SystemPrompt       *string                 `json:"system_prompt,omitempty"`
		DefaultModel       *string                 `json:"default_model,omitempty"`
		AllowedTools       *[]string               `json:"allowed_tools,omitempty"`
		Status             *string                 `json:"status,omitempty"`
		CurrentTaskID      *string                 `json:"current_task_id,omitempty"`
		RoleTitle          *string                 `json:"role_title,omitempty"`
		Provider           *identity.Provider      `json:"provider,omitempty"`
		Temperature        *float64                `json:"temperature,omitempty"`
		MaxOutputTokens    *int                    `json:"max_output_tokens,omitempty"`
		AutonomyMode       *agentexec.AutonomyMode `json:"autonomy_mode,omitempty"`
		UserPromptTemplate *string                 `json:"user_prompt_template,omitempty"`
		KnowledgeSourceIDs *[]identity.ID          `json:"knowledge_source_ids,omitempty"`
		Guardrails         *agentexec.Guardrails   `json:"guardrails,omitempty"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	out, err := s.app.Update(r.Context(), identity.ID(r.PathValue("id")), tenancy.WorkspaceIDs(r), domain.AgentUpdate{
		Name: body.Name, Role: body.Role, SystemPrompt: body.SystemPrompt, DefaultModel: body.DefaultModel,
		AllowedTools: body.AllowedTools, Status: body.Status, CurrentTaskID: body.CurrentTaskID,
		RoleTitle: body.RoleTitle, Provider: body.Provider, Temperature: body.Temperature,
		MaxOutputTokens: body.MaxOutputTokens, AutonomyMode: body.AutonomyMode,
		UserPromptTemplate: body.UserPromptTemplate, KnowledgeSourceIDs: body.KnowledgeSourceIDs,
		Guardrails: body.Guardrails,
	})
	httputil.RespondOK(w, s.log, "agent.Update", out, err, domain.ErrAgentNotFound)
}

func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	err := s.app.Delete(r.Context(), identity.ID(r.PathValue("id")), tenancy.WorkspaceIDs(r))
	httputil.RespondDelete(w, s.log, "agent.Delete", err, domain.ErrAgentNotFound)
}

func (s *Server) attachSkill(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SkillID string `json:"skill_id"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	err := s.app.AttachSkill(r.Context(), identity.ID(r.PathValue("id")), identity.ID(body.SkillID))
	s.writeAttachErr(w, r, "agent.AttachSkill", err)
}

func (s *Server) detachSkill(w http.ResponseWriter, r *http.Request) {
	err := s.app.DetachSkill(r.Context(), identity.ID(r.PathValue("id")), identity.ID(r.PathValue("skillId")))
	httputil.RespondDelete(w, s.log, "agent.DetachSkill", err, domain.ErrAgentNotFound)
}

func (s *Server) attachMcp(w http.ResponseWriter, r *http.Request) {
	var body struct {
		McpServerID string `json:"mcp_server_id"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	err := s.app.AttachMcp(r.Context(), identity.ID(r.PathValue("id")), identity.ID(body.McpServerID))
	s.writeAttachErr(w, r, "agent.AttachMcp", err)
}

func (s *Server) detachMcp(w http.ResponseWriter, r *http.Request) {
	err := s.app.DetachMcp(r.Context(), identity.ID(r.PathValue("id")), identity.ID(r.PathValue("mcpId")))
	httputil.RespondDelete(w, s.log, "agent.DetachMcp", err, domain.ErrAgentNotFound)
}

// writeAttachErr maps the attachment outcomes: cross-workspace/unknown
// definitions are client errors (400); an unknown agent is 404.
func (s *Server) writeAttachErr(w http.ResponseWriter, r *http.Request, where string, err error) {
	switch {
	case errors.Is(err, domain.ErrCrossWorkspace), errors.Is(err, domain.ErrUnknownDefinition):
		httputil.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrAgentNotFound):
		httputil.Error(w, http.StatusNotFound, "not found")
	case err != nil:
		httputil.ServerError(w, s.log, where, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// agentMcpServers returns an agent's attached MCP server *definitions*
// (hydrated from Catalog) so the Runner can bridge them as tools (task 5.5).
// Trusted callers only (no workspace gate); hydration failures degrade to an
// empty list rather than failing the run setup.
func (s *Server) agentMcpServers(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.AgentMcpServers(r.Context(), identity.ID(r.PathValue("id")))
	if errors.Is(err, domain.ErrAgentNotFound) {
		httputil.Error(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, s.log, "agent.McpServers", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}
