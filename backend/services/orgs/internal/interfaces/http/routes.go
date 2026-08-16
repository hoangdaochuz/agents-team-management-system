// Package http exposes the Orgs use cases as thin HTTP handlers: decode →
// call application handler → encode. All business rules live in application.
package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/workspaces"
	httputil "github.com/aaks/server/internal/platform/http"
	"github.com/aaks/server/internal/platform/tenancy"
	"github.com/aaks/server/services/orgs/internal/application"
	"github.com/aaks/server/services/orgs/internal/domain"
)

// Server wires the Orgs routes to the application service.
type Server struct {
	app *application.App
	log *slog.Logger
}

// New builds the HTTP adapter.
func New(app *application.App, log *slog.Logger) *Server { return &Server{app: app, log: log} }

// Register mounts all Orgs routes on mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /workspaces", s.listWorkspaces)
	mux.HandleFunc("GET /workspaces/{id}", s.getWorkspace)
	mux.HandleFunc("POST /workspaces", s.createWorkspace)

	mux.HandleFunc("GET /workspaces/{id}/members", s.listMembers)
	mux.HandleFunc("PATCH /workspaces/{id}/members/{mid}", s.updateMemberRole)
	mux.HandleFunc("DELETE /workspaces/{id}/members/{mid}", s.removeMember)
	mux.HandleFunc("POST /workspaces/{id}/members/{mid}/resend", s.resendInvite)

	mux.HandleFunc("GET /workspaces/{id}/requests", s.listPendingRequests)
	mux.HandleFunc("POST /workspaces/{id}/requests/{rid}/approve", s.approveRequest)
	mux.HandleFunc("POST /workspaces/{id}/requests/{rid}/decline", s.declineRequest)
	mux.HandleFunc("POST /workspaces/{id}/invites", s.sendInvites)

	// Sysadmin surface (orgs half). The Gateway injects X-User-Superadmin.
	mux.HandleFunc("GET /sysadmin/orgs", s.listOrgs)
	mux.HandleFunc("POST /sysadmin/orgs", s.createOrg)
	mux.HandleFunc("POST /sysadmin/orgs/{id}/suspend", s.suspendOrg)
	mux.HandleFunc("POST /sysadmin/orgs/{id}/restore", s.restoreOrg)
	mux.HandleFunc("GET /sysadmin/requests", s.listOrgRequests)
	mux.HandleFunc("POST /sysadmin/requests/{rid}/approve", s.approveOrgRequest)

	// Internal surface used only by the Gateway (Session + KPI composition).
	mux.HandleFunc("GET /internal/users/{uid}/workspaces", s.internalUserWorkspaces)
	mux.HandleFunc("GET /internal/stats", s.internalStats)
}

// ── Identity helpers ────────────────────────────────────────────────────────

// userID extracts the Gateway-injected identity (X-User-ID, scoping contract).
func userID(r *http.Request) (identity.ID, error) {
	id := tenancy.UserID(r)
	if id == "" {
		return "", errMissingIdentity
	}
	return id, nil
}

var errMissingIdentity = errors.New("missing X-User-ID")

// isSuperadmin reads the Gateway-injected superadmin flag.
func isSuperadmin(r *http.Request) bool { return tenancy.UserSuperadmin(r) }

// requireMember verifies active membership and returns the caller's role.
func (s *Server) requireMember(r *http.Request, workspaceID identity.ID) (identity.ID, identity.Role, error) {
	uid, err := userID(r)
	if err != nil {
		return "", "", err
	}
	role, err := s.app.RequireMember(r.Context(), uid, workspaceID)
	if err != nil {
		return "", "", err
	}
	return uid, role, nil
}

// requireAdmin is requireMember plus an admin/owner role gate.
func (s *Server) requireAdmin(r *http.Request, workspaceID identity.ID) (identity.ID, error) {
	uid, err := userID(r)
	if err != nil {
		return "", err
	}
	if err := s.app.RequireAdmin(r.Context(), uid, workspaceID); err != nil {
		return "", err
	}
	return uid, nil
}

func writeMemberErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotMember):
		httputil.Error(w, http.StatusForbidden, "not a member of this workspace")
	case errors.Is(err, domain.ErrForbidden):
		httputil.Error(w, http.StatusForbidden, "admin role required")
	default:
		httputil.Error(w, http.StatusUnauthorized, "missing user identity")
	}
}

// ── Workspaces ──────────────────────────────────────────────────────────────

func (s *Server) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	uid, err := userID(r)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, "missing user identity")
		return
	}
	wss, err := s.app.ListWorkspaces(r.Context(), uid)
	if err != nil {
		httputil.ServerError(w, s.log, "orgs.ListWorkspaces", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, wss)
}

func (s *Server) getWorkspace(w http.ResponseWriter, r *http.Request) {
	uid, err := userID(r)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, "missing user identity")
		return
	}
	ws, err := s.app.GetWorkspace(r.Context(), uid, identity.ID(r.PathValue("id")))
	if errors.Is(err, domain.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "workspace not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, s.log, "orgs.GetWorkspace", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, ws)
}

