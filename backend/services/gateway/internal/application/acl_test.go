package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/workspaces"
	"github.com/aaks/server/internal/platform/tenancy"
)

// ── Fakes ───────────────────────────────────────────────────────────────────

// fakeIdentities scripts full identity resolutions (user + workspace union)
// per token — the single call the Identity service serves.
type fakeIdentities struct {
	calls           int
	byTok           map[string]Identity
	rejectTransient bool // transport/5xx-style failure (no ErrSessionRejected)
}

func (f *fakeIdentities) Resolve(_ context.Context, token string) (Identity, error) {
	f.calls++
	if f.rejectTransient {
		return Identity{}, errors.New("dial tcp: connection refused")
	}
	if id, ok := f.byTok[token]; ok {
		return id, nil
	}
	// Mirrors the real client: an unknown token is a definitive rejection.
	return Identity{}, fmt.Errorf("%w: no session for token", ErrSessionRejected)
}

// fakeTasks scripts task → workspace ownership.
type fakeTasks struct {
	calls          int
	byID           map[string]string
	failTransient  bool // transport/5xx-style failure
}

func (f *fakeTasks) Workspace(_ context.Context, taskID identity.ID) (identity.ID, error) {
	f.calls++
	if f.failTransient {
		return "", errors.New("context deadline exceeded")
	}
	if ws, ok := f.byID[string(taskID)]; ok {
		return identity.ID(ws), nil
	}
	// Mirrors the real client: a 404 is a definitive ErrTaskNotFound.
	return "", fmt.Errorf("%w: no such task", ErrTaskNotFound)
}

func newTestACL(ids *fakeIdentities, ts *fakeTasks) (*ACL, *fakeIdentities, *fakeTasks) {
	if ids == nil {
		ids = &fakeIdentities{byTok: map[string]Identity{}}
	}
	if ts == nil {
		ts = &fakeTasks{byID: map[string]string{}}
	}
	acl := NewACL(ids, ts, slog.New(slog.DiscardHandler))
	return acl, ids, ts
}

// ── Session → identity (single call) ────────────────────────────────────────

