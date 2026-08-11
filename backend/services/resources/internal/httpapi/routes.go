// Package httpapi registers the Resources service routes, matching
// frontend/src/api/{knowledgeSources,plugins,rules,workspaceMcp}.ts. Every
// handler is scoped by workspace_id and gated on membership by the Gateway
// (X-Aaks-User-Id → membership check forwarded from Orgs; resources trusts
// the Gateway-injected X-Aaks-Workspace-Member header for MVP).
//
// Consumers: mcp.created/mcp.deleted (connection projection), workspace.created
// (default rule seed).
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/aaks/server/internal/contracts"
	"github.com/aaks/server/internal/httputil"
	"github.com/aaks/server/internal/kafka"
	"github.com/aaks/server/services/resources/internal/store"
)

// App holds the Resources service dependencies.
type App struct {
	store *store.Store
	log   *slog.Logger
}

// Register wires resources routes + consumers.
func Register(mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("RESOURCES_DB_DSN")
	if dsn == "" {
		return errors.New("RESOURCES_DB_DSN is not set")
	}
	st, err := store.New(context.Background(), dsn, log)
	if err != nil {
		return err
	}
	app := &App{store: st, log: log}

	mux.HandleFunc("GET /workspaces/{id}/knowledge", app.listKnowledge)
	mux.HandleFunc("POST /workspaces/{id}/knowledge", app.createKnowledge)
	mux.HandleFunc("GET /workspaces/{id}/plugins", app.listPlugins)
	mux.HandleFunc("PATCH /workspaces/{id}/plugins/{rid}", app.setPluginEnabled)
	mux.HandleFunc("GET /workspaces/{id}/rules", app.listRules)
	mux.HandleFunc("PATCH /workspaces/{id}/rules/{rid}", app.setRuleEnabled)
	mux.HandleFunc("GET /workspaces/{id}/mcp", app.listMcp)
	mux.HandleFunc("POST /workspaces/{id}/mcp/{rid}/reconnect", app.reconnectMcp)

	// Internal surface used by the Runner (guardrails + knowledge references).
	mux.HandleFunc("GET /internal/workspaces/{id}/enabled-rules", app.internalEnabledRules)

	app.startConsumers()

	log.Info("resources routes registered", "endpoints", 9)
	return nil
}

// startConsumers subscribes to mcp + workspace events (best-effort).
func (a *App) startConsumers() {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		return
	}
	bs := kafka.Brokers(strings.Split(brokers, ","))
	cg, err := kafka.NewConsumerGroup(bs, "resources-mcp", a.log)
	if err != nil {
		a.log.Warn("resources consumers unavailable", "error", err)
		return
	}
	go func() {
		if err := cg.Run(context.Background(),
			[]string{contracts.TopicMcpCreated, contracts.TopicMcpDeleted, contracts.TopicWorkspaceCreated},
			a.consume); err != nil {
			a.log.Error("resources consumer stopped", "error", err)
		}
	}()
}

// consume projects MCP connections and seeds default rules on workspace creation.
func (a *App) consume(ctx context.Context, env contracts.EventEnvelope) error {
	switch env.EventType {
	case contracts.TopicMcpCreated:
		var d contracts.McpCreatedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		return a.store.UpsertMcpConnection(ctx, d.McpServerID, d.WorkspaceID, d.Name)
	case contracts.TopicMcpDeleted:
		var d contracts.McpDeletedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		return a.store.DeleteMcpConnection(ctx, d.WorkspaceID, d.McpServerID)
	case contracts.TopicWorkspaceCreated:
		var d contracts.WorkspaceCreatedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		return a.seedWorkspace(ctx, d.WorkspaceID)
	}
	return nil
}