func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request) {
	uid, err := userID(r)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, "missing user identity")
		return
	}
	var body struct {
		Name          string        `json:"name"`
		RepoSource    string        `json:"repo_source"`
		DefaultBranch string        `json:"default_branch"`
		Role          identity.Role `json:"role"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	if body.Name == "" {
		httputil.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	ws, err := s.app.CreateWorkspace(r.Context(), uid, body.Name, body.RepoSource, body.DefaultBranch)
	if err != nil {
		httputil.ServerError(w, s.log, "orgs.CreateWorkspace", err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, ws)
}

// ── Members ─────────────────────────────────────────────────────────────────

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request) {
	wsID := identity.ID(r.PathValue("id"))
	if _, _, err := s.requireMember(r, wsID); err != nil {
		writeMemberErr(w, err)
		return
	}
	rows, err := s.app.ListMembers(r.Context(), wsID)
	if err != nil {
		httputil.ServerError(w, s.log, "orgs.ListMembers", err)
		return
	}
	out := make([]workspaces.Member, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.Member)
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) updateMemberRole(w http.ResponseWriter, r *http.Request) {
	wsID := identity.ID(r.PathValue("id"))
	mid := identity.ID(r.PathValue("mid"))
	uid, err := s.requireAdmin(r, wsID)
	if err != nil {
		writeMemberErr(w, err)
		return
	}
	var body struct {
		Role identity.Role `json:"role"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	if body.Role != identity.RoleOwner && body.Role != identity.RoleAdmin && body.Role != identity.RoleMember {
		httputil.Error(w, http.StatusBadRequest, "role must be owner, admin or member")
		return
	}
	target, err := s.app.UpdateRole(r.Context(), wsID, mid, body.Role)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httputil.Error(w, http.StatusNotFound, "member not found")
	case errors.Is(err, domain.ErrLastOwner):
		httputil.Error(w, http.StatusConflict, "cannot demote the last owner")
	case err != nil:
		httputil.ServerError(w, s.log, "orgs.SetMemberRole", err)
	default:
		s.app.Audit(r.Context(), uid, wsID, "member.role-changed", "member", string(target.UserID))
		httputil.WriteJSON(w, http.StatusOK, target.Member)
	}
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	wsID := identity.ID(r.PathValue("id"))
	mid := identity.ID(r.PathValue("mid"))
	uid, err := s.requireAdmin(r, wsID)
	if err != nil {
		writeMemberErr(w, err)
		return
	}
	err = s.app.Remove(r.Context(), uid, wsID, mid)
	switch {
	case errors.Is(err, application.ErrSelfRemoval):
		httputil.Error(w, http.StatusBadRequest, "cannot remove yourself")
	case errors.Is(err, domain.ErrNotFound):
		httputil.Error(w, http.StatusNotFound, "member not found")
	case errors.Is(err, domain.ErrLastOwner):
		httputil.Error(w, http.StatusConflict, "cannot remove the last owner")
	case err != nil:
		httputil.ServerError(w, s.log, "orgs.RemoveMember", err)
	default:
		s.app.Audit(r.Context(), uid, wsID, "member.removed", "member", string(mid))
		w.WriteHeader(http.StatusNoContent)
	}
}

// resendInvite is idempotent (notifications are out of scope for the MVP).
func (s *Server) resendInvite(w http.ResponseWriter, r *http.Request) {
	wsID := identity.ID(r.PathValue("id"))
	if _, err := s.requireAdmin(r, wsID); err != nil {
		writeMemberErr(w, err)
		return
	}
	s.log.Info("invite notification resent (stub)", "workspace", wsID, "member", r.PathValue("mid"))
	w.WriteHeader(http.StatusNoContent)
}

// ── Invites / join requests ─────────────────────────────────────────────────

