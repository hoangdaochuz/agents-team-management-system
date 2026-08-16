// Package main is the Resources service entrypoint: workspace knowledge
// sources, plugins, rules, and MCP connections.
package main

import (
	"os"

	"github.com/aaks/server/internal/platform/svcrun"
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
