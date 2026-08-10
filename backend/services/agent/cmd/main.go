// Package main is the Agent service entrypoint (owns Agent CRUD + skill/mcp links).
package main

import (
	"os"

	"github.com/aaks/server/internal/svcrun"
	"github.com/aaks/server/services/agent/internal/httpapi"
)

func main() {
	svcrun.Run("agent", getenv("HTTP_ADDR", ":8083"), httpapi.Register)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
