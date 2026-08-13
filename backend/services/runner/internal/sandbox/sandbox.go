// Package sandbox is the per-task execution substrate for the Runner.
//
// Each task moving to execution gets its own git worktree (branch
// `agent/<task-id>-<slug>`) bind-mounted read-write into a credential-less
// sandbox container at /workspace. The Runner drives build/test/edit commands
// in the container via the Docker exec API; file read/write/list ops hit the
// worktree on the host (the bind mount means backend and container see the same
// files — see design.md §3.4). Concurrent doing tasks use disjoint worktrees.
//
// SECURITY: the container holds NO API keys and NO git credentials. Only
// run_command crosses into the container; the agent loop, LLM provider calls,
// MCP client, and all git operations run on the host.
//
// The sandbox is opt-in (RUNNER_SANDBOX=docker|local). When unset or "none",
// Setup returns a nil Env and the Runner falls back to its stub tools — so the
// default simulated driver and CI (no Docker) are unaffected.
package sandbox

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aaks/server/internal/contracts"
)

// ExecResult is the outcome of a sandbox command.
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// Env is one task's execution environment: a worktree plus a credential-less
// sandbox that runs commands. File ops (read/write/list) act on the worktree
// directly; Exec runs a command in the sandbox (container or host fallback).
type Env interface {
	// Exec runs a command in the sandbox worktree. For the Docker driver this
	// is the container exec API against /workspace; for the local fallback it
	// runs in the worktree directory on the host.
	Exec(ctx context.Context, cmd string, args ...string) (ExecResult, error)
	// ReadFile reads a path relative to the worktree root (host bind mount).
	ReadFile(ctx context.Context, path string) (string, error)
	// WriteFile writes a path relative to the worktree root (host bind mount).
	WriteFile(ctx context.Context, path, content string) error
	// ListFiles lists entries under a path relative to the worktree root.
	ListFiles(ctx context.Context, path string) ([]string, error)
	// WorktreeBranch is the agent branch this env is bound to.
	WorktreeBranch() string
	// Close tears down the container (Docker driver) and removes the worktree.
	Close(ctx context.Context) error
}

// Config selects and parameterizes the sandbox driver.
type Config struct {
	// Kind: "docker" (Docker exec API), "local" (host fallback), or "" / "none"
	// (disabled — Setup returns nil).
	Kind string
	// Image is the credential-less sandbox image (Docker driver). Default
	// aaks/runner-sandbox.
	Image string
	// Socket is the Docker daemon unix socket path. Default /var/run/docker.sock.
	Socket string
	// CloneRoot is the managed git clone worktrees are created under.
	CloneRoot string
}

// Manager builds per-task Env instances.
type Manager struct {
	cfg Config
	log *slog.Logger
}

// New returns a sandbox Manager. A Config with an empty/"none" Kind still
// returns a Manager; Setup then returns (nil, nil) so callers can stay
// unconditional.
func New(cfg Config, log *slog.Logger) *Manager {
	if cfg.Image == "" {
		cfg.Image = "aaks-runner:latest"
	}
	if cfg.Socket == "" {
		cfg.Socket = "/var/run/docker.sock"
	}
	return &Manager{cfg: cfg, log: log}
}

// Enabled reports whether a real sandbox driver is configured.
func (m *Manager) Enabled() bool {
	switch m.cfg.Kind {
	case "docker", "local":
		return true
	default:
		return false
	}
}

// Setup prepares a per-task execution environment. Returns (nil, nil) when the
// sandbox is disabled so callers can branch once on the result.
func (m *Manager) Setup(ctx context.Context, taskID contracts.ID, slug string) (Env, error) {
	if !m.Enabled() {
		return nil, nil
	}
	if m.cfg.CloneRoot == "" {
		return nil, fmt.Errorf("sandbox: clone root not configured")
	}
	wt, err := addWorktree(ctx, m.cfg.CloneRoot, string(taskID), slug)
	if err != nil {
		return nil, err
	}
	switch m.cfg.Kind {
	case "docker":
		env, err := newDockerEnv(ctx, m.cfg, wt, m.log)
		if err != nil {
			// Best-effort worktree cleanup so a failed container setup does
			// not strand a branch/worktree.
			_ = removeWorktree(ctx, m.cfg.CloneRoot, string(taskID))
			return nil, err
		}
		return env, nil
	case "local":
		return &localEnv{wt: wt, log: m.log}, nil
	default:
		return nil, nil
	}
}
