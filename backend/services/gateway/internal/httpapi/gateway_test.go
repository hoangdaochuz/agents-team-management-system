package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aaks/server/internal/contracts"
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

// startTaskBackend fakes the Task service: it echoes ordinary paths (routing
// assertions) and resolves the internal task→workspace endpoint like the real
// service, which the Gateway's ownership check depends on.
func startTaskBackend(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/internal/tasks/") {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"workspace_id":"w1"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// startAuthBackend fakes the Auth service: user JSON for /auth/* session
// routes and an identity for /internal/identity.
func startAuthBackend(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/identity":
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"user_id":"u1","name":"Ada","email":"ada@aaks.dev","is_superadmin":false}`)
		case "/auth/login", "/auth/me":
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":"u1","name":"Ada","email":"ada@aaks.dev"}`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// startOrgsBackend fakes the Orgs service: workspace JSON lists.
func startOrgsBackend(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/users/u1/workspaces":
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `[{"id":"w1","name":"A","role":"owner"},{"id":"w2","name":"B","role":"member"}]`)
		case "/workspaces":
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `[{"id":"w1","name":"A","role":"owner"}]`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newTestGateway builds a gateway against fake backends for all upstreams.
func newTestGateway(t *testing.T) (*gateway, *httptest.Server) {
	t.Helper()
	project := startBackend(t)
	task := startTaskBackend(t)
	agent := startBackend(t)
	catalog := startBackend(t)
	settings := startBackend(t)
	runner := startBackend(t)
	auth := startAuthBackend(t)
	orgs := startOrgsBackend(t)
	resources := startBackend(t)
	admin := startBackend(t)

	t.Setenv("UPSTREAM_PROJECT", project.URL)
	t.Setenv("UPSTREAM_TASK", task.URL)
	t.Setenv("UPSTREAM_AGENT", agent.URL)
	t.Setenv("UPSTREAM_CATALOG", catalog.URL)
	t.Setenv("UPSTREAM_SETTINGS", settings.URL)
	t.Setenv("UPSTREAM_RUNNER", runner.URL)
	t.Setenv("UPSTREAM_AUTH", auth.URL)
	t.Setenv("UPSTREAM_ORGS", orgs.URL)
	t.Setenv("UPSTREAM_RESOURCES", resources.URL)
	t.Setenv("UPSTREAM_ADMIN", admin.URL)

	gw, err := newGateway(nil)
	if err != nil {
		t.Fatalf("newGateway: %v", err)
	}
	return gw, auth
}

func TestGatewayRouting(t *testing.T) {
	gw, _ := newTestGateway(t)

	cases := []struct {
		name    string
		path    string // full path including /api
		want    string // expected echoed path from the backend
		status  int
		session bool
	}{
		{"projects to project", "/api/projects", "/projects", 200, true},
		{"project by id", "/api/projects/abc", "/projects/abc", 200, true},
		{"tasks to task", "/api/tasks", "/tasks", 200, true},
		{"agents to agent", "/api/agents/1", "/agents/1", 200, true},
		{"skills to catalog", "/api/skills", "/skills", 200, true},
		{"mcp-servers to catalog", "/api/mcp-servers", "/mcp-servers", 200, true},
		{"provider-keys to settings", "/api/provider-keys", "/provider-keys", 200, true},
		// task sub-routes owned by runner (require a session)
		{"task runs to runner", "/api/tasks/1/runs", "/tasks/1/runs", 200, true},
		{"task artifacts to runner", "/api/tasks/1/artifacts", "/tasks/1/artifacts", 200, true},
		{"runs to runner", "/api/runs/9/steps", "/runs/9/steps", 200, true},
		// workspace sub-routes to resources
		{"workspace rules to resources", "/api/workspaces/w1/rules", "/workspaces/w1/rules", 200, true},
		{"workspace knowledge to resources", "/api/workspaces/w1/knowledge", "/workspaces/w1/knowledge", 200, true},
		// protected routes reject requests without a session
		{"tasks without session 401", "/api/tasks", "", 401, false},
		{"agents without session 401", "/api/agents", "", 401, false},
		{"projects without session 401", "/api/projects", "", 401, false},
		{"provider-keys without session 401", "/api/provider-keys", "", 401, false},
		// public auth surface passes through without a session
		{"signup without session", "/api/auth/signup", "/auth/signup", 200, false},
		{"signup-status without session", "/api/auth/signup-status", "/auth/signup-status", 200, false},
		{"logout without session", "/api/auth/logout", "/auth/logout", 200, false},
		// tenant domains without a session pass through (public surface)
		{"unknown domain 404", "/api/whatever", "", 404, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, c.path, nil)
			if c.session {
				req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
			}
			rec := httptest.NewRecorder()
			gw.serve(rec, req)

			if rec.Code != c.status {
				t.Fatalf("status: got %d want %d (body %s)", rec.Code, c.status, rec.Body.String())
			}
			if c.status == http.StatusOK {
				if got := rec.Body.String(); got != c.want {
					t.Fatalf("body: got %q want %q", got, c.want)
				}
			}
		})
	}
}

