// Package httpapi registers the Orgs/Workspaces service routes, matching
// frontend/src/api/{workspaces,members,invites,sysadmin}.ts: workspace CRUD,
// members (list/role/remove/resend), invites (pending join requests,
// approve/decline, send), and the orgs half of the sysadmin surface.
//
// User identity is injected by the Gateway per the scoping contract
// (X-User-ID, X-User-Superadmin — resolved from the session cookie against the
// Auth service). Every /workspaces/:id/... handler verifies membership first
// (task 11.3).
//
// Producers: workspace.created, invite.created, signup.approved/declined
// (join mode for approve/decline, create mode on sysadmin approval).
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
	"github.com/aaks/server/internal/httputil"
	"github.com/aaks/server/internal/kafka"
	"github.com/aaks/server/services/orgs/internal/store"
)

// App holds the Orgs service dependencies.
type App struct {
	store *store.Store
	prod  sarama.SyncProducer
	log   *slog.Logger
}

// Register wires orgs routes + the Kafka producer.
func Register(mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("ORGS_DB_DSN")
	if dsn == "" {
		return errors.New("ORGS_DB_DSN is not set")
	}
	st, err := store.New(context.Background(), dsn, log)
	if err != nil {
		return err
	}
	app := &App{store: st, log: log}
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		if p, err := kafka.NewProducer(kafka.Brokers(strings.Split(brokers, ",")), log); err != nil {
			log.Warn("kafka producer unavailable; orgs emits no workspace/invite/signup events", "error", err)
		} else {
			app.prod = p
		}
	}

	mux.HandleFunc("GET /workspaces", app.listWorkspaces)
	mux.HandleFunc("GET /workspaces/{id}", app.getWorkspace)
	mux.HandleFunc("POST /workspaces", app.createWorkspace)

	mux.HandleFunc("GET /workspaces/{id}/members", app.listMembers)
	mux.HandleFunc("PATCH /workspaces/{id}/members/{mid}", app.updateMemberRole)
	mux.HandleFunc("DELETE /workspaces/{id}/members/{mid}", app.removeMember)
	mux.HandleFunc("POST /workspaces/{id}/members/{mid}/resend", app.resendInvite)

	mux.HandleFunc("GET /workspaces/{id}/requests", app.listPendingRequests)
	mux.HandleFunc("POST /workspaces/{id}/requests/{rid}/approve", app.approveRequest)
	mux.HandleFunc("POST /workspaces/{id}/requests/{rid}/decline", app.declineRequest)
	mux.HandleFunc("POST /workspaces/{id}/invites", app.sendInvites)

	// Sysadmin surface (orgs half). The Gateway injects X-User-Superadmin.
	mux.HandleFunc("GET /sysadmin/orgs", app.listOrgs)
	mux.HandleFunc("POST /sysadmin/orgs", app.createOrg)
	mux.HandleFunc("POST /sysadmin/orgs/{id}/suspend", app.suspendOrg)
	mux.HandleFunc("POST /sysadmin/orgs/{id}/restore", app.restoreOrg)
	mux.HandleFunc("GET /sysadmin/requests", app.listOrgRequests)
	mux.HandleFunc("POST /sysadmin/requests/{rid}/approve", app.approveOrgRequest)

	// Internal surface used only by the Gateway (Session + KPI composition).
	mux.HandleFunc("GET /internal/users/{uid}/workspaces", app.internalUserWorkspaces)
	mux.HandleFunc("GET /internal/stats", app.internalStats)

	app.startConsumers()

	log.Info("orgs routes registered", "endpoints", 19)
	return nil
}

// startConsumers subscribes to signup.requested (create mode) so create-mode
// requests surface in /sysadmin/requests for approval. Idempotent: the store
// upsert ignores duplicates on redelivery.
func (a *App) startConsumers() {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		return
	}
	cg, err := kafka.NewConsumerGroup(kafka.Brokers(strings.Split(brokers, ",")), "orgs-signups", a.log)
	if err != nil {
		a.log.Warn("orgs consumers unavailable", "error", err)
		return
	}
	go func() {
		if err := cg.Run(context.Background(), []string{contracts.TopicSignupRequested}, a.consume); err != nil {
			a.log.Error("orgs consumer stopped", "error", err)
		}
	}()
}

