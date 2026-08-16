// Package agentexec holds the shared-kernel agent-execution DTOs: Agent,
// Run/Step/Finding/Artifact, and guardrails. Field names mirror the
// frontend's declared API contract (snake_case JSON).
package agentexec

import (
	"encoding/json"

	"github.com/aaks/server/internal/contracts/identity"
)

// ID re-exports the shared identifier for convenience within this domain.
type ID = identity.ID

// ISOTime re-exports the shared timestamp alias.
type ISOTime = identity.ISOTime

// RunRole is a run's role: "implementer" | "reviewer".
type RunRole string

// RunStatus is a run's lifecycle state: "running" | "done" | "aborted" | "stopped".
type RunStatus string

// StepKind is a step payload kind: "message" | "tool_call" | "tool_result" | "reasoning".
type StepKind string

// Severity is a finding severity: "info" | "warning" | "error" | "critical".
type Severity string

// AutonomyMode is an agent's assignment mode: "assigned" | "matching" | "autonomous".
type AutonomyMode string

const (
	RunRoleImplementer = RunRole("implementer")
	RunRoleReviewer    = RunRole("reviewer")
)

const (
	RunRunning = RunStatus("running")
	RunDone    = RunStatus("done")
	RunAborted = RunStatus("aborted")
	RunStopped = RunStatus("stopped")
)

const (
	StepMessage    = StepKind("message")
	StepToolCall   = StepKind("tool_call")
	StepToolResult = StepKind("tool_result")
	StepReasoning  = StepKind("reasoning")
)

const (
	AutonomyAssigned   = AutonomyMode("assigned")
	AutonomyMatching   = AutonomyMode("matching")
	AutonomyAutonomous = AutonomyMode("autonomous")
)

// VerdictDecision is the reviewer's verdict on a run.
type VerdictDecision string

const (
	VerdictApprove        = VerdictDecision("APPROVE")
	VerdictRequestChanges = VerdictDecision("REQUEST_CHANGES")
)

// ── Guardrails (agent builder) ─────────────────────────────────────────────

// Guardrails express an agent's execution policy. Mirrors frontend Guardrails.
type Guardrails struct {
	AutoPauseOnTestFail        *bool `json:"auto_pause_on_test_fail,omitempty"`
	AllowDirectCommits         *bool `json:"allow_direct_commits,omitempty"`
	AllowShellCommands         *bool `json:"allow_shell_commands,omitempty"`
	RequireApprovalBeforeMerge *bool `json:"require_approval_before_merge,omitempty"`
	MaxStepsPerRun             *int  `json:"max_steps_per_run,omitempty"`
	WallClockCapMin            *int  `json:"wall_clock_cap_min,omitempty"`
}

// ── DTOs ───────────────────────────────────────────────────────────────────

// Agent mirrors frontend Agent.
type Agent struct {
	ID                 ID                `json:"id"`
	WorkspaceID        ID                `json:"workspace_id"`
	Name               string            `json:"name"`
	Role               string            `json:"role"`
	SystemPrompt       string            `json:"system_prompt"`
	DefaultModel       string            `json:"default_model"`
	AllowedTools       []string          `json:"allowed_tools"`
	Status             string            `json:"status,omitempty"` // running | paused | idle
	Load               *int              `json:"load,omitempty"`   // 0..100
	CurrentTaskID      *ID               `json:"current_task_id,omitempty"`
	Capabilities       []string          `json:"capabilities,omitempty"`
	SkillIDs           []ID              `json:"skill_ids,omitempty"`
	McpIDs             []ID              `json:"mcp_ids,omitempty"`
	RoleTitle          string            `json:"role_title,omitempty"`
	Provider           identity.Provider `json:"provider,omitempty"`
	Temperature        *float64          `json:"temperature,omitempty"`
	MaxOutputTokens    *int              `json:"max_output_tokens,omitempty"`
	AutonomyMode       AutonomyMode      `json:"autonomy_mode,omitempty"`
	UserPromptTemplate string            `json:"user_prompt_template,omitempty"`
	KnowledgeSourceIDs []ID              `json:"knowledge_source_ids,omitempty"`
	Guardrails         *Guardrails       `json:"guardrails,omitempty"`
	CreatedAt          ISOTime           `json:"created_at"`
}

// Run mirrors frontend Run.
type Run struct {
	ID         ID        `json:"id"`
	TaskID     ID        `json:"task_id"`
	Role       RunRole   `json:"role"`
	AgentID    ID        `json:"agent_id"`
	Model      string    `json:"model"`
	Status     RunStatus `json:"status"`
	RoundNo    int       `json:"round_no"`
	StartedAt  ISOTime   `json:"started_at"`
	EndedAt    *ISOTime  `json:"ended_at,omitempty"`
	TokenUsage int       `json:"token_usage"`
	Error      string    `json:"error,omitempty"`
}

// Step mirrors frontend Step. Payload is kept as raw JSON because its shape varies
// by kind (message/reasoning => {content}; tool_call => {tool,args}; tool_result => {tool,result}).
type Step struct {
	ID        ID       `json:"id"`
	RunID     ID       `json:"run_id"`
	Seq       int      `json:"seq"`
	Kind      StepKind `json:"kind"`
	Payload   Payload  `json:"payload"`
	CreatedAt ISOTime  `json:"created_at"`
}

// Payload is the raw JSON payload of a Step.
type Payload = json.RawMessage

// Finding mirrors frontend Finding (reviewer feedback).
type Finding struct {
	ID             ID       `json:"id"`
	RunID          ID       `json:"run_id"`
	File           string   `json:"file"`
	Line           int      `json:"line"`
	Severity       Severity `json:"severity"`
	Issue          string   `json:"issue"`
	Recommendation string   `json:"recommendation"`
	Status         string   `json:"status"` // open | resolved
}

// Artifact mirrors frontend Artifact (derived from a run's patches / documents).
type Artifact struct {
	ID        ID      `json:"id"`
	TaskID    ID      `json:"task_id"`
	RunID     *ID     `json:"run_id,omitempty"`
	Filename  string  `json:"filename"`
	Kind      string  `json:"kind"` // patch | document
	Summary   string  `json:"summary"`
	Additions *int    `json:"additions,omitempty"`
	Deletions *int    `json:"deletions,omitempty"`
	CreatedAt ISOTime `json:"created_at"`
}