func (s *Server) listPendingRequests(w http.ResponseWriter, r *http.Request) {
	wsID := identity.ID(r.PathValue("id"))
	if _, err := s.requireAdmin(r, wsID); err != nil {
		writeMemberErr(w, err)
		return
	}
	out, err := s.app.ListPendingRequests(r.Context(), wsID)
	if err != nil {
		httputil.ServerError(w, s.log, "orgs.ListRequests", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) approveRequest(w http.ResponseWriter, r *http.Request) {
	wsID := identity.ID(r.PathValue("id"))
	uid, err := s.requireAdmin(r, wsID)
	if err != nil {
		writeMemberErr(w, err)
		return
	}
	rid := identity.ID(r.PathValue("rid"))
	err = s.app.ApproveJoinRequest(r.Context(), uid, wsID, rid)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httputil.Error(w, http.StatusNotFound, "request not found")
	case errors.Is(err, domain.ErrNotPending):
		httputil.Error(w, http.StatusConflict, "request is not pending")
	case err != nil:
		httputil.ServerError(w, s.log, "orgs.ApproveRequest", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) declineRequest(w http.ResponseWriter, r *http.Request) {
	wsID := identity.ID(r.PathValue("id"))
	if _, err := s.requireAdmin(r, wsID); err != nil {
		writeMemberErr(w, err)
		return
	}
	rid := identity.ID(r.PathValue("rid"))
	err := s.app.DeclineJoinRequest(r.Context(), wsID, rid)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httputil.Error(w, http.StatusNotFound, "request not found")
	case err != nil:
		httputil.ServerError(w, s.log, "orgs.DeclineRequest", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) sendInvites(w http.ResponseWriter, r *http.Request) {
	wsID := identity.ID(r.PathValue("id"))
	if _, err := s.requireAdmin(r, wsID); err != nil {
		writeMemberErr(w, err)
		return
	}
	var body struct {
		Emails []string      `json:"emails"`
		Role   identity.Role `json:"role"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	if len(body.Emails) == 0 {
		httputil.Error(w, http.StatusBadRequest, "emails is required")
		return
	}
	invites, err := s.app.SendInvites(r.Context(), wsID, body.Emails, body.Role)
	if err != nil {
		httputil.ServerError(w, s.log, "orgs.SendInvites", err)
		return
	}
	out := make([]workspaces.Invite, 0, len(invites))
	for _, inv := range invites {
		out = append(out, inv.Invite)
	}
	httputil.WriteJSON(w, http.StatusCreated, out)
}

// ── Sysadmin (orgs half) ────────────────────────────────────────────────────

func (s *Server) listOrgs(w http.ResponseWriter, r *http.Request) {
	if !isSuperadmin(r) {
		httputil.Error(w, http.StatusForbidden, "superadmin required")
		return
	}
	orgs, err := s.app.ListOrgs(r.Context())
	if err != nil {
		httputil.ServerError(w, s.log, "orgs.ListOrgs", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, orgs)
}

func (s *Server) createOrg(w http.ResponseWriter, r *http.Request) {
	if !isSuperadmin(r) {
		httputil.Error(w, http.StatusForbidden, "superadmin required")
		return
	}
	var body struct {
		Name string        `json:"name"`
		Plan identity.Plan `json:"plan"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	if body.Name == "" {
		httputil.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	org, err := s.app.CreateOrg(r.Context(), body.Name, body.Plan)
	if err != nil {
		httputil.ServerError(w, s.log, "orgs.CreateOrg", err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, org)
}

func (s *Server) suspendOrg(w http.ResponseWriter, r *http.Request) {
	s.setOrgStatus(w, r, identity.OrgSuspended)
}

func (s *Server) restoreOrg(w http.ResponseWriter, r *http.Request) {
	s.setOrgStatus(w, r, identity.OrgActive)
}

func (s *Server) setOrgStatus(w http.ResponseWriter, r *http.Request, status identity.OrgStatus) {
	if !isSuperadmin(r) {
		httputil.Error(w, http.StatusForbidden, "superadmin required")
		return
	}
	org, err := s.app.SetOrgStatus(r.Context(), identity.ID(r.PathValue("id")), status)
	if errors.Is(err, domain.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "organization not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, s.log, "orgs.SetOrgStatus", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, org)
}

func (s *Server) listOrgRequests(w http.ResponseWriter, r *http.Request) {
	if !isSuperadmin(r) {
		httputil.Error(w, http.StatusForbidden, "superadmin required")
		return
	}
	out, err := s.app.ListOrgRequests(r.Context())
	if err != nil {
		httputil.ServerError(w, s.log, "orgs.ListOrgRequests", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) approveOrgRequest(w http.ResponseWriter, r *http.Request) {
	if !isSuperadmin(r) {
		httputil.Error(w, http.StatusForbidden, "superadmin required")
		return
	}
	rid := identity.ID(r.PathValue("rid"))
	err := s.app.ApproveOrgRequest(r.Context(), rid)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httputil.Error(w, http.StatusNotFound, "request not found")
	case errors.Is(err, domain.ErrNotPending):
		httputil.Error(w, http.StatusConflict, "request is not pending")
	case err != nil:
		httputil.ServerError(w, s.log, "orgs.ApproveOrgRequest", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// ── Internal (Gateway composition) ──────────────────────────────────────────

func (s *Server) internalUserWorkspaces(w http.ResponseWriter, r *http.Request) {
	wss, err := s.app.InternalWorkspaces(r.Context(), identity.ID(r.PathValue("uid")))
	if err != nil {
		httputil.ServerError(w, s.log, "orgs.InternalWorkspaces", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, wss)
}

func (s *Server) internalStats(w http.ResponseWriter, r *http.Request) {
	orgs, wss, seats, err := s.app.Stats(r.Context())
	if err != nil {
		httputil.ServerError(w, s.log, "orgs.InternalStats", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]int{
		"organizations": orgs, "workspaces": wss, "open_seats": seats,
	})
}
