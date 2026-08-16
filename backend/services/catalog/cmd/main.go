// Package main is the Catalog service entrypoint (owns Skill + McpServer CRUD).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/aaks/server/internal/platform/svcrun"
	"github.com/aaks/server/services/catalog/internal/application"
	"github.com/aaks/server/services/catalog/internal/infrastructure/repository"
	interfacehttp "github.com/aaks/server/services/catalog/internal/interfaces/http"
)

func main() {
	svcrun.Run("catalog", getenv("HTTP_ADDR", ":8084"), register)
}

// register is the explicit composition root (D5): config → platform deps →
// repositories → application handlers → HTTP adapter.
func register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("CATALOG_DB_DSN")
	if dsn == "" {
		return errors.New("CATALOG_DB_DSN is not set")
	}
	st, err := repository.New(ctx, dsn, log)
	if err != nil {
		return err
	}

	repo := &application.Repository{
		Skills: st.Skills,
		Mcps:   st.Mcps,
	}
	pub := repository.NewPublisher(os.Getenv("KAFKA_BROKERS"), log)
	app := application.New(repo, repository.NewUnitOfWork(st), pub, log)

	interfacehttp.New(app, log).Register(mux)

	log.Info("catalog routes registered", "endpoints", 13)
	return nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
