// Package main is the Orgs/Workspaces service entrypoint: organizations,
// workspaces, memberships, invites, and the workspace-scoping contract.
// Implemented in phase 11.
package main

import (
	"os"

	"github.com/aaks/server/internal/svcrun"
	"github.com/aaks/server/services/orgs/internal/httpapi"
)

func main() {
	svcrun.Run("orgs", getenv("HTTP_ADDR", ":8088"), httpapi.Register)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
