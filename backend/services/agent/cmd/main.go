// Package main is the Agent service entrypoint (owns Agent CRUD + skill/mcp links).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/aaks/server/internal/platform/svcrun"
	"github.com/aaks/server/services/agent/internal/application"
	"github.com/aaks/server/services/agent/internal/infrastructure/acl"
	"github.com/aaks/server/services/agent/internal/infrastructure/repository"
	interfacehttp "github.com/aaks/server/services/agent/internal/interfaces/http"
)

func main() {
	svcrun.Run("agent", getenv("HTTP_ADDR", ":8083"), register)
}

// register is the explicit composition root (D5): config → platform deps →
// repositories/ACL client → application handlers → HTTP adapter.
func register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("AGENT_DB_DSN")
	if dsn == "" {
		return errors.New("AGENT_DB_DSN is not set")
	}
	st, err := repository.New(ctx, dsn, log)
	if err != nil {
		return err
	}

	repo := &application.Repository{
		Agents:      st.Agents,
		Projections: st.Projections,
	}
	// The Catalog client is optional: without CATALOG_URL the internal
	// mcp-servers endpoint returns an empty list.
	var catalog application.McpCatalogClient
	if url := os.Getenv("CATALOG_URL"); url != "" {
		catalog = acl.NewCatalogClient(url)
	}
	app := application.New(repo, catalog, log)

	interfacehttp.New(app, log).Register(mux)

	log.Info("agent routes registered", "endpoints", 11)
	return nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
