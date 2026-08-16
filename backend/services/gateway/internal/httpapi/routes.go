// Package httpapi is the API Gateway / BFF. It is a path-aware reverse proxy:
// it inspects /api/<domain>/... requests, routes them to the owning service
// (stripping /api), and composes cross-service responses where the frontend
// contract needs fan-out:
//
//   - Session composition: /auth/login and /auth/me return the full Session
//     (user from Auth + workspace memberships from Orgs).
//   - Workspace scoping: the session cookie is resolved to an identity, the
//     workspace union is fetched from Orgs, and the X-User-ID /
//     X-Workspace-IDs / X-User-Superadmin headers are injected per the scoping
//     contract (internal/httputil/scope.go). Identity+workspaces are cached in
//     memory for 60s.
//   - Workspace list enrichment: agent_count (Agent svc) and open_task_count
//     (Task svc) are merged into GET /workspaces.
//   - Sysadmin composition: /sysadmin/kpis and /sysadmin/health fan out to the
//     owning services.
//   - Realtime: GET /tasks/:id/stream replays persisted steps (Runner) then
//     tails Kafka by task_id (SSE, phase 8).
//
// Identity resolution failures return 401 for tenant domains; unauthenticated
// public routes (login/signup) pass through untouched.
package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aaks/server/internal/contracts"
	apiutil "github.com/aaks/server/internal/platform/http"
	"github.com/aaks/server/internal/platform/tenancy"
	"github.com/aaks/server/internal/platform/kafka"
	"github.com/aaks/server/services/gateway/internal/proxy"
)

const sessionCookie = "aaks_session"

// Register wires the gateway. It reads UPSTREAM_* env vars (each an absolute
// URL like http://project:8081) and registers a catch-all /api/ handler.
func Register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
	gw, err := newGateway(log)
	if err != nil {
		return err
	}
	mux.HandleFunc("/api/", gw.serve)
	log.Info("gateway routes registered", "upstreams", gw.domains)
	return nil
}

type gateway struct {
	log  *slog.Logger
	auth *httputil.ReverseProxy
	// domains maps a routing domain to its reverse proxy.
	domains map[string]*httputil.ReverseProxy
	// tenantDomains require identity resolution (X-User-* headers).
	tenantDomains map[string]bool
	// orgs is the workspace/sysadmin upstream (used for composition calls).
	orgs *httputil.ReverseProxy

	// sessionCache caches (sessionToken -> identity) for 60s.
	sessionCache sync.Map
	// identity holds the resolved identity for the current request chain.
}

// identity is the Gateway's resolved session view.
type identity struct {
	UserID      string
	Name        string
	Email       string
	Superadmin  bool
	Workspaces  []contracts.Workspace
	resolvedAt  time.Time
}

func newGateway(log *slog.Logger) (*gateway, error) {
	mk := func(env string) (*httputil.ReverseProxy, error) {
		u := os.Getenv(env)
		if u == "" {
			return nil, errors.New(env + " is not set")
		}
		return proxy.New(u)
	}
	project, err := mk("UPSTREAM_PROJECT")
	if err != nil {
		return nil, err
	}
	task, err := mk("UPSTREAM_TASK")
	if err != nil {
		return nil, err
	}
	agent, err := mk("UPSTREAM_AGENT")
	if err != nil {
		return nil, err
	}
	catalog, err := mk("UPSTREAM_CATALOG")
	if err != nil {
		return nil, err
	}
	settings, err := mk("UPSTREAM_SETTINGS")
	if err != nil {
		return nil, err
	}
	runner, err := mk("UPSTREAM_RUNNER")
	if err != nil {
		return nil, err
	}
	auth, err := mk("UPSTREAM_AUTH")
	if err != nil {
		return nil, err
	}
	orgs, err := mk("UPSTREAM_ORGS")
	if err != nil {
		return nil, err
	}
	resources, err := mk("UPSTREAM_RESOURCES")
	if err != nil {
		return nil, err
	}
	admin, err := mk("UPSTREAM_ADMIN")
	if err != nil {
		return nil, err
	}
	return &gateway{
		log:   log,
		auth:  auth,
		orgs:  orgs,
		domains: map[string]*httputil.ReverseProxy{
			"projects":      project,
			"tasks":         task,
			"agents":        agent,
			"skills":        catalog,
			"mcp-servers":   catalog,
			"provider-keys": settings,
			"runs":          runner,
			"auth":          auth,
			"sysadmin":      orgs,
			"workspaces":    orgs,
			"orgs":          orgs,
			"resources":     resources,
			"admin":         admin,
		},
		tenantDomains: map[string]bool{
			"workspaces": true, "sysadmin": true, "tasks": true, "agents": true,
			"projects": true, "skills": true, "mcp-servers": true, "provider-keys": true,
			"runs": true, "auth": true, "orgs": true, "resources": true, "admin": true,
		},
	}, nil
}