// seedWorkspace creates the default rule set for a new workspace (idempotent).
func (a *App) seedWorkspace(ctx context.Context, workspaceID contracts.ID) error {
	defaults := []struct{ name, desc string }{
		{"no-auto-merge", "never auto-merge pull requests"},
		{"review-before-merge", "require reviewer approval before merging"},
		{"test-gate", "run tests before merging"},
	}
	for _, d := range defaults {
		if err := a.store.CreateRule(ctx, workspaceID, d.name, d.desc, true); err != nil {
			return err
		}
	}
	return nil
}

// ── Handlers ────────────────────────────────────────────────────────────────

func (a *App) listKnowledge(w http.ResponseWriter, r *http.Request) {
	ks, err := a.store.ListKnowledge(r.Context(), contracts.ID(r.PathValue("id")))
	if err != nil {
		httputil.ServerError(w, a.log, "resources.ListKnowledge", err)
		return
	}
	if ks == nil {
		ks = []contracts.KnowledgeSource{}
	}
	httputil.WriteJSON(w, http.StatusOK, ks)
}

func (a *App) createKnowledge(w http.ResponseWriter, r *http.Request) {
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
	k, err := a.store.CreateKnowledge(r.Context(), contracts.ID(r.PathValue("id")), body.Title, body.Kind)
	if err != nil {
		httputil.ServerError(w, a.log, "resources.CreateKnowledge", err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, k)
}

func (a *App) listPlugins(w http.ResponseWriter, r *http.Request) {
	ps, err := a.store.ListPlugins(r.Context(), contracts.ID(r.PathValue("id")))
	if err != nil {
		httputil.ServerError(w, a.log, "resources.ListPlugins", err)
		return
	}
	if ps == nil {
		ps = []contracts.Plugin{}
	}
	httputil.WriteJSON(w, http.StatusOK, ps)
}

func (a *App) setPluginEnabled(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	p, err := a.store.SetPluginEnabled(r.Context(), contracts.ID(r.PathValue("id")), contracts.ID(r.PathValue("rid")), body.Enabled)
	if errors.Is(err, store.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "plugin not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, a.log, "resources.SetPluginEnabled", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, p)
}

func (a *App) listRules(w http.ResponseWriter, r *http.Request) {
	rs, err := a.store.ListRules(r.Context(), contracts.ID(r.PathValue("id")))
	if err != nil {
		httputil.ServerError(w, a.log, "resources.ListRules", err)
		return
	}
	if rs == nil {
		rs = []contracts.Rule{}
	}
	httputil.WriteJSON(w, http.StatusOK, rs)
}

func (a *App) setRuleEnabled(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	rule, err := a.store.SetRuleEnabled(r.Context(), contracts.ID(r.PathValue("id")), contracts.ID(r.PathValue("rid")), body.Enabled)
	if errors.Is(err, store.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "rule not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, a.log, "resources.SetRuleEnabled", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, rule)
}

func (a *App) listMcp(w http.ResponseWriter, r *http.Request) {
	conns, err := a.store.ListMcpConnections(r.Context(), contracts.ID(r.PathValue("id")))
	if err != nil {
		httputil.ServerError(w, a.log, "resources.ListMcp", err)
		return
	}
	if conns == nil {
		conns = []contracts.McpConnection{}
	}
	httputil.WriteJSON(w, http.StatusOK, conns)
}

func (a *App) reconnectMcp(w http.ResponseWriter, r *http.Request) {
	m, err := a.store.ReconnectMcpConnection(r.Context(), contracts.ID(r.PathValue("id")), contracts.ID(r.PathValue("rid")))
	if errors.Is(err, store.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "connection not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, a.log, "resources.ReconnectMcp", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, m)
}

func (a *App) internalEnabledRules(w http.ResponseWriter, r *http.Request) {
	rs, err := a.store.EnabledRules(r.Context(), contracts.ID(r.PathValue("id")))
	if err != nil {
		httputil.ServerError(w, a.log, "resources.EnabledRules", err)
		return
	}
	if rs == nil {
		rs = []contracts.Rule{}
	}
	httputil.WriteJSON(w, http.StatusOK, rs)
}
