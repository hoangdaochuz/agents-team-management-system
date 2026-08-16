// Package http exposes the Catalog use cases as thin HTTP handlers: decode →
// call application handler → encode. All business rules live in application.
package http

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/aaks/server/internal/contracts/identity"
	httputil "github.com/aaks/server/internal/platform/http"
	"github.com/aaks/server/internal/platform/tenancy"
	"github.com/aaks/server/services/catalog/internal/application"
	"github.com/aaks/server/services/catalog/internal/domain"
)

// Server wires the Catalog routes to the application service.
type Server struct {
	app *application.App
	log *slog.Logger
}

// New builds the HTTP adapter.
func New(app *application.App, log *slog.Logger) *Server { return &Server{app: app, log: log} }

// Register mounts all Catalog routes on mux, matching frontend/src/api/skills.ts
// + mcpServers.ts.
func (s *Server) Register(mux *http.ServeMux) {
	// Skills
	mux.HandleFunc("GET /skills", s.listSkills)
	mux.HandleFunc("POST /skills", s.createSkill)
	mux.HandleFunc("GET /skills/{id}", s.getSkill)
	mux.HandleFunc("PUT /skills/{id}", s.updateSkill)
	mux.HandleFunc("DELETE /skills/{id}", s.deleteSkill)
	// MCP servers
	mux.HandleFunc("GET /mcp-servers", s.listMcp)
	mux.HandleFunc("POST /mcp-servers", s.createMcp)
	mux.HandleFunc("GET /mcp-servers/{id}", s.getMcp)
	mux.HandleFunc("PUT /mcp-servers/{id}", s.updateMcp)
	mux.HandleFunc("DELETE /mcp-servers/{id}", s.deleteMcp)
	// Workspace-scoped skill surface (resources screen; frontend skills.ts).
	mux.HandleFunc("GET /workspaces/{wid}/skills", s.listWorkspaceSkills)
	mux.HandleFunc("PATCH /workspaces/{wid}/skills/{id}", s.setSkillEnabled)

	// Internal: MCP server definitions by ID list (trusted callers, e.g. the
	// Agent service hydrating an agent's attached servers for the Runner).
	mux.HandleFunc("GET /internal/mcp-servers", s.listMcpByIDs)
}

// ── Skills ──────────────────────────────────────────────────────────────────

func (s *Server) listSkills(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.ListSkills(r.Context(), tenancy.WorkspaceIDs(r))
	httputil.RespondOK(w, s.log, "catalog.ListSkills", out, err)
}

func (s *Server) getSkill(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.GetSkill(r.Context(), identity.ID(r.PathValue("id")), tenancy.WorkspaceIDs(r))
	httputil.RespondOK(w, s.log, "catalog.GetSkill", out, err, domain.ErrSkillNotFound)
}

func (s *Server) createSkill(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		BodyMd      string `json:"body_md"`
		Trigger     string `json:"trigger,omitempty"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	if body.Name == "" || body.BodyMd == "" {
		httputil.Error(w, http.StatusBadRequest, "name and body_md are required")
		return
	}
	ws := tenancy.WorkspaceID(r)
	if ws == "" {
		httputil.Error(w, http.StatusBadRequest, "no workspace context")
		return
	}
	out, err := s.app.CreateSkill(r.Context(), identity.ID(ws), domain.SkillCreate{
		Name: body.Name, Description: body.Description, BodyMd: body.BodyMd, Trigger: body.Trigger,
	})
	httputil.RespondCreated(w, s.log, "catalog.CreateSkill", out, err)
}

func (s *Server) updateSkill(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          *string `json:"name,omitempty"`
		Description   *string `json:"description,omitempty"`
		BodyMd        *string `json:"body_md,omitempty"`
		ResourcesPath *string `json:"resources_path,omitempty"`
		Enabled       *bool   `json:"enabled,omitempty"`
		Trigger       *string `json:"trigger,omitempty"`
		StepCount     *int    `json:"step_count,omitempty"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	out, err := s.app.UpdateSkill(r.Context(), identity.ID(r.PathValue("id")), tenancy.WorkspaceIDs(r), domain.SkillUpdate{
		Name: body.Name, Description: body.Description, BodyMd: body.BodyMd,
		ResourcesPath: body.ResourcesPath, Enabled: body.Enabled, Trigger: body.Trigger, StepCount: body.StepCount,
	})
	httputil.RespondOK(w, s.log, "catalog.UpdateSkill", out, err, domain.ErrSkillNotFound)
}

