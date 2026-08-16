// Package httpapi registers the Project service routes, matching the frontend
// contract in frontend/src/api/projects.ts (routes served without /api prefix;
// the Gateway strips /api and proxies).
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/aaks/server/internal/contracts"
	"github.com/aaks/server/internal/platform/kafka"
	"github.com/aaks/server/services/project/internal/store"
)

// Register opens the project_db pool, runs migrations, and wires CRUD routes.
func Register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
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

	app.startConsumers()

	log.Info("project routes registered", "endpoints", 5)
	return nil
}

// startConsumers subscribes to workspace.created so a default repo binding
// (project) is established for every new workspace (idempotent, best-effort).
func (a *App) startConsumers() {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		return
	}
	bs := kafka.Brokers(strings.Split(brokers, ","))
	cg, err := kafka.NewConsumerGroup(bs, "project-workspaces", a.log)
	if err != nil {
		a.log.Warn("project consumers unavailable", "error", err)
		return
	}
	go func() {
		if err := cg.Run(context.Background(), []string{contracts.TopicWorkspaceCreated}, a.consume); err != nil {
			a.log.Error("project consumer stopped", "error", err)
		}
	}()
}

// consume projects a default repo binding for a new workspace.
func (a *App) consume(ctx context.Context, env contracts.EventEnvelope) error {
	var d contracts.WorkspaceCreatedData
	if err := env.DecodeData(&d); err != nil {
		return err
	}
	if d.RepoSource == "" {
		return nil // no repo to bind; the user creates projects manually
	}
	name := d.Name
	if name == "" {
		name = "default"
	}
	if _, err := a.store.Create(ctx, d.WorkspaceID, store.CreateInput{
		Name: name, RepoSource: d.RepoSource, RepoType: "git", DefaultBranch: d.DefaultBranch,
	}); err != nil {
		// Duplicate project name per workspace is possible on redelivery; the
		// binding is best-effort so a failure is logged, not fatal.
		a.log.Warn("workspace repo binding failed", "workspace", d.WorkspaceID, "error", err)
	}
	return nil
}

func errDSN(env string) error {
	return &missingDSNError{env: env}
}

type missingDSNError struct{ env string }

func (e *missingDSNError) Error() string { return e.env + " is not set" }
