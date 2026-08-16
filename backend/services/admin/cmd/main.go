// Package main is the Admin service entrypoint: workspace audit log, feature
// flags, and the admin half of the sysadmin surface.
package main

import (
	"os"

	"github.com/aaks/server/internal/platform/svcrun"
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
