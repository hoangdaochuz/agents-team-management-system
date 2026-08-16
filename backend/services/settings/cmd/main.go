// Package main is the Settings service entrypoint: sole decryptor of provider
// keys + git credentials, exposed to the runner over mTLS only.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/aaks/server/internal/platform/svcrun"
	"github.com/aaks/server/services/settings/internal/application"
	"github.com/aaks/server/services/settings/internal/infrastructure/crypto"
	"github.com/aaks/server/services/settings/internal/infrastructure/store"
	interfacehttp "github.com/aaks/server/services/settings/internal/interfaces/http"
)

func main() {
	svcrun.Run("settings", getenv("HTTP_ADDR", ":8085"), register)
}

// register is the explicit composition root (D5): config → platform deps →
// repository + key cipher → application handlers → HTTP adapter.
func register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("SETTINGS_DB_DSN")
	if dsn == "" {
		return errors.New("SETTINGS_DB_DSN is not set")
	}
	st, err := store.New(ctx, dsn, log)
	if err != nil {
		return err
	}
	cipher, err := crypto.New(os.Getenv("SETTINGS_MASTER_KEY"))
	if err != nil {
		return err
	}

	repo := &application.Repository{Keys: st.Keys}
	app := application.New(repo, cipher, log)

	interfacehttp.New(app, log, os.Getenv("SETTINGS_INTERNAL_TOKEN")).Register(mux)

	log.Info("settings routes registered", "endpoints", 5, "mtls", os.Getenv("SETTINGS_MTLS") == "on")
	return nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}