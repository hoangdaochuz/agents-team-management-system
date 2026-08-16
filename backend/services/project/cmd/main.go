// Package main is the Project service entrypoint (owns Project CRUD).
package main

import (
	"os"

	"github.com/aaks/server/internal/platform/svcrun"
	"github.com/aaks/server/services/project/internal/httpapi"
)

func main() {
	svcrun.Run("project", getenv("HTTP_ADDR", ":8081"), httpapi.Register)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
