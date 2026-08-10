// Package httpapi registers the Admin/Sysadmin service routes. Phase-1 skeleton;
// audit + sysadmin handlers + superadmin gate land in phase 13.
package httpapi

import (
	"log/slog"
	"net/http"
)

// Register wires admin/sysadmin routes. Phase-1 stub; handlers land in phase 13.
func Register(mux *http.ServeMux, log *slog.Logger) error {
	_ = mux
	log.Info("admin routes registered (phase-1 stub; handlers land in phase 13)")
	return nil
}
