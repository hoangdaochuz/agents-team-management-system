// Package main is the Agent-Runner entrypoint: consumes run/review/stop commands,
// drives the agent loop (LLM + MCP + git), manages worktrees + credential-less
// sandbox containers, owns Run/Step/Finding/Artifact, and emits execution facts.
package main

import (
	"os"

	"github.com/aaks/server/internal/platform/svcrun"
	"github.com/aaks/server/services/runner/internal/httpapi"
)

func main() {
	svcrun.Run("runner", getenv("HTTP_ADDR", ":8086"), httpapi.Register)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
