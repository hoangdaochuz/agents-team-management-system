// Package httpapi is the API Gateway / BFF. It is a path-aware reverse proxy:
// it inspects /api/<domain>/... requests, routes them to the owning service
// (stripping /api), and returns 501 for domains whose services are not yet
// implemented (auth/orgs/resources/admin — they land in phases 10–13) or 502
// when an upstream is unreachable.
//
// The current frontend composes cross-service reads client-side (it fetches
// tasks and agents separately), so no server-side fan-out is required today;
// task 4.3 (fan-out) is therefore deferred. SSE (/tasks/:id/stream) is owned by
// the gateway and wired in phase 8; until then it returns 501.
package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"

	apiutil "github.com/aaks/server/internal/httputil"
	"github.com/aaks/server/services/gateway/internal/proxy"
)

// Register wires the gateway. It reads UPSTREAM_* env vars (each an absolute
// URL like http://project:8081) and registers a catch-all /api/ handler.
func Register(mux *http.ServeMux, log *slog.Logger) error {
	gw, err := newGateway()
	if err != nil {
		return err
	}
	mux.HandleFunc("/api/", gw.serve)
	log.Info("gateway routes registered",
		"upstreams", gw.domains, "not_yet_routed", notYetRouted)
	return nil
}

// notYetRouted lists /api domains the frontend declares whose backing services
// (auth/orgs/resources/admin) are not yet implemented (phases 10–13). They 501
// until those services are built and the route table is expanded (task 4.6).
var notYetRouted = []string{"auth", "sysadmin", "workspaces"}

type gateway struct {
	// domains maps a routing domain to its reverse proxy.
	domains map[string]*httputil.ReverseProxy
	// taskSpecial proxies task sub-routes owned by another service (runner).
	runner *httputil.ReverseProxy
	task   *httputil.ReverseProxy
}

func newGateway() (*gateway, error) {
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
	return &gateway{
		domains: map[string]*httputil.ReverseProxy{
			"projects":       project,
			"tasks":          task,
			"agents":         agent,
			"skills":         catalog,
			"mcp-servers":    catalog,
			"provider-keys":  settings,
		},
		task:   task,
		runner: runner,
	}, nil
}

// serve routes a single /api/... request.
func (g *gateway) serve(w http.ResponseWriter, r *http.Request) {
	// path after "/api/": e.g. "tasks/123/runs"
	rest := strings.TrimPrefix(r.URL.Path, "/api/")
	rest = strings.TrimPrefix(rest, "/")
	segs := strings.Split(rest, "/")
	if len(segs) == 0 || segs[0] == "" {
		apiutil.Error(w, http.StatusNotFound, "not found")
		return
	}
	domain := segs[0]

	// Not-yet-implemented domains (phases 10–13).
	for _, n := range notYetRouted {
		if domain == n {
			apiutil.Error(w, http.StatusNotImplemented, "not implemented yet (lands in phases 10–13): "+domain)
			return
		}
	}

	// /api/runs/* -> runner (owns Run/Step/Finding/Artifact).
	if domain == "runs" {
		g.runner.ServeHTTP(w, r)
		return
	}

	// Task sub-routes owned by the runner: /tasks/{id}/runs|artifacts|stream.
	if domain == "tasks" && len(segs) >= 3 {
		switch segs[2] {
		case "runs", "artifacts":
			g.runner.ServeHTTP(w, r)
			return
		case "stream":
			// SSE owned by the gateway; wired in phase 8.
			apiutil.Error(w, http.StatusNotImplemented, "SSE stream wired in phase 8")
			return
		}
	}

	rp, ok := g.domains[domain]
	if !ok {
		apiutil.Error(w, http.StatusNotFound, "no route for domain: " + domain)
		return
	}
	rp.ServeHTTP(w, r)
}
