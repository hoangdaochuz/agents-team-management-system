// Package main is the Admin/Sysadmin service entrypoint: workspace audit,
// sysadmin organizations, feature flags, system KPIs/health/audit, maintenance.
// Implemented in phase 13.
package main

import (
	"os"

	"github.com/aaks/server/internal/svcrun"
	"github.com/aaks/server/services/admin/internal/httpapi"
)

func main() {
	svcrun.Run("admin", getenv("HTTP_ADDR", ":8090"), httpapi.Register)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
