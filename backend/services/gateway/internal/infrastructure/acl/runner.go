package acl

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/gateway/internal/application"
)

// StepsClient fetches a task's persisted steps from the Runner for the SSE
// replay.
type StepsClient struct {
	url           string
	internalToken string
	hc            *http.Client
	log           *slog.Logger
}

// NewStepsClient builds the Runner step-replay client.
func NewStepsClient(url, internalToken string, log *slog.Logger) *StepsClient {
	return &StepsClient{
		url: strings.TrimSuffix(url, "/"), internalToken: internalToken,
		hc: &http.Client{Timeout: 5 * time.Second}, log: log,
	}
}

// Steps implements application.StepsClient.
func (c *StepsClient) Steps(ctx context.Context, taskID identity.ID) ([]agentexec.Step, error) {
	var steps []agentexec.Step
	err := doGet(c.hc, c.log, ctx, c.url+"/internal/tasks/"+string(taskID)+"/steps", internalHeaders(c.internalToken), &steps)
	return steps, err
}

// StatsClient composes the cross-service reads behind the gateway's fan-out
// endpoints: workspace counts (Agent + Workspace) and sysadmin KPIs
// (Identity: stats + active users).
type StatsClient struct {
	agentURL      string
	workspaceURL  string
	identityURL   string
	internalToken string
	hc            *http.Client
	log           *slog.Logger
}

// NewStatsClient builds the stats fan-out client over the three distinct
// upstreams it actually calls.
func NewStatsClient(agentURL, workspaceURL, identityURL, internalToken string, log *slog.Logger) *StatsClient {
	return &StatsClient{
		agentURL:      strings.TrimSuffix(agentURL, "/"),
		workspaceURL:  strings.TrimSuffix(workspaceURL, "/"),
		identityURL:   strings.TrimSuffix(identityURL, "/"),
		internalToken: internalToken,
		hc:            &http.Client{Timeout: 5 * time.Second},
		log:           log,
	}
}

// AgentCount returns the agent count for a workspace (Agent service).
func (c *StatsClient) AgentCount(ctx context.Context, workspaceID identity.ID) (int, error) {
	var res struct {
		AgentCount int `json:"agent_count"`
	}
	if err := doGet(c.hc, c.log, ctx, c.agentURL+"/internal/workspace/"+string(workspaceID)+"/agent-count", internalHeaders(c.internalToken), &res); err != nil {
		return 0, err
	}
	return res.AgentCount, nil
}

// OpenTaskCount returns the open task count for a workspace (Task service).
func (c *StatsClient) OpenTaskCount(ctx context.Context, workspaceID identity.ID) (int, error) {
	var res struct {
		OpenTaskCount int `json:"open_task_count"`
	}
	if err := doGet(c.hc, c.log, ctx, c.workspaceURL+"/internal/workspace/"+string(workspaceID)+"/open-task-count", internalHeaders(c.internalToken), &res); err != nil {
		return 0, err
	}
	return res.OpenTaskCount, nil
}

// OrgStats returns the org/workspace/seat KPIs (Orgs service).
func (c *StatsClient) OrgStats(ctx context.Context) (application.OrgStats, error) {
	var res struct {
		Organizations int `json:"organizations"`
		Workspaces    int `json:"workspaces"`
		OpenSeats     int `json:"open_seats"`
	}
	if err := doGet(c.hc, c.log, ctx, c.identityURL+"/internal/stats", internalHeaders(c.internalToken), &res); err != nil {
		return application.OrgStats{}, err
	}
	return application.OrgStats{Organizations: res.Organizations, Workspaces: res.Workspaces, OpenSeats: res.OpenSeats}, nil
}

// ActiveUsers24h returns the active-user KPI (Auth service).
func (c *StatsClient) ActiveUsers24h(ctx context.Context) (int, error) {
	var res struct {
		ActiveUsers24h int `json:"active_users_24h"`
	}
	if err := doGet(c.hc, c.log, ctx, c.identityURL+"/internal/active-users-24h", internalHeaders(c.internalToken), &res); err != nil {
		return 0, err
	}
	return res.ActiveUsers24h, nil
}
