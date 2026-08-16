// Package main is the Admin service entrypoint: workspace audit log, feature
// flags, and the admin half of the sysadmin surface.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/aaks/server/internal/platform/svcrun"
	"github.com/aaks/server/services/admin/internal/application"
	"github.com/aaks/server/services/admin/internal/infrastructure/repository"
	interfacehttp "github.com/aaks/server/services/admin/internal/interfaces/http"
	"github.com/aaks/server/services/admin/internal/interfaces/messaging"
)

func main() {
	svcrun.Run("admin", getenv("HTTP_ADDR", ":8090"), register)
}

// register is the explicit composition root (D5): config → platform deps →
// repositories → application handlers → HTTP/Kafka adapters. The lifecycle ctx
// is threaded into the Kafka consumer so SIGTERM drains in-flight work.
func register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("ADMIN_DB_DSN")
	if dsn == "" {
		return errors.New("ADMIN_DB_DSN is not set")
	}
	st, err := repository.New(ctx, dsn, log)
	if err != nil {
		return err
	}

	repo := &application.Repository{
		Audit: st.Audit,
		Flags: st.Flags,
	}
	app := application.New(repo, log)

	interfacehttp.New(app, log).Register(mux)
	messaging.New(log, messaging.AuditRecordedHandler{App: app}).Start(ctx, os.Getenv("KAFKA_BROKERS"))

	log.Info("admin routes registered", "endpoints", 6)
	return nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