// newTaggedGateway builds a gateway whose admin/resources/orgs backends echo
// their role + path, so ownership of a route can be asserted exactly. The
// orgs/task fakes additionally answer the internal membership/ownership
// endpoints that the Gateway's scoping checks depend on.
func newTaggedGateway(t *testing.T) *gateway {
	t.Helper()
	tag := func(role string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, role+":"+r.URL.Path)
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	taggedOrgs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/users/u1/workspaces" {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `[{"id":"w1","name":"A","role":"owner"},{"id":"w2","name":"B","role":"member"}]`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "orgs:"+r.URL.Path)
	}))
	t.Cleanup(taggedOrgs.Close)
	taggedTask := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/internal/tasks/") {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"workspace_id":"w1"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "task:"+r.URL.Path)
	}))
	t.Cleanup(taggedTask.Close)
	auth := startAuthBackend(t)
	t.Setenv("UPSTREAM_PROJECT", tag("project").URL)
	t.Setenv("UPSTREAM_TASK", taggedTask.URL)
	t.Setenv("UPSTREAM_AGENT", tag("agent").URL)
	t.Setenv("UPSTREAM_CATALOG", tag("catalog").URL)
	t.Setenv("UPSTREAM_SETTINGS", tag("settings").URL)
	t.Setenv("UPSTREAM_RUNNER", tag("runner").URL)
	t.Setenv("UPSTREAM_AUTH", auth.URL)
	t.Setenv("UPSTREAM_ORGS", taggedOrgs.URL)
	t.Setenv("UPSTREAM_RESOURCES", tag("resources").URL)
	t.Setenv("UPSTREAM_ADMIN", tag("admin").URL)
	gw, err := newGateway(nil)
	if err != nil {
		t.Fatalf("newGateway: %v", err)
	}
	return gw
}

func TestOwnerSplit(t *testing.T) {
	gw := newTaggedGateway(t)

	cases := []struct {
		name   string
		method string
		path   string
		want   string
		status int
	}{
		{"workspace audit to admin", http.MethodGet, "/api/workspaces/w1/audit", "admin:/workspaces/w1/audit", 200},
		{"audit export to admin", http.MethodGet, "/api/workspaces/w1/audit/export", "admin:/workspaces/w1/audit/export", 200},
		{"workspace rules to resources", http.MethodGet, "/api/workspaces/w1/rules", "resources:/workspaces/w1/rules", 200},
		{"workspace mcp to resources", http.MethodGet, "/api/workspaces/w1/mcp", "resources:/workspaces/w1/mcp", 200},
		{"sysadmin orgs to orgs", http.MethodGet, "/api/sysadmin/orgs", "orgs:/sysadmin/orgs", 200},
		{"sysadmin requests to orgs", http.MethodGet, "/api/sysadmin/requests", "orgs:/sysadmin/requests", 200},
		{"sysadmin flags to admin", http.MethodGet, "/api/sysadmin/flags", "admin:/sysadmin/flags", 200},
		{"sysadmin audit to admin", http.MethodGet, "/api/sysadmin/audit", "admin:/sysadmin/audit", 200},
		{"sysadmin maintenance to admin", http.MethodGet, "/api/sysadmin/maintenance", "admin:/sysadmin/maintenance", 200},
		{"workspace members to orgs", http.MethodGet, "/api/workspaces/w1/members", "orgs:/workspaces/w1/members", 200},
		{"workspace requests to orgs", http.MethodGet, "/api/workspaces/w1/requests", "orgs:/workspaces/w1/requests", 200},
		{"workspace get to orgs", http.MethodGet, "/api/workspaces/w1", "orgs:/workspaces/w1", 200},
		{"workspace create to orgs", http.MethodPost, "/api/workspaces", "orgs:/workspaces", 200},
		// protected routes reject sessions that fail to resolve
		{"workspace rules without session 401", http.MethodGet, "/api/workspaces/w1/rules", "", 401},
		{"workspace audit without session 401", http.MethodGet, "/api/workspaces/w1/audit", "", 401},
		{"sysadmin flags without session 401", http.MethodGet, "/api/sysadmin/flags", "", 401},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			method := c.method
			if method == "" {
				method = http.MethodGet
			}
			req := httptest.NewRequest(method, c.path, nil)
			if !strings.Contains(c.name, "without session") {
				req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
			}
			rec := httptest.NewRecorder()
			gw.serve(rec, req)
			if rec.Code != c.status {
				t.Fatalf("status: got %d want %d (body %s)", rec.Code, c.status, rec.Body.String())
			}
			if c.status == http.StatusOK && rec.Body.String() != c.want {
				t.Fatalf("owner: got %q want %q", rec.Body.String(), c.want)
			}
		})
	}
}

