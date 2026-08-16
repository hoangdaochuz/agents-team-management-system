// Package http exposes the Gateway's routing as thin HTTP handlers: strip
// forged identity headers, resolve the handling plan (application RouteTable),
// inject the session scoping headers (application ACL), and proxy / compose /
// stream. All decision logic lives in application; this package only adapts
// requests and responses.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

	"github.com/aaks/server/internal/contracts/admin"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/workspaces"
	apiutil "github.com/aaks/server/internal/platform/http"
	"github.com/aaks/server/internal/platform/tenancy"
	"github.com/aaks/server/services/gateway/internal/application"
)

// SessionCookie is the session cookie name shared with the Auth service.
const SessionCookie = "aaks_session"

// Server wires the Gateway routes to the application service and the injected
// upstream proxies.
type Server struct {
	app     *application.App
	proxies map[application.Upstream]*httputil.ReverseProxy
	bases   map[application.Upstream]string // upstream base URLs (health probes)
	brokers string                          // KAFKA_BROKERS; "" disables SSE
	log     *slog.Logger
}

// New builds the HTTP adapter. proxies is the upstream reverse-proxy set;
// bases mirrors the same upstream URLs for the health probes.
func New(app *application.App, proxies map[application.Upstream]*httputil.ReverseProxy, bases map[application.Upstream]string, brokers string, log *slog.Logger) *Server {
	return &Server{app: app, proxies: proxies, bases: bases, brokers: brokers, log: log}
}

// Register mounts the catch-all /api/ handler, mirroring the pre-DDD gateway.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/", s.serve)
	s.log.Info("gateway routes registered", "upstreams", len(s.proxies))
}

// serve routes a single /api/... request.
func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	// Identity/scoping headers must never survive a round trip from the
	// client: an attacker could spoof another user, workspace membership, or
	// superadmin status. Strip them at the gateway boundary BEFORE injection
	// below — only ACL.Inject may set them afterwards (the proxy Director runs
	// after this handler and must not strip again).
	stripInboundIdentity(r)

	rest := strings.TrimPrefix(r.URL.Path, "/api/")
	rest = strings.TrimPrefix(rest, "/")
	segs := strings.Split(rest, "/")

	route, err := s.app.Routes.Resolve(segs, r.Method)
	if err != nil {
		s.writeRouteError(w, err)
		return
	}

	switch route.Kind {
	case application.RouteStream:
		s.serveStream(w, r, route)
	case application.RouteSession:
		s.serveSession(w, r)
	case application.RouteWorkspaceRemap:
		if !s.inject(w, r, route.RequireIdentity) {
			return
		}
		if !s.requireMember(w, r, route.WorkspaceID) {
			return
		}
		r.Header.Set(tenancy.HeaderWorkspaceID, route.WorkspaceID)
		s.proxy(route.Upstream).ServeHTTP(w, r)
	case application.RouteTaskRuns:
		if !s.inject(w, r, route.RequireIdentity) {
			return
		}
		if !s.requireTask(w, r, route.TaskID) {
			return
		}
		s.proxy(route.Upstream).ServeHTTP(w, r)
	case application.RouteWorkspacesList:
		if !s.inject(w, r, route.RequireIdentity) {
			return
		}
		s.serveWorkspaces(w, r)
	case application.RouteKpis:
		if !s.inject(w, r, route.RequireIdentity) {
			return
		}
		s.serveKpis(w, r)
	case application.RouteHealth:
		if !s.inject(w, r, route.RequireIdentity) {
			return
		}
		s.serveHealth(w, r)
	case application.RouteSysadminAdmin:
		if !s.inject(w, r, route.RequireIdentity) {
			return
		}
		s.proxy(route.Upstream).ServeHTTP(w, r)
	default: // RouteProxy
		if !s.inject(w, r, route.RequireIdentity) {
			return
		}
		s.proxy(route.Upstream).ServeHTTP(w, r)
	}
}

// writeRouteError maps route-resolution failures to their 404 responses.
func (s *Server) writeRouteError(w http.ResponseWriter, err error) {
	var noRoute application.NoRouteError
	if errors.As(err, &noRoute) {
		apiutil.Error(w, http.StatusNotFound, noRoute.Error())
		return
	}
	apiutil.Error(w, http.StatusNotFound, "not found")
}