// consume projects signup requests into the orgs DB: create-mode requests
// surface under /sysadmin/requests, join-mode under /workspaces/{id}/requests.
func (a *App) consume(ctx context.Context, env contracts.EventEnvelope) error {
	var d contracts.SignupRequestedData
	if err := env.DecodeData(&d); err != nil {
		return err
	}
	if d.Mode == "create" {
		return a.store.UpsertOrgRequest(ctx, d)
	}
	return a.store.UpsertJoinRequest(ctx, d)
}

// ── Identity helpers ────────────────────────────────────────────────────────

// userID extracts the Gateway-injected identity (X-User-ID, scoping contract).
func userID(r *http.Request) (contracts.ID, error) {
	id := httputil.UserID(r)
	if id == "" {
		return "", errors.New("missing X-User-ID")
	}
	return id, nil
}

func isSuperadmin(r *http.Request) bool {
	return httputil.UserSuperadmin(r)
}

// requireMember resolves the caller and verifies active membership in the
// workspace, returning the caller's role. 403 when not a member.
func (a *App) requireMember(r *http.Request, workspaceID contracts.ID) (contracts.ID, contracts.Role, error) {
	uid, err := userID(r)
	if err != nil {
		return "", "", err
	}
	role, err := a.store.UserRoleIn(r.Context(), uid, workspaceID)
	if errors.Is(err, store.ErrNotFound) {
		return "", "", errNotMember
	}
	if err != nil {
		return "", "", err
	}
	return uid, role, nil
}

var errNotMember = errors.New("not a member of this workspace")

// requireAdmin is requireMember plus an admin/owner role gate.
func (a *App) requireAdmin(r *http.Request, workspaceID contracts.ID) (contracts.ID, error) {
	uid, role, err := a.requireMember(r, workspaceID)
	if err != nil {
		return "", err
	}
	if role != contracts.RoleOwner && role != contracts.RoleAdmin {
		return "", errForbidden
	}
	return uid, nil
}

var errForbidden = errors.New("admin role required")

// ── Workspaces ──────────────────────────────────────────────────────────────

// listWorkspaces returns the caller's workspaces with their role.
func (a *App) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	uid, err := userID(r)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, "missing user identity")
		return
	}
	wss, err := a.store.ListUserWorkspaces(r.Context(), uid)
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.ListWorkspaces", err)
		return
	}
	if wss == nil {
		wss = []contracts.Workspace{}
	}
	httputil.WriteJSON(w, http.StatusOK, wss)
}

// getWorkspace returns one workspace the caller belongs to.
func (a *App) getWorkspace(w http.ResponseWriter, r *http.Request) {
	uid, err := userID(r)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, "missing user identity")
		return
	}
	ws, err := a.store.GetUserWorkspace(r.Context(), uid, contracts.ID(r.PathValue("id")))
	if errors.Is(err, store.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "workspace not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.GetWorkspace", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, ws)
}

