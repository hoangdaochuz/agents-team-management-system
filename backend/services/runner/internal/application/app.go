// Package application holds the Runner orchestration (run/review/PR flows),
// query handlers and the command dispatcher. It depends only on domain ports,
// the driver port, and the abstractions declared here (DIP: no sarama, no
// pgx, no http.DefaultClient, no net/http).
package application

import (
	"context"
	"errors"
	"log/slog"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
	"github.com/aaks/server/services/runner/internal/driver"
	"github.com/aaks/server/services/runner/internal/domain"
)

// ErrNotConfigured is returned by ACL clients whose upstream is unset; the
// orchestration treats it as a silent no-op (parity with the pre-refactor
// behavior where unconfigured URLs were simply not queried).
var ErrNotConfigured = errors.New("client not configured")

// EventPublisher publishes execution facts to the bus (DIP).
type EventPublisher interface {
	Publish(ctx context.Context, topic string, data any, key identity.ID)
}

// SettingsKeyClient fetches provider keys from Settings (Anti-Corruption Layer
// port: the plaintext key never leaves the process).
type SettingsKeyClient interface {
	FetchKey(ctx context.Context, provider string) (string, error)
}

// ResourcesRulesClient fetches the workspace's enabled guardrail rules.
type ResourcesRulesClient interface {
	FetchEnabledRules(ctx context.Context, workspaceID identity.ID) ([]string, error)
}

// AgentMcpClient fetches the agent's attached MCP server definitions.
type AgentMcpClient interface {
	FetchMcpServers(ctx context.Context, agentID identity.ID) ([]resources.McpServer, error)
}

// ToolProvisioner assembles the per-run tool set (sandbox worktree/container
// and MCP bridge). The implementation lives in infrastructure; application
// only knows the seam.
type ToolProvisioner interface {
	SetupTools(ctx context.Context, taskID, agentID, workspaceID identity.ID, prompt string) (driver.ToolSet, func())
}

// Runner is the Runner application service: run/review orchestration, the PR
// flow, cancellation, and the query surface.
type Runner struct {
	runs      domain.RunRepository
	steps     domain.StepRepository
	findings  domain.FindingRepository
	artifacts domain.ArtifactRepository

	driver    driver.Driver
	caps      driver.Caps
	settings  SettingsKeyClient
	resources ResourcesRulesClient
	agents    AgentMcpClient
	tools     ToolProvisioner
	pub       EventPublisher
	log       *slog.Logger
	prBaseURL string

	// cancels cancels in-flight runs per task (stop command); running guards
	// against concurrent runs for the same task.
	cancels syncMap
	running syncMap
}

// New builds the Runner application service with its injected dependencies.
func New(runs domain.RunRepository, steps domain.StepRepository, findings domain.FindingRepository, artifacts domain.ArtifactRepository, d driver.Driver, caps driver.Caps, settings SettingsKeyClient, resources ResourcesRulesClient, agents AgentMcpClient, tools ToolProvisioner, pub EventPublisher, log *slog.Logger, prBaseURL string) *Runner {
	return &Runner{
		runs: runs, steps: steps, findings: findings, artifacts: artifacts,
		driver: d, caps: caps, settings: settings, resources: resources, agents: agents,
		tools: tools, pub: pub, log: log, prBaseURL: prBaseURL,
	}
}