func (s *Server) deleteSkill(w http.ResponseWriter, r *http.Request) {
	err := s.app.DeleteSkill(r.Context(), identity.ID(r.PathValue("id")), tenancy.WorkspaceIDs(r))
	httputil.RespondDelete(w, s.log, "catalog.DeleteSkill", err, domain.ErrSkillNotFound)
}

// listWorkspaceSkills serves GET /workspaces/{wid}/skills (scoped path).
func (s *Server) listWorkspaceSkills(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.ListWorkspaceSkills(r.Context(), identity.ID(r.PathValue("wid")))
	httputil.RespondOK(w, s.log, "catalog.ListWorkspaceSkills", out, err)
}

// setSkillEnabled serves PATCH /workspaces/{wid}/skills/{id} {enabled}.
func (s *Server) setSkillEnabled(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	if body.Enabled == nil {
		httputil.Error(w, http.StatusBadRequest, "enabled is required")
		return
	}
	out, err := s.app.SetSkillEnabled(r.Context(), identity.ID(r.PathValue("wid")), identity.ID(r.PathValue("id")), *body.Enabled)
	httputil.RespondOK(w, s.log, "catalog.SetSkillEnabled", out, err, domain.ErrSkillNotFound)
}

// ── MCP servers ─────────────────────────────────────────────────────────────

func (s *Server) listMcp(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.ListMcp(r.Context(), tenancy.WorkspaceIDs(r))
	httputil.RespondOK(w, s.log, "catalog.ListMcp", out, err)
}

// listMcpByIDs serves `GET /internal/mcp-servers?ids=a,b,c` — definitions for
// the listed IDs (internal trusted callers). Used by the Agent service to
// hydrate an agent's attached MCP servers for the Runner bridge (task 5.5).
func (s *Server) listMcpByIDs(w http.ResponseWriter, r *http.Request) {
	ids := parseIDList(r.URL.Query().Get("ids"))
	out, err := s.app.ListMcpByIDs(r.Context(), ids)
	httputil.RespondOK(w, s.log, "catalog.ListMcpByIDs", out, err)
}

// parseIDList splits a comma-separated id list, trimming blanks.
func parseIDList(s string) []identity.ID {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]identity.ID, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, identity.ID(p))
		}
	}
	return out
}

func (s *Server) getMcp(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.GetMcp(r.Context(), identity.ID(r.PathValue("id")), tenancy.WorkspaceIDs(r))
	httputil.RespondOK(w, s.log, "catalog.GetMcp", out, err, domain.ErrMcpNotFound)
}

func (s *Server) createMcp(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string            `json:"name"`
		Command string            `json:"command"`
		Args    []string          `json:"args,omitempty"`
		Env     map[string]string `json:"env,omitempty"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	if body.Name == "" || body.Command == "" {
		httputil.Error(w, http.StatusBadRequest, "name and command are required")
		return
	}
	ws := tenancy.WorkspaceID(r)
	if ws == "" {
		httputil.Error(w, http.StatusBadRequest, "no workspace context")
		return
	}
	out, err := s.app.CreateMcp(r.Context(), identity.ID(ws), domain.McpCreate{
		Name: body.Name, Command: body.Command, Args: body.Args, Env: body.Env,
	})
	httputil.RespondCreated(w, s.log, "catalog.CreateMcp", out, err)
}

func (s *Server) updateMcp(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    *string            `json:"name,omitempty"`
		Command *string            `json:"command,omitempty"`
		Args    *[]string          `json:"args,omitempty"`
		Env     *map[string]string `json:"env,omitempty"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	out, err := s.app.UpdateMcp(r.Context(), identity.ID(r.PathValue("id")), tenancy.WorkspaceIDs(r), domain.McpUpdate{
		Name: body.Name, Command: body.Command, Args: body.Args, Env: body.Env,
	})
	httputil.RespondOK(w, s.log, "catalog.UpdateMcp", out, err, domain.ErrMcpNotFound)
}

func (s *Server) deleteMcp(w http.ResponseWriter, r *http.Request) {
	err := s.app.DeleteMcp(r.Context(), identity.ID(r.PathValue("id")), tenancy.WorkspaceIDs(r))
	httputil.RespondDelete(w, s.log, "catalog.DeleteMcp", err, domain.ErrMcpNotFound)
}