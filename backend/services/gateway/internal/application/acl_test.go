package application

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/workspaces"
	"github.com/aaks/server/internal/platform/tenancy"
)

// ── Fakes ───────────────────────────────────────────────────────────────────

// fakeSessions scripts session resolutions per token.
type fakeSessions struct {
	calls int
	byTok map[string]Session
}

func (f *fakeSessions) Resolve(_ context.Context, token string) (Session, error) {
	f.calls++
	if s, ok := f.byTok[token]; ok {
		return s, nil
	}
	return Session{}, errors.New("no session for token")
}

// fakeMemberships scripts workspace unions per user id.
type fakeMemberships struct {
	calls int
	byUID map[string][]workspaces.Workspace
	err   error // when set, every call fails (membership outage)
}

func (f *fakeMemberships) List(_ context.Context, userID string) ([]workspaces.Workspace, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.byUID[userID], nil
}

// fakeTasks scripts task → workspace ownership.
type fakeTasks struct {
	calls int
	byID  map[string]string
}

func (f *fakeTasks) Workspace(_ context.Context, taskID identity.ID) (identity.ID, error) {
	f.calls++
	if ws, ok := f.byID[string(taskID)]; ok {
		return identity.ID(ws), nil
	}
	return "", errors.New("task not found")
}

func newTestACL(s *fakeSessions, m *fakeMemberships, ts *fakeTasks) (*ACL, *fakeSessions, *fakeMemberships, *fakeTasks) {
	if s == nil {
		s = &fakeSessions{byTok: map[string]Session{}}
	}
	if m == nil {
		m = &fakeMemberships{byUID: map[string][]workspaces.Workspace{}}
	}
	if ts == nil {
		ts = &fakeTasks{byID: map[string]string{}}
	}
	acl := NewACL(s, m, ts, slog.New(slog.DiscardHandler))
	return acl, s, m, ts
}

// ── Session → identity → memberships ────────────────────────────────────────

func TestResolveValidSession(t *testing.T) {
	acl, s, m, _ := newTestACL(nil, nil, nil)
	s.byTok["tok"] = Session{UserID: "u1", Name: "Ada", Email: "ada@aaks.dev"}
	m.byUID["u1"] = []workspaces.Workspace{
		{ID: "w1", Name: "A", Role: identity.RoleOwner},
		{ID: "w2", Name: "B", Role: identity.RoleMember},
	}

	id, ok := acl.Resolve(context.Background(), "tok")
	if !ok {
		t.Fatal("valid session must resolve")
	}
	if id.UserID != "u1" || id.Name != "Ada" || id.Email != "ada@aaks.dev" || id.Superadmin {
		t.Fatalf("identity: got %+v", id)
	}
	if len(id.Workspaces) != 2 || id.Workspaces[0].ID != "w1" {
		t.Fatalf("workspaces: got %+v", id.Workspaces)
	}
}

func TestResolveInvalidSession(t *testing.T) {
	acl, _, _, _ := newTestACL(nil, nil, nil)

	if _, ok := acl.Resolve(context.Background(), "bogus"); ok {
		t.Fatal("unresolvable token must not resolve")
	}
}

// TestResolveSuperadmin checks the superadmin flag round-trips into the
// identity and the injected header.
func TestResolveSuperadmin(t *testing.T) {
	acl, s, _, _ := newTestACL(nil, nil, nil)
	s.byTok["sadm"] = Session{UserID: "u9", Name: "Root", Email: "root@aaks.dev", Superadmin: true}

	id, ok := acl.Resolve(context.Background(), "sadm")
	if !ok || !id.Superadmin {
		t.Fatalf("superadmin session: ok=%v id=%+v", ok, id)
	}
}

// ── Scoping header injection ────────────────────────────────────────────────

