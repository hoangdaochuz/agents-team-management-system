// Package main is the Workspace service entrypoint: the consolidated
// Project + Task + Catalog + Resources bounded context over a single
// workspace_db. It serves project CRUD, task CRUD + feedback, the
// task-lifecycle saga (run/review/stop/open-pr coordinated with the Executor
// over Kafka), the skill/MCP-server catalog, and workspace knowledge/plugins/
// rules/MCP connections. The former Catalog→Resources Kafka projections
// (mcp.created/mcp.deleted) dispatch over the in-process event bus.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/platform/svcrun"
	catalogapp "github.com/aaks/server/services/workspace/internal/application/catalog"
	projectapp "github.com/aaks/server/services/workspace/internal/application/project"
	resourcesapp "github.com/aaks/server/services/workspace/internal/application/resources"
	taskapp "github.com/aaks/server/services/workspace/internal/application/task"
	"github.com/aaks/server/services/workspace/internal/infrastructure/bus"
	"github.com/aaks/server/services/workspace/internal/infrastructure/repository"
	cataloghttp "github.com/aaks/server/services/workspace/internal/interfaces/http/catalog"
	projecthttp "github.com/aaks/server/services/workspace/internal/interfaces/http/project"
	provisionhttp "github.com/aaks/server/services/workspace/internal/interfaces/http/provision"
	resourceshttp "github.com/aaks/server/services/workspace/internal/interfaces/http/resources"
	taskhttp "github.com/aaks/server/services/workspace/internal/interfaces/http/task"
	"github.com/aaks/server/services/workspace/internal/interfaces/messaging"
)

func main() {
	svcrun.Run("workspace", getenv("HTTP_ADDR", ":8081"), register)
}

// register is the explicit composition root: config → platform deps →
// repositories → event bus → application handlers → HTTP/Kafka adapters. The
// lifecycle ctx is threaded into the Kafka consumer so SIGTERM drains
// in-flight saga facts. The producer is best-effort (no-op when KAFKA_BROKERS
// is unset), so the service still runs without Kafka.
func register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("WORKSPACE_DB_DSN")
	if dsn == "" {
		return errors.New("WORKSPACE_DB_DSN is not set")
	}
	st, err := repository.New(ctx, dsn, log)
	if err != nil {
		return err
	}

	// Event bus: intra-service topics (the former Catalog→Resources
	// projections) dispatch in-process; the saga commands to the Executor stay
	// external (the only Kafka traffic this service produces).
	routes := map[string][]bus.HandlerFunc{}
	pub := bus.NewPublisher(os.Getenv("KAFKA_BROKERS"), log, bus.NewInProc(log, routes),
		map[string]bool{
			events.TopicTaskRunRequested:    true,
			events.TopicTaskReviewRequested: true,
			events.TopicTaskStopRequested:   true,
			events.TopicPrOpenRequested:     true,
		})

	// Application handlers for the four merged planes. The saga coordinator
	// shares the workspace pool, so project/skill data is a local read away.
	projectApp := projectapp.New(&projectapp.Repository{Projects: st.Projects}, log)
	taskApp := taskapp.New(&taskapp.Repository{Tasks: st.Tasks, Feedback: st.Feedback}, pub, log)
	catalogApp := catalogapp.New(&catalogapp.Repository{Skills: st.Skills, Mcps: st.Mcps},
		repository.NewCatalogUnitOfWork(st), pub, log)
	resourcesApp := resourcesapp.New(&resourcesapp.Repository{
		Knowledge: st.Knowledge, Plugins: st.Plugins, Rules: st.Rules, Mcp: st.Mcp,
	}, repository.NewResourcesUnitOfWork(st), log)

	routes[events.TopicMcpCreated] = []bus.HandlerFunc{forward(resourcesApp.ProjectMcpCreated)}
	routes[events.TopicMcpDeleted] = []bus.HandlerFunc{forward(resourcesApp.ProjectMcpDeleted)}

	projecthttp.New(projectApp, log).Register(mux)
	taskhttp.New(taskApp, log).Register(mux)
	cataloghttp.New(catalogApp, log).Register(mux)
	resourceshttp.New(resourcesApp, log).Register(mux)

	messaging.New(log,
		messaging.DispatchHandler{App: taskApp},
	).Start(ctx, os.Getenv("KAFKA_BROKERS"))

	// Workspace provisioning: the Identity service calls the internal endpoint
	// below instead of the former workspace.created Kafka event (repo binding +
	// default rule seeding, both idempotent).
	provisionhttp.New(projectApp, resourcesApp, log).Register(mux)

	log.Info("workspace routes registered", "endpoints", 40, "saga_enabled", pub.Enabled())
	return nil
}

// forward adapts a typed application handler to a bus.HandlerFunc via the
// shared envelope decoder.
func forward[T any](fn func(context.Context, T) error) bus.HandlerFunc {
	return func(ctx context.Context, msg events.EventEnvelope) error {
		return events.Forward(ctx, msg, fn)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