// createWorkspace creates a workspace under the caller's organization (the
// organization is auto-created on first workspace), makes the caller owner,
// and emits workspace.created.
func (a *App) createWorkspace(w http.ResponseWriter, r *http.Request) {
	uid, err := userID(r)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, "missing user identity")
		return
	}
	var body struct {
		Name          string         `json:"name"`
		RepoSource    string         `json:"repo_source"`
		DefaultBranch string         `json:"default_branch"`
		Role          contracts.Role `json:"role"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	if body.Name == "" {
		httputil.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	if body.Role != contracts.RoleOwner && body.Role != contracts.RoleAdmin {
		body.Role = contracts.RoleOwner
	}

	org, err := a.store.OrgForUser(r.Context(), uid)
	if errors.Is(err, store.ErrNoOrg) {
		org, err = a.store.CreateOrg(r.Context(), uid, "", contracts.PlanFree)
	}
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.CreateWorkspace.Org", err)
		return
	}

	ws, err := a.store.CreateWorkspace(r.Context(), org.ID, body.Name, body.RepoSource, body.DefaultBranch, "", "")
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.CreateWorkspace", err)
		return
	}
	if _, err := a.store.AddMembership(r.Context(), ws.ID, uid, "", "", contracts.RoleOwner); err != nil {
		httputil.ServerError(w, a.log, "orgs.CreateWorkspace.Member", err)
		return
	}
	a.publish(r.Context(), contracts.TopicWorkspaceCreated, contracts.WorkspaceCreatedData{
		WorkspaceID: ws.ID, Name: ws.Name, RepoSource: ws.RepoSource, DefaultBranch: ws.DefaultBranch,
	}, ws.ID)

	ws.Role = contracts.RoleOwner
	httputil.WriteJSON(w, http.StatusCreated, ws)
}

// ── Members ─────────────────────────────────────────────────────────────────

func (a *App) listMembers(w http.ResponseWriter, r *http.Request) {
	wsID := contracts.ID(r.PathValue("id"))
	if _, _, err := a.requireMember(r, wsID); err != nil {
		a.writeMemberErr(w, err)
		return
	}
	rows, err := a.store.ListMembers(r.Context(), wsID)
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.ListMembers", err)
		return
	}
	out := make([]contracts.Member, 0, len(rows))
	for _, m := range rows {
		out = append(out, toMember(m))
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (a *App) updateMemberRole(w http.ResponseWriter, r *http.Request) {
	wsID := contracts.ID(r.PathValue("id"))
	mid := contracts.ID(r.PathValue("mid"))
	if _, err := a.requireAdmin(r, wsID); err != nil {
		a.writeMemberErr(w, err)
		return
	}
	var body struct {
		Role contracts.Role `json:"role"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	if body.Role != contracts.RoleOwner && body.Role != contracts.RoleAdmin && body.Role != contracts.RoleMember {
		httputil.Error(w, http.StatusBadRequest, "role must be owner, admin or member")
		return
	}
	target, err := a.store.SetMemberRole(r.Context(), wsID, mid, body.Role)
	if errors.Is(err, store.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "member not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.SetMemberRole", err)
		return
	}
	if err := a.checkLastOwner(w, r, wsID, target); err != nil {
		return
	}
	a.audit(r, wsID, "member.role-changed", "member", string(target.UserID))
	httputil.WriteJSON(w, http.StatusOK, toMember(target))
}

func (a *App) removeMember(w http.ResponseWriter, r *http.Request) {
	wsID := contracts.ID(r.PathValue("id"))
	mid := contracts.ID(r.PathValue("mid"))
	uid, err := a.requireAdmin(r, wsID)
	if err != nil {
		a.writeMemberErr(w, err)
		return
	}
	if mid == uid {
		httputil.Error(w, http.StatusBadRequest, "cannot remove yourself")
		return
	}
	members, err := a.store.ListMembers(r.Context(), wsID)
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.RemoveMember.List", err)
		return
	}
	var target *store.MemberRow
	for i := range members {
		if members[i].ID == mid {
			target = &members[i]
		}
	}
	if target == nil {
		httputil.Error(w, http.StatusNotFound, "member not found")
		return
	}
	if target.Role == contracts.RoleOwner {
		n, err := a.store.OwnerCount(r.Context(), wsID)
		if err != nil {
			httputil.ServerError(w, a.log, "orgs.RemoveMember.Owners", err)
			return
		}
		if n <= 1 {
			httputil.Error(w, http.StatusConflict, "cannot remove the last owner")
			return
		}
	}
	if err := a.store.RemoveMember(r.Context(), wsID, mid); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httputil.Error(w, http.StatusNotFound, "member not found")
			return
		}
		httputil.ServerError(w, a.log, "orgs.RemoveMember", err)
		return
	}
	a.audit(r, wsID, "member.removed", "member", string(mid))
	w.WriteHeader(http.StatusNoContent)
}

// resendInvite is idempotent (notifications are out of scope for the MVP).
func (a *App) resendInvite(w http.ResponseWriter, r *http.Request) {
	wsID := contracts.ID(r.PathValue("id"))
	if _, err := a.requireAdmin(r, wsID); err != nil {
		a.writeMemberErr(w, err)
		return
	}
	a.log.Info("invite notification resent (stub)", "workspace", wsID, "member", r.PathValue("mid"))
	w.WriteHeader(http.StatusNoContent)
}