// TestInjectHeaders locks the full scoping-header contract: user identity,
// workspace union, single-workspace context, superadmin flag, and the
// derived strongest role.
func TestInjectHeaders(t *testing.T) {
	acl, _, _, _ := newTestACL(nil, nil, nil)
	id := Identity{
		UserID: "u1", Name: "Ada", Email: "ada@aaks.dev", Superadmin: true,
		Workspaces: []workspaces.Workspace{
			{ID: "w1", Name: "A", Role: identity.RoleOwner},
			{ID: "w2", Name: "B", Role: identity.RoleMember},
		},
	}

	h := http.Header{}
	acl.Inject(h, id)

	if got := h.Get(tenancy.HeaderUserID); got != "u1" {
		t.Errorf("X-User-ID: got %q want u1", got)
	}
	if got := h.Get(tenancy.HeaderUserName); got != "Ada" {
		t.Errorf("X-User-Name: got %q want Ada", got)
	}
	if got := h.Get(tenancy.HeaderUserEmail); got != "ada@aaks.dev" {
		t.Errorf("X-User-Email: got %q want ada@aaks.dev", got)
	}
	if got := h.Get(tenancy.HeaderUserSuperadmin); got != "true" {
		t.Errorf("X-User-Superadmin: got %q want true", got)
	}
	if got := h.Get(tenancy.HeaderUserRole); got != "owner" {
		t.Errorf("X-User-Role: got %q want owner", got)
	}
	// Multi-workspace union: no single X-Workspace-ID, full X-Workspace-IDs.
	if got := h.Get(tenancy.HeaderWorkspaceID); got != "" {
		t.Errorf("X-Workspace-ID: got %q want empty (union)", got)
	}
	if got := h.Get(tenancy.HeaderWorkspaceIDs); got != "w1,w2" {
		t.Errorf("X-Workspace-IDs: got %q want w1,w2", got)
	}
}

func TestInjectHeadersSingleWorkspace(t *testing.T) {
	acl, _, _, _ := newTestACL(nil, nil, nil)
	id := Identity{
		UserID: "u1", Name: "Ada", Email: "ada@aaks.dev",
		Workspaces: []workspaces.Workspace{{ID: "w1", Name: "A", Role: identity.RoleMember}},
	}

	h := http.Header{}
	acl.Inject(h, id)

	if got := h.Get(tenancy.HeaderWorkspaceID); got != "w1" {
		t.Errorf("X-Workspace-ID: got %q want w1", got)
	}
	if got := h.Get(tenancy.HeaderWorkspaceIDs); got != "w1" {
		t.Errorf("X-Workspace-IDs: got %q want w1", got)
	}
	if got := h.Get(tenancy.HeaderUserSuperadmin); got != "" {
		t.Errorf("X-User-Superadmin: got %q want empty", got)
	}
}

// TestInjectOverwritesPriorValues: Inject must replace stale values (the
// request was stripped at the boundary, but Inject is the sole writer).
func TestInjectOverwritesPriorValues(t *testing.T) {
	acl, _, _, _ := newTestACL(nil, nil, nil)
	h := http.Header{}
	h.Set(tenancy.HeaderUserID, "attacker")
	h.Set(tenancy.HeaderUserRole, "owner")

	acl.Inject(h, Identity{UserID: "u1", Name: "Ada", Email: "a@b.c", Workspaces: []workspaces.Workspace{{ID: "w1", Role: identity.RoleMember}}})

	if got := h.Get(tenancy.HeaderUserID); got != "u1" {
		t.Errorf("X-User-ID: got %q want u1", got)
	}
	if got := h.Get(tenancy.HeaderUserRole); got != "member" {
		t.Errorf("X-User-Role: got %q want member", got)
	}
}

// ── Role derivation ─────────────────────────────────────────────────────────

