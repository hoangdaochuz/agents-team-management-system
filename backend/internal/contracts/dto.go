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
	RunRunning  = RunStatus("running")
	RunDone     = RunStatus("done")
	RunAborted  = RunStatus("aborted")
	RunStopped  = RunStatus("stopped")
)

const (
	StepMessage    = StepKind("message")
	StepToolCall   = StepKind("tool_call")
	StepToolResult = StepKind("tool_result")
	StepReasoning  = StepKind("reasoning")
)

// Multi-tenant enumerations.
type Role string // "owner" | "admin" | "member"

const (
	RoleOwner  = Role("owner")
	RoleAdmin  = Role("admin")
	RoleMember = Role("member")
)

type MemberStatus string // "active" | "invited" | "suspended"

const (
	MemberActive    = MemberStatus("active")
	MemberInvited   = MemberStatus("invited")
	MemberSuspended = MemberStatus("suspended")
)

type Plan string // "free" | "team" | "pro" | "enterprise"

const (
	PlanFree       = Plan("free")
	PlanTeam       = Plan("team")
	PlanPro        = Plan("pro")
	PlanEnterprise = Plan("enterprise")
)

type OrgStatus string // "active" | "trial" | "suspended"

const (
	OrgActive    = OrgStatus("active")
	OrgTrial     = OrgStatus("trial")
	OrgSuspended = OrgStatus("suspended")
)

type AutonomyMode string // "assigned" | "matching" | "autonomous"

const (
	AutonomyAssigned   = AutonomyMode("assigned")
	AutonomyMatching   = AutonomyMode("matching")
	AutonomyAutonomous = AutonomyMode("autonomous")
)

type SignupState string // "pending" | "approved" | "declined"

const (
	SignupPending  = SignupState("pending")
	SignupApproved = SignupState("approved")
	SignupDeclined = SignupState("declined")
)

type IndexStatus string // "indexed" | "reindexing" | "failed" | "pending"

const (
	IndexPending    = IndexStatus("pending")
	IndexIndexed    = IndexStatus("indexed")
	IndexReindexing = IndexStatus("reindexing")
	IndexFailed     = IndexStatus("failed")
)

// ── Guardrails (agent builder) ─────────────────────────────────────────────

// Guardrails express an agent's execution policy. Mirrors frontend Guardrails.
type Guardrails struct {
	AutoPauseOnTestFail       *bool `json:"auto_pause_on_test_fail,omitempty"`
	AllowDirectCommits        *bool `json:"allow_direct_commits,omitempty"`
	AllowShellCommands        *bool `json:"allow_shell_commands,omitempty"`
	RequireApprovalBeforeMerge *bool `json:"require_approval_before_merge,omitempty"`
	MaxStepsPerRun            *int  `json:"max_steps_per_run,omitempty"`
	WallClockCapMin           *int  `json:"wall_clock_cap_min,omitempty"`
}

// ── Domain resources ───────────────────────────────────────────────────────

// Project mirrors frontend Project.
type Project struct {
	ID            ID       `json:"id"`
	WorkspaceID   ID       `json:"workspace_id"`
	Name          string   `json:"name"`
	RepoSource    string   `json:"repo_source"`
	RepoType      RepoType `json:"repo_type"`
	ClonedPath    string   `json:"cloned_path"`
	DefaultBranch string   `json:"default_branch"`
	CreatedAt     ISOTime  `json:"created_at"`
}

// Task mirrors frontend Task. UI-only fields (type, priority, labels, points,
// due, progress) are first-class on the resource.
type Task struct {
	ID               ID         `json:"id"`
	WorkspaceID      ID         `json:"workspace_id"`
	ProjectID        ID         `json:"project_id"`
	AgentID          *ID        `json:"agent_id,omitempty"`
	ModelOverride    *string    `json:"model_override,omitempty"`
	Title            string     `json:"title"`
	Prompt           string     `json:"prompt"`
	Description      string     `json:"description,omitempty"`
	Status           TaskStatus `json:"status"`
	Type             TaskType   `json:"type,omitempty"`
	Priority         Priority   `json:"priority,omitempty"`
	Labels           []string   `json:"labels,omitempty"`
	Points           *int       `json:"points,omitempty"`
	DueAt            *ISOTime   `json:"due_at,omitempty"`
	Progress         *int       `json:"progress,omitempty"`
	BranchName       string     `json:"branch_name,omitempty"`
	WorktreePath     string     `json:"worktree_path,omitempty"`
	CommentsCount    int        `json:"comments_count,omitempty"`
	AttachmentsCount int        `json:"attachments_count,omitempty"`
	CreatedAt        ISOTime    `json:"created_at"`
	UpdatedAt        ISOTime    `json:"updated_at"`
	RoundNo          int        `json:"-"` // internal saga state, not exposed by Task svc directly
}

