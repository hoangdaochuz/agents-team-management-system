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
	app := &App{store: st, log: log}

	mux.HandleFunc("GET /agents", app.list)
	mux.HandleFunc("POST /agents", app.create)
	mux.HandleFunc("GET /agents/{id}", app.get)
	mux.HandleFunc("PUT /agents/{id}", app.update)
	mux.HandleFunc("DELETE /agents/{id}", app.delete)
	mux.HandleFunc("POST /agents/{id}/skills", app.attachSkill)
	mux.HandleFunc("DELETE /agents/{id}/skills/{skillId}", app.detachSkill)
	mux.HandleFunc("POST /agents/{id}/mcps", app.attachMcp)
	mux.HandleFunc("DELETE /agents/{id}/mcps/{mcpId}", app.detachMcp)

	log.Info("agent routes registered", "endpoints", 9)
	return nil
}

func (a *App) list(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.List(r.Context())
	httputil.RespondOK(w, a.log, "agent.List", out, err)
}

func (a *App) get(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.Get(r.Context(), r.PathValue("id"))
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
	out, err := a.store.Create(r.Context(), in)
	httputil.RespondCreated(w, a.log, "agent.Create", out, err)
}

func (a *App) update(w http.ResponseWriter, r *http.Request) {
	var in store.UpdateInput
	if httputil.Decode(w, r, &in) {
		return
	}
	out, err := a.store.Update(r.Context(), r.PathValue("id"), in)
	httputil.RespondOK(w, a.log, "agent.Update", out, err, store.ErrAgentNotFound)
}

func (a *App) delete(w http.ResponseWriter, r *http.Request) {
	err := a.store.Delete(r.Context(), r.PathValue("id"))
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
	httputil.RespondDelete(w, a.log, "agent.AttachMcp", err, store.ErrAgentNotFound)
}

func (a *App) detachMcp(w http.ResponseWriter, r *http.Request) {
	err := a.store.DetachMcp(r.Context(), r.PathValue("id"), r.PathValue("mcpId"))
	httputil.RespondDelete(w, a.log, "agent.DetachMcp", err, store.ErrAgentNotFound)
}