// serve routes a single /api/... request.
func (g *gateway) serve(w http.ResponseWriter, r *http.Request) {
	// Identity/scoping headers must never survive a round trip from the
	// client: an attacker could spoof another user, workspace membership, or
	// superadmin status. Strip them at the gateway boundary BEFORE injection
	// below — only injectIdentity may set them afterwards (the proxy Director
	// runs after this handler and must not strip again).
	stripInboundIdentity(r)

	rest := strings.TrimPrefix(r.URL.Path, "/api/")
	rest = strings.TrimPrefix(rest, "/")
	segs := strings.Split(rest, "/")
	if len(segs) == 0 || segs[0] == "" {
		apiutil.Error(w, http.StatusNotFound, "not found")
		return
	}
	domain := segs[0]

	// Realtime: /tasks/{id}/stream — replay then tail (SSE).
	if domain == "tasks" && len(segs) >= 3 && segs[2] == "stream" {
		g.serveStream(w, r, segs[1])
		return
	}

	// Session composition for /auth/login (POST) + /auth/me (GET).
	if domain == "auth" && len(segs) >= 2 && (segs[1] == "login" || segs[1] == "me") {
		g.serveSession(w, r)
		return
	}

	// Workspace sub-routes owned by other services.
	if domain == "workspaces" && len(segs) >= 3 {
		switch segs[2] {
		case "skills":
			if !g.injectIdentity(w, r, true) {
				return
			}
			if !g.requireWorkspaceMember(w, r, segs[1]) {
				return
			}
			r.Header.Set(tenancy.HeaderWorkspaceID, segs[1])
			g.domains["skills"].ServeHTTP(w, r)
			return
		case "knowledge", "plugins", "rules", "mcp":
			if !g.injectIdentity(w, r, true) {
				return
			}
			if !g.requireWorkspaceMember(w, r, segs[1]) {
				return
			}
			r.Header.Set(tenancy.HeaderWorkspaceID, segs[1])
			g.domains["resources"].ServeHTTP(w, r)
			return
		case "audit":
			if !g.injectIdentity(w, r, true) {
				return
			}
			if !g.requireWorkspaceMember(w, r, segs[1]) {
				return
			}
			r.Header.Set(tenancy.HeaderWorkspaceID, segs[1])
			g.domains["admin"].ServeHTTP(w, r)
			return
		default:
			// Workspace sub-routes owned by orgs (members, invites, requests).
			if !g.injectIdentity(w, r, true) {
				return
			}
			if !g.requireWorkspaceMember(w, r, segs[1]) {
				return
			}
			r.Header.Set(tenancy.HeaderWorkspaceID, segs[1])
			g.domains["orgs"].ServeHTTP(w, r)
			return
		}
	}

	// Task sub-routes owned by the runner: /tasks/{id}/runs|artifacts.
	if domain == "tasks" && len(segs) >= 3 {
		switch segs[2] {
		case "runs", "artifacts":
			if !g.injectIdentity(w, r, true) {
				return
			}
			if !g.requireTaskWorkspace(w, r, segs[1]) {
				return
			}
			g.domains["runs"].ServeHTTP(w, r)
			return
		}
	}

	// Tenant domains: resolve the session and inject scoping headers.
	// Identity is REQUIRED except on the auth domain, whose routes are public
	// (signup, signup-status, login) or cookie-driven (logout) — requiring a
	// session here would make first-time signup impossible.
	if g.tenantDomains[domain] {
		if !g.injectIdentity(w, r, domain != "auth") {
			return
		}
	}

	// Workspace list enrichment (derived stats from Agent + Task).
	if domain == "workspaces" && len(segs) == 1 && r.Method == http.MethodGet {
		g.serveWorkspaces(w, r)
		return
	}

	// Sysadmin composition endpoints.
	if domain == "sysadmin" {
		switch {
		case len(segs) >= 2 && segs[1] == "kpis":
			g.serveKpis(w, r)
			return
		case len(segs) >= 2 && segs[1] == "health":
			g.serveHealth(w, r)
			return
		case (len(segs) >= 2 && segs[1] == "flags") || (len(segs) >= 2 && segs[1] == "audit") || (len(segs) >= 2 && segs[1] == "maintenance"):
			// Admin-owned sysadmin surface (flags/audit/maintenance).
			if !g.injectIdentity(w, r, true) {
				return
			}
			g.domains["admin"].ServeHTTP(w, r)
			return
		}
	}

	rp, ok := g.domains[domain]
	if !ok {
		apiutil.Error(w, http.StatusNotFound, "no route for domain: "+domain)
		return
	}
	rp.ServeHTTP(w, r)
}

