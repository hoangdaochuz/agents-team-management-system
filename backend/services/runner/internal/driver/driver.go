// Package driver executes agent runs. The Runner is provider-agnostic: a
// Driver produces steps through a StepSink and returns a Result. The
// Simulated driver is the default (deterministic, zero-infra, used by dev +
// E2E); the LLM driver is selected with RUNNER_DRIVER=llm.
package driver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/identity"
)

// Caps bound a single run (task 5.3): hard step cap, token budget, wall clock.
type Caps struct {
	MaxSteps  int           // default 50
	MaxTokens int           // default 100_000
	WallClock time.Duration // default 30 min
	StepDelay time.Duration // simulated driver pacing (0 = as fast as possible)
}

// RunContext carries everything a driver needs for one run.
type RunContext struct {
	TaskID   identity.ID
	RunID    identity.ID
	AgentID  identity.ID
	Role     agentexec.RunRole
	RoundNo  int
	Prompt   string
	Model    string
	Provider identity.Provider
	APIKey   string   // fetched from Settings over mTLS at run start; in-memory only
	Rules    []string // enabled workspace rules (task 12.3 guardrails)
	Caps     Caps
	Log      *slog.Logger
	// Tools is the sandbox backend for built-in tools (run_command/read_file/
	// write_file/list_files) and bridged MCP tools. When nil the driver stubs
	// tool calls (dev/CI without a sandbox).
	Tools ToolSet
}

// ToolExecResult is the outcome of a sandbox run_command.
type ToolExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// ToolExec is the credential-less sandbox backend for built-in tools. File ops
// hit the host worktree (bind-mounted RW into the container); Run executes a
// command in the sandbox via the container exec API. This is an adapter seam —
// the driver does not depend on the sandbox package directly.
type ToolExec interface {
	Run(ctx context.Context, cmd string, args ...string) (ToolExecResult, error)
	ReadFile(ctx context.Context, path string) (string, error)
	WriteFile(ctx context.Context, path, content string) error
	ListFiles(ctx context.Context, path string) ([]string, error)
}

// McpTool is a tool bridged from an attached MCP server.
type McpTool struct {
	Server      string // owning MCP server name (namespacing)
	Name        string
	Description string
	InputSchema json.RawMessage // JSON schema for the tool's arguments
}

// McpBridge calls tools on attached MCP servers.
type McpBridge interface {
	Tools() []McpTool
	Call(ctx context.Context, server, name string, args json.RawMessage) (string, error)
	Close(ctx context.Context) error
}

// ToolSet bundles the built-in sandbox backend and the MCP bridge. Either may be
// nil (stubbed). It is constructed by the Runner per run.
type ToolSet struct {
	Exec ToolExec
	MCP  McpBridge
}

// Result is the outcome of a driver execution.
type Result struct {
	Status         agentexec.RunStatus // done | aborted | stopped
	TokenUsage     int
	Error          string
	Verdict        agentexec.VerdictDecision // reviewer runs
	VerdictSummary string
	Findings       []agentexec.Finding
	Artifacts      []ArtifactRef
}

// ArtifactRef is a produced artifact (patch/document).
type ArtifactRef struct {
	Filename  string
	Kind      string
	Summary   string
	Additions int
	Deletions int
}

// StepSink receives steps as they are produced (persist + publish downstream).
type StepSink func(st agentexec.Step) error

// Driver executes one run.
type Driver interface {
	Execute(ctx context.Context, rc RunContext, sink StepSink) (Result, error)
}

// New returns the configured driver (RUNNER_DRIVER: simulated | llm). An
// unset (or unknown) value defaults to "llm": a silent no-op simulated driver
// in production is worse than a hard failure, so the real driver is the safe
// default — dev/E2E must set RUNNER_DRIVER=simulated explicitly.
func New(driverName string, log *slog.Logger) Driver {
	switch driverName {
	case "simulated":
		return &Simulated{log: log}
	case "llm", "":
		return &LLMDriver{log: log}
	default:
		log.Warn("unknown RUNNER_DRIVER; defaulting to llm", "driver", driverName)
		return &LLMDriver{log: log}
	}
}

// ── Simulated driver ────────────────────────────────────────────────────────

// Simulated is a deterministic driver for dev + E2E. It emits a small step
// sequence, obeys caps and cancellation, and decides:
//
//	implementer: always done (optionally producing a patch artifact)
//	reviewer:    REQUEST_CHANGES when the task prompt contains "[needs-work]",
//	             else APPROVE — with matching findings.
type Simulated struct {
	log *slog.Logger
}