func TestStrongestRole(t *testing.T) {
	cases := []struct {
		name string
		wss  []workspaces.Workspace
		want identity.Role
	}{
		{"empty", nil, ""},
		{"member only", []workspaces.Workspace{{Role: identity.RoleMember}}, identity.RoleMember},
		{"admin over member", []workspaces.Workspace{{Role: identity.RoleMember}, {Role: identity.RoleAdmin}}, identity.RoleAdmin},
		{"owner over admin", []workspaces.Workspace{{Role: identity.RoleAdmin}, {Role: identity.RoleOwner}}, identity.RoleOwner},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StrongestRole(c.wss); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

// ── Cache: TTL, expiry, failed-resolution caching ───────────────────────────

// newClockACL builds an ACL with a fake clock for expiry tests.
func newClockACL(s *fakeSessions, m *fakeMemberships, now *time.Time) (*ACL, *fakeSessions, *fakeMemberships) {
	acl, fs, fm, _ := newTestACL(s, m, nil)
	acl.now = func() time.Time { return *now }
	return acl, fs, fm
}

func TestMembershipCacheTTL(t *testing.T) {
	now := time.Now()
	acl, fs, fm := newClockACL(nil, nil, &now)
	fs.byTok["tok"] = Session{UserID: "u1", Name: "Ada", Email: "a@b.c"}
	fm.byUID["u1"] = []workspaces.Workspace{{ID: "w1", Name: "A", Role: identity.RoleMember}}

	if _, ok := acl.Resolve(context.Background(), "tok"); !ok {
		t.Fatal("first resolve failed")
	}
	if _, ok := acl.Resolve(context.Background(), "tok"); !ok {
		t.Fatal("cached resolve failed")
	}
	if fs.calls != 1 || fm.calls != 1 {
		t.Fatalf("cache must absorb the second resolve: session calls=%d membership calls=%d", fs.calls, fm.calls)
	}

	// Advance past the TTL: the entry is evicted and refetched.
	now = now.Add(61 * time.Second)
	if _, ok := acl.Resolve(context.Background(), "tok"); !ok {
		t.Fatal("post-expiry resolve failed")
	}
	if fs.calls != 2 || fm.calls != 2 {
		t.Fatalf("expired cache must refetch: session calls=%d membership calls=%d", fs.calls, fm.calls)
	}
}

func TestFailedResolutionCached(t *testing.T) {
	now := time.Now()
	acl, fs, _ := newClockACL(nil, nil, &now)

	if _, ok := acl.Resolve(context.Background(), "bogus"); ok {
		t.Fatal("bogus token must not resolve")
	}
	if _, ok := acl.Resolve(context.Background(), "bogus"); ok {
		t.Fatal("bogus token must stay unresolvable")
	}
	if fs.calls != 1 {
		t.Fatalf("failed resolution must be cached: session calls=%d", fs.calls)
	}

	// The token becomes valid later; the cached failure expires.
	now = now.Add(61 * time.Second)
	fs.byTok["bogus"] = Session{UserID: "u1"}
	if _, ok := acl.Resolve(context.Background(), "bogus"); !ok {
		t.Fatal("expired failure must be refetched")
	}
}

// ── Degradation ─────────────────────────────────────────────────────────────

// TestMembershipFailureNonFatal: an Orgs outage must not invalidate a valid
// session — the identity resolves with an empty workspace union.
func TestMembershipFailureNonFatal(t *testing.T) {
	acl, s, m, _ := newTestACL(nil, nil, nil)
	s.byTok["tok"] = Session{UserID: "u1", Name: "Ada", Email: "a@b.c"}
	m.err = errors.New("orgs down")

	id, ok := acl.Resolve(context.Background(), "tok")
	if !ok {
		t.Fatal("valid session must resolve despite membership outage")
	}
	if len(id.Workspaces) != 0 {
		t.Fatalf("workspaces must be empty on outage, got %+v", id.Workspaces)
	}
}

// ── Workspace + task ownership checks ───────────────────────────────────────

func TestIsWorkspaceMember(t *testing.T) {
	acl, _, _, _ := newTestACL(nil, nil, nil)
	if !acl.IsWorkspaceMember([]string{"w1", "w2"}, "w2") {
		t.Fatal("w2 must be a member workspace")
	}
	if acl.IsWorkspaceMember([]string{"w1"}, "w9") {
		t.Fatal("w9 must not be a member workspace")
	}
	if acl.IsWorkspaceMember(nil, "w1") {
		t.Fatal("empty union must reject")
	}
}

func TestTaskAccessible(t *testing.T) {
	acl, _, _, ts := newTestACL(nil, nil, nil)
	ts.byID["t1"] = "w1"

	// Task in the caller's union.
	ok, err := acl.TaskAccessible(context.Background(), []string{"w1", "w2"}, "t1")
	if err != nil || !ok {
		t.Fatalf("accessible task: ok=%v err=%v", ok, err)
	}
	// Task outside the caller's union: exists but forbidden.
	ok, err = acl.TaskAccessible(context.Background(), []string{"w2"}, "t1")
	if err != nil {
		t.Fatalf("out-of-union task must not error: %v", err)
	}
	if ok {
		t.Fatal("out-of-union task must be rejected")
	}
	// Unknown task → ErrTaskNotFound.
	_, err = acl.TaskAccessible(context.Background(), []string{"w1"}, "nope")
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("unknown task: got %v want ErrTaskNotFound", err)
	}
}