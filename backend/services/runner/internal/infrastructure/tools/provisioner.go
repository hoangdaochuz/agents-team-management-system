// Package tools implements the application ToolProvisioner port: it assembles
// the per-run ToolSet (sandbox worktree/container + MCP bridge) from the
// preserved sandbox/mcp leaf packages.
package tools

import (
	"context"
	"log/slog"
	"strings"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
	"github.com/aaks/server/services/runner/internal/driver"
	"github.com/aaks/server/services/runner/internal/mcp"
	"github.com/aaks/server/services/runner/internal/sandbox"
)

// Provisioner satisfies application.ToolProvisioner.
type Provisioner struct {
	sandbox    *sandbox.Manager
	fetchMcpFn func(ctx context.Context, agentID identity.ID) []resources.McpServer
	log        *slog.Logger
}

// New builds the provisioner over the sandbox manager.
func New(sb *sandbox.Manager, log *slog.Logger) *Provisioner {
	return &Provisioner{sandbox: sb, log: log}
}

// WithMcpFetcher attaches the agent MCP fetch function (injected by the
// composition root; returns empty when unconfigured).
func (p *Provisioner) WithMcpFetcher(fn func(ctx context.Context, agentID identity.ID) []resources.McpServer) *Provisioner {
	p.fetchMcpFn = fn
	return p
}

// SetupTools prepares the per-run ToolSet: a sandbox (worktree + container or
// host fallback) for built-in tools, plus an MCP bridge over the agent's
// attached servers. The returned cleanup tears both down; it is nil when
// nothing was set up. Failures are non-fatal: a run proceeds with stubbed
// tools.
func (p *Provisioner) SetupTools(ctx context.Context, taskID, agentID, workspaceID identity.ID, prompt string) (driver.ToolSet, func()) {
	var servers []resources.McpServer
	if p.fetchMcpFn != nil {
		servers = p.fetchMcpFn(ctx, agentID)
	}
	return p.setup(ctx, taskID, prompt, servers)
}

// setup builds the ToolSet from the sandbox manager and MCP bridge.
func (p *Provisioner) setup(ctx context.Context, taskID identity.ID, prompt string, mcpServers []resources.McpServer) (driver.ToolSet, func()) {
	var (
		ts     driver.ToolSet
		cleans []func()
	)
	cleanup := func() {
		for _, fn := range cleans {
			fn()
		}
	}

	if p.sandbox != nil && p.sandbox.Enabled() {
		env, err := p.sandbox.Setup(ctx, taskID, promptSlug(prompt))
		if err != nil {
			p.log.Warn("sandbox setup failed; tools stubbed", "task", taskID, "error", err)
		} else if env != nil {
			ts.Exec = sandboxExec{env: env}
			cleans = append(cleans, func() {
				if err := env.Close(context.Background()); err != nil {
					p.log.Warn("sandbox close failed", "task", taskID, "error", err)
				}
			})
		}
	}

	if len(mcpServers) > 0 {
		bridge, err := mcp.New(ctx, mcpServers, p.log)
		if err != nil {
			p.log.Warn("mcp bridge failed; mcp tools unavailable", "task", taskID, "error", err)
		} else if bridge != nil && len(bridge.Tools()) > 0 {
			ts.MCP = bridge
			cleans = append(cleans, func() {
				_ = bridge.Close(context.Background())
			})
		}
	}

	if ts.Exec == nil && ts.MCP == nil {
		return driver.ToolSet{}, nil
	}
	return ts, cleanup
}

// sandboxExec adapts a sandbox.Env to the driver.ToolExec seam. File ops hit
// the host worktree; Run delegates to the sandbox (container exec or host
// fallback).
type sandboxExec struct{ env sandbox.Env }

func (s sandboxExec) Run(ctx context.Context, cmd string, args ...string) (driver.ToolExecResult, error) {
	r, err := s.env.Exec(ctx, cmd, args...)
	if err != nil {
		return driver.ToolExecResult{}, err
	}
	return driver.ToolExecResult{ExitCode: r.ExitCode, Stdout: r.Stdout, Stderr: r.Stderr}, nil
}
func (s sandboxExec) ReadFile(ctx context.Context, path string) (string, error) {
	return s.env.ReadFile(ctx, path)
}
func (s sandboxExec) WriteFile(ctx context.Context, path, content string) error {
	return s.env.WriteFile(ctx, path, content)
}
func (s sandboxExec) ListFiles(ctx context.Context, path string) ([]string, error) {
	return s.env.ListFiles(ctx, path)
}

// promptSlug derives a branch-safe slug for the worktree from the task prompt
// (first line, first few words); falls back to a stable token.
func promptSlug(prompt string) string {
	first := strings.TrimSpace(prompt)
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	words := strings.Fields(first)
	if len(words) > 6 {
		words = words[:6]
	}
	slug := strings.Join(words, "-")
	if slug == "" {
		slug = "task"
	}
	return sandbox.Slug(slug)
}