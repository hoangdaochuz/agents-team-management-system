package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/auth/internal/domain"
)

// ── Fakes ───────────────────────────────────────────────────────────────────

type fakeUsers struct {
	users []domain.User
	next  int
	err   error
}

func (f *fakeUsers) GetByEmail(_ context.Context, email string) (domain.User, error) {
	for _, u := range f.users {
		if strings.EqualFold(u.Email, email) {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrNotFound
}

func (f *fakeUsers) Get(_ context.Context, id identity.ID) (domain.User, error) {
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrNotFound
}

func (f *fakeUsers) Create(_ context.Context, name, email, passwordHash string, superadmin bool) (identity.User, error) {
	if f.err != nil {
		return identity.User{}, f.err
	}
	f.next++
	u := identity.User{ID: identity.ID(fmt.Sprintf("u-%d", f.next)), Name: name, Email: email, Role: identity.RoleMember, IsSuperadmin: &superadmin}
	f.users = append(f.users, domain.User{User: u, PasswordHash: passwordHash})
	return u, nil
}

func (f *fakeUsers) Activate(_ context.Context, id identity.ID) error {
	for i := range f.users {
		if f.users[i].ID == id {
			f.users[i].Active = true
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeUsers) ActivateByEmail(_ context.Context, email string) error {
	for i := range f.users {
		if strings.EqualFold(f.users[i].Email, email) {
			f.users[i].Active = true
			return nil
		}
	}
	return domain.ErrNotFound
}

type fakeSessions struct {
	tokens map[string]identity.ID
	next   int
	users  *fakeUsers
}

func (f *fakeSessions) Create(_ context.Context, userID identity.ID, _ time.Duration) (string, error) {
	if f.tokens == nil {
		f.tokens = map[string]identity.ID{}
	}
	f.next++
	token := fmt.Sprintf("tok-%d", f.next)
	f.tokens[token] = userID
	return token, nil
}

func (f *fakeSessions) User(_ context.Context, token string) (domain.User, error) {
	uid, ok := f.tokens[token]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return f.users.Get(context.Background(), uid)
}

func (f *fakeSessions) Delete(_ context.Context, token string) error {
	delete(f.tokens, token)
	return nil
}

func (f *fakeSessions) CountActiveUsers24h(context.Context) (int, error) { return len(f.tokens), nil }

type fakeSignupRequests struct {
	reqs []domain.SignupRequest
	next int
	err  error
}

func (f *fakeSignupRequests) Create(_ context.Context, name, email, passwordHash, mode, inviteCode, workspaceName, organizationName string, workspaceID identity.ID, role identity.Role) (domain.SignupRequest, error) {
	if f.err != nil {
		return domain.SignupRequest{}, f.err
	}
	f.next++
	r := domain.SignupRequest{
		ID: identity.ID(fmt.Sprintf("sr-%d", f.next)), UserID: identity.ID(fmt.Sprintf("su-%d", f.next)),
		Email: email, Mode: mode, InviteCode: inviteCode, WorkspaceID: workspaceID,
		WorkspaceName: workspaceName, OrganizationName: organizationName, RequestedRole: role,
		Status: identity.SignupPending, RequestedAt: time.Now(),
	}
	f.reqs = append(f.reqs, r)
	return r, nil
}

func (f *fakeSignupRequests) Get(_ context.Context, id identity.ID) (domain.SignupRequest, error) {
	for _, r := range f.reqs {
		if r.ID == id {
			return r, nil
		}
	}
	return domain.SignupRequest{}, domain.ErrNotFound
}

func (f *fakeSignupRequests) GetByEmail(_ context.Context, email string) (domain.SignupRequest, error) {
	for _, r := range f.reqs {
		if strings.EqualFold(r.Email, email) {
			return r, nil
		}
	}
	return domain.SignupRequest{}, domain.ErrNotFound
}

func (f *fakeSignupRequests) SetStatus(_ context.Context, id identity.ID, status identity.SignupState) error {
	for i := range f.reqs {
		if f.reqs[i].ID == id {
			f.reqs[i].Status = status
			return nil
		}
	}
	return domain.ErrNotFound
}

type fakeInvites struct {
	codes []domain.InviteCode
}

func (f *fakeInvites) Lookup(_ context.Context, code string) (domain.InviteCode, error) {
	for _, c := range f.codes {
		if c.Code == code {
			return c, nil
		}
	}
	return domain.InviteCode{}, domain.ErrNotFound
}

func (f *fakeInvites) Upsert(_ context.Context, code, email string, role identity.Role, workspaceID identity.ID, workspaceName string) error {
	for i := range f.codes {
		if f.codes[i].Code == code {
			f.codes[i] = domain.InviteCode{Code: code, Email: email, Role: role, WorkspaceID: workspaceID, WorkspaceName: workspaceName}
			return nil
		}
	}
	f.codes = append(f.codes, domain.InviteCode{Code: code, Email: email, Role: role, WorkspaceID: workspaceID, WorkspaceName: workspaceName})
	return nil
}

type fakePublisher struct {
	events []string
}

func (p *fakePublisher) Publish(_ context.Context, topic string, _ any, _ identity.ID) {
	p.events = append(p.events, topic)
}

type fakeRepo struct {
	users    *fakeUsers
	sessions *fakeSessions
	signups  *fakeSignupRequests
	invites  *fakeInvites
	baseRepo *Repository
}

func newFakeRepo() *fakeRepo {
	f := &fakeRepo{
		users:   &fakeUsers{},
		signups: &fakeSignupRequests{},
		invites: &fakeInvites{},
	}
	f.sessions = &fakeSessions{users: f.users}
	f.baseRepo = &Repository{
		Users:          f.users,
		Sessions:       f.sessions,
		SignupRequests: f.signups,
		Invites:        f.invites,
	}
	return f
}

func newTestApp() (*App, *fakeRepo, *fakePublisher) {
	f := newFakeRepo()
	p := &fakePublisher{}
	app := New(f.baseRepo, p, slog.New(slog.DiscardHandler))
	return app, f, p
}

// ── Signup workflow ─────────────────────────────────────────────────────────

func TestSignupCreateMode(t *testing.T) {
	app, f, p := newTestApp()

	reqID, err := app.Signup(context.Background(), SignupInput{
		FullName: "Alice", Email: "alice@x.io", Password: "password123",
		StartMode: "create", OrganizationName: "Acme",
	})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	if reqID == "" {
		t.Fatal("request id must be returned")
	}
	if len(f.signups.reqs) != 1 || f.signups.reqs[0].Status != identity.SignupPending {
		t.Fatalf("request must persist as pending, got %+v", f.signups.reqs)
	}
	if f.signups.reqs[0].RequestedRole != identity.RoleOwner {
		t.Fatalf("create-mode requester must request owner, got %s", f.signups.reqs[0].RequestedRole)
	}
	want := []string{events.TopicSignupRequested}
	if len(p.events) != 1 || p.events[0] != want[0] {
		t.Fatalf("expected signup.requested only, got %v", p.events)
	}
}

func TestSignupJoinModeResolvesInvite(t *testing.T) {
	app, f, p := newTestApp()
	f.invites.codes = []domain.InviteCode{
		{Code: "code-abc", Email: "bob@x.io", Role: identity.RoleAdmin, WorkspaceID: "ws1", WorkspaceName: "Team"},
	}

	reqID, err := app.Signup(context.Background(), SignupInput{
		FullName: "Bob", Email: "bob@x.io", Password: "password123",
		StartMode: "join", InviteCode: "code-abc",
	})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	if reqID == "" {
		t.Fatal("request id must be returned")
	}
	r := f.signups.reqs[0]
	if r.WorkspaceID != "ws1" || r.WorkspaceName != "Team" || r.RequestedRole != identity.RoleAdmin {
		t.Fatalf("join-mode request must carry the invite target, got %+v", r)
	}
	if len(p.events) != 1 || p.events[0] != events.TopicSignupRequested {
		t.Fatalf("expected signup.requested only, got %v", p.events)
	}
}

func TestSignupValidationErrorsPublishNothing(t *testing.T) {
	app, _, p := newTestApp()
	cases := []struct {
		name string
		in   SignupInput
		want error
	}{
		{"missing fields", SignupInput{Email: "a@x.io", Password: "password123", StartMode: "create"}, ErrFieldsRequired},
		{"short password", SignupInput{FullName: "A", Email: "a@x.io", Password: "short", StartMode: "create"}, ErrPasswordTooShort},
		{"bad mode", SignupInput{FullName: "A", Email: "a@x.io", Password: "password123", StartMode: "hack"}, ErrStartMode},
		{"join without code", SignupInput{FullName: "A", Email: "a@x.io", Password: "password123", StartMode: "join"}, ErrInviteCodeRequired},
		{"create without org", SignupInput{FullName: "A", Email: "a@x.io", Password: "password123", StartMode: "create"}, ErrOrganizationRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := app.Signup(context.Background(), tc.in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
			if len(p.events) != 0 {
				t.Fatalf("no event may be published on validation failure, got %v", p.events)
			}
		})
	}
}

func TestSignupUnknownInviteCode(t *testing.T) {
	app, _, p := newTestApp()
	_, err := app.Signup(context.Background(), SignupInput{
		FullName: "Bob", Email: "bob@x.io", Password: "password123",
		StartMode: "join", InviteCode: "nope",
	})
	if !errors.Is(err, ErrUnknownInviteCode) {
		t.Fatalf("expected unknown invite code, got %v", err)
	}
	if len(p.events) != 0 {
		t.Fatalf("no event may be published on lookup failure, got %v", p.events)
	}
}

// TestSignupPublishAfterPersistence injects a persistence failure and asserts
// signup.requested is never emitted (publish-after-commit ordering).
func TestSignupPublishAfterPersistence(t *testing.T) {
	app, f, p := newTestApp()
	f.signups.err = domain.ErrEmailTaken

	_, err := app.Signup(context.Background(), SignupInput{
		FullName: "Alice", Email: "alice@x.io", Password: "password123",
		StartMode: "create", OrganizationName: "Acme",
	})
	if !errors.Is(err, domain.ErrEmailTaken) {
		t.Fatalf("expected email taken, got %v", err)
	}
	if len(p.events) != 0 {
		t.Fatalf("no event may be published when persistence fails, got %v", p.events)
	}
}

// ── Signup approval / decline (state machine) ───────────────────────────────

func TestHandleSignupApprovedActivatesUser(t *testing.T) {
	app, f, _ := newTestApp()
	f.users.users = []domain.User{
		{User: identity.User{ID: "u1", Name: "Alice", Email: "alice@x.io"}, PasswordHash: "h", Active: false},
	}
	f.signups.reqs = []domain.SignupRequest{{ID: "sr1", UserID: "u1", Status: identity.SignupPending}}

	err := app.HandleSignupApproved(context.Background(), events.SignupApprovedData{
		RequestID: "sr1", UserID: "u1", Email: "alice@x.io",
	})
	if err != nil {
		t.Fatalf("handle approved: %v", err)
	}
	if f.signups.reqs[0].Status != identity.SignupApproved {
		t.Fatalf("request must be approved, got %s", f.signups.reqs[0].Status)
	}
	if !f.users.users[0].Active {
		t.Fatal("approved user must be activated")
	}
}

func TestHandleSignupApprovedFallsBackToEmail(t *testing.T) {
	app, f, _ := newTestApp()
	f.users.users = []domain.User{
		{User: identity.User{ID: "u1", Name: "Alice", Email: "alice@x.io"}, PasswordHash: "h", Active: false},
	}
	f.signups.reqs = []domain.SignupRequest{{ID: "sr1", UserID: "u1", Status: identity.SignupPending}}

	err := app.HandleSignupApproved(context.Background(), events.SignupApprovedData{
		RequestID: "sr1", Email: "alice@x.io",
	})
	if err != nil {
		t.Fatalf("handle approved: %v", err)
	}
	if !f.users.users[0].Active {
		t.Fatal("user must be activated by email fallback")
	}
}

func TestHandleSignupDeclined(t *testing.T) {
	app, f, _ := newTestApp()
	f.signups.reqs = []domain.SignupRequest{{ID: "sr1", UserID: "u1", Status: identity.SignupPending}}

	err := app.HandleSignupDeclined(context.Background(), events.SignupDeclinedData{RequestID: "sr1"})
	if err != nil {
		t.Fatalf("handle declined: %v", err)
	}
	if f.signups.reqs[0].Status != identity.SignupDeclined {
		t.Fatalf("request must be declined, got %s", f.signups.reqs[0].Status)
	}
	if f.users.users != nil {
		t.Fatal("declined request must not activate a user")
	}
}

// ── Invite projection ───────────────────────────────────────────────────────

func TestHandleInviteCreatedProjection(t *testing.T) {
	app, f, _ := newTestApp()

	err := app.HandleInviteCreated(context.Background(), events.InviteCreatedData{
		InviteCode: "code-abc", Email: "bob@x.io", Role: identity.RoleMember,
		WorkspaceID: "ws1", WorkspaceName: "Team",
	})
	if err != nil {
		t.Fatalf("handle invite: %v", err)
	}
	inv, err := f.invites.Lookup(context.Background(), "code-abc")
	if err != nil {
		t.Fatalf("invite code must be resolvable: %v", err)
	}
	if inv.WorkspaceID != "ws1" || inv.WorkspaceName != "Team" || inv.Role != identity.RoleMember {
		t.Fatalf("projected invite must carry the workspace target, got %+v", inv)
	}

	// The projected code must now power a join-mode signup end-to-end.
	reqID, err := app.Signup(context.Background(), SignupInput{
		FullName: "Bob", Email: "bob@x.io", Password: "password123",
		StartMode: "join", InviteCode: "code-abc",
	})
	if err != nil {
		t.Fatalf("join signup via projected code: %v", err)
	}
	if reqID == "" {
		t.Fatal("request id must be returned")
	}
}

// ── Sessions / login ────────────────────────────────────────────────────────

func newUser(password string) domain.User {
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	return domain.User{User: identity.User{ID: "u1", Name: "Alice", Email: "alice@x.io", Role: identity.RoleMember}, PasswordHash: string(hash), Active: true}
}

func TestLoginSuccess(t *testing.T) {
	app, f, _ := newTestApp()
	f.users.users = []domain.User{newUser("password123")}

	res, err := app.Login(context.Background(), "alice@x.io", "password123", "1.2.3.4", false)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if res.Token == "" {
		t.Fatal("session token must be issued")
	}
	if res.MaxAge != 86400 {
		t.Fatalf("default session max-age must be 24h, got %d", res.MaxAge)
	}
	if res.User.ID != "u1" {
		t.Fatalf("logged-in user must be returned, got %+v", res.User)
	}
	if len(f.sessions.tokens) != 1 {
		t.Fatalf("session must be persisted, got %v", f.sessions.tokens)
	}
}

func TestLoginRememberExtendsSession(t *testing.T) {
	app, f, _ := newTestApp()
	f.users.users = []domain.User{newUser("password123")}

	res, err := app.Login(context.Background(), "alice@x.io", "password123", "1.2.3.4", true)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if res.MaxAge != 30*86400 {
		t.Fatalf("remembered session max-age must be 30d, got %d", res.MaxAge)
	}
}

func TestLoginWrongPasswordAndThrottle(t *testing.T) {
	app, f, _ := newTestApp()
	f.users.users = []domain.User{newUser("password123")}

	ip := "9.9.9.9"
	for i := 0; i < loginLimit-1; i++ {
		_, err := app.Login(context.Background(), "alice@x.io", "wrong", ip, false)
		if !errors.Is(err, domain.ErrBadPassword) {
			t.Fatalf("attempt %d: expected invalid credentials, got %v", i+1, err)
		}
		if app.LoginGate(ip) {
			t.Fatalf("attempt %d: gate must not trip before the limit", i+1)
		}
	}
	// The 5th failure reaches the limit; the gate trips from then on.
	if _, err := app.Login(context.Background(), "alice@x.io", "wrong", ip, false); !errors.Is(err, domain.ErrBadPassword) {
		t.Fatalf("5th failure must still be a credential error, got %v", err)
	}
	if !app.LoginGate(ip) {
		t.Fatal("IP must be throttled after 5 failures")
	}
	_, err := app.Login(context.Background(), "alice@x.io", "password123", ip, false)
	if !errors.Is(err, domain.ErrThrottled) {
		t.Fatalf("throttled login must be rejected even with the right password, got %v", err)
	}
}

func TestLoginClearsThrottleOnSuccess(t *testing.T) {
	app, f, _ := newTestApp()
	f.users.users = []domain.User{newUser("password123")}

	ip := "8.8.8.8"
	for i := 0; i < 2; i++ {
		_, _ = app.Login(context.Background(), "alice@x.io", "wrong", ip, false)
	}
	if _, err := app.Login(context.Background(), "alice@x.io", "password123", ip, false); err != nil {
		t.Fatalf("login: %v", err)
	}
	if app.LoginGate(ip) {
		t.Fatal("successful login must clear the throttle keys")
	}
	if _, err := app.Login(context.Background(), "alice@x.io", "wrong", ip, false); !errors.Is(err, domain.ErrBadPassword) {
		t.Fatalf("attempts after success must be counted fresh, got %v", err)
	}
}

func TestLoginUnapprovedUserPending(t *testing.T) {
	app, f, _ := newTestApp()
	u := newUser("password123")
	u.Active = false
	f.users.users = []domain.User{u}

	_, err := app.Login(context.Background(), "alice@x.io", "password123", "1.2.3.4", false)
	if !errors.Is(err, domain.ErrPending) {
		t.Fatalf("unapproved login must be pending, got %v", err)
	}
	if len(f.sessions.tokens) != 0 {
		t.Fatal("no session may be issued to an unapproved user")
	}
}

func TestSessionUserAndLogout(t *testing.T) {
	app, f, _ := newTestApp()
	f.users.users = []domain.User{newUser("password123")}

	res, err := app.Login(context.Background(), "alice@x.io", "password123", "1.2.3.4", false)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	u, err := app.SessionUser(context.Background(), res.Token)
	if err != nil {
		t.Fatalf("session user: %v", err)
	}
	if u.ID != "u1" {
		t.Fatalf("session must resolve to the user, got %+v", u)
	}
	if err := app.Logout(context.Background(), res.Token); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := app.SessionUser(context.Background(), res.Token); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("invalidated token must not resolve, got %v", err)
	}
	if _, err := app.SessionUser(context.Background(), "bogus"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown token must not resolve, got %v", err)
	}
}

// ── Superadmin seeding ──────────────────────────────────────────────────────

func TestSeedSuperadmin(t *testing.T) {
	app, f, _ := newTestApp()

	app.SeedSuperadmin(context.Background(), "admin@x.io", "adminpass123")
	if len(f.users.users) != 1 || !f.users.users[0].Active {
		t.Fatalf("superadmin must be created active, got %+v", f.users.users)
	}
	if f.users.users[0].IsSuperadmin == nil || !*f.users.users[0].IsSuperadmin {
		t.Fatal("seeded user must be a superadmin")
	}
	// Idempotent: a second seed must not duplicate.
	app.SeedSuperadmin(context.Background(), "admin@x.io", "adminpass123")
	if len(f.users.users) != 1 {
		t.Fatalf("seed must be idempotent, got %d users", len(f.users.users))
	}
}

// ── Active users KPI ────────────────────────────────────────────────────────

func TestActiveUsers24h(t *testing.T) {
	app, f, _ := newTestApp()
	f.users.users = []domain.User{newUser("password123")}

	if _, err := app.Login(context.Background(), "alice@x.io", "password123", "1.2.3.4", false); err != nil {
		t.Fatalf("login: %v", err)
	}
	n, err := app.ActiveUsers24h(context.Background())
	if err != nil {
		t.Fatalf("active users: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 active user, got %d", n)
	}
}
