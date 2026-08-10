// Package httpapi registers the Settings service routes. Phase 1: healthz only.
// Phase 7 adds /provider-keys CRUD + the mTLS-only internal decrypt endpoint.
package httpapi

import (
	"log/slog"
	"net/http"
)

// Register wires settings routes. Phase-1 stub; keys + decrypt land in phase 7.
func Register(mux *http.ServeMux, log *slog.Logger) error {
	_ = mux
	log.Info("settings routes registered (phase-1 stub)")
	return nil
}