func (d *Simulated) Execute(ctx context.Context, rc RunContext, sink StepSink) (Result, error) {
	log := d.log.With("driver", "simulated", "run", rc.RunID, "role", rc.Role)
	res := Result{Status: agentexec.RunDone}
	seq := 0
	emit := func(kind agentexec.StepKind, payload any) error {
		seq++
		buf, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		st := agentexec.Step{ID: newStepID(), RunID: rc.RunID, Seq: seq, Kind: kind, Payload: buf, CreatedAt: time.Now().UTC()}
		if err := sink(st); err != nil {
			return err
		}
		if rc.Caps.StepDelay > 0 {
			select {
			case <-time.After(rc.Caps.StepDelay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	step := func(kind agentexec.StepKind, text string) error {
		return emit(kind, map[string]string{"text": text})
	}

	log.Info("run started", "task", rc.TaskID, "round", rc.RoundNo, "prompt", truncate(rc.Prompt, 120), "rules", rc.Rules)
	if err := step(agentexec.StepReasoning, "planning approach for task "+rc.TaskID); err != nil {
		return d.aborted(ctx, res, err, log)
	}
	if err := step(agentexec.StepMessage, fmt.Sprintf("I will implement the requested change (%s).", rc.Model)); err != nil {
		return d.aborted(ctx, res, err, log)
	}
	// Guardrails (task 12.3): the workspace's enabled rules shape the run.
	// test-gate requires the implementer to run the test suite before
	// finishing; no-auto-merge / review-before-merge are enforced by the
	// orchestrator (PRs are only ever opened on demand, and the reviewer is
	// the sole path to done).
	if hasRule(rc.Rules, "test-gate") {
		if err := step(agentexec.StepToolCall, `{"tool":"run_tests","input":"./..."}`); err != nil {
			return d.aborted(ctx, res, err, log)
		}
		if err := step(agentexec.StepToolResult, `{"tool":"run_tests","output":"ok: all tests pass"}`); err != nil {
			return d.aborted(ctx, res, err, log)
		}
	}
	if err := step(agentexec.StepToolCall, `{"tool":"read","input":"src/main.go"}`); err != nil {
		return d.aborted(ctx, res, err, log)
	}
	if err := step(agentexec.StepToolResult, `{"tool":"read","output":"ok"}`); err != nil {
		return d.aborted(ctx, res, err, log)
	}
	if err := step(agentexec.StepMessage, "Change applied; tests pass."); err != nil {
		return d.aborted(ctx, res, err, log)
	}

	// Reviewer variant: decide deterministically (task 5.7). Round 1 always
	// requests changes, exercising the saga's review loop; round ≥ 2 approves.
	// (round_no is 0-based: 0 = first review round, 1 = second.)
	if rc.Role == agentexec.RunRoleReviewer {
		if rc.RoundNo < 1 {
			res.Verdict = agentexec.VerdictRequestChanges
			res.VerdictSummary = "tests fail on the new branch; please fix and re-run"
			res.Findings = []agentexec.Finding{{
				File: "src/main_test.go", Line: 42, Severity: "error",
				Issue:          "test suite fails on branch",
				Recommendation: "run go test ./... and fix the failing assertion",
				Status:         "open",
			}}
			if err := step(agentexec.StepMessage, "Reviewer verdict: REQUEST_CHANGES (test failures)."); err != nil {
				return d.aborted(ctx, res, err, log)
			}
		} else {
			res.Verdict = agentexec.VerdictApprove
			res.VerdictSummary = "implementation looks correct; tests pass"
			if err := step(agentexec.StepMessage, "Reviewer verdict: APPROVE."); err != nil {
				return d.aborted(ctx, res, err, log)
			}
		}
	} else {
		// Implementer: attach a synthetic patch artifact (dev mode).
		res.Artifacts = []ArtifactRef{{
			Filename: "changes.patch", Kind: "patch",
			Summary:   "simulated implementation of " + rc.TaskID,
			Additions: 24, Deletions: 3,
		}}
		res.TokenUsage = 1200 + 100*rc.RoundNo
	}
	return res, nil
}

// aborted converts a context/step error into a terminated result.
func (d *Simulated) aborted(ctx context.Context, res Result, err error, log *slog.Logger) (Result, error) {
	if ctx.Err() != nil {
		res.Status = agentexec.RunStopped
		log.Info("run stopped by context")
		return res, nil
	}
	res.Status = agentexec.RunAborted
	res.Error = err.Error()
	log.Warn("run aborted", "error", err)
	return res, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// hasRule reports whether the workspace rules include the named guardrail.
func hasRule(rules []string, name string) bool {
	for _, r := range rules {
		if r == name {
			return true
		}
	}
	return false
}

func newStepID() identity.ID {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "step_" + hex.EncodeToString(b)
}
