// Package httpapi registers the Catalog service routes: Skill CRUD + McpServer
// CRUD, matching frontend/src/api/skills.ts + mcpServers.ts.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/aaks/server/internal/httputil"
	"github.com/aaks/server/services/catalog/internal/store"
)

type App struct {
	store *store.Store
	log   *slog.Logger
}

func Register(mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("CATALOG_DB_DSN")
	if dsn == "" {
		return errors.New("CATALOG_DB_DSN is not set")
	}
	st, err := store.New(context.Background(), dsn, log)
	if err != nil {
		return err
	}
	app := &App{store: st, log: log}

	// Skills
	mux.HandleFunc("GET /skills", app.listSkills)
	mux.HandleFunc("POST /skills", app.createSkill)
	mux.HandleFunc("GET /skills/{id}", app.getSkill)
	mux.HandleFunc("PUT /skills/{id}", app.updateSkill)
	mux.HandleFunc("DELETE /skills/{id}", app.deleteSkill)
	// MCP servers
	mux.HandleFunc("GET /mcp-servers", app.listMcp)
	mux.HandleFunc("POST /mcp-servers", app.createMcp)
	mux.HandleFunc("GET /mcp-servers/{id}", app.getMcp)
	mux.HandleFunc("PUT /mcp-servers/{id}", app.updateMcp)
	mux.HandleFunc("DELETE /mcp-servers/{id}", app.deleteMcp)

	log.Info("catalog routes registered", "endpoints", 10)
	return nil
}

// ── Skill handlers ──────────────────────────────────────────────────────────

func (a *App) listSkills(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.ListSkills(r.Context())
	respond(w, a.log, "skill.List", out, err)
}

func (a *App) getSkill(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.GetSkill(r.Context(), r.PathValue("id"))
	respond(w, a.log, "skill.Get", out, err, store.ErrSkillNotFound)
}

func (a *App) createSkill(w http.ResponseWriter, r *http.Request) {
	var in store.SkillCreateInput
	if bad := decode(w, r, &in); bad {
		return
	}
	if in.Name == "" || in.BodyMd == "" {
		httputil.Error(w, http.StatusBadRequest, "name and body_md are required")
		return
	}
	out, err := a.store.CreateSkill(r.Context(), in)
	respondCreated(w, a.log, "skill.Create", out, err)
}

func (a *App) updateSkill(w http.ResponseWriter, r *http.Request) {
	var in store.SkillUpdateInput
	if bad := decode(w, r, &in); bad {
		return
	}
	out, err := a.store.UpdateSkill(r.Context(), r.PathValue("id"), in)
	respond(w, a.log, "skill.Update", out, err, store.ErrSkillNotFound)
}

func (a *App) deleteSkill(w http.ResponseWriter, r *http.Request) {
	err := a.store.DeleteSkill(r.Context(), r.PathValue("id"))
	respondDelete(w, a.log, "skill.Delete", err, store.ErrSkillNotFound)
}

// ── MCP handlers ────────────────────────────────────────────────────────────

func (a *App) listMcp(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.ListMcp(r.Context())
	respond(w, a.log, "mcp.List", out, err)
}

func (a *App) getMcp(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.GetMcp(r.Context(), r.PathValue("id"))
	respond(w, a.log, "mcp.Get", out, err, store.ErrMcpNotFound)
}

func (a *App) createMcp(w http.ResponseWriter, r *http.Request) {
	var in store.McpCreateInput
	if bad := decode(w, r, &in); bad {
		return
	}
	if in.Name == "" || in.Command == "" {
		httputil.Error(w, http.StatusBadRequest, "name and command are required")
		return
	}
	out, err := a.store.CreateMcp(r.Context(), in)
	respondCreated(w, a.log, "mcp.Create", out, err)
}

func (a *App) updateMcp(w http.ResponseWriter, r *http.Request) {
	var in store.McpUpdateInput
	if bad := decode(w, r, &in); bad {
		return
	}
	out, err := a.store.UpdateMcp(r.Context(), r.PathValue("id"), in)
	respond(w, a.log, "mcp.Update", out, err, store.ErrMcpNotFound)
}

func (a *App) deleteMcp(w http.ResponseWriter, r *http.Request) {
	err := a.store.DeleteMcp(r.Context(), r.PathValue("id"))
	respondDelete(w, a.log, "mcp.Delete", err, store.ErrMcpNotFound)
}
