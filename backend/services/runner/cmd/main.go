// Package main is the Agent-Runner service entrypoint: run/step/finding/
// artifact query endpoints plus the command consumers that drive runs.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
	"github.com/aaks/server/internal/platform/svcrun"
	"github.com/aaks/server/services/runner/internal/application"
	"github.com/aaks/server/services/runner/internal/driver"
	"github.com/aaks/server/services/runner/internal/infrastructure/acl"
	"github.com/aaks/server/services/runner/internal/infrastructure/bus"
	"github.com/aaks/server/services/runner/internal/infrastructure/repository"
	"github.com/aaks/server/services/runner/internal/infrastructure/tools"
	interfacehttp "github.com/aaks/server/services/runner/internal/interfaces/http"
	"github.com/aaks/server/services/runner/internal/interfaces/messaging"
	"github.com/aaks/server/services/runner/internal/sandbox"
)

func main() {
	svcrun.Run("runner", getenv("HTTP_ADDR", ":8086"), register)
}

// register is the explicit composition root (D5): config → platform deps →
// repositories/publisher/ACL clients/tool provisioner → application handlers →
// HTTP/Kafka adapters. The lifecycle ctx is threaded into the Kafka consumer
// so SIGTERM drains in-flight commands.
func register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("RUNNER_DB_DSN")
	if dsn == "" {
		return errors.New("RUNNER_DB_DSN is not set")
	}
	st, err := repository.New(ctx, dsn, log)
	if err != nil {
		return err
	}

	settings := acl.NewSettingsClient(os.Getenv("SETTINGS_URL"), os.Getenv("SETTINGS_INTERNAL_TOKEN"))
	resClient := acl.NewResourcesClient(os.Getenv("RESOURCES_URL"))
	agents := acl.NewAgentClient(os.Getenv("AGENT_URL"), log)

	prov := tools.New(sandbox.New(sandbox.Config{
		Kind:      os.Getenv("RUNNER_SANDBOX"),
		Image:     os.Getenv("RUNNER_SANDBOX_IMAGE"),
		Socket:    os.Getenv("RUNNER_DOCKER_SOCKET"),
		CloneRoot: os.Getenv("RUNNER_CLONE_ROOT"),
	}, log), log).WithMcpFetcher(func(ctx context.Context, agentID identity.ID) []resources.McpServer {
		servers, err := agents.FetchMcpServers(ctx, agentID)
		if err != nil {
			return nil
		}
		return servers
	})

	pub := bus.NewPublisher(os.Getenv("KAFKA_BROKERS"), log)
	app := application.New(
		st.Runs, st.Steps, st.Findings, st.Artifacts,
		driver.New(os.Getenv("RUNNER_DRIVER"), log),
		driver.Caps{
			MaxSteps:  envInt("RUNNER_MAX_STEPS", 50),
			MaxTokens: envInt("RUNNER_MAX_TOKENS", 100_000),
			WallClock: time.Duration(envInt("RUNNER_WALL_CLOCK_MIN", 30)) * time.Minute,
			StepDelay: 0,
		},
		settings, resClient, agents, prov, pub, log,
		os.Getenv("RUNNER_PR_BASE_URL"),
	)

	interfacehttp.New(app, log).Register(mux)
	messaging.New(app, log).Start(ctx, os.Getenv("KAFKA_BROKERS"))

	log.Info("runner routes registered", "endpoints", 4, "driver", os.Getenv("RUNNER_DRIVER"))
	return nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}
