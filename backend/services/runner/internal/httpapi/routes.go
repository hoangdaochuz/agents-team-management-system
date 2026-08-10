// Package httpapi registers the Agent-Runner's HTTP routes. Phase 1: healthz
// only. Phase 5 adds the run/step/finding/artifact query endpoints the Gateway
// reads from; the runner is primarily a Kafka consumer, not a request server.
package httpapi

import (
	"log/slog"
	"net/http"
)

// Register wires runner routes. Phase-1 stub; query endpoints + consumers land
// in phase 5.
func Register(mux *http.ServeMux, log *slog.Logger) error {
	_ = mux
	log.Info("runner routes registered (phase-1 stub)")
	return nil
}