// ── Identity resolution ─────────────────────────────────────────────────────

// injectIdentity resolves the session cookie (via Auth) and the workspace
// union (via Orgs), then sets the X-User-* / X-Workspace-* headers. Returns
// false after writing a 401 when the session is missing and required=true.
// Any inbound X-User-*/X-Workspace-* values are deleted first so a stale or
// forged value can never survive into the upstream request.
func (g *gateway) injectIdentity(w http.ResponseWriter, r *http.Request, required bool) bool {
	token := sessionValue(r)
	if token == "" {
		if required {
			apiutil.Error(w, http.StatusUnauthorized, "not authenticated")
			return false
		}
		return true
	}
	id, ok := g.resolveIdentity(r.Context(), token)
	if !ok {
		if required {
			apiutil.Error(w, http.StatusUnauthorized, "not authenticated")
			return false
		}
		return true
	}
	r.Header.Del(tenancy.HeaderUserID)
	r.Header.Del(tenancy.HeaderUserName)
	r.Header.Del(tenancy.HeaderUserEmail)
	r.Header.Del(tenancy.HeaderUserSuperadmin)
	r.Header.Del(tenancy.HeaderUserRole)
	r.Header.Del(tenancy.HeaderWorkspaceID)
	r.Header.Del(tenancy.HeaderWorkspaceIDs)
	r.Header.Set(tenancy.HeaderUserID, id.UserID)
	r.Header.Set(tenancy.HeaderUserName, id.Name)
	r.Header.Set(tenancy.HeaderUserEmail, id.Email)
	if id.Superadmin {
		r.Header.Set(tenancy.HeaderUserSuperadmin, "true")
	}
	ids := make([]string, 0, len(id.Workspaces))
	for _, ws := range id.Workspaces {
		ids = append(ids, string(ws.ID))
	}
	if len(ids) == 1 {
		r.Header.Set(tenancy.HeaderWorkspaceID, ids[0])
	}
	if len(ids) > 0 {
		r.Header.Set(tenancy.HeaderWorkspaceIDs, strings.Join(ids, ","))
	}
	// X-User-Role is the strongest role the session holds across its workspace
	// union (owner > admin > member). Trustworthy because it is derived from
	// the Orgs memberships the Gateway resolved — never from the client.
	if role := strongestRole(id.Workspaces); role != "" {
		r.Header.Set(tenancy.HeaderUserRole, string(role))
	}
	return true
}

// strongestRole returns the highest-privilege role in the workspace union.
func strongestRole(workspaces []contracts.Workspace) contracts.Role {
	role := contracts.Role("")
	for _, ws := range workspaces {
		switch ws.Role {
		case contracts.RoleOwner:
			return contracts.RoleOwner
		case contracts.RoleAdmin:
			if role != contracts.RoleAdmin {
				role = contracts.RoleAdmin
			}
		case contracts.RoleMember:
			if role == "" {
				role = contracts.RoleMember
			}
		}
	}
	return role
}

// stripInboundIdentity removes any identity/scoping header the client supplied
// so no forged value can reach the upstream services. Only injectIdentity (via
// resolveIdentity against Auth + Orgs) may populate them.
func stripInboundIdentity(r *http.Request) {
	for _, h := range tenancy.IdentityHeaders {
		r.Header.Del(h)
	}
}