// checkLastOwner reverts a demotion that would leave a workspace without owners.
func (a *App) checkLastOwner(w http.ResponseWriter, r *http.Request, wsID contracts.ID, target store.MemberRow) error {
	if target.Role != contracts.RoleOwner {
		n, err := a.store.OwnerCount(r.Context(), wsID)
		if err != nil {
			httputil.ServerError(w, a.log, "orgs.CheckLastOwner", err)
			return err
		}
		if n == 0 {
			if _, err := a.store.SetMemberRole(r.Context(), wsID, target.ID, contracts.RoleOwner); err != nil {
				httputil.ServerError(w, a.log, "orgs.CheckLastOwner.Revert", err)
				return err
			}
			httputil.Error(w, http.StatusConflict, "cannot demote the last owner")
			return errLastOwner
		}
	}
	return nil
}

var errLastOwner = errors.New("last owner protection")

func (a *App) writeMemberErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNotMember):
		httputil.Error(w, http.StatusForbidden, "not a member of this workspace")
	case errors.Is(err, errForbidden):
		httputil.Error(w, http.StatusForbidden, "admin role required")
	default:
		httputil.Error(w, http.StatusUnauthorized, "missing user identity")
	}
}

func toMember(m store.MemberRow) contracts.Member {
	out := contracts.Member{
		ID:     m.ID,
		User:   m.User,
		Role:   m.Role,
		Status: m.Status,
	}
	if m.LastActiveAt != nil {
		out.LastActiveAt = (*contracts.ISOTime)(m.LastActiveAt)
	}
	if m.IsServiceAccount != nil {
		out.IsServiceAccount = m.IsServiceAccount
	}
	return out
}

// ── Invites / join requests ─────────────────────────────────────────────────