// Agent mirrors frontend Agent.
type Agent struct {
	ID            ID          `json:"id"`
	WorkspaceID   ID          `json:"workspace_id"`
	Name          string      `json:"name"`
	Role          string      `json:"role"`
	SystemPrompt  string      `json:"system_prompt"`
	DefaultModel  string      `json:"default_model"`
	AllowedTools  []string    `json:"allowed_tools"`
	Status        string      `json:"status,omitempty"`  // running | paused | idle
	Load          *int        `json:"load,omitempty"`    // 0..100
	CurrentTaskID *ID         `json:"current_task_id,omitempty"`
	Capabilities  []string    `json:"capabilities,omitempty"`
	SkillIDs      []ID        `json:"skill_ids,omitempty"`
	McpIDs        []ID        `json:"mcp_ids,omitempty"`
	RoleTitle     string      `json:"role_title,omitempty"`
	Provider      Provider    `json:"provider,omitempty"`
	Temperature   *float64    `json:"temperature,omitempty"`
	MaxOutputTokens *int      `json:"max_output_tokens,omitempty"`
	AutonomyMode  AutonomyMode `json:"autonomy_mode,omitempty"`
	UserPromptTemplate string `json:"user_prompt_template,omitempty"`
	KnowledgeSourceIDs []ID   `json:"knowledge_source_ids,omitempty"`
	Guardrails    *Guardrails `json:"guardrails,omitempty"`
	CreatedAt     ISOTime     `json:"created_at"`
}

// Skill mirrors frontend Skill.
type Skill struct {
	ID            ID      `json:"id"`
	WorkspaceID   ID      `json:"workspace_id"`
	Name          string  `json:"name"`
	Description   string  `json:"description,omitempty"`
	BodyMd        string  `json:"body_md"`
	ResourcesPath string  `json:"resources_path,omitempty"`
	Enabled       *bool   `json:"enabled,omitempty"`
	Trigger       string  `json:"trigger,omitempty"`
	StepCount     *int    `json:"step_count,omitempty"`
	CreatedAt     ISOTime `json:"created_at"`
}

