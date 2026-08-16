// Package main is the Auth (identity) service entrypoint: user/session
// lifecycle, signup + admin approval, SSO begin stub. Implemented in phase 10.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/aaks/server/internal/platform/svcrun"
	"github.com/aaks/server/services/auth/internal/application"
	"github.com/aaks/server/services/auth/internal/infrastructure/bus"
	"github.com/aaks/server/services/auth/internal/infrastructure/repository"
	interfacehttp "github.com/aaks/server/services/auth/internal/interfaces/http"
	"github.com/aaks/server/services/auth/internal/interfaces/messaging"
)

func main() {
	svcrun.Run("auth", getenv("HTTP_ADDR", ":8087"), register)
}

// register is the explicit composition root (D5): config → platform deps →
// repositories → application handlers → HTTP/Kafka adapters. The lifecycle ctx
// is threaded into the Kafka consumer so SIGTERM drains in-flight work.
func register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("AUTH_DB_DSN")
	if dsn == "" {
		return errors.New("AUTH_DB_DSN is not set")
	}
	st, err := repository.New(ctx, dsn, log)
	if err != nil {
		return err
	}

	repo := &application.Repository{
		Users:          st.Users,
		Sessions:       st.Sessions,
		SignupRequests: st.SignupRequests,
		Invites:        st.Invites,
	}
	pub := bus.NewPublisher(os.Getenv("KAFKA_BROKERS"), log)
	app := application.New(repo, pub, log)

	if email, pass := os.Getenv("AUTH_SEED_SUPERADMIN_EMAIL"), os.Getenv("AUTH_SEED_SUPERADMIN_PASSWORD"); email != "" && pass != "" {
		app.SeedSuperadmin(ctx, email, pass)
	}

	ssoCfg := map[string]string{
		"google": os.Getenv("SSO_GOOGLE_REDIRECT_URL"),
		"saml":   os.Getenv("SSO_SAML_REDIRECT_URL"),
	}
	interfacehttp.New(app, log, ssoCfg).Register(mux)
	messaging.New(log,
		messaging.SignupApprovedHandler{App: app},
		messaging.SignupDeclinedHandler{App: app},
		messaging.InviteCreatedHandler{App: app},
	).Start(ctx, os.Getenv("KAFKA_BROKERS"))

	log.Info("auth routes registered", "endpoints", 9)
	return nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