// ── Identity resolution ─────────────────────────────────────────────────────

// inject resolves the session cookie (via ACL) and injects the scoping
// headers. Returns false after writing a 401 when the session is missing or
// unresolvable and required=true. Any inbound X-User-*/X-Workspace-* values
// were deleted at the boundary (stripInboundIdentity) so only the ACL's
// header values populate them.
func (s *Server) inject(w http.ResponseWriter, r *http.Request, required bool) bool {
	token := sessionValue(r)
	if token == "" {
		if required {
			apiutil.Error(w, http.StatusUnauthorized, "not authenticated")
			return false
		}
		return true
	}
	id, ok := s.app.ACL.Resolve(r.Context(), token)
	if !ok {
		if required {
			apiutil.Error(w, http.StatusUnauthorized, "not authenticated")
			return false
		}
		return true
	}
	for k, v := range s.app.ACL.Headers(id) {
		r.Header.Set(k, v)
	}
	return true
}

// requireMember returns false after writing a 403 when wid is not in the
// caller's resolved workspace union. The identity headers must already be
// injected so X-Workspace-IDs reflects the session.
func (s *Server) requireMember(w http.ResponseWriter, r *http.Request, wid string) bool {
	if s.app.ACL.IsWorkspaceMember(tenancy.WorkspaceIDs(r), wid) {
		return true
	}
	apiutil.Error(w, http.StatusForbidden, "not a member of workspace "+wid)
	return false
}

// requireTask returns false after writing a 403/404 when the task does not
// belong to a workspace in the caller's union.
func (s *Server) requireTask(w http.ResponseWriter, r *http.Request, taskID identity.ID) bool {
	ok, err := s.app.ACL.TaskAccessible(r.Context(), tenancy.WorkspaceIDs(r), taskID)
	if err != nil {
		apiutil.Error(w, http.StatusNotFound, "task not found")
		return false
	}
	if !ok {
		apiutil.Error(w, http.StatusForbidden, "task is not in an accessible workspace")
		return false
	}
	return true
}

// stripInboundIdentity removes any identity/scoping header the client supplied
// so no forged value can reach the upstream services. Only ACL.Inject (via
// resolveIdentity against Auth + Orgs) may populate them.
func stripInboundIdentity(r *http.Request) {
	for _, h := range tenancy.IdentityHeaders {
		r.Header.Del(h)
	}
}

