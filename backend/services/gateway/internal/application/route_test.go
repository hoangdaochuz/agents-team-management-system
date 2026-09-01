package application

import (
	"errors"
	"testing"
)

func TestResolveRouting(t *testing.T) {
	tbl := NewRouteTable()

	cases := []struct {
		name     string
		segs     []string
		method   string
		kind     RouteKind
		upstream Upstream
		wsID     string
		taskID   string
		require  bool
		err      error
	}{
		{"empty path", []string{""}, "GET", 0, "", "", "", false, ErrNotFound},
		{"no segments", nil, "GET", 0, "", "", "", false, ErrNotFound},
		{"projects proxy to workspace", []string{"projects"}, "GET", RouteProxy, UpstreamWorkspace, "", "", true, nil},
		{"project by id to workspace", []string{"projects", "abc"}, "GET", RouteProxy, UpstreamWorkspace, "", "", true, nil},
		{"tasks proxy to workspace", []string{"tasks"}, "GET", RouteProxy, UpstreamWorkspace, "", "", true, nil},
		{"agents proxy", []string{"agents", "1"}, "GET", RouteProxy, UpstreamAgent, "", "", true, nil},
		{"skills to workspace", []string{"skills"}, "GET", RouteProxy, UpstreamWorkspace, "", "", true, nil},
		{"mcp-servers to workspace", []string{"mcp-servers"}, "GET", RouteProxy, UpstreamWorkspace, "", "", true, nil},
		{"provider-keys to agent", []string{"provider-keys"}, "GET", RouteProxy, UpstreamAgent, "", "", true, nil},
		{"runs to executor", []string{"runs", "9", "steps"}, "GET", RouteProxy, UpstreamExecutor, "", "", true, nil},
		{"resources proxy", []string{"resources"}, "GET", RouteProxy, UpstreamWorkspace, "", "", true, nil},
		{"admin proxy to identity", []string{"admin"}, "GET", RouteProxy, UpstreamIdentity, "", "", true, nil},
		{"orgs proxy to identity", []string{"orgs"}, "GET", RouteProxy, UpstreamIdentity, "", "", true, nil},
		{"sysadmin orgs to identity", []string{"sysadmin", "orgs"}, "GET", RouteProxy, UpstreamIdentity, "", "", true, nil},
		// Task sub-routes owned by the runner.
		{"task runs to executor", []string{"tasks", "1", "runs"}, "GET", RouteTaskRuns, UpstreamExecutor, "", "1", true, nil},
		{"task artifacts to executor", []string{"tasks", "1", "artifacts"}, "GET", RouteTaskRuns, UpstreamExecutor, "", "1", true, nil},
		{"task stream", []string{"tasks", "1", "stream"}, "GET", RouteStream, "", "", "1", true, nil},
		// Workspace sub-routes.
		{"skills remap to workspace", []string{"workspaces", "w1", "skills"}, "GET", RouteWorkspaceRemap, UpstreamWorkspace, "w1", "", true, nil},
		{"knowledge remap to workspace", []string{"workspaces", "w1", "knowledge"}, "GET", RouteWorkspaceRemap, UpstreamWorkspace, "w1", "", true, nil},
		{"plugins remap to workspace", []string{"workspaces", "w1", "plugins"}, "GET", RouteWorkspaceRemap, UpstreamWorkspace, "w1", "", true, nil},
		{"rules remap to workspace", []string{"workspaces", "w1", "rules"}, "GET", RouteWorkspaceRemap, UpstreamWorkspace, "w1", "", true, nil},
		{"mcp remap to workspace", []string{"workspaces", "w1", "mcp"}, "GET", RouteWorkspaceRemap, UpstreamWorkspace, "w1", "", true, nil},
		{"audit remap to identity", []string{"workspaces", "w1", "audit"}, "GET", RouteWorkspaceRemap, UpstreamIdentity, "w1", "", true, nil},
		{"audit export remap to identity", []string{"workspaces", "w1", "audit", "export"}, "GET", RouteWorkspaceRemap, UpstreamIdentity, "w1", "", true, nil},
		{"members remap to identity", []string{"workspaces", "w1", "members"}, "GET", RouteWorkspaceRemap, UpstreamIdentity, "w1", "", true, nil},
		{"requests remap to identity", []string{"workspaces", "w1", "requests"}, "GET", RouteWorkspaceRemap, UpstreamIdentity, "w1", "", true, nil},
		{"workspace get to identity", []string{"workspaces", "w1"}, "GET", RouteProxy, UpstreamIdentity, "", "", true, nil},
		{"workspace list (GET)", []string{"workspaces"}, "GET", RouteWorkspacesList, "", "", "", true, nil},
		{"workspace create (POST)", []string{"workspaces"}, "POST", RouteProxy, UpstreamIdentity, "", "", true, nil},
		// Sysadmin surface.
		{"sysadmin kpis", []string{"sysadmin", "kpis"}, "GET", RouteKpis, "", "", "", true, nil},
		{"sysadmin health", []string{"sysadmin", "health"}, "GET", RouteHealth, "", "", "", true, nil},
		{"sysadmin flags to identity", []string{"sysadmin", "flags"}, "GET", RouteSysadminAdmin, UpstreamIdentity, "", "", true, nil},
		{"sysadmin audit to identity", []string{"sysadmin", "audit"}, "GET", RouteSysadminAdmin, UpstreamIdentity, "", "", true, nil},
		{"sysadmin maintenance to identity", []string{"sysadmin", "maintenance"}, "GET", RouteSysadminAdmin, UpstreamIdentity, "", "", true, nil},
		// Session composition.
		{"auth login", []string{"auth", "login"}, "POST", RouteSession, "", "", "", false, nil},
		{"auth me", []string{"auth", "me"}, "GET", RouteSession, "", "", "", false, nil},
		// Public auth surface passes through without identity requirement.
		{"auth signup", []string{"auth", "signup"}, "POST", RouteProxy, UpstreamIdentity, "", "", false, nil},
		{"auth signup-status", []string{"auth", "signup-status"}, "GET", RouteProxy, UpstreamIdentity, "", "", false, nil},
		{"auth logout", []string{"auth", "logout"}, "POST", RouteProxy, UpstreamIdentity, "", "", false, nil},
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

// TestResolveTaskRoutes pins the task sub-route kinds: runs/artifacts remap to
// the executor and stream resolves as the SSE route (ownership is always
// checked against the workspace ACL in the HTTP adapter).
func TestResolveTaskRoutes(t *testing.T) {
	tbl := NewRouteTable()

	r, err := tbl.Resolve([]string{"tasks", "1", "runs"}, "GET")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if r.Kind != RouteTaskRuns || r.TaskID != "1" {
		t.Fatalf("runs route: got kind=%v taskID=%q", r.Kind, r.TaskID)
	}
	r, err = tbl.Resolve([]string{"tasks", "1", "stream"}, "GET")
	if err != nil {
		t.Fatalf("resolve stream: %v", err)
	}
	if r.Kind != RouteStream || r.TaskID != "1" {
		t.Fatalf("stream route: got kind=%v taskID=%q", r.Kind, r.TaskID)
	}
}
