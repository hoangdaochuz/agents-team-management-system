// Package main is the Catalog service entrypoint (owns Skill + McpServer CRUD).
package main

import (
	"os"

	"github.com/aaks/server/internal/svcrun"
	"github.com/aaks/server/services/catalog/internal/httpapi"
)

func main() {
	svcrun.Run("catalog", getenv("HTTP_ADDR", ":8084"), httpapi.Register)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
