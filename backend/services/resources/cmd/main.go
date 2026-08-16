// Package main is the Resources service entrypoint: workspace knowledge
// sources, plugins, rules, and MCP connections (projected from the Catalog),
// plus default rule seeding on workspace creation.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/aaks/server/internal/platform/svcrun"
	"github.com/aaks/server/services/resources/internal/application"
	"github.com/aaks/server/services/resources/internal/infrastructure/repository"
	interfacehttp "github.com/aaks/server/services/resources/internal/interfaces/http"
	"github.com/aaks/server/services/resources/internal/interfaces/messaging"
)

func main() {
	svcrun.Run("resources", getenv("HTTP_ADDR", ":8089"), register)
}

// register is the explicit composition root (D5): config → platform deps →
// repositories → application handlers → HTTP/Kafka adapters. The lifecycle ctx
// is threaded into the Kafka consumer so SIGTERM drains in-flight work.
func register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("RESOURCES_DB_DSN")
	if dsn == "" {
		return errors.New("RESOURCES_DB_DSN is not set")
	}
	st, err := repository.New(ctx, dsn, log)
	if err != nil {
		return err
	}

	repo := &application.Repository{
		Knowledge: st.Knowledge,
		Plugins:   st.Plugins,
		Rules:     st.Rules,
		Mcp:       st.Mcp,
	}
	app := application.New(repo, repository.NewUnitOfWork(st), log)

	interfacehttp.New(app, log).Register(mux)
	messaging.New(app, log).Start(ctx, os.Getenv("KAFKA_BROKERS"))

	log.Info("resources routes registered", "endpoints", 9)
	return nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
