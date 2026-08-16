// Package http tests the Gateway HTTP adapter end-to-end against fake upstream
// backends: routing and remaps, session/identity header injection, the composed
// endpoints (session, workspace list, KPIs, health), the SSE session gate, and
// the downstream-enforced superadmin 403 contract.
package http

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"sync"
	"testing"

	"github.com/aaks/server/internal/contracts/admin"
	"github.com/aaks/server/internal/contracts/workspaces"
	"github.com/aaks/server/internal/platform/tenancy"
	"github.com/aaks/server/services/gateway/internal/application"
	"github.com/aaks/server/services/gateway/internal/infrastructure/acl"
	"github.com/aaks/server/services/gateway/internal/infrastructure/proxy"
)

// ── fake upstream backends ──────────────────────────────────────────────────

func testLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func writeJSON(t *testing.T, w http.ResponseWriter, code int, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode: %v", err)
	}
}

// startBackend echoes the request path (routing/ownership assertions).
func startBackend(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// tag returns a backend echoing "name:" + path so upstream ownership is
// asserted by body prefix.
func tag(t *testing.T, name string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, name+":"+r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// taskWorkspaceBackend resolves /internal/tasks/{id}/workspace to ws and
// echoes every other path.
func taskWorkspaceBackend(t *testing.T, ws string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/internal/tasks/") {
			writeJSON(t, w, http.StatusOK, map[string]string{"workspace_id": ws})
			return
		}
		_, _ = io.WriteString(w, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// missingTaskBackend fails every task-ownership lookup (404).
func missingTaskBackend(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/internal/tasks/") {
			writeJSON(t, w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		_, _ = io.WriteString(w, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func memberships() []map[string]string {
	return []map[string]string{
		{"id": "w1", "name": "A", "role": "owner"},
		{"id": "w2", "name": "B", "role": "member"},
	}
}

// startAuthBackend resolves sessions (/internal/identity: "tok" → u1
// non-superadmin, "sadm" → u9 superadmin, anything else → 401) and serves the
// public session surface (/auth/login POST → 201 + Set-Cookie, /auth/me GET →
// user JSON).
func startAuthBackend(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/identity":
			switch sessionValue(r) {
			case "tok":
				writeJSON(t, w, http.StatusOK, map[string]string{"user_id": "u1", "name": "Ada", "email": "ada@aaks.dev"})
			case "sadm":
				writeJSON(t, w, http.StatusOK, map[string]any{"user_id": "u9", "name": "Root", "email": "root@aaks.dev", "is_superadmin": true})
			default:
				writeJSON(t, w, http.StatusUnauthorized, map[string]string{"error": "invalid session"})
			}
		case "/auth/login":
			if r.Method == http.MethodPost {
				w.Header().Set("Set-Cookie", SessionCookie+"=tok; Path=/")
				writeJSON(t, w, http.StatusCreated, map[string]string{"id": "u1", "name": "Ada", "email": "ada@aaks.dev"})
				return
			}
			writeJSON(t, w, http.StatusOK, map[string]string{"id": "u1", "name": "Ada", "email": "ada@aaks.dev"})
		case "/auth/me":
			writeJSON(t, w, http.StatusOK, map[string]string{"id": "u1", "name": "Ada", "email": "ada@aaks.dev"})
		default:
			_, _ = io.WriteString(w, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// orgsHandler serves memberships (/internal/users/{id}/workspaces) and the
// public workspace list (/workspaces); other paths fall through to echo.
func orgsHandler(echo string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/users/u1/workspaces", "/internal/users/u9/workspaces":
			wss := memberships()
			if r.URL.Path == "/internal/users/u9/workspaces" {
				wss = wss[:1]
			}
			_ = json.NewEncoder(w).Encode(wss)
		case "/workspaces":
			_ = json.NewEncoder(w).Encode(memberships()[:1])
		default:
			_, _ = io.WriteString(w, echo+r.URL.Path)
		}
	})
}

// startOrgsBackend is the standard orgs fixture: u1 → w1+w2, u9 → w1.
func startOrgsBackend(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(orgsHandler(""))
	t.Cleanup(srv.Close)
	return srv
}

// taggedOrgs echoes non-special paths with an "orgs:" prefix for ownership
// assertions.
func taggedOrgs(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(orgsHandler("orgs:"))
	t.Cleanup(srv.Close)
	return srv
}

// headerBackend records the identity/scoping headers it received; the returned
// snapshot returns a copy of them.
func headerBackend(t *testing.T) (*httptest.Server, func() map[string]string) {
	t.Helper()
	var mu sync.Mutex
	got := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		for _, h := range tenancy.IdentityHeaders {
			if v := r.Header.Get(h); v != "" {
				got[h] = v
			}
		}
		mu.Unlock()
		_, _ = io.WriteString(w, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	snapshot := func() map[string]string {
		mu.Lock()
		defer mu.Unlock()
		out := make(map[string]string, len(got))
		for k, v := range got {
			out[k] = v
		}
		return out
	}
	return srv, snapshot
}

// superadminGate serves like startOrgsBackend but rejects /sysadmin/* requests
// without the injected X-User-Superadmin header — mirroring how the real Orgs
// and Admin services enforce the boundary downstream of the gateway.
func superadminGate(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/users/u1/workspaces", "/internal/users/u9/workspaces":
			wss := memberships()
			if r.URL.Path == "/internal/users/u9/workspaces" {
				wss = wss[:1]
			}
			writeJSON(t, w, http.StatusOK, wss)
		case "/workspaces":
			writeJSON(t, w, http.StatusOK, memberships()[:1])
		default:
			if strings.HasPrefix(r.URL.Path, "/sysadmin/") && !tenancy.UserSuperadmin(r) {
				writeJSON(t, w, http.StatusForbidden, map[string]string{"error": "superadmin required"})
				return
			}
			_, _ = io.WriteString(w, "gate:"+r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ── server wiring ───────────────────────────────────────────────────────────

// buildServer wires the gateway composition root around the given backends.
// brokers is "" so the SSE path is disabled (503), exactly as in CI.
func buildServer(t *testing.T, project, task, agent, catalog, settings, runner, auth, orgs, resources, adminSrv *httptest.Server) *Server {
	t.Helper()
	log := testLog()
	mkProxy := func(u application.Upstream, srv *httptest.Server) *httputil.ReverseProxy {
		rp, err := proxy.New(srv.URL)
		if err != nil {
			t.Fatalf("proxy %s: %v", u, err)
		}
		return rp
	}
	proxies := map[application.Upstream]*httputil.ReverseProxy{
		application.UpstreamProject:   mkProxy(application.UpstreamProject, project),
		application.UpstreamTask:      mkProxy(application.UpstreamTask, task),
		application.UpstreamAgent:     mkProxy(application.UpstreamAgent, agent),
		application.UpstreamCatalog:   mkProxy(application.UpstreamCatalog, catalog),
		application.UpstreamSettings:  mkProxy(application.UpstreamSettings, settings),
		application.UpstreamRunner:    mkProxy(application.UpstreamRunner, runner),
		application.UpstreamAuth:      mkProxy(application.UpstreamAuth, auth),
		application.UpstreamOrgs:      mkProxy(application.UpstreamOrgs, orgs),
		application.UpstreamResources: mkProxy(application.UpstreamResources, resources),
		application.UpstreamAdmin:     mkProxy(application.UpstreamAdmin, adminSrv),
	}
	bases := make(map[application.Upstream]string, len(proxies))
	for u, rp := range proxies {
		bases[u] = proxy.BaseURL(rp)
	}
	app := application.New(
		application.NewACL(
			acl.NewAuthClient(auth.URL, SessionCookie, log),
			acl.NewOrgsClient(orgs.URL, log),
			acl.NewTaskClient(task.URL, log),
			log,
		),
		application.NewStream(acl.NewStepsClient(runner.URL, log), nil, log),
		application.NewRouteTable(),
		acl.NewStatsClient(agent.URL, task.URL, orgs.URL, auth.URL, log),
	)
	return New(app, proxies, bases, "", log)
}

// newTestServer builds the standard fixture: task ownership resolves to w1,
// session "tok" → u1 (member of w1+w2), "sadm" → u9 (superadmin, w1).
func newTestServer(t *testing.T) *Server {
	t.Helper()
	return buildServer(t,
		startBackend(t), taskWorkspaceBackend(t, "w1"), startBackend(t), startBackend(t),
		startBackend(t), startBackend(t), startAuthBackend(t), startOrgsBackend(t),
		startBackend(t), startBackend(t))
}

// newTaggedServer tags every upstream so ownership is asserted by body prefix.
func newTaggedServer(t *testing.T) *Server {
	t.Helper()
	return buildServer(t,
		tag(t, "project"), taskWorkspaceBackend(t, "w1"), tag(t, "agent"), tag(t, "catalog"),
		tag(t, "settings"), tag(t, "runner"), startAuthBackend(t), taggedOrgs(t),
		tag(t, "resources"), tag(t, "admin"))
}

func request(t *testing.T, s *Server, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: SessionCookie, Value: token})
	}
	rec := httptest.NewRecorder()
	s.serve(rec, req)
	return rec
}

func get(t *testing.T, s *Server, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	return request(t, s, http.MethodGet, path, token)
}

// ── tests ───────────────────────────────────────────────────────────────────

func TestGatewayRouting(t *testing.T) {
	s := newTestServer(t)
	cases := []struct {
		name, path, token, wantPath string
		wantCode                    int
	}{
		{"projects to project", "/api/projects", "tok", "/projects", 200},
		{"project by id", "/api/projects/abc", "tok", "/projects/abc", 200},
		{"tasks to task", "/api/tasks", "tok", "/tasks", 200},
		{"task by id", "/api/tasks/1", "tok", "/tasks/1", 200},
		{"agents to agent", "/api/agents/1", "tok", "/agents/1", 200},
		{"skills to catalog", "/api/skills", "tok", "/skills", 200},
		{"mcp-servers to catalog", "/api/mcp-servers", "tok", "/mcp-servers", 200},
		{"provider-keys to settings", "/api/provider-keys", "tok", "/provider-keys", 200},
		{"runs to runner", "/api/runs/9/steps", "tok", "/runs/9/steps", 200},
		{"orgs to orgs", "/api/orgs", "tok", "/orgs", 200},
		{"workspace members to orgs", "/api/workspaces/w1/members", "tok", "/workspaces/w1/members", 200},
		{"workspace skills to catalog", "/api/workspaces/w1/skills", "tok", "/workspaces/w1/skills", 200},
		{"workspace rules to resources", "/api/workspaces/w1/rules", "tok", "/workspaces/w1/rules", 200},
		{"workspace knowledge to resources", "/api/workspaces/w1/knowledge", "tok", "/workspaces/w1/knowledge", 200},
		{"workspace mcp to resources", "/api/workspaces/w1/mcp", "tok", "/workspaces/w1/mcp", 200},
		{"workspace audit to admin", "/api/workspaces/w1/audit", "tok", "/workspaces/w1/audit", 200},
		{"workspace remap outside union 403", "/api/workspaces/w9/rules", "tok", "", 403},
		{"tasks without session 401", "/api/tasks", "", "", 401},
		{"agents without session 401", "/api/agents", "", "", 401},
		{"projects without session 401", "/api/projects", "", "", 401},
		{"provider-keys without session 401", "/api/provider-keys", "", "", 401},
		{"tasks with invalid session 401", "/api/tasks", "bogus", "", 401},
		{"signup without session", "/api/auth/signup", "", "/auth/signup", 200},
		{"signup-status without session", "/api/auth/signup-status", "", "/auth/signup-status", 200},
		{"logout without session", "/api/auth/logout", "", "/auth/logout", 200},
		{"auth login without session", "/api/auth/login", "", "", 200},
		{"unknown domain 404", "/api/whatever", "", "", 404},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := get(t, s, tc.path, tc.token)
			if rec.Code != tc.wantCode {
				t.Fatalf("code = %d, want %d (body %s)", rec.Code, tc.wantCode, rec.Body.String())
			}
			if tc.wantPath != "" && rec.Body.String() != tc.wantPath {
				t.Fatalf("body = %q, want %q", rec.Body.String(), tc.wantPath)
			}
		})
	}
}

func TestOwnerSplit(t *testing.T) {
	s := newTaggedServer(t)
	cases := []struct {
		name, path, token, wantPrefix string
		wantCode                      int
	}{
		{"sysadmin orgs to orgs", "/api/sysadmin/orgs", "tok", "orgs:", 200},
		{"sysadmin requests to orgs", "/api/sysadmin/requests", "tok", "orgs:", 200},
		{"sysadmin flags to admin", "/api/sysadmin/flags", "tok", "admin:", 200},
		{"sysadmin audit to admin", "/api/sysadmin/audit", "tok", "admin:", 200},
		{"sysadmin maintenance to admin", "/api/sysadmin/maintenance", "tok", "admin:", 200},
		{"sysadmin kpis composed", "/api/sysadmin/kpis", "tok", "", 200},
		{"sysadmin health composed", "/api/sysadmin/health", "tok", "", 200},
		{"workspace rules to resources", "/api/workspaces/w1/rules", "tok", "resources:", 200},
		{"workspace knowledge to resources", "/api/workspaces/w1/knowledge", "tok", "resources:", 200},
		{"workspace plugins to resources", "/api/workspaces/w1/plugins", "tok", "resources:", 200},
		{"workspace mcp to resources", "/api/workspaces/w1/mcp", "tok", "resources:", 200},
		{"workspace audit to admin", "/api/workspaces/w1/audit", "tok", "admin:", 200},
		{"workspace skills to catalog", "/api/workspaces/w1/skills", "tok", "catalog:", 200},
		{"task runs to runner", "/api/tasks/1/runs", "tok", "runner:", 200},
		{"task artifacts to runner", "/api/tasks/1/artifacts", "tok", "runner:", 200},
		{"projects to project", "/api/projects", "tok", "project:", 200},
		{"agents to agent", "/api/agents", "tok", "agent:", 200},
		{"skills to catalog", "/api/skills", "tok", "catalog:", 200},
		{"provider-keys to settings", "/api/provider-keys", "tok", "settings:", 200},
		{"sysadmin without session 401", "/api/sysadmin/orgs", "", "", 401},
		{"sysadmin with invalid session 401", "/api/sysadmin/orgs", "bogus", "", 401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := get(t, s, tc.path, tc.token)
			if rec.Code != tc.wantCode {
				t.Fatalf("code = %d, want %d (body %s)", rec.Code, tc.wantCode, rec.Body.String())
			}
			if tc.wantPrefix != "" && !strings.HasPrefix(rec.Body.String(), tc.wantPrefix) {
				t.Fatalf("body = %q, want prefix %q", rec.Body.String(), tc.wantPrefix)
			}
		})
	}
}

func TestTaskOwnershipChecks(t *testing.T) {
	t.Run("task outside union 403", func(t *testing.T) {
		s := buildServer(t,
			startBackend(t), taskWorkspaceBackend(t, "w9"), startBackend(t), startBackend(t),
			startBackend(t), startBackend(t), startAuthBackend(t), startOrgsBackend(t),
			startBackend(t), startBackend(t))
		rec := get(t, s, "/api/tasks/1/runs", "tok")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("code = %d, want 403 (body %s)", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "task is not in an accessible workspace") {
			t.Fatalf("body = %q, want task-ownership 403 message", rec.Body.String())
		}
	})

	t.Run("unknown task 404", func(t *testing.T) {
		s := buildServer(t,
			startBackend(t), missingTaskBackend(t), startBackend(t), startBackend(t),
			startBackend(t), startBackend(t), startAuthBackend(t), startOrgsBackend(t),
			startBackend(t), startBackend(t))
		rec := get(t, s, "/api/tasks/1/runs", "tok")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("code = %d, want 404 (body %s)", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "task not found") {
			t.Fatalf("body = %q, want task-not-found 404 message", rec.Body.String())
		}
	})
}

func TestSessionComposition(t *testing.T) {
	s := newTestServer(t)

	t.Run("logged in", func(t *testing.T) {
		rec := get(t, s, "/api/auth/me", "tok")
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want 200 (body %s)", rec.Code, rec.Body.String())
		}
		var sess workspaces.Session
		if err := json.Unmarshal(rec.Body.Bytes(), &sess); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if sess.User.ID != "u1" {
			t.Fatalf("user.id = %q, want u1", sess.User.ID)
		}
		if len(sess.Workspaces) != 2 || sess.Workspaces[0].ID != "w1" || sess.Workspaces[1].ID != "w2" {
			t.Fatalf("workspaces = %+v, want [w1 w2]", sess.Workspaces)
		}
		if sess.ActiveWorkspaceID != "w1" {
			t.Fatalf("active_workspace_id = %q, want w1", sess.ActiveWorkspaceID)
		}
		if sess.User.IsSuperadmin != nil && *sess.User.IsSuperadmin {
			t.Fatalf("u1 must not be superadmin")
		}
	})

	t.Run("login sets cookie and returns 201", func(t *testing.T) {
		rec := request(t, s, http.MethodPost, "/api/auth/login", "tok")
		if rec.Code != http.StatusCreated {
			t.Fatalf("code = %d, want 201 (body %s)", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Set-Cookie"); got != SessionCookie+"=tok; Path=/" {
			t.Fatalf("Set-Cookie = %q, want %q", got, SessionCookie+"=tok; Path=/")
		}
		var sess workspaces.Session
		if err := json.Unmarshal(rec.Body.Bytes(), &sess); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(sess.Workspaces) != 2 {
			t.Fatalf("workspaces = %+v, want 2 memberships", sess.Workspaces)
		}
	})

	t.Run("without session", func(t *testing.T) {
		rec := get(t, s, "/api/auth/me", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want 200 (body %s)", rec.Code, rec.Body.String())
		}
		var sess workspaces.Session
		if err := json.Unmarshal(rec.Body.Bytes(), &sess); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(sess.Workspaces) != 0 {
			t.Fatalf("workspaces = %+v, want empty", sess.Workspaces)
		}
		if sess.ActiveWorkspaceID != "" {
			t.Fatalf("active_workspace_id = %q, want empty", sess.ActiveWorkspaceID)
		}
	})

	t.Run("superadmin flagged", func(t *testing.T) {
		rec := get(t, s, "/api/auth/me", "sadm")
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want 200 (body %s)", rec.Code, rec.Body.String())
		}
		var sess workspaces.Session
		if err := json.Unmarshal(rec.Body.Bytes(), &sess); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if sess.User.IsSuperadmin == nil || !*sess.User.IsSuperadmin {
			t.Fatalf("is_superadmin = %v, want true", sess.User.IsSuperadmin)
		}
		if len(sess.Workspaces) != 1 || sess.Workspaces[0].ID != "w1" {
			t.Fatalf("workspaces = %+v, want [w1]", sess.Workspaces)
		}
	})
}

func TestWorkspaceListEnrichment(t *testing.T) {
	s := newTestServer(t)
	rec := get(t, s, "/api/workspaces", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var wss []workspaces.Workspace
	if err := json.Unmarshal(rec.Body.Bytes(), &wss); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(wss) != 1 || wss[0].ID != "w1" {
		t.Fatalf("workspaces = %+v, want [w1]", wss)
	}
	// The plain echo backends answer the stats endpoints with non-JSON paths,
	// so both derived counts stay nil (decode failure is swallowed).
	if wss[0].AgentCount != nil {
		t.Fatalf("agent_count = %v, want nil", *wss[0].AgentCount)
	}
	if wss[0].OpenTaskCount != nil {
		t.Fatalf("open_task_count = %v, want nil", *wss[0].OpenTaskCount)
	}
}

func TestStreamRequiresSession(t *testing.T) {
	s := newTestServer(t)

	rec := get(t, s, "/api/tasks/1/stream", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401 (body %s)", rec.Code, rec.Body.String())
	}

	rec = get(t, s, "/api/tasks/1/stream", "tok")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503 (body %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "kafka is not configured") {
		t.Fatalf("body = %q, want kafka 503 message", rec.Body.String())
	}
}

func TestKpisComposition(t *testing.T) {
	s := newTestServer(t)
	rec := get(t, s, "/api/sysadmin/kpis", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var kpis admin.SystemKpis
	if err := json.Unmarshal(rec.Body.Bytes(), &kpis); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// The stats backends answer with non-JSON paths, so every KPI decodes to
	// zero instead of failing the fan-out.
	if kpis.Organizations != 0 || kpis.Workspaces != 0 || kpis.OpenSeats != 0 || kpis.ActiveUsers24h != 0 {
		t.Fatalf("kpis = %+v, want all-zero", kpis)
	}
}

func TestHealthProbes(t *testing.T) {
	s := newTestServer(t)
	rec := get(t, s, "/api/sysadmin/health", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var h admin.SystemHealth
	if err := json.Unmarshal(rec.Body.Bytes(), &h); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(h.Services) != 11 {
		t.Fatalf("services = %d, want 11", len(h.Services))
	}
	for _, svc := range h.Services {
		if svc.Status != "ok" || svc.Pct != 100 {
			t.Fatalf("service %s = %s/%d, want ok/100", svc.Name, svc.Status, svc.Pct)
		}
	}
}

func TestInjectedIdentityReachesUpstream(t *testing.T) {
	project, snapshot := headerBackend(t)
	s := buildServer(t,
		project, taskWorkspaceBackend(t, "w1"), startBackend(t), startBackend(t),
		startBackend(t), startBackend(t), startAuthBackend(t), startOrgsBackend(t),
		startBackend(t), startBackend(t))

	t.Run("multi-workspace session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
		req.AddCookie(&http.Cookie{Name: SessionCookie, Value: "tok"})
		req.Header.Set(tenancy.HeaderUserID, "attacker")
		req.Header.Set(tenancy.HeaderUserSuperadmin, "true")
		rec := httptest.NewRecorder()
		s.serve(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want 200 (body %s)", rec.Code, rec.Body.String())
		}
		hdrs := snapshot()
		if hdrs[tenancy.HeaderUserID] != "u1" {
			t.Fatalf("X-User-ID = %q, want u1 (forged value must be replaced)", hdrs[tenancy.HeaderUserID])
		}
		if hdrs[tenancy.HeaderUserSuperadmin] != "" {
			t.Fatalf("X-User-Superadmin = %q, want unset for u1", hdrs[tenancy.HeaderUserSuperadmin])
		}
		if hdrs[tenancy.HeaderWorkspaceIDs] != "w1,w2" {
			t.Fatalf("X-Workspace-IDs = %q, want w1,w2", hdrs[tenancy.HeaderWorkspaceIDs])
		}
		if hdrs[tenancy.HeaderWorkspaceID] != "" {
			t.Fatalf("X-Workspace-ID = %q, want unset for a multi-workspace session", hdrs[tenancy.HeaderWorkspaceID])
		}
		if hdrs[tenancy.HeaderUserRole] != "owner" {
			t.Fatalf("X-User-Role = %q, want owner", hdrs[tenancy.HeaderUserRole])
		}
	})

	t.Run("single-workspace superadmin session", func(t *testing.T) {
		rec := get(t, s, "/api/projects", "sadm")
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want 200 (body %s)", rec.Code, rec.Body.String())
		}
		hdrs := snapshot()
		if hdrs[tenancy.HeaderUserID] != "u9" {
			t.Fatalf("X-User-ID = %q, want u9", hdrs[tenancy.HeaderUserID])
		}
		if hdrs[tenancy.HeaderUserSuperadmin] != "true" {
			t.Fatalf("X-User-Superadmin = %q, want true", hdrs[tenancy.HeaderUserSuperadmin])
		}
		if hdrs[tenancy.HeaderWorkspaceID] != "w1" {
			t.Fatalf("X-Workspace-ID = %q, want w1", hdrs[tenancy.HeaderWorkspaceID])
		}
		if hdrs[tenancy.HeaderUserRole] != "owner" {
			t.Fatalf("X-User-Role = %q, want owner", hdrs[tenancy.HeaderUserRole])
		}
	})
}

func TestStripInboundIdentity(t *testing.T) {
	project, snapshot := headerBackend(t)
	s := buildServer(t,
		project, taskWorkspaceBackend(t, "w1"), startBackend(t), startBackend(t),
		startBackend(t), startBackend(t), startAuthBackend(t), startOrgsBackend(t),
		startBackend(t), startBackend(t))

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	for _, h := range tenancy.IdentityHeaders {
		req.Header.Set(h, "forged")
	}
	rec := httptest.NewRecorder()
	s.serve(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401 (body %s)", rec.Code, rec.Body.String())
	}
	if got := snapshot(); len(got) != 0 {
		t.Fatalf("forged identity headers reached upstream: %v", got)
	}
}

func TestSysadminSuperadminGate(t *testing.T) {
	orgs := superadminGate(t)
	adminSrv := superadminGate(t)
	s := buildServer(t,
		startBackend(t), taskWorkspaceBackend(t, "w1"), startBackend(t), startBackend(t),
		startBackend(t), startBackend(t), startAuthBackend(t), orgs,
		startBackend(t), adminSrv)

	t.Run("non-superadmin rejected downstream 403", func(t *testing.T) {
		rec := get(t, s, "/api/sysadmin/orgs", "tok")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("code = %d, want 403 (body %s)", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "superadmin required") {
			t.Fatalf("body = %q, want downstream 403 message", rec.Body.String())
		}
	})

	t.Run("superadmin passes through", func(t *testing.T) {
		rec := get(t, s, "/api/sysadmin/orgs", "sadm")
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want 200 (body %s)", rec.Code, rec.Body.String())
		}
		if rec.Body.String() != "gate:/sysadmin/orgs" {
			t.Fatalf("body = %q, want gate:/sysadmin/orgs", rec.Body.String())
		}
	})

	t.Run("admin flags gate", func(t *testing.T) {
		rec := get(t, s, "/api/sysadmin/flags", "tok")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("code = %d, want 403 (body %s)", rec.Code, rec.Body.String())
		}
		rec = get(t, s, "/api/sysadmin/flags", "sadm")
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want 200 (body %s)", rec.Code, rec.Body.String())
		}
		if rec.Body.String() != "gate:/sysadmin/flags" {
			t.Fatalf("body = %q, want gate:/sysadmin/flags", rec.Body.String())
		}
	})
}