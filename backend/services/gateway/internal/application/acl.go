// ACL logic: the session cookie → identity (Auth) → workspace union (Orgs)
// chain, cached for 60s, plus the scoping-header injection and the workspace /
// task ownership checks that gate cross-service routes.
package application

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/workspaces"
	"github.com/aaks/server/internal/platform/tenancy"
)

// Session is the identity the Auth service resolves for a session token.
type Session struct {
	UserID     string
	Name       string
	Email      string
	Superadmin bool
}

// SessionClient resolves a session token to a user identity (Auth service).
type SessionClient interface {
	Resolve(ctx context.Context, token string) (Session, error)
}

// MembershipClient lists a user's workspace memberships (Orgs service).
type MembershipClient interface {
	List(ctx context.Context, userID string) ([]workspaces.Workspace, error)
}

// TaskWorkspaceClient resolves the workspace that owns a task (Task service).
type TaskWorkspaceClient interface {
	Workspace(ctx context.Context, taskID identity.ID) (identity.ID, error)
}

// ErrTaskNotFound signals that the task service could not resolve the task.
var ErrTaskNotFound = errors.New("task not found")

// Identity is the Gateway's resolved session view: the Auth user plus the
// Orgs workspace union.
type Identity struct {
	UserID     string
	Name       string
	Email      string
	Superadmin bool
	Workspaces []workspaces.Workspace
	resolvedAt time.Time
}

// ACL resolves sessions into identities and injects the tenancy scoping
// headers. Identity + workspace union are cached per token for 60s; failed
// resolutions are cached too so a bad token cannot hammer Auth/Orgs.
type ACL struct {
	sessions    SessionClient
	memberships MembershipClient
	tasks       TaskWorkspaceClient
	log         *slog.Logger
	ttl         time.Duration
	now         func() time.Time
	cache       sync.Map // token -> Identity
}

// NewACL builds the ACL service with the injected inter-service clients.
func NewACL(sessions SessionClient, memberships MembershipClient, tasks TaskWorkspaceClient, log *slog.Logger) *ACL {
	return &ACL{
		sessions: sessions, memberships: memberships, tasks: tasks,
		log: log, ttl: 60 * time.Second, now: time.Now,
	}
}

// Resolve returns the cached identity for token, or fetches it from Auth +
// Orgs and caches it for the TTL. The second result reports whether the token
// resolved to a real user (a cached failed resolution returns false).
func (a *ACL) Resolve(ctx context.Context, token string) (Identity, bool) {
	if v, ok := a.cache.Load(token); ok {
		id := v.(Identity)
		if a.now().Sub(id.resolvedAt) < a.ttl {
			return id, id.UserID != ""
		}
		// Stale entry: evict it now so the cache cannot grow without bound
		// (each distinct token is only ever held for one TTL window).
		a.cache.Delete(token)
	}
	id := Identity{resolvedAt: a.now()}
	u, err := a.sessions.Resolve(ctx, token)
	if err != nil {
		a.cache.Store(token, Identity{resolvedAt: a.now()})
		return Identity{}, false
	}
	id.UserID, id.Name, id.Email, id.Superadmin = u.UserID, u.Name, u.Email, u.Superadmin
	// Membership failure is non-fatal: the identity stays valid with an empty
	// workspace union (the pre-DDD gateway behaved the same way).
	if wss, err := a.memberships.List(ctx, u.UserID); err == nil {
		id.Workspaces = wss
	}
	a.cache.Store(token, id)
	return id, true
}

// Inject writes the identity/scoping headers for id onto h, replacing any
// prior values. Only the gateway may populate these headers — the request was
// stripped of forged values before routing, so Inject is the sole writer.
func (a *ACL) Inject(h http.Header, id Identity) {
	h.Del(tenancy.HeaderUserID)
	h.Del(tenancy.HeaderUserName)
	h.Del(tenancy.HeaderUserEmail)
	h.Del(tenancy.HeaderUserSuperadmin)
	h.Del(tenancy.HeaderUserRole)
	h.Del(tenancy.HeaderWorkspaceID)
	h.Del(tenancy.HeaderWorkspaceIDs)
	h.Set(tenancy.HeaderUserID, id.UserID)
	h.Set(tenancy.HeaderUserName, id.Name)
	h.Set(tenancy.HeaderUserEmail, id.Email)
	if id.Superadmin {
		h.Set(tenancy.HeaderUserSuperadmin, "true")
	}
	ids := make([]string, 0, len(id.Workspaces))
	for _, ws := range id.Workspaces {
		ids = append(ids, string(ws.ID))
	}
	if len(ids) == 1 {
		h.Set(tenancy.HeaderWorkspaceID, ids[0])
	}
	if len(ids) > 0 {
		h.Set(tenancy.HeaderWorkspaceIDs, strings.Join(ids, ","))
	}
	// X-User-Role is the strongest role the session holds across its workspace
	// union (owner > admin > member). Trustworthy because it is derived from
	// the Orgs memberships the Gateway resolved — never from the client.
	if role := StrongestRole(id.Workspaces); role != "" {
		h.Set(tenancy.HeaderUserRole, string(role))
	}
}

// StrongestRole returns the highest-privilege role in the workspace union.
func StrongestRole(wss []workspaces.Workspace) identity.Role {
	role := identity.Role("")
	for _, ws := range wss {
		switch ws.Role {
		case identity.RoleOwner:
			return identity.RoleOwner
		case identity.RoleAdmin:
			if role != identity.RoleAdmin {
				role = identity.RoleAdmin
			}
		case identity.RoleMember:
			if role == "" {
				role = identity.RoleMember
			}
		}
	}
	return role
}

// IsWorkspaceMember reports whether wid is in the caller's resolved workspace
// union (the caller passes the union extracted from the injected headers).
func (a *ACL) IsWorkspaceMember(workspaceIDs []string, wid string) bool {
	for _, id := range workspaceIDs {
		if id == wid {
			return true
		}
	}
	return false
}

// TaskAccessible reports whether taskID belongs to a workspace in the
// caller's union. ErrTaskNotFound is returned when the task service cannot
// resolve the task (404); a false result with nil error means the task exists
// but is outside the caller's workspaces (403).
func (a *ACL) TaskAccessible(ctx context.Context, workspaceIDs []string, taskID identity.ID) (bool, error) {
	ws, err := a.tasks.Workspace(ctx, taskID)
	if err != nil || ws == "" {
		return false, ErrTaskNotFound
	}
	for _, id := range workspaceIDs {
		if id == string(ws) {
			return true, nil
		}
	}
	return false, nil
}