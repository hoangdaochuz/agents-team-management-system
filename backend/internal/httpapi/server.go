// Package httpapi wires the HTTP server, routes, and middleware.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/aaks/server/internal/config"
)

// Server is the HTTP front-end. It owns the *http.Server and its handler tree.
type Server struct {
	cfg     config.Config
	logger  *slog.Logger
	handler http.Handler
	httpd   *http.Server
}

// NewServer builds the router and an *http.Server with sane production timeouts.
func NewServer(cfg config.Config, logger *slog.Logger) *Server {
	mux := http.NewServeMux()
	s := &Server{cfg: cfg, logger: logger}

	mux.HandleFunc("GET /healthz", s.healthz)
	// TODO(T2): REST handlers (projects/tasks/agents/skills/mcp), SSE stream,
	// request-ID + recovery middleware.

	s.handler = mux
	s.httpd = &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return s
}

// ListenAndServe starts the HTTP listener.
func (s *Server) ListenAndServe() error { return s.httpd.ListenAndServe() }

// Shutdown gracefully drains in-flight requests.
func (s *Server) Shutdown(ctx context.Context) error { return s.httpd.Shutdown(ctx) }

// healthz is a liveness probe. TODO(T1): add a DB ping for readiness.
func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		s.logger.Error("encoding health response", "error", err)
	}
}
