package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// startBackend returns an httptest server that echoes the (post-strip) path it
// received in its body, so the test can assert routing + /api stripping.
func startBackend(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestGatewayRouting(t *testing.T) {
	// One fake backend per service.
	project := startBackend(t)
	task := startBackend(t)
	agent := startBackend(t)
	catalog := startBackend(t)
	settings := startBackend(t)
	runner := startBackend(t)

	t.Setenv("UPSTREAM_PROJECT", project.URL)
	t.Setenv("UPSTREAM_TASK", task.URL)
	t.Setenv("UPSTREAM_AGENT", agent.URL)
	t.Setenv("UPSTREAM_CATALOG", catalog.URL)
	t.Setenv("UPSTREAM_SETTINGS", settings.URL)
	t.Setenv("UPSTREAM_RUNNER", runner.URL)

	gw, err := newGateway()
	if err != nil {
		t.Fatalf("newGateway: %v", err)
	}

	cases := []struct {
		name   string
		path   string // full path including /api
		want   string // expected echoed path from the backend
		status int
	}{
		{"projects to project", "/api/projects", "/projects", 200},
		{"project by id", "/api/projects/abc", "/projects/abc", 200},
		{"tasks to task", "/api/tasks", "/tasks", 200},
		{"agents to agent", "/api/agents/1", "/agents/1", 200},
		{"skills to catalog", "/api/skills", "/skills", 200},
		{"mcp-servers to catalog", "/api/mcp-servers", "/mcp-servers", 200},
		{"provider-keys to settings", "/api/provider-keys", "/provider-keys", 200},
		// task sub-routes owned by runner
		{"task runs to runner", "/api/tasks/1/runs", "/tasks/1/runs", 200},
		{"task artifacts to runner", "/api/tasks/1/artifacts", "/tasks/1/artifacts", 200},
		{"runs to runner", "/api/runs/9/steps", "/runs/9/steps", 200},
		// SSE + out-of-scope domains
		{"task stream not yet wired", "/api/tasks/1/stream", "", 501},
		{"auth out of scope", "/api/auth/me", "", 501},
		{"workspaces out of scope", "/api/workspaces", "", 501},
		{"sysadmin out of scope", "/api/sysadmin/health", "", 501},
		{"unknown domain 404", "/api/whatever", "", 404},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, c.path, nil)
			rec := httptest.NewRecorder()
			gw.serve(rec, req)

			if rec.Code != c.status {
				t.Fatalf("status: got %d want %d", rec.Code, c.status)
			}
			if c.status == http.StatusOK {
				if got := rec.Body.String(); got != c.want {
					t.Fatalf("body: got %q want %q", got, c.want)
				}
			}
		})
	}
}
