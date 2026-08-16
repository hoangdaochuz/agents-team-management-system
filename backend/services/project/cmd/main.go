// Package main is the Project service entrypoint: serves project CRUD and
// consumes workspace.created to establish default repo bindings.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/aaks/server/internal/platform/svcrun"
	"github.com/aaks/server/services/project/internal/application"
	"github.com/aaks/server/services/project/internal/infrastructure/store"
	interfacehttp "github.com/aaks/server/services/project/internal/interfaces/http"
	"github.com/aaks/server/services/project/internal/interfaces/messaging"
)

func main() {
	svcrun.Run("project", getenv("HTTP_ADDR", ":8081"), register)
}

// register is the explicit composition root (D5): config → platform deps →
// repository → application handlers → HTTP/Kafka adapters. The lifecycle ctx
// is threaded into the Kafka consumer so SIGTERM drains in-flight work.
func register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("PROJECT_DB_DSN")
	if dsn == "" {
		return errors.New("PROJECT_DB_DSN is not set")
	}
	st, err := store.New(ctx, dsn, log)
	if err != nil {
		return err
	}

	repo := &application.Repository{Projects: st.Projects}
	app := application.New(repo, log)

	interfacehttp.New(app, log).Register(mux)
	messaging.New(app, log).Start(ctx, os.Getenv("KAFKA_BROKERS"))

	log.Info("project routes registered", "endpoints", 5)
	return nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}