// sessionValue returns the session cookie value, or "".
func sessionValue(r *http.Request) string {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// proxy returns the reverse proxy for an upstream.
func (s *Server) proxy(u application.Upstream) *httputil.ReverseProxy {
	return s.proxies[u]
}

// ── Composed endpoints ──────────────────────────────────────────────────────

// serveSession composes /auth/login + /auth/me into the full Session shape.
func (s *Server) serveSession(w http.ResponseWriter, r *http.Request) {
	rec := &responseRecorder{}
	s.proxy(application.UpstreamAuth).ServeHTTP(rec, r)
	// The recorder captures headers (session cookie on login) — put them back
	// on the wire or the client never receives the cookie.
	copyHeaders(w.Header(), rec.Header())
	if rec.code != http.StatusOK && (r.Method != http.MethodPost || rec.code != http.StatusCreated) {
		w.WriteHeader(rec.code)
		_, _ = w.Write(rec.body)
		return
	}
	var u identity.User
	if err := json.Unmarshal(rec.body, &u); err != nil {
		apiutil.Error(w, http.StatusBadGateway, "auth returned an unparsable user")
		return
	}
	session := workspaces.Session{User: u, Workspaces: []workspaces.Workspace{}}

	token := sessionValue(r)
	if token == "" {
		apiutil.WriteJSON(w, rec.code, session)
		return
	}
	id, ok := s.app.ACL.Resolve(r.Context(), token)
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
	apiutil.WriteJSON(w, rec.code, session)
}

// serveWorkspaces merges agent_count + open_task_count into the workspace list.
func (s *Server) serveWorkspaces(w http.ResponseWriter, r *http.Request) {
	rec := &responseRecorder{}
	s.proxy(application.UpstreamOrgs).ServeHTTP(rec, r)
	if rec.code != http.StatusOK {
		w.WriteHeader(rec.code)
		_, _ = w.Write(rec.body)
		return
	}
	var wss []workspaces.Workspace
	if err := json.Unmarshal(rec.body, &wss); err != nil {
		apiutil.Error(w, http.StatusBadGateway, "orgs returned an unparsable workspace list")
		return
	}
	for i := range wss {
		if n, err := s.app.Stats.AgentCount(r.Context(), wss[i].ID); err == nil {
			wss[i].AgentCount = &n
		}
		if n, err := s.app.Stats.OpenTaskCount(r.Context(), wss[i].ID); err == nil {
			wss[i].OpenTaskCount = &n
		}
	}
	apiutil.WriteJSON(w, http.StatusOK, wss)
}

// serveKpis fans out to Auth (active users) + Orgs (org/workspace/seats).
func (s *Server) serveKpis(w http.ResponseWriter, _ *http.Request) {
	kpis := admin.SystemKpis{}
	if org, err := s.app.Stats.OrgStats(context.Background()); err == nil {
		kpis.Organizations, kpis.Workspaces, kpis.OpenSeats = org.Organizations, org.Workspaces, org.OpenSeats
	}
	if n, err := s.app.Stats.ActiveUsers24h(context.Background()); err == nil {
		kpis.ActiveUsers24h = n
	}
	apiutil.WriteJSON(w, http.StatusOK, kpis)
}

// serveHealth probes every service /healthz and reports ok/warn/down.
func (s *Server) serveHealth(w http.ResponseWriter, _ *http.Request) {
	probes := []struct {
		name     string
		upstream application.Upstream
	}{
		{"gateway", ""}, {"project", application.UpstreamProject}, {"task", application.UpstreamTask},
		{"agent", application.UpstreamAgent}, {"catalog", application.UpstreamCatalog},
		{"settings", application.UpstreamSettings}, {"runner", application.UpstreamRunner},
		{"auth", application.UpstreamAuth}, {"orgs", application.UpstreamOrgs},
		{"resources", application.UpstreamResources}, {"admin", application.UpstreamAdmin},
	}
	out := admin.SystemHealth{Services: []admin.ServiceHealth{}}
	for _, p := range probes {
		sh := admin.ServiceHealth{Name: p.name, Pct: 100, Status: "ok"}
		if base := s.bases[p.upstream]; p.upstream != "" && base != "" {
			client := &http.Client{Timeout: 3 * time.Second}
			resp, err := client.Get(base + "/healthz")
			if err != nil {
				sh.Pct, sh.Status = 0, "down"
			} else {
				_ = resp.Body.Close()
				sh.Pct, sh.Status = 100, "ok"
			}
		}
		out.Services = append(out.Services, sh)
	}
	apiutil.WriteJSON(w, http.StatusOK, out)
}

// ── Realtime (SSE) ──────────────────────────────────────────────────────────

// serveStream replays persisted steps (Runner) then tails Kafka by task_id.
func (s *Server) serveStream(w http.ResponseWriter, r *http.Request, route application.Route) {
	if !s.inject(w, r, true) {
		return
	}
	if !s.requireTask(w, r, route.TaskID) {
		return
	}
	if s.brokers == "" {
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

	_ = s.app.Stream.Serve(ctx, route.TaskID, &sseWriter{w: w, fl: fl})
}

// sseWriter adapts an http.ResponseWriter + Flusher to the application's
// SSEWriter, matching the pre-DDD gateway's wire format byte-for-byte. The
// mutex serializes writers: the stream loop (replay/ping/terminal events)
// and the Kafka tail goroutine both emit events, and http.ResponseWriter is
// not safe for concurrent use.
type sseWriter struct {
	w  http.ResponseWriter
	fl http.Flusher
	mu sync.Mutex
}

// Event writes one SSE event and flushes.
func (s *sseWriter) Event(event string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.w.Write([]byte("event: " + event + "\n"))
	_, _ = s.w.Write(append([]byte("data: "), append(data, '\n')...))
	_, _ = s.w.Write([]byte("\n"))
	s.fl.Flush()
}

// ── Helpers ─────────────────────────────────────────────────────────────────

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
