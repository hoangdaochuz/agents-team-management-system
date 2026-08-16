// Package application holds the Gateway's use-case logic: the path-aware
// routing config, the session identity/membership ACL, and the SSE stream
// orchestration. It depends only on the focused ports declared here (DIP) and
// the shared-kernel contract subpackages — no net/http handler registration,
// no Kafka client, no concrete infrastructure.
package application

import (
	"errors"

	"github.com/aaks/server/internal/contracts/identity"
)

// Upstream identifies a backend service the gateway proxies to.
type Upstream string

// The ten backend services behind the gateway.
const (
	UpstreamProject   Upstream = "project"
	UpstreamTask      Upstream = "task"
	UpstreamAgent     Upstream = "agent"
	UpstreamCatalog   Upstream = "catalog"
	UpstreamSettings  Upstream = "settings"
	UpstreamRunner    Upstream = "runner"
	UpstreamAuth      Upstream = "auth"
	UpstreamOrgs      Upstream = "orgs"
	UpstreamResources Upstream = "resources"
	UpstreamAdmin     Upstream = "admin"
)

// RouteKind discriminates how a resolved request is handled.
type RouteKind int

const (
	// RouteProxy forwards to the owning upstream (after identity injection).
	RouteProxy RouteKind = iota
	// RouteSession composes /auth/login + /auth/me into the full Session shape.
	RouteSession
	// RouteStream serves the SSE step stream for a task.
	RouteStream
	// RouteWorkspaceRemap forwards a /workspaces/{wid}/... sub-route to the
	// owning upstream after the workspace membership check.
	RouteWorkspaceRemap
	// RouteTaskRuns forwards /tasks/{id}/runs|artifacts to the runner after the
	// task ownership check.
	RouteTaskRuns
	// RouteWorkspacesList composes the enriched workspace list (GET /workspaces).
	RouteWorkspacesList
	// RouteKpis composes the sysadmin KPIs.
	RouteKpis
	// RouteHealth composes the sysadmin service health snapshot.
	RouteHealth
	// RouteSysadminAdmin forwards the admin-owned sysadmin surface.
	RouteSysadminAdmin
)

// Route is the handling plan for one /api request, resolved by RouteTable.
type Route struct {
	Kind            RouteKind
	Domain          string
	Upstream        Upstream    // proxy target (Proxy/WorkspaceRemap/TaskRuns/SysadminAdmin)
	TaskCheck       Upstream    // upstream resolving task ownership (TaskRuns/Stream)
	RequireIdentity bool        // session required (401 when absent)
	WorkspaceID     string      // workspace membership check target (WorkspaceRemap)
	TaskID          identity.ID // task ownership check target (TaskRuns/Stream)
}

// ErrNotFound is returned for a request with no routable path.
var ErrNotFound = errors.New("not found")

// NoRouteError carries the unknown routing domain (404 "no route for domain").
type NoRouteError struct {
	Domain string
}

// Error implements error.
func (e NoRouteError) Error() string { return "no route for domain: " + e.Domain }

// RouteTable is the static routing config: it maps /api/<domain>/... to its
// owning upstream and encodes the cross-service remaps the gateway performs
// (workspace sub-routes to catalog/resources/admin, task sub-routes to the
// runner, and the composed session/workspaces/kpis/health endpoints).
type RouteTable struct {
	domains map[string]Upstream
	tenant  map[string]bool
}

// NewRouteTable builds the static routing config, mirroring the pre-DDD
// gateway's domain table and tenant-domain set.
func NewRouteTable() *RouteTable {
	return &RouteTable{
		domains: map[string]Upstream{
			"projects":      UpstreamProject,
			"tasks":         UpstreamTask,
			"agents":        UpstreamAgent,
			"skills":        UpstreamCatalog,
			"mcp-servers":   UpstreamCatalog,
			"provider-keys": UpstreamSettings,
			"runs":          UpstreamRunner,
			"auth":          UpstreamAuth,
			"sysadmin":      UpstreamOrgs,
			"workspaces":    UpstreamOrgs,
			"orgs":          UpstreamOrgs,
			"resources":     UpstreamResources,
			"admin":         UpstreamAdmin,
		},
		tenant: map[string]bool{
			"workspaces": true, "sysadmin": true, "tasks": true, "agents": true,
			"projects": true, "skills": true, "mcp-servers": true, "provider-keys": true,
			"runs": true, "auth": true, "orgs": true, "resources": true, "admin": true,
		},
	}
}

