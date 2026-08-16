package application

import (
	"errors"
	"testing"
)

func TestResolveRouting(t *testing.T) {
	tbl := NewRouteTable()

	cases := []struct {
		name    string
		segs    []string
		method  string
		kind    RouteKind
		upstream Upstream
		wsID    string
		taskID  string
		require bool
		err     error
	}{
		{"empty path", []string{""}, "GET", 0, "", "", "", false, ErrNotFound},
		{"no segments", nil, "GET", 0, "", "", "", false, ErrNotFound},
		{"projects proxy", []string{"projects"}, "GET", RouteProxy, UpstreamProject, "", "", true, nil},
		{"project by id", []string{"projects", "abc"}, "GET", RouteProxy, UpstreamProject, "", "", true, nil},
		{"tasks proxy", []string{"tasks"}, "GET", RouteProxy, UpstreamTask, "", "", true, nil},
		{"agents proxy", []string{"agents", "1"}, "GET", RouteProxy, UpstreamAgent, "", "", true, nil},
		{"skills to catalog", []string{"skills"}, "GET", RouteProxy, UpstreamCatalog, "", "", true, nil},
		{"mcp-servers to catalog", []string{"mcp-servers"}, "GET", RouteProxy, UpstreamCatalog, "", "", true, nil},
		{"provider-keys to settings", []string{"provider-keys"}, "GET", RouteProxy, UpstreamSettings, "", "", true, nil},
		{"runs to runner", []string{"runs", "9", "steps"}, "GET", RouteProxy, UpstreamRunner, "", "", true, nil},
		{"resources proxy", []string{"resources"}, "GET", RouteProxy, UpstreamResources, "", "", true, nil},
		{"admin proxy", []string{"admin"}, "GET", RouteProxy, UpstreamAdmin, "", "", true, nil},
		{"orgs proxy", []string{"orgs"}, "GET", RouteProxy, UpstreamOrgs, "", "", true, nil},
		{"sysadmin orgs to orgs", []string{"sysadmin", "orgs"}, "GET", RouteProxy, UpstreamOrgs, "", "", true, nil},
		// Task sub-routes owned by the runner.
		{"task runs to runner", []string{"tasks", "1", "runs"}, "GET", RouteTaskRuns, UpstreamRunner, "", "1", true, nil},
		{"task artifacts to runner", []string{"tasks", "1", "artifacts"}, "GET", RouteTaskRuns, UpstreamRunner, "", "1", true, nil},
		{"task stream", []string{"tasks", "1", "stream"}, "GET", RouteStream, "", "", "1", true, nil},
		// Workspace sub-routes.
		{"skills remap to catalog", []string{"workspaces", "w1", "skills"}, "GET", RouteWorkspaceRemap, UpstreamCatalog, "w1", "", true, nil},
		{"knowledge remap to resources", []string{"workspaces", "w1", "knowledge"}, "GET", RouteWorkspaceRemap, UpstreamResources, "w1", "", true, nil},
		{"plugins remap to resources", []string{"workspaces", "w1", "plugins"}, "GET", RouteWorkspaceRemap, UpstreamResources, "w1", "", true, nil},
		{"rules remap to resources", []string{"workspaces", "w1", "rules"}, "GET", RouteWorkspaceRemap, UpstreamResources, "w1", "", true, nil},
		{"mcp remap to resources", []string{"workspaces", "w1", "mcp"}, "GET", RouteWorkspaceRemap, UpstreamResources, "w1", "", true, nil},
		{"audit remap to admin", []string{"workspaces", "w1", "audit"}, "GET", RouteWorkspaceRemap, UpstreamAdmin, "w1", "", true, nil},
		{"audit export remap to admin", []string{"workspaces", "w1", "audit", "export"}, "GET", RouteWorkspaceRemap, UpstreamAdmin, "w1", "", true, nil},
		{"members remap to orgs", []string{"workspaces", "w1", "members"}, "GET", RouteWorkspaceRemap, UpstreamOrgs, "w1", "", true, nil},
		{"requests remap to orgs", []string{"workspaces", "w1", "requests"}, "GET", RouteWorkspaceRemap, UpstreamOrgs, "w1", "", true, nil},
		{"workspace get to orgs", []string{"workspaces", "w1"}, "GET", RouteProxy, UpstreamOrgs, "", "", true, nil},
		{"workspace list (GET)", []string{"workspaces"}, "GET", RouteWorkspacesList, "", "", "", true, nil},
		{"workspace create (POST)", []string{"workspaces"}, "POST", RouteProxy, UpstreamOrgs, "", "", true, nil},
		// Sysadmin surface.
		{"sysadmin kpis", []string{"sysadmin", "kpis"}, "GET", RouteKpis, "", "", "", true, nil},
		{"sysadmin health", []string{"sysadmin", "health"}, "GET", RouteHealth, "", "", "", true, nil},
		{"sysadmin flags to admin", []string{"sysadmin", "flags"}, "GET", RouteSysadminAdmin, UpstreamAdmin, "", "", true, nil},
		{"sysadmin audit to admin", []string{"sysadmin", "audit"}, "GET", RouteSysadminAdmin, UpstreamAdmin, "", "", true, nil},
		{"sysadmin maintenance to admin", []string{"sysadmin", "maintenance"}, "GET", RouteSysadminAdmin, UpstreamAdmin, "", "", true, nil},
		// Session composition.
		{"auth login", []string{"auth", "login"}, "POST", RouteSession, "", "", "", false, nil},
		{"auth me", []string{"auth", "me"}, "GET", RouteSession, "", "", "", false, nil},
		// Public auth surface passes through without identity requirement.
		{"auth signup", []string{"auth", "signup"}, "POST", RouteProxy, UpstreamAuth, "", "", false, nil},
		{"auth signup-status", []string{"auth", "signup-status"}, "GET", RouteProxy, UpstreamAuth, "", "", false, nil},
		{"auth logout", []string{"auth", "logout"}, "POST", RouteProxy, UpstreamAuth, "", "", false, nil},
		// Unknown domain.
		{"unknown domain", []string{"whatever"}, "GET", 0, "", "", "", false, NoRouteError{Domain: "whatever"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := tbl.Resolve(c.segs, c.method)
			if c.err != nil {
				if !errors.Is(err, c.err) {
					t.Fatalf("error: got %v want %v", err, c.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if got.Kind != c.kind {
				t.Errorf("kind: got %v want %v", got.Kind, c.kind)
			}
			if got.Upstream != c.upstream {
				t.Errorf("upstream: got %q want %q", got.Upstream, c.upstream)
			}
			if got.WorkspaceID != c.wsID {
				t.Errorf("workspace id: got %q want %q", got.WorkspaceID, c.wsID)
			}
			if string(got.TaskID) != c.taskID {
				t.Errorf("task id: got %q want %q", got.TaskID, c.taskID)
			}
			if got.RequireIdentity != c.require {
				t.Errorf("require identity: got %v want %v", got.RequireIdentity, c.require)
			}
		})
	}
}

// TestResolveTaskRunsCheckUpstream locks the ownership-check upstream: task
// sub-routes owned by the runner resolve their ownership against the Task
// service.
func TestResolveTaskRunsCheckUpstream(t *testing.T) {
	tbl := NewRouteTable()

	r, err := tbl.Resolve([]string{"tasks", "1", "runs"}, "GET")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if r.TaskCheck != UpstreamTask {
		t.Fatalf("task check: got %q want %q", r.TaskCheck, UpstreamTask)
	}
	r, err = tbl.Resolve([]string{"tasks", "1", "stream"}, "GET")
	if err != nil {
		t.Fatalf("resolve stream: %v", err)
	}
	if r.TaskCheck != UpstreamTask {
		t.Fatalf("stream task check: got %q want %q", r.TaskCheck, UpstreamTask)
	}
}