// McpServer mirrors frontend McpServer.
type McpServer struct {
	ID          ID                `json:"id"`
	WorkspaceID ID                `json:"workspace_id"`
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	Args        []string          `json:"args"`
	Env         map[string]string `json:"env"`
	CreatedAt   ISOTime           `json:"created_at"`
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

// VerdictDecision is the reviewer's verdict on a run.
type VerdictDecision string

const (
	VerdictApprove        = VerdictDecision("APPROVE")
	VerdictRequestChanges = VerdictDecision("REQUEST_CHANGES")
)

// ── Multi-tenant: auth, orgs, workspaces, members ───────────────────────────

// User mirrors frontend User (identity + role in the active workspace).
type User struct {
	ID           ID       `json:"id"`
	Name         string   `json:"name"`
	Email        string   `json:"email"`
	Avatar       string   `json:"avatar,omitempty"`
	Role         Role     `json:"role"`
	IsSuperadmin *bool    `json:"is_superadmin,omitempty"`
	CreatedAt    ISOTime  `json:"created_at"`
}

// Session mirrors frontend Session, assembled by the Gateway from Auth (user)
// and Orgs (memberships).
type Session struct {
	User             User        `json:"user"`
	Workspaces       []Workspace `json:"workspaces"`
	ActiveWorkspaceID ID         `json:"active_workspace_id,omitempty"`
}

// Organization mirrors frontend Organization.
type Organization struct {
	ID             ID        `json:"id"`
	Name           string    `json:"name"`
	Subdomain      string    `json:"subdomain,omitempty"`
	Plan           Plan      `json:"plan"`
	WorkspaceCount int       `json:"workspace_count"`
	SeatsUsed      int       `json:"seats_used"`
	SeatsTotal     int       `json:"seats_total"`
	Status         OrgStatus `json:"status"`
	CreatedAt      ISOTime   `json:"created_at"`
}

// Workspace mirrors frontend Workspace. agent_count / open_task_count are
// derived counts composed by the Gateway; role is the current user's role.
type Workspace struct {
	ID            ID       `json:"id"`
	Name          string   `json:"name"`
	RepoSource    string   `json:"repo_source,omitempty"`
	DefaultBranch string   `json:"default_branch,omitempty"`
	Glyph         string   `json:"glyph,omitempty"`
	Description   string   `json:"description,omitempty"`
	AgentCount    *int     `json:"agent_count,omitempty"`
	OpenTaskCount *int     `json:"open_task_count,omitempty"`
	Role          Role     `json:"role"`
	CreatedAt     ISOTime  `json:"created_at"`
}

// Member mirrors frontend Member.
type Member struct {
	ID              ID             `json:"id"`
	User            MemberUser     `json:"user"`
	Role            Role           `json:"role"`
	Status          MemberStatus   `json:"status"`
	LastActiveAt    *ISOTime       `json:"last_active_at,omitempty"`
	IsServiceAccount *bool         `json:"is_service_account,omitempty"`
}

// MemberUser is the nested user identity in a Member.
type MemberUser struct {
	ID    ID     `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Invite mirrors frontend Invite.
type Invite struct {
	ID          ID      `json:"id"`
	Email       string  `json:"email"`
	Name        string  `json:"name,omitempty"`
	Role        Role    `json:"role"`
	RequestedAt ISOTime `json:"requested_at"`
}

// SignupRequest mirrors frontend SignupRequest (pending join/org requests).
type SignupRequest struct {
	ID            ID      `json:"id"`
	Name          string  `json:"name"`
	Email         string  `json:"email"`
	WorkspaceName string  `json:"workspace_name,omitempty"`
	WorkspaceID   ID      `json:"workspace_id,omitempty"`
	RequestedRole Role    `json:"requested_role"`
	RequestedAt   ISOTime `json:"requested_at"`
}

// ── Workspace resources ────────────────────────────────────────────────────

// KnowledgeSource mirrors frontend KnowledgeSource.
type KnowledgeSource struct {
	ID     ID          `json:"id"`
	Title  string      `json:"title"`
	Kind   string      `json:"kind"` // file | folder | url | upload
	Chunks *int        `json:"chunks,omitempty"`
	Pages  *int        `json:"pages,omitempty"`
	Status IndexStatus `json:"status"`
}

// Plugin mirrors frontend Plugin.
type Plugin struct {
	ID           ID       `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	Enabled      bool     `json:"enabled"`
}

// Rule mirrors frontend Rule.
type Rule struct {
	ID          ID      `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Enabled     bool    `json:"enabled"`
}

// McpConnection mirrors frontend McpConnection.
type McpConnection struct {
	ID         ID       `json:"id"`
	Name       string   `json:"name"`
	Transport  string   `json:"transport"` // stdio | http
	ToolCount  int      `json:"tool_count"`
	ToolNames  []string `json:"tool_names,omitempty"`
	Status     string   `json:"status"` // connected | offline
}

// ── Admin / sysadmin ───────────────────────────────────────────────────────

// AuditEntry mirrors frontend AuditEntry.
type AuditEntry struct {
	ID         ID           `json:"id"`
	Actor      AuditActor   `json:"actor"`
	Action     string       `json:"action"`
	ActionKind string       `json:"action_kind,omitempty"`
	Target     string       `json:"target,omitempty"`
	CreatedAt  ISOTime      `json:"created_at"`
	IP         string       `json:"ip,omitempty"`
}

// AuditActor is the actor nested in an AuditEntry.
type AuditActor struct {
	Name string `json:"name"`
}

// FeatureFlag mirrors frontend FeatureFlag.
type FeatureFlag struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
}

// ServiceHealth mirrors frontend ServiceHealth.
type ServiceHealth struct {
	Name   string `json:"name"`
	Pct    int    `json:"pct"` // 0..100
	Status string `json:"status"` // ok | warn | down
}

// SystemHealth mirrors frontend SystemHealth.
type SystemHealth struct {
	Services []ServiceHealth `json:"services"`
}

// SystemKpis mirrors frontend SystemKpis.
type SystemKpis struct {
	Organizations   int  `json:"organizations"`
	OrgsDelta       *int `json:"orgs_delta,omitempty"`
	Workspaces      int  `json:"workspaces"`
	ActiveUsers24h  int  `json:"active_users_24h"`
	ActiveUsersDelta *int `json:"active_users_delta,omitempty"`
	OpenSeats       int  `json:"open_seats"`
	OpenSeatsDelta  *int `json:"open_seats_delta,omitempty"`
}
