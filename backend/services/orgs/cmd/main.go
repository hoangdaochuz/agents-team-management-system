// Package main is the Orgs/Workspaces service entrypoint: serves workspace
// CRUD, members, invites, and the orgs half of the sysadmin surface, and
// emits workspace.created / invite.created / signup approval events.
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
