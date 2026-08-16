// Package httpapi registers the Catalog service routes: Skill CRUD + McpServer
// CRUD + the workspace-scoped skill surface, matching frontend/src/api/skills.ts
// + mcpServers.ts.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/IBM/sarama"

	"github.com/aaks/server/internal/contracts"
	"github.com/aaks/server/internal/platform/http"
	"github.com/aaks/server/internal/platform/tenancy"
	"github.com/aaks/server/internal/platform/kafka"
	"github.com/aaks/server/services/catalog/internal/store"
)

type App struct {
	store *store.Store
	prod  sarama.SyncProducer // nilable: definition events are best-effort
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
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		if p, err := kafka.NewProducer(kafka.Brokers(strings.Split(brokers, ",")), log); err != nil {
			log.Warn("kafka producer unavailable; catalog emits no definition events", "error", err)
		} else {
			app.prod = p
		}
	}

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
	// Workspace-scoped skill surface (resources screen; frontend skills.ts).
	mux.HandleFunc("GET /workspaces/{wid}/skills", app.listWorkspaceSkills)
	mux.HandleFunc("PATCH /workspaces/{wid}/skills/{id}", app.setSkillEnabled)

	// Internal: MCP server definitions by ID list (trusted callers, e.g. the
	// Agent service hydrating an agent's attached servers for the Runner).
	mux.HandleFunc("GET /internal/mcp-servers", app.listMcpByIDs)

	log.Info("catalog routes registered", "endpoints", 13)
	return nil
}

// publish emits a definition event; non-fatal when no producer is configured.
func (a *App) publish(ctx context.Context, topic string, data any, workspaceID contracts.ID) {
	if a.prod == nil {
		return
	}
	env := contracts.EventEnvelope{TaskID: workspaceID, Data: data}
	if err := kafka.Publish(ctx, a.prod, topic, env, a.log); err != nil {
		a.log.Error("publish definition event failed", "topic", topic, "error", err)
	}
}

// ── Skill handlers ──────────────────────────────────────────────────────────

func (a *App) listSkills(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.ListSkills(r.Context(), tenancy.WorkspaceIDs(r))
	respond(w, a.log, "skill.List", out, err)
}

func (a *App) getSkill(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.GetSkill(r.Context(), r.PathValue("id"), tenancy.WorkspaceIDs(r))
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
	ws := tenancy.WorkspaceID(r)
	if ws == "" {
		httputil.Error(w, http.StatusBadRequest, "no workspace context")
		return
	}
	out, err := a.store.CreateSkill(r.Context(), ws, in)
	if err == nil {
		a.publish(r.Context(), contracts.TopicSkillCreated,
			contracts.SkillCreatedData{SkillID: out.ID, WorkspaceID: out.WorkspaceID}, out.WorkspaceID)
	}
	respondCreated(w, a.log, "skill.Create", out, err)
}

func (a *App) updateSkill(w http.ResponseWriter, r *http.Request) {
	var in store.SkillUpdateInput
	if bad := decode(w, r, &in); bad {
		return
	}
	out, err := a.store.UpdateSkill(r.Context(), r.PathValue("id"), tenancy.WorkspaceIDs(r), in)
	respond(w, a.log, "skill.Update", out, err, store.ErrSkillNotFound)
}

func (a *App) deleteSkill(w http.ResponseWriter, r *http.Request) {
	err := a.store.DeleteSkill(r.Context(), r.PathValue("id"), tenancy.WorkspaceIDs(r))
	if err == nil {
		a.publish(r.Context(), contracts.TopicSkillDeleted,
			contracts.SkillDeletedData{SkillID: r.PathValue("id")}, "")
	}
	respondDelete(w, a.log, "skill.Delete", err, store.ErrSkillNotFound)
}

// listWorkspaceSkills serves GET /workspaces/{wid}/skills (scoped path).
func (a *App) listWorkspaceSkills(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.ListSkillsByWorkspace(r.Context(), r.PathValue("wid"))
	respond(w, a.log, "skill.ListForWorkspace", out, err)
}

// setSkillEnabled serves PATCH /workspaces/{wid}/skills/{id} {enabled}.
func (a *App) setSkillEnabled(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if bad := decode(w, r, &in); bad {
		return
	}
	if in.Enabled == nil {
		httputil.Error(w, http.StatusBadRequest, "enabled is required")
		return
	}
	out, err := a.store.SetSkillEnabled(r.Context(), r.PathValue("wid"), r.PathValue("id"), *in.Enabled)
	respond(w, a.log, "skill.SetEnabled", out, err, store.ErrSkillNotFound)
}

// ── MCP handlers ────────────────────────────────────────────────────────────

func (a *App) listMcp(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.ListMcp(r.Context(), tenancy.WorkspaceIDs(r))
	respond(w, a.log, "mcp.List", out, err)
}

// listMcpByIDs serves `GET /internal/mcp-servers?ids=a,b,c` — definitions for
// the listed IDs (internal trusted callers). Used by the Agent service to
// hydrate an agent's attached MCP servers for the Runner bridge (task 5.5).
func (a *App) listMcpByIDs(w http.ResponseWriter, r *http.Request) {
	ids := parseIDList(r.URL.Query().Get("ids"))
	out, err := a.store.ListMcpByIDs(r.Context(), ids)
	respond(w, a.log, "mcp.ListByIDs", out, err)
}

// parseIDList splits a comma-separated id list, trimming blanks.
func parseIDList(s string) []contracts.ID {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]contracts.ID, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, contracts.ID(p))
		}
	}
	return out
}

func (a *App) getMcp(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.GetMcp(r.Context(), r.PathValue("id"), tenancy.WorkspaceIDs(r))
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
	ws := tenancy.WorkspaceID(r)
	if ws == "" {
		httputil.Error(w, http.StatusBadRequest, "no workspace context")
		return
	}
	out, err := a.store.CreateMcp(r.Context(), ws, in)
	if err == nil {
		a.publish(r.Context(), contracts.TopicMcpCreated,
			contracts.McpCreatedData{McpServerID: out.ID, WorkspaceID: out.WorkspaceID,
				Name: out.Name, Command: out.Command, Args: out.Args, Env: out.Env}, out.WorkspaceID)
	}
	respondCreated(w, a.log, "mcp.Create", out, err)
}

func (a *App) updateMcp(w http.ResponseWriter, r *http.Request) {
	var in store.McpUpdateInput
	if bad := decode(w, r, &in); bad {
		return
	}
	out, err := a.store.UpdateMcp(r.Context(), r.PathValue("id"), tenancy.WorkspaceIDs(r), in)
	respond(w, a.log, "mcp.Update", out, err, store.ErrMcpNotFound)
}

func (a *App) deleteMcp(w http.ResponseWriter, r *http.Request) {
	err := a.store.DeleteMcp(r.Context(), r.PathValue("id"), tenancy.WorkspaceIDs(r))
	if err == nil {
		a.publish(r.Context(), contracts.TopicMcpDeleted,
			contracts.McpDeletedData{McpServerID: r.PathValue("id")}, "")
	}
	respondDelete(w, a.log, "mcp.Delete", err, store.ErrMcpNotFound)
}
