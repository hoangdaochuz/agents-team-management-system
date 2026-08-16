// Package http exposes the Admin use cases as thin HTTP handlers: decode →
// call application handler → encode. All business rules live in application.
// The sysadmin surface is gated on the Gateway-injected superadmin flag.
package http

import (
	"log/slog"
	"net/http"

	"github.com/aaks/server/internal/contracts/admin"
	"github.com/aaks/server/internal/contracts/identity"
	httputil "github.com/aaks/server/internal/platform/http"
	"github.com/aaks/server/internal/platform/tenancy"
	"github.com/aaks/server/services/admin/internal/application"
	"github.com/aaks/server/services/admin/internal/domain"
)

// Server wires the Admin routes to the application service.
type Server struct {
	app *application.App
	log *slog.Logger
}

// New builds the HTTP adapter.
func New(app *application.App, log *slog.Logger) *Server { return &Server{app: app, log: log} }

// Register mounts all Admin routes on mux, matching
// frontend/src/api/{audit,sysadmin}.ts.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /workspaces/{id}/audit", s.listAudit)
	mux.HandleFunc("POST /workspaces/{id}/audit/export", s.exportAudit)

	// Sysadmin surface (admin half). The Gateway injects X-User-Superadmin.
	mux.HandleFunc("GET /sysadmin/flags", s.listFlags)
	mux.HandleFunc("PATCH /sysadmin/flags/{key}", s.toggleFlag)
	mux.HandleFunc("GET /sysadmin/audit", s.systemAudit)
	mux.HandleFunc("POST /sysadmin/maintenance", s.runMaintenance)
}

// isSuperadmin reads the Gateway-injected superadmin flag.
func isSuperadmin(r *http.Request) bool { return tenancy.UserSuperadmin(r) }

func requireSuperadmin(w http.ResponseWriter, r *http.Request) bool {
	if !isSuperadmin(r) {
		httputil.Error(w, http.StatusForbidden, "superadmin required")
		return false
	}
	return true
}

// ── Audit ───────────────────────────────────────────────────────────────────

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	wsID := identity.ID(r.PathValue("id"))
	kind := r.URL.Query().Get("kind")
	rows, err := s.app.ListWorkspaceAudit(r.Context(), wsID, kind)
	if err != nil {
		httputil.ServerError(w, s.log, "admin.ListAudit", err)
		return
	}
	out := make([]admin.AuditEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.AuditEntry)
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

// exportAudit is a stub (spec: export is out of scope; returns ok).
func (s *Server) exportAudit(w http.ResponseWriter, r *http.Request) {
	wsID := identity.ID(r.PathValue("id"))
	n, err := s.app.CountAudit24h(r.Context(), wsID)
	if err != nil {
		httputil.ServerError(w, s.log, "admin.ExportAudit", err)
		return
	}
	s.log.Info("audit export requested (stub)", "workspace", wsID, "entries_24h", n)
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ── Sysadmin ────────────────────────────────────────────────────────────────

func (s *Server) listFlags(w http.ResponseWriter, r *http.Request) {
	if !requireSuperadmin(w, r) {
		return
	}
	out, err := s.app.ListFlags(r.Context())
	httputil.RespondOK(w, s.log, "admin.ListFlags", out, err)
}

func (s *Server) toggleFlag(w http.ResponseWriter, r *http.Request) {
	if !requireSuperadmin(w, r) {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	out, err := s.app.SetFlagEnabled(r.Context(), r.PathValue("key"), body.Enabled)
	httputil.RespondOK(w, s.log, "admin.ToggleFlag", out, err, domain.ErrNotFound)
}

func (s *Server) systemAudit(w http.ResponseWriter, r *http.Request) {
	if !requireSuperadmin(w, r) {
		return
	}
	rows, err := s.app.ListSystemAudit(r.Context(), 200)
	if err != nil {
		httputil.ServerError(w, s.log, "admin.SystemAudit", err)
		return
	}
	out := make([]admin.AuditEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.AuditEntry)
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

// runMaintenance is a stub (vacuum/compaction is out of scope for the MVP).
func (s *Server) runMaintenance(w http.ResponseWriter, r *http.Request) {
	if !requireSuperadmin(w, r) {
		return
	}
	s.log.Info("system maintenance requested (stub)")
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
