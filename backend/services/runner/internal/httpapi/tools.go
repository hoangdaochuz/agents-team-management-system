package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aaks/server/internal/contracts"
	"github.com/aaks/server/services/runner/internal/driver"
	"github.com/aaks/server/services/runner/internal/mcp"
	"github.com/aaks/server/services/runner/internal/sandbox"
)

// newMcpBridge constructs the MCP bridge over attached servers (task 5.5). It
// implements driver.McpBridge. Returns nil on failure; callers then skip MCP.
func newMcpBridge(ctx context.Context, servers []contracts.McpServer, log *slog.Logger) (*mcp.Bridge, error) {
	return mcp.New(ctx, servers, log)
}

// sandboxExec adapts a sandbox.Env to the driver.ToolExec seam. File ops hit the
// host worktree; Run delegates to the sandbox (container exec or host fallback).
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

// setupTools prepares the per-run ToolSet: a sandbox (worktree + container or
// host fallback) for built-in tools, plus an MCP bridge over the agent's
// attached MCP servers. The returned cleanup tears both down; it is nil when
// nothing was set up. Failures are non-fatal: a run proceeds with stubbed tools.
func (a *App) setupTools(ctx context.Context, taskID contracts.ID, prompt string, agentID contracts.ID, mcpServers []contracts.McpServer) (driver.ToolSet, func()) {
	var (
		ts     driver.ToolSet
		cleans []func()
	)
	cleanup := func() {
		for _, fn := range cleans {
			fn()
		}
	}

	// Sandbox (worktree + container / host fallback). Only when a manager is
	// configured and enabled.
	if a.sandbox != nil && a.sandbox.Enabled() {
		env, err := a.sandbox.Setup(ctx, taskID, promptSlug(prompt))
		if err != nil {
			a.log.Warn("sandbox setup failed; tools stubbed", "task", taskID, "error", err)
		} else if env != nil {
			ts.Exec = sandboxExec{env: env}
			cleans = append(cleans, func() {
				if err := env.Close(context.Background()); err != nil {
					a.log.Warn("sandbox close failed", "task", taskID, "error", err)
				}
			})
		}
	}

	// MCP bridge over attached servers (task 5.5).
	if len(mcpServers) > 0 {
		bridge, err := newMcpBridge(ctx, mcpServers, a.log)
		if err != nil {
			a.log.Warn("mcp bridge failed; mcp tools unavailable", "task", taskID, "error", err)
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

// setupToolsForRun is the run-path wrapper around setupTools: it fetches the
// agent's attached MCP servers from Catalog (best-effort), then builds the
// per-run ToolSet (sandbox + MCP bridge). Failures degrade to stubbed tools.
func (a *App) setupToolsForRun(ctx context.Context, taskID contracts.ID, prompt string, agentID, workspaceID contracts.ID) (driver.ToolSet, func()) {
	mcpServers := a.fetchMcpServers(agentID, workspaceID)
	return a.setupTools(ctx, taskID, prompt, agentID, mcpServers)
}

// fetchMcpServers pulls the agent's attached MCP server definitions from the
// Agent service (which hydrates them from Catalog). Non-fatal: empty list when
// unconfigured or unavailable.
func (a *App) fetchMcpServers(agentID, workspaceID contracts.ID) []contracts.McpServer {
	if a.agentURL == "" || agentID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimSuffix(a.agentURL, "/")+"/internal/agents/"+string(agentID)+"/mcp-servers", nil)
	if err != nil {
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		a.log.Warn("mcp servers fetch failed", "agent", agentID, "error", err)
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		a.log.Warn("mcp servers fetch non-200", "agent", agentID, "status", resp.Status)
		return nil
	}
	var out []contracts.McpServer
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		a.log.Warn("mcp servers decode failed", "error", err)
		return nil
	}
	return out
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
