// Package main is the Task service entrypoint: serves Task CRUD, feedback, and
// the task-lifecycle saga (run/review/stop/open-pr) coordinated over Kafka.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/aaks/server/internal/platform/svcrun"
	"github.com/aaks/server/services/task/internal/application"
	"github.com/aaks/server/services/task/internal/infrastructure/bus"
	"github.com/aaks/server/services/task/internal/infrastructure/repository"
	interfacehttp "github.com/aaks/server/services/task/internal/interfaces/http"
	"github.com/aaks/server/services/task/internal/interfaces/messaging"
)

func main() {
	svcrun.Run("task", getenv("HTTP_ADDR", ":8082"), register)
}

// register is the explicit composition root (D5): config → platform deps →
// repositories + publisher → application handlers → HTTP/Kafka adapters. The
// lifecycle ctx is threaded into the Kafka consumer so SIGTERM drains
// in-flight saga facts. The producer is best-effort (no-op when KAFKA_BROKERS
// is unset), so the service still runs without Kafka.
func register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("TASK_DB_DSN")
	if dsn == "" {
		return errors.New("TASK_DB_DSN is not set")
	}
	st, err := repository.New(ctx, dsn, log)
	if err != nil {
		return err
	}

	repo := &application.Repository{Tasks: st.Tasks, Feedback: st.Feedback}
	pub := bus.NewPublisher(os.Getenv("KAFKA_BROKERS"), log)
	app := application.New(repo, pub, log)

	interfacehttp.New(app, log).Register(mux)
	messaging.New(app, log).Start(ctx, os.Getenv("KAFKA_BROKERS"))

	log.Info("task routes registered", "endpoints", 13, "saga_enabled", pub.Enabled())
	return nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
