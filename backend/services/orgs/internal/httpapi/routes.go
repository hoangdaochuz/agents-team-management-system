// Package httpapi registers the Orgs/Workspaces service routes. Phase-1 skeleton;
// workspaces/members/invites handlers + membership authz land in phase 11.
package httpapi

import (
	"log/slog"
	"net/http"
)

// Register wires orgs/workspaces routes. Phase-1 stub; handlers land in phase 11.
func Register(mux *http.ServeMux, log *slog.Logger) error {
	_ = mux
	log.Info("orgs routes registered (phase-1 stub; handlers land in phase 11)")
	return nil
}