// listPendingRequests returns pending join-mode signup requests (admin+).
func (a *App) listPendingRequests(w http.ResponseWriter, r *http.Request) {
	wsID := contracts.ID(r.PathValue("id"))
	if _, err := a.requireAdmin(r, wsID); err != nil {
		a.writeMemberErr(w, err)
		return
	}
	rows, err := a.store.ListPendingJoinRequests(r.Context(), wsID)
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.ListRequests", err)
		return
	}
	out := make([]contracts.SignupRequest, 0, len(rows))
	for _, jr := range rows {
		out = append(out, jr.SignupRequest)
	}
	if out == nil {
		out = []contracts.SignupRequest{}
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (a *App) approveRequest(w http.ResponseWriter, r *http.Request) {
	wsID := contracts.ID(r.PathValue("id"))
	if _, err := a.requireAdmin(r, wsID); err != nil {
		a.writeMemberErr(w, err)
		return
	}
	rid := contracts.ID(r.PathValue("rid"))
	jr, err := a.store.GetJoinRequest(r.Context(), rid)
	if errors.Is(err, store.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "request not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.ApproveRequest.Get", err)
		return
	}
	if jr.Status != contracts.SignupPending {
		httputil.Error(w, http.StatusConflict, "request is not pending")
		return
	}
	if _, err := a.store.AddMembership(r.Context(), jr.WorkspaceID, jr.UserID, jr.Name, jr.Email, jr.RequestedRole); err != nil {
		httputil.ServerError(w, a.log, "orgs.ApproveRequest.Member", err)
		return
	}
	if err := a.store.SetJoinRequestStatus(r.Context(), rid, contracts.SignupApproved); err != nil {
		httputil.ServerError(w, a.log, "orgs.ApproveRequest.Set", err)
		return
	}
	a.publish(r.Context(), contracts.TopicSignupApproved, contracts.SignupApprovedData{
		RequestID: rid, UserID: jr.UserID, Email: jr.Email, Name: jr.Name,
		Mode: "join", WorkspaceID: jr.WorkspaceID, Role: jr.RequestedRole,
	}, jr.UserID)
	a.audit(r, wsID, "signup.approved", "join-request", string(rid))
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) declineRequest(w http.ResponseWriter, r *http.Request) {
	wsID := contracts.ID(r.PathValue("id"))
	if _, err := a.requireAdmin(r, wsID); err != nil {
		a.writeMemberErr(w, err)
		return
	}
	rid := contracts.ID(r.PathValue("rid"))
	jr, err := a.store.GetJoinRequest(r.Context(), rid)
	if errors.Is(err, store.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "request not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.DeclineRequest.Get", err)
		return
	}
	if err := a.store.SetJoinRequestStatus(r.Context(), rid, contracts.SignupDeclined); err != nil {
		httputil.ServerError(w, a.log, "orgs.DeclineRequest.Set", err)
		return
	}
	a.publish(r.Context(), contracts.TopicSignupDeclined, contracts.SignupDeclinedData{
		RequestID: rid, UserID: jr.UserID,
	}, jr.UserID)
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) sendInvites(w http.ResponseWriter, r *http.Request) {
	wsID := contracts.ID(r.PathValue("id"))
	if _, err := a.requireAdmin(r, wsID); err != nil {
		a.writeMemberErr(w, err)
		return
	}
	var body struct {
		Emails []string       `json:"emails"`
		Role   contracts.Role `json:"role"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	if len(body.Emails) == 0 {
		httputil.Error(w, http.StatusBadRequest, "emails is required")
		return
	}
	if body.Role != contracts.RoleAdmin && body.Role != contracts.RoleMember {
		body.Role = contracts.RoleMember
	}
	ws, err := a.store.WorkspaceByID(r.Context(), wsID)
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.SendInvites.Get", err)
		return
	}
	invites, err := a.store.CreateInvites(r.Context(), wsID, body.Emails, body.Role)
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.SendInvites", err)
		return
	}
	out := make([]contracts.Invite, 0, len(invites))
	for _, inv := range invites {
		a.publish(r.Context(), contracts.TopicInviteCreated, contracts.InviteCreatedData{
			InviteID: inv.ID, Email: inv.Email, Role: inv.Role,
			InviteCode: inv.InviteCode, WorkspaceID: wsID, WorkspaceName: ws.Name,
		}, wsID)
		out = append(out, contracts.Invite{ID: inv.ID, Email: inv.Email, Role: inv.Role, RequestedAt: inv.RequestedAt})
	}
	httputil.WriteJSON(w, http.StatusCreated, out)
}

// ── Sysadmin (orgs half) ────────────────────────────────────────────────────

func (a *App) listOrgs(w http.ResponseWriter, r *http.Request) {
	if !isSuperadmin(r) {
		httputil.Error(w, http.StatusForbidden, "superadmin required")
		return
	}
	orgs, err := a.store.ListOrgs(r.Context())
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.ListOrgs", err)
		return
	}
	if orgs == nil {
		orgs = []contracts.Organization{}
	}
	httputil.WriteJSON(w, http.StatusOK, orgs)
}

func (a *App) createOrg(w http.ResponseWriter, r *http.Request) {
	if !isSuperadmin(r) {
		httputil.Error(w, http.StatusForbidden, "superadmin required")
		return
	}
	var body struct {
		Name string          `json:"name"`
		Plan contracts.Plan  `json:"plan"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	if body.Name == "" {
		httputil.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	org, err := a.store.CreateOrg(r.Context(), "", body.Name, body.Plan)
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.CreateOrg", err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, org)
}

func (a *App) suspendOrg(w http.ResponseWriter, r *http.Request) {
	a.setOrgStatus(w, r, contracts.OrgSuspended)
}

func (a *App) restoreOrg(w http.ResponseWriter, r *http.Request) {
	a.setOrgStatus(w, r, contracts.OrgActive)
}

func (a *App) setOrgStatus(w http.ResponseWriter, r *http.Request, status contracts.OrgStatus) {
	if !isSuperadmin(r) {
		httputil.Error(w, http.StatusForbidden, "superadmin required")
		return
	}
	org, err := a.store.SetOrgStatus(r.Context(), contracts.ID(r.PathValue("id")), status)
	if errors.Is(err, store.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "organization not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.SetOrgStatus", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, org)
}

func (a *App) listOrgRequests(w http.ResponseWriter, r *http.Request) {
	if !isSuperadmin(r) {
		httputil.Error(w, http.StatusForbidden, "superadmin required")
		return
	}
	rows, err := a.store.ListPendingOrgRequests(r.Context())
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.ListOrgRequests", err)
		return
	}
	out := make([]contracts.SignupRequest, 0, len(rows))
	for _, o := range rows {
		sr := o.SignupRequest
		sr.WorkspaceName = o.OrganizationName
		out = append(out, sr)
	}
	if out == nil {
		out = []contracts.SignupRequest{}
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (a *App) approveOrgRequest(w http.ResponseWriter, r *http.Request) {
	if !isSuperadmin(r) {
		httputil.Error(w, http.StatusForbidden, "superadmin required")
		return
	}
	rid := contracts.ID(r.PathValue("rid"))
	o, err := a.store.GetOrgRequest(r.Context(), rid)
	if errors.Is(err, store.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "request not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.ApproveOrgRequest.Get", err)
		return
	}
	if o.Status != contracts.SignupPending {
		httputil.Error(w, http.StatusConflict, "request is not pending")
		return
	}
	name := o.OrganizationName
	if name == "" {
		name = o.Name + "'s Org"
	}
	org, err := a.store.CreateOrg(r.Context(), o.UserID, name, contracts.PlanFree)
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.ApproveOrgRequest.Org", err)
		return
	}
	ws, err := a.store.CreateWorkspace(r.Context(), org.ID, name+" Workspace", "", "main", "", "")
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.ApproveOrgRequest.Ws", err)
		return
	}
	if _, err := a.store.AddMembership(r.Context(), ws.ID, o.UserID, o.Name, o.Email, contracts.RoleOwner); err != nil {
		httputil.ServerError(w, a.log, "orgs.ApproveOrgRequest.Member", err)
		return
	}
	if err := a.store.SetOrgRequestStatus(r.Context(), rid, contracts.SignupApproved); err != nil {
		httputil.ServerError(w, a.log, "orgs.ApproveOrgRequest.Set", err)
		return
	}
	a.publish(r.Context(), contracts.TopicSignupApproved, contracts.SignupApprovedData{
		RequestID: rid, UserID: o.UserID, Email: o.Email, Name: o.Name,
		Mode: "create", WorkspaceID: ws.ID, OrganizationName: name, Role: contracts.RoleOwner,
	}, o.UserID)
	a.publish(r.Context(), contracts.TopicWorkspaceCreated, contracts.WorkspaceCreatedData{
		WorkspaceID: ws.ID, Name: ws.Name, RepoSource: ws.RepoSource, DefaultBranch: ws.DefaultBranch,
	}, ws.ID)
	w.WriteHeader(http.StatusNoContent)
}