// resolveIdentity returns the cached identity or fetches it (Auth + Orgs).
func (g *gateway) resolveIdentity(ctx context.Context, token string) (identity, bool) {
	if v, ok := g.sessionCache.Load(token); ok {
		id := v.(identity)
		if time.Since(id.resolvedAt) < 60*time.Second {
			return id, id.UserID != ""
		}
		// Stale entry: evict it now so the cache cannot grow without bound
		// (each distinct token is only ever held for one TTL window).
		g.sessionCache.Delete(token)
	}
	id := identity{resolvedAt: time.Now()}
	// 1. Auth: session cookie → user identity.
	base := proxy.BaseURL(g.auth)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/internal/identity", nil)
	if err != nil {
		return id, false
	}
	req.Header.Set("Cookie", sessionCookie+"="+token)
	var u struct {
		UserID      string `json:"user_id"`
		Name        string `json:"name"`
		Email       string `json:"email"`
		IsSuperadmin bool   `json:"is_superadmin"`
	}
	if !g.doJSON(req, &u) || u.UserID == "" {
		g.sessionCache.Store(token, identity{resolvedAt: time.Now()})
		return identity{}, false
	}
	id.UserID, id.Name, id.Email, id.Superadmin = u.UserID, u.Name, u.Email, u.IsSuperadmin
	// 2. Orgs: user → workspace union.
	var wss []contracts.Workspace
	if g.internalJSON(g.orgs, "/internal/users/"+u.UserID+"/workspaces", &wss) {
		id.Workspaces = wss
	}
	g.sessionCache.Store(token, id)
	return id, true
}

// requireWorkspaceMember returns false after writing a 403 when wid is not in
// the caller's resolved workspace union. The identity headers must already be
// injected (injectIdentity) so X-Workspace-IDs reflects the session.
func (g *gateway) requireWorkspaceMember(w http.ResponseWriter, r *http.Request, wid string) bool {
	for _, id := range tenancy.WorkspaceIDs(r) {
		if string(id) == wid {
			return true
		}
	}
	apiutil.Error(w, http.StatusForbidden, "not a member of workspace "+wid)
	return false
}

// requireTaskWorkspace returns false after writing a 403/404 when the task does
// not belong to a workspace in the caller's union. It resolves the task's
// owning workspace from the Task service (internal, unscoped).
func (g *gateway) requireTaskWorkspace(w http.ResponseWriter, r *http.Request, taskID string) bool {
	var res struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if !g.internalJSON(g.domains["tasks"], "/internal/tasks/"+taskID+"/workspace", &res) || res.WorkspaceID == "" {
		apiutil.Error(w, http.StatusNotFound, "task not found")
		return false
	}
	for _, id := range tenancy.WorkspaceIDs(r) {
		if string(id) == res.WorkspaceID {
			return true
		}
	}
	apiutil.Error(w, http.StatusForbidden, "task is not in an accessible workspace")
	return false
}

// doJSON is internalJSON with an already-built request (cookie case).
func (g *gateway) doJSON(req *http.Request, out any) bool {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		g.log.Warn("internal call failed", "url", req.URL.String(), "error", err)
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	return json.NewDecoder(resp.Body).Decode(out) == nil
}

