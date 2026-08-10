// Package httpapi registers the Auth (identity) service routes. Phase-1 skeleton;
// session/signup/approval handlers land in phase 10 (see specs/auth).
package httpapi

import (
	"log/slog"
	"net/http"
)

// Register wires auth routes. Phase-1 stub; /auth/* handlers land in phase 10.
func Register(mux *http.ServeMux, log *slog.Logger) error {
	_ = mux
	log.Info("auth routes registered (phase-1 stub; handlers land in phase 10)")
	return nil
}