// ── Internal (Gateway composition) ──────────────────────────────────────────

func (a *App) internalUserWorkspaces(w http.ResponseWriter, r *http.Request) {
	uid := contracts.ID(r.PathValue("uid"))
	wss, err := a.store.ListUserWorkspaces(r.Context(), uid)
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.InternalWorkspaces", err)
		return
	}
	if wss == nil {
		wss = []contracts.Workspace{}
	}
	httputil.WriteJSON(w, http.StatusOK, wss)
}

func (a *App) internalStats(w http.ResponseWriter, r *http.Request) {
	orgs, wss, seats, err := a.store.OrgsStats(r.Context())
	if err != nil {
		httputil.ServerError(w, a.log, "orgs.InternalStats", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]int{
		"organizations": orgs, "workspaces": wss, "open_seats": seats,
	})
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func (a *App) publish(ctx context.Context, topic string, data any, key contracts.ID) {
	if a.prod == nil {
		return
	}
	env := contracts.EventEnvelope{TaskID: key, Data: data}
	if err := kafka.Publish(ctx, a.prod, topic, env, a.log); err != nil {
		a.log.Error("publish event failed", "topic", topic, "error", err)
	}
}

// audit records a workspace admin action for the Admin service (best-effort).
func (a *App) audit(r *http.Request, workspaceID contracts.ID, action, kind, target string) {
	a.publish(r.Context(), contracts.TopicAuditRecorded, contracts.AuditRecordedData{
		WorkspaceID: workspaceID, ActorID: httputil.UserID(r),
		Action: action, ActionKind: kind, Target: target,
	}, workspaceID)
}
