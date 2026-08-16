package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/workspaces"
	"github.com/aaks/server/services/orgs/internal/domain"
)

// ── Fakes ───────────────────────────────────────────────────────────────────

type fakeOrgs struct {
	orgs []workspaces.Organization
	next int
	err  error
}

func (f *fakeOrgs) List(context.Context) ([]workspaces.Organization, error)          { return f.orgs, f.err }
func (f *fakeOrgs) Get(_ context.Context, id identity.ID) (workspaces.Organization, error) {
	for _, o := range f.orgs {
		if o.ID == id {
			return o, nil
		}
	}
	return workspaces.Organization{}, domain.ErrNotFound
}
func (f *fakeOrgs) Create(_ context.Context, owner identity.ID, name string, plan identity.Plan) (workspaces.Organization, error) {
	f.next++
	o := workspaces.Organization{ID: identity.ID(fmt.Sprintf("o-%s", owner)), Name: name, Plan: plan, Status: identity.OrgActive}
	f.orgs = append(f.orgs, o)
	return o, nil
}
func (f *fakeOrgs) SetStatus(_ context.Context, id identity.ID, status identity.OrgStatus) (workspaces.Organization, error) {
	for i := range f.orgs {
		if f.orgs[i].ID == id {
			f.orgs[i].Status = status
			return f.orgs[i], nil
		}
	}
	return workspaces.Organization{}, domain.ErrNotFound
}
func (f *fakeOrgs) ByUser(_ context.Context, owner identity.ID) (workspaces.Organization, error) {
	for _, o := range f.orgs {
		if o.ID == identity.ID("o-"+owner) {
			return o, nil
		}
	}
	return workspaces.Organization{}, domain.ErrNoOrg
}
func (f *fakeOrgs) Stats(context.Context) (int, int, int, error) { return len(f.orgs), 0, 0, f.err }

type fakeWorkspaces struct {
	wss  []workspaces.Workspace
	next int
}

func (f *fakeWorkspaces) Create(_ context.Context, orgID identity.ID, name, repoSource, defaultBranch, glyph, description string) (workspaces.Workspace, error) {
	f.next++
	w := workspaces.Workspace{ID: identity.ID(fmt.Sprintf("w-%d", f.next)), Name: name, RepoSource: repoSource, DefaultBranch: defaultBranch}
	f.wss = append(f.wss, w)
	return w, nil
}
func (f *fakeWorkspaces) ByID(_ context.Context, id identity.ID) (workspaces.Workspace, error) {
	for _, w := range f.wss {
		if w.ID == id {
			return w, nil
		}
	}
	return workspaces.Workspace{}, domain.ErrNotFound
}
func (f *fakeWorkspaces) ListByUser(context.Context, identity.ID) ([]workspaces.Workspace, error) {
	return f.wss, nil
}
func (f *fakeWorkspaces) GetByUser(context.Context, identity.ID, identity.ID) (workspaces.Workspace, error) {
	return workspaces.Workspace{}, domain.ErrNotFound
}

type fakeMembers struct {
	members []domain.Member
	next    int
}

