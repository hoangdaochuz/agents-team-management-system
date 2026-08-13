// Package httpapi registers the Agent service routes: Agent CRUD + attach/detach
// skill & mcp, matching frontend/src/api/agents.ts.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/aaks/server/internal/httputil"
	"github.com/aaks/server/services/agent/internal/store"
)

type App struct {
	store *store.Store
	log   *slog.Logger
	// catalogURL lets the Agent service hydrate an agent's attached MCP server
	// definitions from Catalog (for the Runner bridge, task 5.5). Empty = the
	// internal mcp-servers endpoint returns empty IDs.
	catalogURL string
}

func Register(mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("AGENT_DB_DSN")
	if dsn == "" {
		return errors.New("AGENT_DB_DSN is not set")
	}
	st, err := store.New(context.Background(), dsn, log)
	if err != nil {
		return err
	}
	app := &App{store: st, log: log, catalogURL: os.Getenv("CATALOG_URL")}

	mux.HandleFunc("GET /agents", app.list)
	mux.HandleFunc("POST /agents", app.create)
	mux.HandleFunc("GET /agents/{id}", app.get)
	mux.HandleFunc("PUT /agents/{id}", app.update)
	mux.HandleFunc("DELETE /agents/{id}", app.delete)
	mux.HandleFunc("POST /agents/{id}/skills", app.attachSkill)
	mux.HandleFunc("DELETE /agents/{id}/skills/{skillId}", app.detachSkill)
	mux.HandleFunc("POST /agents/{id}/mcps", app.attachMcp)
	mux.HandleFunc("DELETE /agents/{id}/mcps/{mcpId}", app.detachMcp)

	// Internal surface used only by the Gateway (workspace stats composition).
	mux.HandleFunc("GET /internal/workspace/{wid}/agent-count", app.agentCount)
	// Internal: the agent's attached MCP server definitions, hydrated from
	// Catalog. Consumed by the Runner to bridge MCP tools (task 5.5).
	mux.HandleFunc("GET /internal/agents/{id}/mcp-servers", app.agentMcpServers)

	log.Info("agent routes registered", "endpoints", 11)
	return nil
}

// agentCount serves the workspace agent count for the Gateway's workspace list.
func (a *App) agentCount(w http.ResponseWriter, r *http.Request) {
	n, err := a.store.CountByWorkspace(r.Context(), r.PathValue("wid"))
	if err != nil {
		httputil.ServerError(w, a.log, "agent.AgentCount", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]int{"agent_count": n})
}

func (a *App) list(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.List(r.Context(), httputil.WorkspaceIDs(r))
	httputil.RespondOK(w, a.log, "agent.List", out, err)
}

func (a *App) get(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.Get(r.Context(), r.PathValue("id"), httputil.WorkspaceIDs(r))
	httputil.RespondOK(w, a.log, "agent.Get", out, err, store.ErrAgentNotFound)
}

func (a *App) create(w http.ResponseWriter, r *http.Request) {
	var in store.CreateInput
	if httputil.Decode(w, r, &in) {
		return
	}
	if in.Name == "" || in.Role == "" {
		httputil.Error(w, http.StatusBadRequest, "name and role are required")
		return
	}
	ws := httputil.WorkspaceID(r)
	if ws == "" {
		httputil.Error(w, http.StatusBadRequest, "no workspace context")
		return
	}
	out, err := a.store.Create(r.Context(), ws, in)
	httputil.RespondCreated(w, a.log, "agent.Create", out, err)
}

func (a *App) update(w http.ResponseWriter, r *http.Request) {
	var in store.UpdateInput
	if httputil.Decode(w, r, &in) {
		return
	}
	out, err := a.store.Update(r.Context(), r.PathValue("id"), httputil.WorkspaceIDs(r), in)
	httputil.RespondOK(w, a.log, "agent.Update", out, err, store.ErrAgentNotFound)
}

func (a *App) delete(w http.ResponseWriter, r *http.Request) {
	err := a.store.Delete(r.Context(), r.PathValue("id"), httputil.WorkspaceIDs(r))
	httputil.RespondDelete(w, a.log, "agent.Delete", err, store.ErrAgentNotFound)
}

func (a *App) attachSkill(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SkillID string `json:"skill_id"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	err := a.store.AttachSkill(r.Context(), r.PathValue("id"), body.SkillID)
	if errors.Is(err, store.ErrCrossWorkspace) || errors.Is(err, store.ErrUnknownDefinition) {
		httputil.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.RespondDelete(w, a.log, "agent.AttachSkill", err, store.ErrAgentNotFound)
}

func (a *App) detachSkill(w http.ResponseWriter, r *http.Request) {
	err := a.store.DetachSkill(r.Context(), r.PathValue("id"), r.PathValue("skillId"))
	httputil.RespondDelete(w, a.log, "agent.DetachSkill", err, store.ErrAgentNotFound)
}

func (a *App) attachMcp(w http.ResponseWriter, r *http.Request) {
	var body struct {
		McpServerID string `json:"mcp_server_id"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	err := a.store.AttachMcp(r.Context(), r.PathValue("id"), body.McpServerID)
	if errors.Is(err, store.ErrCrossWorkspace) || errors.Is(err, store.ErrUnknownDefinition) {
		httputil.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.RespondDelete(w, a.log, "agent.AttachMcp", err, store.ErrAgentNotFound)
}

func (a *App) detachMcp(w http.ResponseWriter, r *http.Request) {
	err := a.store.DetachMcp(r.Context(), r.PathValue("id"), r.PathValue("mcpId"))
	httputil.RespondDelete(w, a.log, "agent.DetachMcp", err, store.ErrAgentNotFound)
}
