// Package main is the Workspace-Resources service entrypoint: knowledge sources,
// plugins, rules, and workspace MCP connections. Implemented in phase 12.
package main

import (
	"os"

	"github.com/aaks/server/internal/svcrun"
	"github.com/aaks/server/services/resources/internal/httpapi"
)

func main() {
	svcrun.Run("resources", getenv("HTTP_ADDR", ":8089"), httpapi.Register)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
