// Package httpapi registers the Workspace-Resources service routes. Phase-1
// skeleton; knowledge/plugins/rules/workspace-mcp handlers land in phase 12.
package httpapi

import (
	"log/slog"
	"net/http"
)

// Register wires resources routes. Phase-1 stub; handlers land in phase 12.
func Register(mux *http.ServeMux, log *slog.Logger) error {
	_ = mux
	log.Info("resources routes registered (phase-1 stub; handlers land in phase 12)")
	return nil
}