// Resolve decides how to handle a request with the given path segments (after
// the /api prefix is stripped) and HTTP method. The decision order mirrors the
// pre-DDD gateway handler: realtime stream, session composition, workspace
// remaps, task remaps, then the tenant/proxy fallbacks.
func (t *RouteTable) Resolve(segs []string, method string) (Route, error) {
	if len(segs) == 0 || segs[0] == "" {
		return Route{}, ErrNotFound
	}
	domain := segs[0]

	// Realtime: /tasks/{id}/stream — replay then tail (SSE).
	if domain == "tasks" && len(segs) >= 3 && segs[2] == "stream" {
		return Route{
			Kind: RouteStream, Domain: domain, TaskCheck: UpstreamTask,
			TaskID: identity.ID(segs[1]), RequireIdentity: true,
		}, nil
	}

	// Session composition for /auth/login (POST) + /auth/me (GET).
	if domain == "auth" && len(segs) >= 2 && (segs[1] == "login" || segs[1] == "me") {
		return Route{Kind: RouteSession, Domain: domain}, nil
	}

	// Workspace sub-routes owned by other services.
	if domain == "workspaces" && len(segs) >= 3 {
		switch segs[2] {
		case "skills":
			return Route{
				Kind: RouteWorkspaceRemap, Domain: domain, Upstream: UpstreamCatalog,
				WorkspaceID: segs[1], RequireIdentity: true,
			}, nil
		case "knowledge", "plugins", "rules", "mcp":
			return Route{
				Kind: RouteWorkspaceRemap, Domain: domain, Upstream: UpstreamResources,
				WorkspaceID: segs[1], RequireIdentity: true,
			}, nil
		case "audit":
			return Route{
				Kind: RouteWorkspaceRemap, Domain: domain, Upstream: UpstreamAdmin,
				WorkspaceID: segs[1], RequireIdentity: true,
			}, nil
		default:
			// Workspace sub-routes owned by orgs (members, invites, requests).
			return Route{
				Kind: RouteWorkspaceRemap, Domain: domain, Upstream: UpstreamOrgs,
				WorkspaceID: segs[1], RequireIdentity: true,
			}, nil
		}
	}

	// Task sub-routes owned by the runner: /tasks/{id}/runs|artifacts.
	if domain == "tasks" && len(segs) >= 3 && (segs[2] == "runs" || segs[2] == "artifacts") {
		return Route{
			Kind: RouteTaskRuns, Domain: domain, Upstream: UpstreamRunner,
			TaskCheck: UpstreamTask, TaskID: identity.ID(segs[1]), RequireIdentity: true,
		}, nil
	}

	// Unknown domains have no route.
	if !t.tenant[domain] {
		return Route{}, NoRouteError{Domain: domain}
	}

	// Workspace list enrichment (derived stats from Agent + Task).
	if domain == "workspaces" && len(segs) == 1 && method == "GET" {
		return Route{Kind: RouteWorkspacesList, Domain: domain, RequireIdentity: true}, nil
	}

	// Sysadmin composition endpoints.
	if domain == "sysadmin" && len(segs) >= 2 {
		switch segs[1] {
		case "kpis":
			return Route{Kind: RouteKpis, Domain: domain, RequireIdentity: true}, nil
		case "health":
			return Route{Kind: RouteHealth, Domain: domain, RequireIdentity: true}, nil
		case "flags", "audit", "maintenance":
			// Admin-owned sysadmin surface (flags/audit/maintenance).
			return Route{
				Kind: RouteSysadminAdmin, Domain: domain, Upstream: UpstreamAdmin,
				RequireIdentity: true,
			}, nil
		}
	}

	// Default: proxy to the owning upstream. Identity is required on every
	// tenant domain except auth, whose routes are public (signup,
	// signup-status, login) or cookie-driven (logout).
	return Route{
		Kind: RouteProxy, Domain: domain, Upstream: t.domains[domain],
		RequireIdentity: domain != "auth",
	}, nil
}