// internalJSON performs an internal (unproxied) upstream call and decodes JSON.
func (g *gateway) internalJSON(rp *httputil.ReverseProxy, path string, out any) bool {
	base := proxy.BaseURL(rp)
	if base == "" {
		g.log.Warn("internal call skipped: unknown upstream", "path", path)
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		g.log.Warn("internal call failed", "url", req.URL.String(), "error", err)
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	return json.NewDecoder(resp.Body).Decode(out) == nil
}

// ── Composed endpoints ──────────────────────────────────────────────────────

// serveSession composes /auth/login + /auth/me into the full Session shape.
func (g *gateway) serveSession(w http.ResponseWriter, r *http.Request) {
	rec2 := &responseRecorder{}
	g.auth.ServeHTTP(rec2, r)
	// The recorder captures headers (session cookie on login) — put them back
	// on the wire or the client never receives the cookie.
	copyHeaders(w.Header(), rec2.Header())
	if rec2.code != http.StatusOK && (r.Method != http.MethodPost || rec2.code != http.StatusCreated) {
		w.WriteHeader(rec2.code)
		_, _ = w.Write(rec2.body)
		return
	}
	var u contracts.User
	if err := json.Unmarshal(rec2.body, &u); err != nil {
		apiutil.Error(w, http.StatusBadGateway, "auth returned an unparsable user")
		return
	}
	session := contracts.Session{User: u, Workspaces: []contracts.Workspace{}}

	token := sessionValue(r)
	if token == "" {
		apiutil.WriteJSON(w, rec2.code, session)
		return
	}
	id, ok := g.resolveIdentity(r.Context(), token)
	if ok {
		session.Workspaces = id.Workspaces
		if len(id.Workspaces) > 0 {
			session.ActiveWorkspaceID = id.Workspaces[0].ID
		}
		if id.Superadmin && session.User.IsSuperadmin == nil {
			b := true
			session.User.IsSuperadmin = &b
		}
	}
	apiutil.WriteJSON(w, rec2.code, session)
}

// serveWorkspaces merges agent_count + open_task_count into the workspace list.
func (g *gateway) serveWorkspaces(w http.ResponseWriter, r *http.Request) {
	rec := &responseRecorder{}
	g.orgs.ServeHTTP(rec, r)
	if rec.code != http.StatusOK {
		w.WriteHeader(rec.code)
		_, _ = w.Write(rec.body)
		return
	}
	var wss []contracts.Workspace
	if err := json.Unmarshal(rec.body, &wss); err != nil {
		apiutil.Error(w, http.StatusBadGateway, "orgs returned an unparsable workspace list")
		return
	}
	for i := range wss {
		var counts struct {
			AgentCount    int `json:"agent_count"`
			OpenTaskCount int `json:"open_task_count"`
		}
		if g.internalJSON(g.domains["agents"], "/internal/workspace/"+string(wss[i].ID)+"/agent-count", &counts) {
			wss[i].AgentCount = &counts.AgentCount
		}
		if g.internalJSON(g.domains["tasks"], "/internal/workspace/"+string(wss[i].ID)+"/open-task-count", &counts) {
			wss[i].OpenTaskCount = &counts.OpenTaskCount
		}
	}
	apiutil.WriteJSON(w, http.StatusOK, wss)
}

// serveKpis fans out to Auth (active users) + Orgs (org/workspace/seats).
func (g *gateway) serveKpis(w http.ResponseWriter, r *http.Request) {
	kpis := contracts.SystemKpis{}
	var orgStats struct {
		Organizations int `json:"organizations"`
		Workspaces    int `json:"workspaces"`
		OpenSeats     int `json:"open_seats"`
	}
	g.internalJSON(g.orgs, "/internal/stats", &orgStats)
	kpis.Organizations, kpis.Workspaces, kpis.OpenSeats = orgStats.Organizations, orgStats.Workspaces, orgStats.OpenSeats
	var au struct {
		ActiveUsers24h int `json:"active_users_24h"`
	}
	g.internalJSON(g.auth, "/internal/active-users-24h", &au)
	kpis.ActiveUsers24h = au.ActiveUsers24h
	apiutil.WriteJSON(w, http.StatusOK, kpis)
}

// serveHealth probes every service /healthz and reports ok/warn/down.
func (g *gateway) serveHealth(w http.ResponseWriter, r *http.Request) {
	probes := []struct {
		name string
		rp   *httputil.ReverseProxy
	}{
		{"gateway", nil}, {"project", g.domains["projects"]}, {"task", g.domains["tasks"]},
		{"agent", g.domains["agents"]}, {"catalog", g.domains["skills"]},
		{"settings", g.domains["provider-keys"]}, {"runner", g.domains["runs"]},
		{"auth", g.auth}, {"orgs", g.orgs},
		{"resources", g.domains["resources"]}, {"admin", g.domains["admin"]},
	}
	out := contracts.SystemHealth{Services: []contracts.ServiceHealth{}}
	for _, p := range probes {
		sh := contracts.ServiceHealth{Name: p.name, Pct: 100, Status: "ok"}
		if p.rp != nil {
			base := upstreamBase(p.rp)
			if base == "" {
				sh.Pct, sh.Status = 0, "down"
			} else {
				client := &http.Client{Timeout: 3 * time.Second}
				resp, err := client.Get(base + "/healthz")
				if err != nil {
					sh.Pct, sh.Status = 0, "down"
				} else {
					_ = resp.Body.Close()
					sh.Pct, sh.Status = 100, "ok"
				}
			}
		}
		out.Services = append(out.Services, sh)
	}
	apiutil.WriteJSON(w, http.StatusOK, out)
}

// upstreamBase extracts the target URL from a ReverseProxy's Director.
func upstreamBase(rp *httputil.ReverseProxy) string {
	// The proxy package targets a fixed URL; expose it via a package-level
	// registry to avoid reflecting into unexported fields.
	return proxy.BaseURL(rp)
}

// ── Realtime (SSE) ──────────────────────────────────────────────────────────

// serveStream replays persisted steps (Runner) then tails Kafka by task_id.
func (g *gateway) serveStream(w http.ResponseWriter, r *http.Request, taskID string) {
	if !g.injectIdentity(w, r, true) {
		return
	}
	if !g.requireTaskWorkspace(w, r, taskID) {
		return
	}
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		apiutil.Error(w, http.StatusServiceUnavailable, "kafka is not configured; SSE unavailable")
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		apiutil.Error(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// 1. Replay persisted steps from the Runner (all runs, seq order).
	var steps []contracts.Step
	if g.internalJSON(g.domains["runs"], "/internal/tasks/"+taskID+"/steps", &steps) {
		for _, st := range steps {
			buf, _ := json.Marshal(st)
			writeSSE(w, fl, "step", buf)
		}
	}

	// 2. Tail Kafka step.* events for this task (dedup by step id). Each
	// connection gets its own consumer group so concurrent viewers (tabs) don't
	// rebalance each other out of the partition, and reads from the newest
	// offset since history is already covered by the replay above.
	cg, err := kafka.NewConsumerGroupFrom(kafka.Brokers(strings.Split(brokers, ",")), "gateway-sse-"+taskID+"-"+connID(), true, g.log)
	if err != nil {
		g.log.Warn("sse tail unavailable", "error", err)
		writeSSE(w, fl, "error", []byte(`{"message":"live tail unavailable; replayed history only"}`))
		return
	}
	defer func() { _ = cg.Close() }()
	seen := map[string]bool{}
	for _, st := range steps {
		seen[string(st.ID)] = true
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = cg.Run(ctx, []string{contracts.TopicStep}, func(ctx context.Context, env contracts.EventEnvelope) error {
			if env.TaskID != contracts.ID(taskID) {
				return nil
			}
			var d contracts.StepData
			if err := env.DecodeData(&d); err != nil {
				return nil
			}
			if seen[string(d.Step.ID)] {
				return nil
			}
			seen[string(d.Step.ID)] = true
			buf, _ := json.Marshal(d.Step)
			writeSSE(w, fl, "step", buf)
			return nil
		})
	}()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-done:
			writeSSE(w, fl, "error", []byte(`{"message":"stream ended"}`))
			return
		case <-ticker.C:
			writeSSE(w, fl, "ping", []byte(`{}`))
		}
	}
}

func writeSSE(w http.ResponseWriter, fl http.Flusher, event string, data []byte) {
	_, _ = w.Write([]byte("event: " + event + "\n"))
	_, _ = w.Write(append([]byte("data: "), append(data, '\n')...))
	_, _ = w.Write([]byte("\n"))
	fl.Flush()
}

// connID returns a random short identifier for a single SSE connection, used
// to give each connection its own consumer group.
func connID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func sessionValue(r *http.Request) string {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// responseRecorder captures an upstream response for composition.
type responseRecorder struct {
	code int
	body []byte
	hdr  http.Header
}

func (rr *responseRecorder) Header() http.Header {
	if rr.hdr == nil {
		rr.hdr = http.Header{}
	}
	return rr.hdr
}

func (rr *responseRecorder) WriteHeader(code int) { rr.code = code }

func (rr *responseRecorder) Write(b []byte) (int, error) {
	if rr.code == 0 {
		rr.code = http.StatusOK
	}
	rr.body = append(rr.body, b...)
	return len(b), nil
}

// copyHeaders copies src onto dst, preserving multiple values (Set-Cookie).
func copyHeaders(dst, src http.Header) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}
