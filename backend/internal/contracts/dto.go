// Package contracts holds the shared domain DTOs and event/message contracts used
// across all services. The DTO field names mirror the frontend's declared API
// contract in frontend/src/api/types.ts (snake_case JSON), so a service response
// serializes to exactly what the SPA expects.
package contracts

import (
	"encoding/json"
	"time"
)

// ID is a UUID string identifier.
type ID = string

// ISOTime values are RFC3339 timestamps; time.Time marshals to RFC3339 by default.
type ISOTime = time.Time

// ── Enumerations (mirror frontend/src/api/types.ts) ────────────────────────

type RepoType string // "path" | "url"

const (
	RepoTypePath = RepoType("path")
	RepoTypeURL  = RepoType("url")
)

type TaskStatus string

const (
	TaskBacklog   = TaskStatus("backlog")
	TaskDoing     = TaskStatus("doing")
	TaskReview    = TaskStatus("review")
	TaskDone      = TaskStatus("done")
	TaskBlocked   = TaskStatus("blocked")
	TaskCancelled = TaskStatus("cancelled")
	TaskStopped   = TaskStatus("stopped")
)

type TaskType string   // "task" | "story" | "bug" | "epic"
type Priority string   // "highest" | "high" | "medium" | "low"
type RunRole string    // "implementer" | "reviewer"
type RunStatus string  // "running" | "done" | "aborted" | "stopped"
type StepKind string   // "message" | "tool_call" | "tool_result" | "reasoning"
type Severity string   // "info" | "warning" | "error" | "critical"
type Provider string   // "openai" | "anthropic" | "gemini" | "glm"

const (
	RunRoleImplementer = RunRole("implementer")
	RunRoleReviewer    = RunRole("reviewer")
)

const (
	StepMessage    = StepKind("message")
	StepToolCall   = StepKind("tool_call")
	StepToolResult = StepKind("tool_result")
	StepReasoning  = StepKind("reasoning")
)

// ── Domain resources ───────────────────────────────────────────────────────

// Project mirrors frontend Project.
type Project struct {
	ID           ID       `json:"id"`
	Name         string   `json:"name"`
	RepoSource   string   `json:"repo_source"`
	RepoType     RepoType `json:"repo_type"`
	ClonedPath   string   `json:"cloned_path"`
	DefaultBranch string  `json:"default_branch"`
	CreatedAt    ISOTime  `json:"created_at"`
}

// Task mirrors frontend Task. UI-only fields (type, priority, labels, points,
// due, progress) are first-class on the resource.
type Task struct {
	ID               ID          `json:"id"`
	ProjectID        ID          `json:"project_id"`
	AgentID          *ID         `json:"agent_id,omitempty"`
	ModelOverride    *string     `json:"model_override,omitempty"`
	Title            string      `json:"title"`
	Prompt           string      `json:"prompt"`
	Description      string      `json:"description,omitempty"`
	Status           TaskStatus  `json:"status"`
	Type             TaskType    `json:"type,omitempty"`
	Priority         Priority    `json:"priority,omitempty"`
	Labels           []string    `json:"labels,omitempty"`
	Points           *int        `json:"points,omitempty"`
	DueAt            *ISOTime    `json:"due_at,omitempty"`
	Progress         *int        `json:"progress,omitempty"`
	BranchName       string      `json:"branch_name,omitempty"`
	WorktreePath     string      `json:"worktree_path,omitempty"`
	CommentsCount    int         `json:"comments_count,omitempty"`
	AttachmentsCount int         `json:"attachments_count,omitempty"`
	CreatedAt        ISOTime     `json:"created_at"`
	UpdatedAt        ISOTime     `json:"updated_at"`
	RoundNo          int         `json:"-"` // internal saga state, not exposed by Task svc directly
}

// Agent mirrors frontend Agent.
type Agent struct {
	ID            ID       `json:"id"`
	Name          string   `json:"name"`
	Role          string   `json:"role"`
	SystemPrompt  string   `json:"system_prompt"`
	DefaultModel  string   `json:"default_model"`
	AllowedTools  []string `json:"allowed_tools"`
	Status        string   `json:"status,omitempty"`  // running | paused | idle
	Load          *int     `json:"load,omitempty"`    // 0..100
	CurrentTaskID *ID      `json:"current_task_id,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	SkillIDs      []ID     `json:"skill_ids,omitempty"`
	McpIDs        []ID     `json:"mcp_ids,omitempty"`
	CreatedAt     ISOTime  `json:"created_at"`
}

// Skill mirrors frontend Skill.
type Skill struct {
	ID            ID      `json:"id"`
	Name          string  `json:"name"`
	Description   string  `json:"description,omitempty"`
	BodyMd        string  `json:"body_md"`
	ResourcesPath string  `json:"resources_path,omitempty"`
	CreatedAt     ISOTime `json:"created_at"`
}

// McpServer mirrors frontend McpServer.
type McpServer struct {
	ID        ID                `json:"id"`
	Name      string            `json:"name"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env"`
	CreatedAt ISOTime           `json:"created_at"`
}

// ProviderKey exposes provider metadata only — the API key never leaves Settings.
type ProviderKey struct {
	Provider  Provider `json:"provider"`
	CreatedAt ISOTime  `json:"created_at"`
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
	ID        ID          `json:"id"`
	RunID     ID          `json:"run_id"`
	Seq       int         `json:"seq"`
	Kind      StepKind    `json:"kind"`
	Payload   Payload     `json:"payload"`
	CreatedAt ISOTime     `json:"created_at"`
}

// Payload is the raw JSON payload of a Step.
type Payload = json.RawMessage

// Finding mirrors frontend Finding (reviewer feedback).
type Finding struct {
	ID            ID       `json:"id"`
	RunID         ID       `json:"run_id"`
	File          string   `json:"file"`
	Line          int      `json:"line"`
	Severity      Severity `json:"severity"`
	Issue         string   `json:"issue"`
	Recommendation string  `json:"recommendation"`
	Status        string   `json:"status"` // open | resolved
}

// Feedback mirrors frontend Feedback (human comments on a task).
type Feedback struct {
	ID        ID      `json:"id"`
	TaskID    ID      `json:"task_id"`
	Author    string  `json:"author"` // always "user"
	Body      string  `json:"body"`
	CreatedAt ISOTime `json:"created_at"`
}

// Artifact mirrors frontend Artifact (derived from a run's patches / documents).
type Artifact struct {
	ID         ID      `json:"id"`
	TaskID     ID      `json:"task_id"`
	RunID      *ID     `json:"run_id,omitempty"`
	Filename   string  `json:"filename"`
	Kind       string  `json:"kind"` // patch | document
	Summary    string  `json:"summary"`
	Additions  *int    `json:"additions,omitempty"`
	Deletions  *int    `json:"deletions,omitempty"`
	CreatedAt  ISOTime `json:"created_at"`
}

// VerdictDecision is the reviewer's verdict on a run.
type VerdictDecision string

const (
	VerdictApprove         = VerdictDecision("APPROVE")
	VerdictRequestChanges  = VerdictDecision("REQUEST_CHANGES")
)