func TestResolveValidSession(t *testing.T) {
	acl, ids, _ := newTestACL(nil, nil)
	ids.byTok["tok"] = Identity{
		UserID: "u1", Name: "Ada", Email: "ada@aaks.dev",
		Workspaces: []workspaces.Workspace{
			{ID: "w1", Name: "A", Role: identity.RoleOwner},
			{ID: "w2", Name: "B", Role: identity.RoleMember},
		},
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
	if ids.calls != 1 {
		t.Fatalf("resolve must be a single identity call, got %d", ids.calls)
	}
}

func TestResolveInvalidSession(t *testing.T) {
	acl, _, _ := newTestACL(nil, nil)

	if _, ok := acl.Resolve(context.Background(), "bogus"); ok {
		t.Fatal("unresolvable token must not resolve")
	}
}

// TestResolveSuperadmin checks the superadmin flag round-trips into the
// identity and the injected header.
func TestResolveSuperadmin(t *testing.T) {
	acl, ids, _ := newTestACL(nil, nil)
	ids.byTok["sadm"] = Identity{UserID: "u9", Name: "Root", Email: "root@aaks.dev", Superadmin: true}

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
	acl, _, _ := newTestACL(nil, nil)
	id := Identity{
		UserID: "u1", Name: "Ada", Email: "ada@aaks.dev", Superadmin: true,
		Workspaces: []workspaces.Workspace{
			{ID: "w1", Name: "A", Role: identity.RoleOwner},
			{ID: "w2", Name: "B", Role: identity.RoleMember},
		},
	}

	h := acl.Headers(id)

	if got := h[tenancy.HeaderUserID]; got != "u1" {
		t.Errorf("X-User-ID: got %q want u1", got)
	}
	if got := h[tenancy.HeaderUserName]; got != "Ada" {
		t.Errorf("X-User-Name: got %q want Ada", got)
	}
	if got := h[tenancy.HeaderUserEmail]; got != "ada@aaks.dev" {
		t.Errorf("X-User-Email: got %q want ada@aaks.dev", got)
	}
	if got := h[tenancy.HeaderUserSuperadmin]; got != "true" {
		t.Errorf("X-User-Superadmin: got %q want true", got)
	}
	if got := h[tenancy.HeaderUserRole]; got != "owner" {
		t.Errorf("X-User-Role: got %q want owner", got)
	}
	// Multi-workspace union: no single X-Workspace-ID, full X-Workspace-IDs.
	if got := h[tenancy.HeaderWorkspaceID]; got != "" {
		t.Errorf("X-Workspace-ID: got %q want empty (union)", got)
	}
	if got := h[tenancy.HeaderWorkspaceIDs]; got != "w1,w2" {
		t.Errorf("X-Workspace-IDs: got %q want w1,w2", got)
	}
}

func TestInjectHeadersSingleWorkspace(t *testing.T) {
	acl, _, _ := newTestACL(nil, nil)
	id := Identity{
		UserID: "u1", Name: "Ada", Email: "ada@aaks.dev",
		Workspaces: []workspaces.Workspace{{ID: "w1", Name: "A", Role: identity.RoleMember}},
	}

	h := acl.Headers(id)

	if got := h[tenancy.HeaderWorkspaceID]; got != "w1" {
		t.Errorf("X-Workspace-ID: got %q want w1", got)
	}
	if got := h[tenancy.HeaderWorkspaceIDs]; got != "w1" {
		t.Errorf("X-Workspace-IDs: got %q want w1", got)
	}
	if got := h[tenancy.HeaderUserSuperadmin]; got != "" {
		t.Errorf("X-User-Superadmin: got %q want empty", got)
	}
}

// TestHeadersNeverContainStaleValues: the map is the sole source of truth for
// the scoping headers; the HTTP adapter overwrites via Header.Set so no stale
// value can survive.
func TestHeadersNeverContainStaleValues(t *testing.T) {
	acl, _, _ := newTestACL(nil, nil)
	h := acl.Headers(Identity{
		UserID: "u1", Name: "Ada", Email: "a@b.c",
		Workspaces: []workspaces.Workspace{{ID: "w1", Role: identity.RoleMember}},
	})

	if got := h[tenancy.HeaderUserID]; got != "u1" {
		t.Errorf("X-User-ID: got %q want u1", got)
	}
	if got := h[tenancy.HeaderUserRole]; got != "member" {
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
func newClockACL(ids *fakeIdentities, now *time.Time) (*ACL, *fakeIdentities) {
	acl, fs, _ := newTestACL(ids, nil)
	acl.now = func() time.Time { return *now }
	return acl, fs
}

func TestIdentityCacheTTL(t *testing.T) {
	now := time.Now()
	acl, fs := newClockACL(nil, &now)
	fs.byTok["tok"] = Identity{UserID: "u1", Name: "Ada", Email: "a@b.c",
		Workspaces: []workspaces.Workspace{{ID: "w1", Name: "A", Role: identity.RoleMember}}}

	if _, ok := acl.Resolve(context.Background(), "tok"); !ok {
		t.Fatal("first resolve failed")
	}
	if _, ok := acl.Resolve(context.Background(), "tok"); !ok {
		t.Fatal("cached resolve failed")
	}
	if fs.calls != 1 {
		t.Fatalf("cache must absorb the second resolve: identity calls=%d", fs.calls)
	}

	// Advance past the TTL: the entry is evicted and refetched.
	now = now.Add(61 * time.Second)
	if _, ok := acl.Resolve(context.Background(), "tok"); !ok {
		t.Fatal("post-expiry resolve failed")
	}
	if fs.calls != 2 {
		t.Fatalf("expired cache must refetch: identity calls=%d", fs.calls)
	}
}

func TestFailedResolutionCached(t *testing.T) {
	now := time.Now()
	acl, fs := newClockACL(nil, &now)

	if _, ok := acl.Resolve(context.Background(), "bogus"); ok {
		t.Fatal("bogus token must not resolve")
	}
	if _, ok := acl.Resolve(context.Background(), "bogus"); ok {
		t.Fatal("bogus token must stay unresolvable")
	}
	if fs.calls != 1 {
		t.Fatalf("failed resolution must be cached: identity calls=%d", fs.calls)
	}

	// The token becomes valid later; the cached failure expires.
	now = now.Add(61 * time.Second)
	fs.byTok["bogus"] = Identity{UserID: "u1"}
	if _, ok := acl.Resolve(context.Background(), "bogus"); !ok {
		t.Fatal("expired failure must be refetched")
	}
}

// TestTransientResolutionNotCached pins the availability fix: a transport
// failure or 5xx during an Identity deploy must NOT be negative-cached —
// users recover on the next request once Identity is healthy, instead of
// waiting out the TTL with 401s.
func TestTransientResolutionNotCached(t *testing.T) {
	now := time.Now()
	acl, fs := newClockACL(nil, &now)
	fs.rejectTransient = true

	if _, ok := acl.Resolve(context.Background(), "tok"); ok {
		t.Fatal("transient failure must not resolve")
	}
	if fs.calls != 1 {
		t.Fatalf("first resolve must call identity, calls=%d", fs.calls)
	}
	// NOT cached: the second resolve retries instead of serving the failure.
	if _, ok := acl.Resolve(context.Background(), "tok"); ok {
		t.Fatal("transient failure must stay unresolvable")
	}
	if fs.calls != 2 {
		t.Fatalf("transient failure must be retried, not cached: calls=%d", fs.calls)
	}

	// Identity recovers: the very next request resolves.
	fs.rejectTransient = false
	fs.byTok["tok"] = Identity{UserID: "u1"}
	if _, ok := acl.Resolve(context.Background(), "tok"); !ok {
		t.Fatal("recovered identity must resolve immediately (no TTL wait)")
	}
}

// ── Workspace + task ownership checks ───────────────────────────────────────

func TestIsWorkspaceMember(t *testing.T) {
	acl, _, _ := newTestACL(nil, nil)
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
	acl, _, ts := newTestACL(nil, nil)
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
	// Upstream failure (timeout, 5xx) → ErrUpstream, not a fake 404.
	ts.failTransient = true
	_, err = acl.TaskAccessible(context.Background(), []string{"w1"}, "t1")
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("upstream failure: got %v want ErrUpstream", err)
	}
}
