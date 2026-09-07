// Package main is the Agent service entrypoint: the consolidated
// Agent + Settings bounded context over a single agent_db. It serves agent
// CRUD (persona, model, tools, attached skills/MCPs) and is the sole
// decryptor of provider keys, exposed to the Executor over the internal token
// channel only. The provider-key cipher stays isolated in
// infrastructure/crypto: the master key lives in memory and plaintext keys
// never leave the process.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/aaks/server/internal/platform/internaltoken"
	"github.com/aaks/server/internal/platform/svcrun"
	agentapp "github.com/aaks/server/services/agent/internal/application/agent"
	settingsapp "github.com/aaks/server/services/agent/internal/application/settings"
	"github.com/aaks/server/services/agent/internal/infrastructure/acl"
	"github.com/aaks/server/services/agent/internal/infrastructure/crypto"
	"github.com/aaks/server/services/agent/internal/infrastructure/repository"
	agenthttp "github.com/aaks/server/services/agent/internal/interfaces/http/agent"
	settingshttp "github.com/aaks/server/services/agent/internal/interfaces/http/settings"
)

func main() {
	svcrun.Run("agent", getenv("HTTP_ADDR", ":8083"), register)
}

// register is the explicit composition root: config → platform deps →
// repositories + key cipher → application handlers → HTTP adapters.
func register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("AGENT_DB_DSN")
	if dsn == "" {
		return errors.New("AGENT_DB_DSN is not set")
	}
	st, err := repository.New(ctx, dsn, log)
	if err != nil {
		return err
	}
	cipher, err := crypto.New(os.Getenv("AGENT_MASTER_KEY"))
	if err != nil {
		return err
	}

	// The Workspace catalog client is optional: without WORKSPACE_URL the
	// internal mcp-servers endpoint returns an empty list.
	var catalog agentapp.McpCatalogClient
	if url := os.Getenv("WORKSPACE_URL"); url != "" {
		catalog = acl.NewCatalogClient(url)
	}
	agentApp := agentapp.New(&agentapp.Repository{
		Agents:      st.Agents,
		Projections: st.Projections,
	}, catalog, log)
	settingsApp := settingsapp.New(&settingsapp.Repository{Keys: st.Keys}, cipher, log)

	inner := http.NewServeMux()
	agenthttp.New(agentApp, log).Register(inner)
	settingshttp.New(settingsApp, log, os.Getenv("AGENT_INTERNAL_TOKEN")).Register(inner)
	// Gate the /internal/* surface (key decrypt, MCP hydration, counts)
	// behind the shared service token (unset = open, for dev/tests; compose
	// sets it). X-Agent-Token remains the key-decrypt-specific gate.
	mux.Handle("/", internaltoken.Wrap(os.Getenv("INTERNAL_TOKEN"), inner))

	log.Info("agent routes registered", "endpoints", 16)
	return nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
