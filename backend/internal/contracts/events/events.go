// Package events holds the Kafka event catalog: topic names, the
// EventEnvelope, and every event payload shape. Wire JSON is the system's
// event contract and must stay byte-for-byte stable.
package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/tasks"
)

// Event topic catalog — the Kafka topics that form the event-driven backbone.
//
// Partitioning: every lifecycle/step topic is partitioned by TaskID so that all
// events for a given task are delivered to one consumer in publish order.
//
// Commands (Task svc -> Agent-Runner): the runner is the sole consumer.
// Facts (Agent-Runner -> consumers): Task svc, Gateway, and any projector consume.
// Status (Task svc -> any): published on every authoritative status change.
const (
	// ── Commands (Task svc -> Agent-Runner) ───────────────────────────────
	TopicTaskRunRequested    = "task.run-requested"
	TopicTaskReviewRequested = "task.review-requested"
	TopicTaskStopRequested   = "task.stop-requested"
	TopicPrOpenRequested     = "task.pr-open-requested"

	// ── Facts (Agent-Runner -> consumers) ─────────────────────────────────
	TopicStep         = "step" // one topic; messages carry task_id + run_id
	TopicRunCompleted = "run.completed"
	TopicFinding      = "finding"
	TopicVerdict      = "verdict"
	TopicPrOpened     = "pr.opened"

	// ── Task state (Task svc -> any consumer) ─────────────────────────────
	TopicTaskStatusChanged = "task.status-changed"

	// ── Multi-tenant plane ────────────────────────────────────────────────
	// Signup flow: Auth emits signup.requested; Orgs + Admin project the
	// request; the approver (Orgs for join mode, Admin for create mode) emits
	// signup.approved / signup.declined; Auth activates the user, Orgs creates
	// the org/workspace/membership for create mode.
	TopicSignupRequested = "signup.requested"
	TopicSignupApproved  = "signup.approved"
	TopicSignupDeclined  = "signup.declined"

	// Invite flow: Orgs emits invite.created so Auth can resolve join-mode
	// invite codes locally.
	TopicInviteCreated = "invite.created"

	// Workspace creation: Orgs emits workspace.created so the Project service
	// establishes the workspace↔repo binding and Resources seeds defaults.
	TopicWorkspaceCreated = "workspace.created"

	// Catalog → Resources: MCP definition changes project into connection rows.
	TopicMcpCreated = "mcp.created"
	TopicMcpDeleted = "mcp.deleted"

	// Catalog → Agent: skill/MCP definitions are projected so attachments can be
	// validated against the agent's workspace (no service-to-service sync calls).
	TopicSkillCreated = "skill.created"
	TopicSkillDeleted = "skill.deleted"

	// Runner → Agent: run lifecycle facts derive agent runtime status.
	TopicRunStarted = "run.started"

	// Orgs → Admin: workspace-level admin actions are recorded as audit facts.
	TopicAuditRecorded = "audit.recorded"
)

// AllTopics returns every topic the system uses, for auto-creation / validation.
func AllTopics() []string {
	return []string{
		TopicTaskRunRequested,
		TopicTaskReviewRequested,
		TopicTaskStopRequested,
		TopicPrOpenRequested,
		TopicStep,
		TopicRunCompleted,
		TopicFinding,
		TopicVerdict,
		TopicPrOpened,
		TopicTaskStatusChanged,
		TopicSignupRequested,
		TopicSignupApproved,
		TopicSignupDeclined,
		TopicInviteCreated,
		TopicWorkspaceCreated,
		TopicMcpCreated,
		TopicMcpDeleted,
		TopicSkillCreated,
		TopicSkillDeleted,
		TopicRunStarted,
		TopicAuditRecorded,
	}
}

// taskPartitionedTopics are the topics keyed by TaskID for per-task ordered
// delivery. Every other topic (signup, invite, workspace, catalog projections,
// audit) keys on its own correlation id and must not require a TaskID.
var taskPartitionedTopics = map[string]bool{
	TopicTaskRunRequested:    true,
	TopicTaskReviewRequested: true,
	TopicTaskStopRequested:   true,
	TopicPrOpenRequested:     true,
	TopicStep:                true,
	TopicRunCompleted:        true,
	TopicFinding:             true,
	TopicVerdict:             true,
	TopicPrOpened:            true,
	TopicTaskStatusChanged:   true,
	TopicRunStarted:          true,
}

