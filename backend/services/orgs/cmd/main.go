// Package main is the Orgs/Workspaces service entrypoint: serves workspace
// CRUD, members, invites, and the orgs half of the sysadmin surface, and
// emits workspace.created / invite.created / signup approval events.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/aaks/server/internal/platform/svcrun"
	"github.com/aaks/server/services/orgs/internal/application"
	"github.com/aaks/server/services/orgs/internal/infrastructure/bus"
	"github.com/aaks/server/services/orgs/internal/infrastructure/repository"
	interfacehttp "github.com/aaks/server/services/orgs/internal/interfaces/http"
	"github.com/aaks/server/services/orgs/internal/interfaces/messaging"
)

func main() {
	svcrun.Run("orgs", getenv("HTTP_ADDR", ":8088"), register)
}

// register is the explicit composition root (D5): config → platform deps →
// repositories → application handlers → HTTP/Kafka adapters. The lifecycle ctx
// is threaded into the Kafka consumer so SIGTERM drains in-flight work.
func register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("ORGS_DB_DSN")
	if dsn == "" {
		return errors.New("ORGS_DB_DSN is not set")
	}
	st, err := repository.New(ctx, dsn, log)
	if err != nil {
		return err
	}

	repo := &application.Repository{
		Organizations: st.Organizations,
		Workspaces:    st.Workspaces,
		Members:       st.Members,
		Invites:       st.Invites,
		JoinRequests:  st.JoinRequests,
		OrgRequests:   st.OrgRequests,
	}
	pub := bus.NewPublisher(os.Getenv("KAFKA_BROKERS"), log)
	app := application.New(repo, repository.NewUnitOfWork(st), pub, log)

	interfacehttp.New(app, log).Register(mux)
	messaging.New(log, messaging.SignupRequestedHandler{App: app}).Start(ctx, os.Getenv("KAFKA_BROKERS"))

	log.Info("orgs routes registered", "endpoints", 19)
	return nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
