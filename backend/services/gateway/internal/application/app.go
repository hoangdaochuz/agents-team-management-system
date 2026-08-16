package application

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
)

// OrgStats is the sysadmin KPI fan-out from the Orgs service.
type OrgStats struct {
	Organizations int
	Workspaces    int
	OpenSeats     int
}

// StatsClient composes cross-service reads for the gateway's fan-out
// endpoints (workspace list enrichment + sysadmin KPIs).
type StatsClient interface {
	AgentCount(ctx context.Context, workspaceID identity.ID) (int, error)
	OpenTaskCount(ctx context.Context, workspaceID identity.ID) (int, error)
	OrgStats(ctx context.Context) (OrgStats, error)
	ActiveUsers24h(ctx context.Context) (int, error)
}

// App is the Gateway application service: route resolution, the session
// identity/membership ACL, and the SSE stream orchestration. It depends only
// on the focused ports declared in this package (DIP); the composition root
// injects the concrete infrastructure clients.
type App struct {
	Routes *RouteTable
	ACL    *ACL
	Stream *Stream
	Stats  StatsClient
}

// New builds the application service with its injected dependencies.
func New(acl *ACL, stream *Stream, routes *RouteTable, stats StatsClient) *App {
	return &App{Routes: routes, ACL: acl, Stream: stream, Stats: stats}
}
