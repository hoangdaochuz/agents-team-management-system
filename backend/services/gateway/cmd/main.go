// Package main is the API Gateway / BFF entrypoint. The gateway is the
// frontend's sole HTTP entrypoint: it routes the full /api surface, composes
// cross-service reads via synchronous fan-out, and serves the SSE stream.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/platform/svcrun"
	"github.com/aaks/server/services/gateway/internal/application"
	"github.com/aaks/server/services/gateway/internal/infrastructure/acl"
	kafkaclient "github.com/aaks/server/services/gateway/internal/infrastructure/kafka"
	"github.com/aaks/server/services/gateway/internal/infrastructure/proxy"
	interfacehttp "github.com/aaks/server/services/gateway/internal/interfaces/http"
)

func main() {
	svcrun.Run("gateway", getenv("HTTP_ADDR", ":8080"), register)
}

// register is the explicit composition root (D5): env config → upstream
// proxies + inter-service clients → application handlers → HTTP adapter.
func register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
	ups, err := readUpstreams()
	if err != nil {
		return err
	}

	proxies := make(map[application.Upstream]*httputil.ReverseProxy, len(ups))
	for name, url := range ups {
		rp, err := proxy.New(url)
		if err != nil {
			return fmt.Errorf("upstream %s: %w", name, err)
		}
		proxies[name] = rp
	}

	brokers := os.Getenv("KAFKA_BROKERS")
	app := application.New(
		application.NewACL(
			acl.NewAuthClient(ups[application.UpstreamAuth], interfacehttp.SessionCookie, log),
			acl.NewOrgsClient(ups[application.UpstreamOrgs], log),
			acl.NewTaskClient(ups[application.UpstreamTask], log),
			log,
		),
		application.NewStream(
			acl.NewStepsClient(ups[application.UpstreamRunner], log),
			kafkaTailerFactory(brokers, log),
			log,
		),
		application.NewRouteTable(),
		acl.NewStatsClient(
			ups[application.UpstreamAgent],
			ups[application.UpstreamTask],
			ups[application.UpstreamOrgs],
			ups[application.UpstreamAuth],
			log,
		),
	)

	interfacehttp.New(app, proxies, ups, brokers, log).Register(mux)
	return nil
}

// readUpstreams reads the UPSTREAM_* env vars (each an absolute URL like
// http://project:8081), failing fast when any is unset.
func readUpstreams() (map[application.Upstream]string, error) {
	ups := map[application.Upstream]string{
		application.UpstreamProject:   os.Getenv("UPSTREAM_PROJECT"),
		application.UpstreamTask:      os.Getenv("UPSTREAM_TASK"),
		application.UpstreamAgent:     os.Getenv("UPSTREAM_AGENT"),
		application.UpstreamCatalog:   os.Getenv("UPSTREAM_CATALOG"),
		application.UpstreamSettings:  os.Getenv("UPSTREAM_SETTINGS"),
		application.UpstreamRunner:    os.Getenv("UPSTREAM_RUNNER"),
		application.UpstreamAuth:      os.Getenv("UPSTREAM_AUTH"),
		application.UpstreamOrgs:      os.Getenv("UPSTREAM_ORGS"),
		application.UpstreamResources: os.Getenv("UPSTREAM_RESOURCES"),
		application.UpstreamAdmin:     os.Getenv("UPSTREAM_ADMIN"),
	}
	for name, url := range ups {
		if url == "" {
			return nil, errors.New("UPSTREAM_" + strings.ToUpper(string(name)) + " is not set")
		}
	}
	return ups, nil
}

// kafkaTailerFactory wires the per-connection step tailer (fresh consumer
// group per SSE connection, reading from the newest offset).
func kafkaTailerFactory(brokers string, log *slog.Logger) application.TailerFactory {
	return func(taskID identity.ID) (application.StepTailer, error) {
		return kafkaclient.NewStepTailer(strings.Split(brokers, ","), taskID, log)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
