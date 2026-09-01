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
	"github.com/aaks/server/internal/platform/internaltoken"
	"github.com/aaks/server/internal/platform/svcrun"
	"github.com/aaks/server/services/executor/internal/application"
	"github.com/aaks/server/services/executor/internal/driver"
	"github.com/aaks/server/services/executor/internal/infrastructure/acl"
	"github.com/aaks/server/services/executor/internal/infrastructure/bus"
	"github.com/aaks/server/services/executor/internal/infrastructure/repository"
	"github.com/aaks/server/services/executor/internal/infrastructure/tools"
	interfacehttp "github.com/aaks/server/services/executor/internal/interfaces/http"
	"github.com/aaks/server/services/executor/internal/interfaces/messaging"
	"github.com/aaks/server/services/executor/internal/sandbox"
)

func main() {
	svcrun.Run("executor", getenv("HTTP_ADDR", ":8086"), register)
}

// register is the explicit composition root (D5): config → platform deps →
// repositories/publisher/ACL clients/tool provisioner → application handlers →
// HTTP/Kafka adapters. The lifecycle ctx is threaded into the Kafka consumer
// so SIGTERM drains in-flight commands.
func register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("EXECUTOR_DB_DSN")
	if dsn == "" {
		return errors.New("EXECUTOR_DB_DSN is not set")
	}
	st, err := repository.New(ctx, dsn, log)
	if err != nil {
		return err
	}

	// Agent config and decrypted provider keys both live in the consolidated
	// Agent service now: one URL serves both clients. The shared internal
	// token authenticates every /internal/* call to the peer services.
	agentURL := os.Getenv("AGENT_URL")
	internalToken := os.Getenv("INTERNAL_TOKEN")
	keys := acl.NewKeyClient(agentURL, os.Getenv("AGENT_INTERNAL_TOKEN"), internalToken)
	resClient := acl.NewResourcesClient(os.Getenv("WORKSPACE_URL"), internalToken)
	agents := acl.NewAgentClient(agentURL, internalToken, log)

	prov := tools.New(sandbox.New(sandbox.Config{
		Kind:        os.Getenv("EXECUTOR_SANDBOX"),
		Image:       os.Getenv("EXECUTOR_SANDBOX_IMAGE"),
		Socket:      os.Getenv("EXECUTOR_DOCKER_SOCKET"),
		CloneRoot:   os.Getenv("EXECUTOR_CLONE_ROOT"),
		NetworkMode: os.Getenv("EXECUTOR_SANDBOX_NETWORK"),
	}, log), log).WithMcpFetcher(func(ctx context.Context, agentID identity.ID) []resources.McpServer {
		servers, err := agents.FetchMcpServers(ctx, agentID)
		if err != nil {
			return nil
		}
		return servers
	})

	pub := bus.NewPublisher(ctx, os.Getenv("KAFKA_BROKERS"), log)
	app := application.New(
		st.Runs, st.Steps, st.Findings, st.Artifacts,
		driver.New(os.Getenv("EXECUTOR_DRIVER"), log),
		driver.Caps{
			MaxSteps:  envInt("EXECUTOR_MAX_STEPS", 50),
			MaxTokens: envInt("EXECUTOR_MAX_TOKENS", 100_000),
			WallClock: time.Duration(envInt("EXECUTOR_WALL_CLOCK_MIN", 30)) * time.Minute,
			StepDelay: 0,
		},
		keys, resClient, agents, prov, pub, log,
		os.Getenv("EXECUTOR_PR_BASE_URL"),
	)

	inner := http.NewServeMux()
	interfacehttp.New(app, log).Register(inner)
	mux.Handle("/", internaltoken.Wrap(internalToken, inner))
	messaging.New(log, messaging.DispatchHandler{App: app}).Start(ctx, os.Getenv("KAFKA_BROKERS"))

	log.Info("executor routes registered", "endpoints", 4, "driver", os.Getenv("EXECUTOR_DRIVER"))
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
