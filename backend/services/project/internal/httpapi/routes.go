// Package httpapi registers the Project service routes, matching the frontend
// contract in frontend/src/api/projects.ts (routes served without /api prefix;
// the Gateway strips /api and proxies).
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/aaks/server/services/project/internal/store"
)

// Register opens the project_db pool, runs migrations, and wires CRUD routes.
func Register(mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("PROJECT_DB_DSN")
	if dsn == "" {
		return errDSN("PROJECT_DB_DSN")
	}
	st, err := store.New(context.Background(), dsn, log)
	if err != nil {
		return err
	}

	app := &App{store: st, log: log}

	mux.HandleFunc("GET /projects", app.list)
	mux.HandleFunc("POST /projects", app.create)
	mux.HandleFunc("GET /projects/{id}", app.get)
	mux.HandleFunc("PUT /projects/{id}", app.update)
	mux.HandleFunc("DELETE /projects/{id}", app.delete)

	log.Info("project routes registered", "endpoints", 5)
	return nil
}

func errDSN(env string) error {
	return &missingDSNError{env: env}
}

type missingDSNError struct{ env string }

func (e *missingDSNError) Error() string { return e.env + " is not set" }