func (f *fakeMembers) List(context.Context, identity.ID) ([]domain.Member, error) { return f.members, nil }
func (f *fakeMembers) Add(_ context.Context, wsID, userID identity.ID, name, email string, role identity.Role) (domain.Member, error) {
	f.next++
	m := domain.Member{Member: workspaces.Member{ID: identity.ID(fmt.Sprintf("m-%d", f.next)), Role: role, Status: identity.MemberActive}, WorkspaceID: wsID, UserID: userID}
	f.members = append(f.members, m)
	return m, nil
}
func (f *fakeMembers) SetRole(_ context.Context, wsID, memberID identity.ID, role identity.Role) (domain.Member, error) {
	for i := range f.members {
		if f.members[i].ID == memberID {
			f.members[i].Role = role
			return f.members[i], nil
		}
	}
	return domain.Member{}, domain.ErrNotFound
}
func (f *fakeMembers) Remove(_ context.Context, wsID, memberID identity.ID) error {
	for i := range f.members {
		if f.members[i].ID == memberID {
			f.members = append(f.members[:i], f.members[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}
func (f *fakeMembers) OwnerCount(_ context.Context, wsID identity.ID) (int, error) {
	n := 0
	for _, m := range f.members {
		if m.WorkspaceID == wsID && m.Role == identity.RoleOwner {
			n++
		}
	}
	return n, nil
}
func (f *fakeMembers) UserRoleIn(_ context.Context, userID, wsID identity.ID) (identity.Role, error) {
	for _, m := range f.members {
		if m.UserID == userID && m.WorkspaceID == wsID {
			return m.Role, nil
		}
	}
	return "", domain.ErrNotFound
}

type fakeJoinRequests struct {
	reqs []domain.JoinRequest
}

func (f *fakeJoinRequests) ListPending(context.Context, identity.ID) ([]domain.JoinRequest, error) {
	return f.reqs, nil
}
func (f *fakeJoinRequests) Get(_ context.Context, id identity.ID) (domain.JoinRequest, error) {
	for _, r := range f.reqs {
		if r.ID == id {
			return r, nil
		}
	}
	return domain.JoinRequest{}, domain.ErrNotFound
}
func (f *fakeJoinRequests) Upsert(context.Context, events.SignupRequestedData) error { return nil }
func (f *fakeJoinRequests) SetStatus(_ context.Context, id identity.ID, status identity.SignupState) error {
	for i := range f.reqs {
		if f.reqs[i].ID == id {
			f.reqs[i].Status = status
			return nil
		}
	}
	return domain.ErrNotFound
}

type fakeOrgRequests struct {
	reqs []domain.OrgRequest
}

func (f *fakeOrgRequests) ListPending(context.Context) ([]domain.OrgRequest, error) { return f.reqs, nil }
func (f *fakeOrgRequests) Get(_ context.Context, id identity.ID) (domain.OrgRequest, error) {
	for _, r := range f.reqs {
		if r.ID == id {
			return r, nil
		}
	}
	return domain.OrgRequest{}, domain.ErrNotFound
}
func (f *fakeOrgRequests) Upsert(context.Context, events.SignupRequestedData) error { return nil }
func (f *fakeOrgRequests) SetStatus(_ context.Context, id identity.ID, status identity.SignupState) error {
	for i := range f.reqs {
		if f.reqs[i].ID == id {
			f.reqs[i].Status = status
			return nil
		}
	}
	return domain.ErrNotFound
}

type fakeInvites struct {
	invites []domain.Invite
	next    int
}

func (f *fakeInvites) Create(_ context.Context, wsID identity.ID, emails []string, role identity.Role) ([]domain.Invite, error) {
	out := []domain.Invite{}
	for _, e := range emails {
		f.next++
		out = append(out, domain.Invite{Invite: workspaces.Invite{ID: identity.ID(fmt.Sprintf("i-%d", f.next)), Email: e, Role: role}, WorkspaceID: wsID, InviteCode: "code-" + e})
	}
	f.invites = append(f.invites, out...)
	return out, nil
}

type fakeRepo struct {
	orgs     *fakeOrgs
	ws       *fakeWorkspaces
	members  *fakeMembers
	joins    *fakeJoinRequests
	orgReqs  *fakeOrgRequests
	invites  *fakeInvites
	baseRepo *Repository
}

func newFakeRepo() *fakeRepo {
	f := &fakeRepo{
		orgs:    &fakeOrgs{},
		ws:      &fakeWorkspaces{},
		members: &fakeMembers{},
		joins:   &fakeJoinRequests{},
		orgReqs: &fakeOrgRequests{},
		invites: &fakeInvites{},
	}
	f.baseRepo = &Repository{
		Organizations: f.orgs,
		Workspaces:    f.ws,
		Members:       f.members,
		Invites:       f.invites,
		JoinRequests:  f.joins,
		OrgRequests:   f.orgReqs,
	}
	return f
}

// fakeUoW runs fn against the same fakes as the plain repo — commit-on-success
// semantics with an optional injected failure for rollback tests. To emulate a
// real transaction it snapshots the fakes before fn and restores them on
// failure, so no partial state survives (mirrors pgx Tx rollback).
type fakeUoW struct {
	repo *fakeRepo
	// failAfter lets a test inject a mid-transaction error after fn succeeded.
	failAfter int
}

func (u *fakeUoW) Do(ctx context.Context, fn func(tx *Tx) error) error {
	snapOrgs := append([]workspaces.Organization(nil), u.repo.orgs.orgs...)
	snapWS := append([]workspaces.Workspace(nil), u.repo.ws.wss...)
	snapMembers := append([]domain.Member(nil), u.repo.members.members...)
	snapJoins := append([]domain.JoinRequest(nil), u.repo.joins.reqs...)
	snapOrgReqs := append([]domain.OrgRequest(nil), u.repo.orgReqs.reqs...)
	snapInvites := append([]domain.Invite(nil), u.repo.invites.invites...)
	snapNext := [4]int{u.repo.orgs.next, u.repo.ws.next, u.repo.members.next, u.repo.invites.next}

	tx := &Tx{
		Organizations: u.repo.orgs,
		Workspaces:    u.repo.ws,
		Members:       u.repo.members,
		Invites:       u.repo.invites,
		JoinRequests:  u.repo.joins,
		OrgRequests:   u.repo.orgReqs,
	}
	if err := fn(tx); err != nil {
		return err
	}
	if u.failAfter > 0 {
		u.repo.orgs.orgs = snapOrgs
		u.repo.ws.wss = snapWS
		u.repo.members.members = snapMembers
		u.repo.joins.reqs = snapJoins
		u.repo.orgReqs.reqs = snapOrgReqs
		u.repo.invites.invites = snapInvites
		u.repo.orgs.next, u.repo.ws.next, u.repo.members.next, u.repo.invites.next = snapNext[0], snapNext[1], snapNext[2], snapNext[3]
		return errors.New("injected mid-transaction failure")
	}
	return nil
}

// fakePublisher records published events.
type fakePublisher struct {
	events []string
}

func (p *fakePublisher) Publish(_ context.Context, topic string, _ any, _ identity.ID) {
	p.events = append(p.events, topic)
}

func newTestApp() (*App, *fakeRepo, *fakeUoW, *fakePublisher) {
	f := newFakeRepo()
	u := &fakeUoW{repo: f}
	p := &fakePublisher{}
	app := New(f.baseRepo, u, p, slog.New(slog.DiscardHandler))
	return app, f, u, p
}

// ── Role enforcement ─────────────────────────────────────────────────────────

func TestRequireMemberAndAdmin(t *testing.T) {
	app, f, _, _ := newTestApp()
	wsID := identity.ID("ws1")
	userID := identity.ID("u1")
	f.members.members = []domain.Member{
		{Member: workspaces.Member{ID: "m1", Role: identity.RoleMember, Status: identity.MemberActive}, WorkspaceID: wsID, UserID: userID},
	}

	if _, err := app.RequireMember(context.Background(), userID, wsID); err != nil {
		t.Fatalf("member role should pass: %v", err)
	}
	if err := app.RequireAdmin(context.Background(), userID, wsID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("member should be rejected for admin: %v", err)
	}
	if err := app.RequireAdmin(context.Background(), "nobody", wsID); !errors.Is(err, domain.ErrNotMember) {
		t.Fatalf("non-member should be rejected: %v", err)
	}
}

// ── Last-owner protection ───────────────────────────────────────────────────

func TestUpdateRoleLastOwnerProtection(t *testing.T) {
	app, f, _, _ := newTestApp()
	wsID := identity.ID("ws1")
	f.members.members = []domain.Member{
		{Member: workspaces.Member{ID: "m1", Role: identity.RoleOwner, Status: identity.MemberActive}, WorkspaceID: wsID, UserID: "u1"},
	}

	_, err := app.UpdateRole(context.Background(), wsID, "m1", identity.RoleMember)
	if !errors.Is(err, domain.ErrLastOwner) {
		t.Fatalf("demoting the last owner must be rejected, got %v", err)
	}
	if f.members.members[0].Role != identity.RoleOwner {
		t.Fatalf("revert must restore owner role, got %s", f.members.members[0].Role)
	}

	// With a second owner the demotion goes through.
	f.members.members = append(f.members.members,
		domain.Member{Member: workspaces.Member{ID: "m2", Role: identity.RoleOwner, Status: identity.MemberActive}, WorkspaceID: wsID, UserID: "u2"})
	target, err := app.UpdateRole(context.Background(), wsID, "m1", identity.RoleMember)
	if err != nil {
		t.Fatalf("demotion with another owner should pass: %v", err)
	}
	if target.Role != identity.RoleMember {
		t.Fatalf("expected member role, got %s", target.Role)
	}
}

func TestRemoveLastOwnerProtection(t *testing.T) {
	app, f, _, _ := newTestApp()
	wsID := identity.ID("ws1")
	f.members.members = []domain.Member{
		{Member: workspaces.Member{ID: "m1", Role: identity.RoleOwner, Status: identity.MemberActive}, WorkspaceID: wsID, UserID: "u1"},
	}

	err := app.Remove(context.Background(), "admin", wsID, "m1")
	if !errors.Is(err, domain.ErrLastOwner) {
		t.Fatalf("removing the last owner must be rejected, got %v", err)
	}
	if len(f.members.members) != 1 {
		t.Fatal("owner must not be removed")
	}
	err = app.Remove(context.Background(), "m1", wsID, "m1")
	if !errors.Is(err, ErrSelfRemoval) {
		t.Fatalf("self-removal must be rejected, got %v", err)
	}
}

// ── CreateWorkspace flow (UoW) ──────────────────────────────────────────────

func TestCreateWorkspaceAutoCreatesOrg(t *testing.T) {
	app, f, _, p := newTestApp()
	userID := identity.ID("u1")

	ws, err := app.CreateWorkspace(context.Background(), userID, "Team", "git@x/y", "main")
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if ws.Role != identity.RoleOwner {
		t.Fatalf("creator must be owner, got %s", ws.Role)
	}
	if len(f.orgs.orgs) != 1 || f.orgs.orgs[0].ID != identity.ID("o-"+userID) {
		t.Fatalf("organization must be auto-created for the user, got %+v", f.orgs.orgs)
	}
	if len(f.members.members) != 1 || f.members.members[0].UserID != userID {
		t.Fatal("creator membership must exist")
	}
	want := []string{events.TopicWorkspaceCreated}
	if len(p.events) != 1 || p.events[0] != want[0] {
		t.Fatalf("expected workspace.created only, got %v", p.events)
	}
}

// ── Approve flows ───────────────────────────────────────────────────────────

func TestApproveJoinRequest(t *testing.T) {
	app, f, _, p := newTestApp()
	wsID := identity.ID("ws1")
	rid := identity.ID("jr1")
	f.joins.reqs = []domain.JoinRequest{{
		SignupRequest: identity.SignupRequest{ID: rid, Name: "Bob", Email: "b@x.io", RequestedRole: identity.RoleMember},
		UserID:        "u2", Status: identity.SignupPending,
	}}

	err := app.ApproveJoinRequest(context.Background(), "admin", wsID, rid)
	if err != nil {
		t.Fatalf("approve join: %v", err)
	}
	if f.joins.reqs[0].Status != identity.SignupApproved {
		t.Fatalf("request must be approved, got %s", f.joins.reqs[0].Status)
	}
	if len(f.members.members) != 1 || f.members.members[0].UserID != "u2" {
		t.Fatal("approved user must become a member")
	}
	if len(p.events) != 2 || p.events[0] != events.TopicSignupApproved || p.events[1] != events.TopicAuditRecorded {
		t.Fatalf("expected signup.approved + audit, got %v", p.events)
	}
}

func TestApproveOrgRequest(t *testing.T) {
	app, f, _, p := newTestApp()
	rid := identity.ID("or1")
	f.orgReqs.reqs = []domain.OrgRequest{{
		SignupRequest:    identity.SignupRequest{ID: rid, Name: "Alice", Email: "a@x.io", RequestedRole: identity.RoleOwner},
		UserID:           "u3", OrganizationName: "Acme", Status: identity.SignupPending,
	}}

	err := app.ApproveOrgRequest(context.Background(), rid)
	if err != nil {
		t.Fatalf("approve org: %v", err)
	}
	if f.orgReqs.reqs[0].Status != identity.SignupApproved {
		t.Fatalf("request must be approved, got %s", f.orgReqs.reqs[0].Status)
	}
	if len(f.orgs.orgs) != 1 || f.orgs.orgs[0].Name != "Acme" {
		t.Fatalf("org must be created, got %+v", f.orgs.orgs)
	}
	if len(f.ws.wss) != 1 || f.ws.wss[0].Name != "Acme Workspace" {
		t.Fatalf("workspace must be created, got %+v", f.ws.wss)
	}
	if len(f.members.members) != 1 || f.members.members[0].UserID != "u3" {
		t.Fatal("requester must become owner member")
	}
	if len(p.events) != 2 || p.events[0] != events.TopicSignupApproved || p.events[1] != events.TopicWorkspaceCreated {
		t.Fatalf("expected signup.approved + workspace.created, got %v", p.events)
	}
}

func TestApproveNonPendingRequest(t *testing.T) {
	app, f, _, _ := newTestApp()
	f.joins.reqs = []domain.JoinRequest{{
		SignupRequest: identity.SignupRequest{ID: "jr1", RequestedRole: identity.RoleMember},
		UserID:        "u2", Status: identity.SignupApproved,
	}}
	err := app.ApproveJoinRequest(context.Background(), "admin", "ws1", "jr1")
	if !errors.Is(err, domain.ErrNotPending) {
		t.Fatalf("non-pending request must be rejected, got %v", err)
	}
}

// ── Rollback (2.9) ──────────────────────────────────────────────────────────

// TestRollbackOnMidTransactionFailure injects a UoW failure after the
// multi-aggregate mutation ran and asserts no partial state survives and no
// events are published.
func TestRollbackOnMidTransactionFailure(t *testing.T) {
	app, f, u, p := newTestApp()
	u.failAfter = 1

	ws, err := app.CreateWorkspace(context.Background(), "u1", "Team", "", "main")
	if err == nil {
		t.Fatal("injected failure must surface as an error")
	}
	if ws.ID != "" {
		t.Fatal("no workspace may be returned on rollback")
	}
	if len(f.orgs.orgs) != 0 || len(f.ws.wss) != 0 || len(f.members.members) != 0 {
		t.Fatalf("partial state must not persist: orgs=%d ws=%d members=%d",
			len(f.orgs.orgs), len(f.ws.wss), len(f.members.members))
	}
	if len(p.events) != 0 {
		t.Fatalf("no events may be published on rollback, got %v", p.events)
	}
}