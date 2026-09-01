// ACL logic: the session cookie → full identity (user + workspace union,
// resolved in a single Identity-service call), cached for 60s, plus the
// scoping-header injection and the workspace / task ownership checks that
// gate cross-service routes.
package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/workspaces"
	"github.com/aaks/server/internal/platform/tenancy"
)

// IdentityClient resolves a session token to the full identity view — user
// plus workspace union — in a single call to the Identity service.
type IdentityClient interface {
	Resolve(ctx context.Context, token string) (Identity, error)
}

// TaskWorkspaceClient resolves the workspace that owns a task (Task service).
type TaskWorkspaceClient interface {
	Workspace(ctx context.Context, taskID identity.ID) (identity.ID, error)
}

// ErrTaskNotFound signals that the task service could not resolve the task.
var ErrTaskNotFound = errors.New("task not found")

// ErrSessionRejected marks a definitive rejection: the Identity service saw
// the token and answered "no session". Only these are negative-cached — a
// transport failure or a 5xx during an Identity deploy is transient and must
// not pin 401s for the whole TTL.
var ErrSessionRejected = errors.New("session rejected")

// ErrUpstream marks an upstream failure that is not a definitive answer (the
// task workspace lookup timed out or 5xx'd). The HTTP layer reports these as
// 502 rather than masquerading as 404.
var ErrUpstream = errors.New("upstream unavailable")

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
// headers. The identity (user + workspace union) is cached per token for 60s;
// failed resolutions are cached too so a bad token cannot hammer Identity.
type ACL struct {
	identities IdentityClient
	tasks      TaskWorkspaceClient
	log        *slog.Logger
	ttl        time.Duration
	now        func() time.Time
	cache      sync.Map // token -> Identity
}

// NewACL builds the ACL service with the injected inter-service clients.
func NewACL(identities IdentityClient, tasks TaskWorkspaceClient, log *slog.Logger) *ACL {
	return &ACL{
		identities: identities, tasks: tasks,
		log: log, ttl: 60 * time.Second, now: time.Now,
	}
}

// Resolve returns the cached identity for token, or fetches it from the
// Identity service (one call: user + workspace union) and caches it for the
// TTL. The second result reports whether the token resolved to a real user.
// A definitive rejection (ErrSessionRejected) is negative-cached so a bad
// token cannot hammer Identity; a transient failure (transport error, 5xx
// during an Identity deploy) returns false WITHOUT caching, so users recover
// on their next request once Identity is healthy instead of waiting out the
// TTL with 401s.
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
	id, err := a.identities.Resolve(ctx, token)
	if err != nil {
		if errors.Is(err, ErrSessionRejected) {
			a.cache.Store(token, Identity{resolvedAt: a.now()})
		} else {
			a.log.Warn("session resolution failed (not cached; transient)", "error", err)
		}
		return Identity{}, false
	}
	id.resolvedAt = a.now()
	a.cache.Store(token, id)
	return id, true
}

// StartJanitor evicts stale cache entries periodically until ctx is done.
// Re-read eviction alone cannot bound the map against a client that rotates
// distinct cookie values (those entries are never re-read); the janitor is
// the backstop. Wire it to the service lifecycle ctx in the composition root.
func (a *ACL) StartJanitor(ctx context.Context) {
	t := time.NewTicker(a.ttl)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.cache.Range(func(key, val any) bool {
				if id, ok := val.(Identity); !ok || a.now().Sub(id.resolvedAt) >= a.ttl {
					a.cache.Delete(key)
				}
				return true
			})
		}
	}
}

// Headers returns the identity/scoping header values for id. The HTTP adapter
// writes them onto the routed request (application stays free of net/http).
// The caller's request was stripped of forged values before routing, so these
// values are the sole source of truth for the scoping headers.
func (a *ACL) Headers(id Identity) map[string]string {
	h := map[string]string{
		tenancy.HeaderUserID:    id.UserID,
		tenancy.HeaderUserName:  id.Name,
		tenancy.HeaderUserEmail: id.Email,
	}
	if id.Superadmin {
		h[tenancy.HeaderUserSuperadmin] = "true"
	}
	ids := make([]string, 0, len(id.Workspaces))
	for _, ws := range id.Workspaces {
		ids = append(ids, string(ws.ID))
	}
	if len(ids) == 1 {
		h[tenancy.HeaderWorkspaceID] = ids[0]
	}
	if len(ids) > 0 {
		h[tenancy.HeaderWorkspaceIDs] = strings.Join(ids, ",")
	}
	// X-User-Role is the strongest role the session holds across its workspace
	// union (owner > admin > member). Trustworthy because it is derived from
	// the Orgs memberships the Gateway resolved - never from the client.
	if role := StrongestRole(id.Workspaces); role != "" {
		h[tenancy.HeaderUserRole] = string(role)
	}
	return h
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
// caller's union. ErrTaskNotFound is returned when the task service gave a
// definitive "no such task" answer; an upstream failure (timeout, 5xx) wraps
// ErrUpstream so the HTTP layer can answer 502 instead of a misleading 404;
// a false result with nil error means the task exists but is outside the
// caller's workspaces (403).
func (a *ACL) TaskAccessible(ctx context.Context, workspaceIDs []string, taskID identity.ID) (bool, error) {
	ws, err := a.tasks.Workspace(ctx, taskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return false, ErrTaskNotFound
		}
		return false, fmt.Errorf("%w: task workspace lookup: %w", ErrUpstream, err)
	}
	if ws == "" {
		return false, ErrTaskNotFound
	}
	for _, id := range workspaceIDs {
		if id == string(ws) {
			return true, nil
		}
	}
	return false, nil
}