func TestSessionComposition(t *testing.T) {
	gw, _ := newTestGateway(t)

	// GET /api/auth/me with a session cookie → full Session (user + workspaces).
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
	rec := httptest.NewRecorder()
	gw.serve(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (%s)", rec.Code, rec.Body.String())
	}
	var sess contracts.Session
	if err := json.Unmarshal(rec.Body.Bytes(), &sess); err != nil {
		t.Fatalf("unmarshal session: %v", err)
	}
	if sess.User.ID != "u1" {
		t.Fatalf("user: got %q want u1", sess.User.ID)
	}
	if len(sess.Workspaces) != 2 || sess.Workspaces[0].ID != "w1" || sess.ActiveWorkspaceID != "w1" {
		t.Fatalf("workspaces: got %+v", sess.Workspaces)
	}
}

func TestWorkspaceListEnrichment(t *testing.T) {
	gw, _ := newTestGateway(t)

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
	rec := httptest.NewRecorder()
	gw.serve(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (%s)", rec.Code, rec.Body.String())
	}
	var wss []contracts.Workspace
	if err := json.Unmarshal(rec.Body.Bytes(), &wss); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// The agent/task count upstreams are echo backends, so counts stay nil;
	// the orgs list itself must still be intact.
	if len(wss) != 1 || wss[0].ID != "w1" || wss[0].Name != "A" {
		t.Fatalf("workspaces: got %+v", wss)
	}
}

func TestStreamRequiresSession(t *testing.T) {
	gw, _ := newTestGateway(t)

	// No cookie → 401.
	req := httptest.NewRequest(http.MethodGet, "/api/tasks/1/stream", nil)
	rec := httptest.NewRecorder()
	gw.serve(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-session stream: got %d want 401", rec.Code)
	}

	// With session but no Kafka → 503 (documented "unavailable" path).
	req = httptest.NewRequest(http.MethodGet, "/api/tasks/1/stream", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
	rec = httptest.NewRecorder()
	gw.serve(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no-kafka stream: got %d want 503", rec.Code)
	}
}

func TestKpisComposition(t *testing.T) {
	gw, _ := newTestGateway(t)

	req := httptest.NewRequest(http.MethodGet, "/api/sysadmin/kpis", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
	rec := httptest.NewRecorder()
	gw.serve(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (%s)", rec.Code, rec.Body.String())
	}
	var k contracts.SystemKpis
	if err := json.Unmarshal(rec.Body.Bytes(), &k); err != nil {
		t.Fatalf("unmarshal kpis: %v", err)
	}
	if k.Organizations != 0 || k.Workspaces != 0 || k.ActiveUsers24h != 0 {
		// Echo backends don't answer the internal stat endpoints, so zeros are
		// expected; the shape must still parse.
		t.Logf("kpis: %+v", k)
	}
}
