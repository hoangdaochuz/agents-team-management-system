// Package httpapi registers the Admin service routes, matching
// frontend/src/api/{audit,sysadmin}.ts: workspace audit (list/export) and the
// sysadmin flags/audit/maintenance surface. The sysadmin orgs/requests half
// lives in the Orgs service; KPIs/health are composed by the Gateway.
//
// Consumer: audit.recorded (workspace admin actions from other services).
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/aaks/server/internal/contracts"
	"github.com/aaks/server/internal/platform/http"
	"github.com/aaks/server/internal/platform/tenancy"
	"github.com/aaks/server/internal/platform/kafka"
	"github.com/aaks/server/services/admin/internal/store"
)

// App holds the Admin service dependencies.
type App struct {
	store *store.Store
	log   *slog.Logger
}

// Register wires admin routes + the audit consumer.
func Register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("ADMIN_DB_DSN")
	if dsn == "" {
		return errors.New("ADMIN_DB_DSN is not set")
	}
	st, err := store.New(context.Background(), dsn, log)
	if err != nil {
		return err
	}
	app := &App{store: st, log: log}

	mux.HandleFunc("GET /workspaces/{id}/audit", app.listAudit)
	mux.HandleFunc("POST /workspaces/{id}/audit/export", app.exportAudit)

	// Sysadmin surface (admin half). The Gateway injects X-User-Superadmin.
	mux.HandleFunc("GET /sysadmin/flags", app.listFlags)
	mux.HandleFunc("PATCH /sysadmin/flags/{key}", app.toggleFlag)
	mux.HandleFunc("GET /sysadmin/audit", app.systemAudit)
	mux.HandleFunc("POST /sysadmin/maintenance", app.runMaintenance)

	app.startConsumers()

	log.Info("admin routes registered", "endpoints", 6)
	return nil
}

// startConsumers subscribes to audit.recorded (best-effort).
func (a *App) startConsumers() {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		return
	}
	bs := kafka.Brokers(strings.Split(brokers, ","))
	cg, err := kafka.NewConsumerGroup(bs, "admin-audit", a.log)
	if err != nil {
		a.log.Warn("admin consumers unavailable", "error", err)
		return
	}
	go func() {
		if err := cg.Run(context.Background(), []string{contracts.TopicAuditRecorded}, a.consume); err != nil {
			a.log.Error("admin consumer stopped", "error", err)
		}
	}()
}

// consume persists workspace audit events.
func (a *App) consume(ctx context.Context, env contracts.EventEnvelope) error {
	if env.EventType != contracts.TopicAuditRecorded {
		return nil
	}
	var d contracts.AuditRecordedData
	if err := env.DecodeData(&d); err != nil {
		return err
	}
	return a.store.AppendAudit(ctx, d.WorkspaceID, d.ActorName, d.ActorID, d.Action, d.ActionKind, d.Target, d.IP)
}

// ── Handlers ────────────────────────────────────────────────────────────────

func (a *App) listAudit(w http.ResponseWriter, r *http.Request) {
	wsID := contracts.ID(r.PathValue("id"))
	kind := r.URL.Query().Get("kind")
	rows, err := a.store.ListAudit(r.Context(), wsID, kind)
	if err != nil {
		httputil.ServerError(w, a.log, "admin.ListAudit", err)
		return
	}
	out := make([]contracts.AuditEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.AuditEntry)
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

// exportAudit is a stub (spec: export is out of scope; returns ok).
func (a *App) exportAudit(w http.ResponseWriter, r *http.Request) {
	wsID := contracts.ID(r.PathValue("id"))
	n, err := a.store.CountAudit24h(r.Context(), wsID)
	if err != nil {
		httputil.ServerError(w, a.log, "admin.ExportAudit", err)
		return
	}
	a.log.Info("audit export requested (stub)", "workspace", wsID, "entries_24h", n)
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) listFlags(w http.ResponseWriter, r *http.Request) {
	if !isSuperadmin(r) {
		httputil.Error(w, http.StatusForbidden, "superadmin required")
		return
	}
	flags, err := a.store.ListFlags(r.Context())
	if err != nil {
		httputil.ServerError(w, a.log, "admin.ListFlags", err)
		return
	}
	if flags == nil {
		flags = []contracts.FeatureFlag{}
	}
	httputil.WriteJSON(w, http.StatusOK, flags)
}

func (a *App) toggleFlag(w http.ResponseWriter, r *http.Request) {
	if !isSuperadmin(r) {
		httputil.Error(w, http.StatusForbidden, "superadmin required")
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	f, err := a.store.SetFlagEnabled(r.Context(), r.PathValue("key"), body.Enabled)
	if errors.Is(err, store.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "flag not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, a.log, "admin.ToggleFlag", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, f)
}

func (a *App) systemAudit(w http.ResponseWriter, r *http.Request) {
	if !isSuperadmin(r) {
		httputil.Error(w, http.StatusForbidden, "superadmin required")
		return
	}
	rows, err := a.store.ListSystemAudit(r.Context(), 200)
	if err != nil {
		httputil.ServerError(w, a.log, "admin.SystemAudit", err)
		return
	}
	out := make([]contracts.AuditEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.AuditEntry)
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

// runMaintenance is a stub (vacuum/compaction is out of scope for the MVP).
func (a *App) runMaintenance(w http.ResponseWriter, r *http.Request) {
	if !isSuperadmin(r) {
		httputil.Error(w, http.StatusForbidden, "superadmin required")
		return
	}
	a.log.Info("system maintenance requested (stub)")
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func isSuperadmin(r *http.Request) bool {
	return tenancy.UserSuperadmin(r)
}