// IsTaskPartitioned reports whether a topic is partitioned by TaskID (and thus
// requires every published envelope to carry a non-empty TaskID key).
func IsTaskPartitioned(topic string) bool {
	return taskPartitionedTopics[topic]
}

// EventEnvelope wraps every Kafka message. Key fields support idempotency and
// partitioning: TaskID is the partition key; EventID lets consumers dedup on
// at-least-once redelivery.
type EventEnvelope struct {
	EventID    string      `json:"event_id"`          // unique id of this event (for dedup)
	EventType  string      `json:"event_type"`        // discriminator; matches the topic
	TaskID     identity.ID `json:"task_id,omitempty"` // partition key + correlation
	RunID      identity.ID `json:"run_id,omitempty"`
	OccurredAt time.Time   `json:"occurred_at"`
	Data       interface{} `json:"data"`
}

// DecodeData unmarshals Data into v.
func (e *EventEnvelope) DecodeData(v interface{}) error {
	buf, err := json.Marshal(e.Data)
	if err != nil {
		return err
	}
	return json.Unmarshal(buf, v)
}

// Decode unmarshals the envelope payload into a typed value.
func Decode[T any](msg EventEnvelope) (T, error) {
	var d T
	err := msg.DecodeData(&d)
	return d, err
}

// Forward decodes the envelope payload into T and hands it to fn, collapsing
// the per-event decode boilerplate in message handlers.
func Forward[T any](ctx context.Context, msg EventEnvelope, fn func(context.Context, T) error) error {
	d, err := Decode[T](msg)
	if err != nil {
		return err
	}
	return fn(ctx, d)
}

// ── Command payloads ───────────────────────────────────────────────────────

// RunRequestedData requests the runner start an implementer run.
type RunRequestedData struct {
	TaskID        identity.ID `json:"task_id"`
	AgentID       identity.ID `json:"agent_id"`
	ProjectID     identity.ID `json:"project_id"`
	WorkspaceID   identity.ID `json:"workspace_id,omitempty"`
	RoundNo       int         `json:"round_no"`
	Prompt        string      `json:"prompt"`
	ModelOverride string      `json:"model_override,omitempty"`
}

// ReviewRequestedData requests the runner start a reviewer run.
type ReviewRequestedData struct {
	TaskID      identity.ID `json:"task_id"`
	AgentID     identity.ID `json:"agent_id"`
	RunID       identity.ID `json:"run_id"` // the implementer run to review
	WorkspaceID identity.ID `json:"workspace_id,omitempty"`
	RoundNo     int         `json:"round_no"`
	Prompt      string      `json:"prompt"`
}

// StopRequestedData requests the runner abort an in-flight run.
type StopRequestedData struct {
	TaskID identity.ID `json:"task_id"`
	RunID  identity.ID `json:"run_id,omitempty"`
}

// PrOpenRequestedData requests the runner create a PR for a task's branch.
type PrOpenRequestedData struct {
	TaskID identity.ID `json:"task_id"`
	RunID  identity.ID `json:"run_id,omitempty"`
}

// ── Fact payloads ──────────────────────────────────────────────────────────

// StepData carries a single agent step for realtime streaming + persistence.
type StepData struct {
	Step agentexec.Step `json:"step"`
}

// RunCompletedData is emitted when a run terminates (done/aborted/stopped).
type RunCompletedData struct {
	TaskID     identity.ID         `json:"task_id"`
	RunID      identity.ID         `json:"run_id"`
	AgentID    identity.ID         `json:"agent_id,omitempty"`
	Role       agentexec.RunRole   `json:"role"`
	Status     agentexec.RunStatus `json:"status"`
	RoundNo    int                 `json:"round_no"`
	TokenUsage int                 `json:"token_usage"`
	Error      string              `json:"error,omitempty"`
}

// FindingData carries a reviewer finding.
type FindingData struct {
	Finding agentexec.Finding `json:"finding"`
}

// VerdictData is the reviewer's verdict on a run.
type VerdictData struct {
	TaskID   identity.ID               `json:"task_id"`
	RunID    identity.ID               `json:"run_id"`
	RoundNo  int                       `json:"round_no"`
	Decision agentexec.VerdictDecision `json:"decision"`
	Summary  string                    `json:"summary,omitempty"`
}

// PrOpenedData is emitted when a PR is created from a task branch.
type PrOpenedData struct {
	TaskID identity.ID `json:"task_id"`
	RunID  identity.ID `json:"run_id,omitempty"`
	URL    string      `json:"url"`
}

// TaskStatusChangedData is published on every authoritative task status change.
type TaskStatusChangedData struct {
	TaskID  identity.ID      `json:"task_id"`
	From    tasks.TaskStatus `json:"from"`
	To      tasks.TaskStatus `json:"to"`
	RoundNo int              `json:"round_no"`
}

// ── Multi-tenant payloads ──────────────────────────────────────────────────

// SignupRequestedData is published by Auth when a signup request is recorded.
type SignupRequestedData struct {
	RequestID        identity.ID   `json:"request_id"`
	UserID           identity.ID   `json:"user_id"`
	Name             string        `json:"name"`
	Email            string        `json:"email"`
	Mode             string        `json:"mode"` // join | create
	InviteCode       string        `json:"invite_code,omitempty"`
	WorkspaceID      identity.ID   `json:"workspace_id,omitempty"`
	OrganizationName string        `json:"organization_name,omitempty"`
	RequestedRole    identity.Role `json:"requested_role"`
}

// SignupApprovedData is published by the approver (Orgs for join mode, Admin
// for create mode); Auth activates the user, Orgs creates the org/workspace
// and membership for create mode.
type SignupApprovedData struct {
	RequestID        identity.ID   `json:"request_id"`
	UserID           identity.ID   `json:"user_id"`
	Email            string        `json:"email"`
	Name             string        `json:"name"`
	Mode             string        `json:"mode"` // join | create
	WorkspaceID      identity.ID   `json:"workspace_id,omitempty"`
	OrganizationName string        `json:"organization_name,omitempty"`
	Role             identity.Role `json:"role"`
}

// SignupDeclinedData is published by the approver; Auth marks the request declined.
type SignupDeclinedData struct {
	RequestID identity.ID `json:"request_id"`
	UserID    identity.ID `json:"user_id"`
}

// InviteCreatedData is published by Orgs when invites are created so Auth can
// resolve join-mode invite codes.
type InviteCreatedData struct {
	InviteID      identity.ID   `json:"invite_id"`
	Email         string        `json:"email"`
	Role          identity.Role `json:"role"`
	InviteCode    string        `json:"invite_code"`
	WorkspaceID   identity.ID   `json:"workspace_id"`
	WorkspaceName string        `json:"workspace_name"`
}

// WorkspaceCreatedData is published by Orgs when a workspace is created so the
// Project service establishes the repo binding and Resources seeds defaults.
type WorkspaceCreatedData struct {
	WorkspaceID   identity.ID `json:"workspace_id"`
	Name          string      `json:"name"`
	RepoSource    string      `json:"repo_source,omitempty"`
	DefaultBranch string      `json:"default_branch,omitempty"`
}

// McpCreatedData is published by Catalog when an MCP definition is created so
// Resources projects it into a connection row.
type McpCreatedData struct {
	McpServerID identity.ID       `json:"mcp_server_id"`
	WorkspaceID identity.ID       `json:"workspace_id"`
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	Args        []string          `json:"args"`
	Env         map[string]string `json:"env"`
}

// McpDeletedData is published by Catalog when an MCP definition is deleted.
type McpDeletedData struct {
	McpServerID identity.ID `json:"mcp_server_id"`
	WorkspaceID identity.ID `json:"workspace_id"`
}

// SkillCreatedData is published by Catalog when a skill is created so the Agent
// service can validate attachments by workspace.
type SkillCreatedData struct {
	SkillID     identity.ID `json:"skill_id"`
	WorkspaceID identity.ID `json:"workspace_id"`
}

// SkillDeletedData is published by Catalog when a skill is deleted.
type SkillDeletedData struct {
	SkillID identity.ID `json:"skill_id"`
}

// RunStartedData is published by the Runner when a run begins so the Agent
// service can derive the agent's runtime status.
type RunStartedData struct {
	TaskID  identity.ID `json:"task_id"`
	RunID   identity.ID `json:"run_id"`
	AgentID identity.ID `json:"agent_id"`
}

// AuditRecordedData carries a workspace-level admin action to the Admin service
// for persistence in the audit log.
type AuditRecordedData struct {
	WorkspaceID identity.ID `json:"workspace_id"`
	ActorName   string      `json:"actor_name"`
	ActorID     identity.ID `json:"actor_id,omitempty"`
	Action      string      `json:"action"`
	ActionKind  string      `json:"action_kind,omitempty"`
	Target      string      `json:"target,omitempty"`
	IP          string      `json:"ip,omitempty"`
}
