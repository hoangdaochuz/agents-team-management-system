// Package events holds the event catalog: the 9 Kafka topics that form the
// execution-boundary backbone, plus the in-process event types used inside the
// consolidated services for flows that no longer cross a service boundary.
// Wire JSON is the system's event contract and must stay byte-for-byte stable.
package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/identity"
)

// Event topic catalog — only execution-boundary events travel over Kafka.
//
// Partitioning: every topic is partitioned by TaskID so that all events for a
// given task are delivered to one consumer in publish order.
//
// Commands (Workspace svc -> Executor): the executor is the sole consumer.
// Facts (Executor -> consumers): the Workspace saga and the Gateway (SSE) consume.
const (
	// ── Commands (Workspace svc -> Executor) ──────────────────────────────
	TopicTaskRunRequested    = "task.run-requested"
	TopicTaskReviewRequested = "task.review-requested"
	TopicTaskStopRequested   = "task.stop-requested"
	TopicPrOpenRequested     = "task.pr-open-requested"

	// ── Facts (Executor -> consumers) ─────────────────────────────────────
	TopicStep         = "step" // one topic; messages carry task_id + run_id
	TopicRunCompleted = "run.completed"
	TopicFinding      = "finding"
	TopicVerdict      = "verdict"
	TopicPrOpened     = "pr.opened"

	// ── In-process event types (never on Kafka) ───────────────────────────
	// These flows live inside one consolidated service; the strings are only
	// dispatch keys for the in-process event bus.

	// Identity service: signup flow (auth ↔ orgs planes) and audit recording.
	TopicSignupRequested = "signup.requested"
	TopicSignupApproved  = "signup.approved"
	TopicSignupDeclined  = "signup.declined"
	TopicInviteCreated   = "invite.created"
	TopicAuditRecorded   = "audit.recorded"

	// Workspace service: catalog → resources MCP projections.
	TopicMcpCreated = "mcp.created"
	TopicMcpDeleted = "mcp.deleted"
)

// AllTopics returns every Kafka topic the system uses, for auto-creation /
// validation. In-process event types are deliberately absent.
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
	}
}

// taskPartitionedTopics are the topics keyed by TaskID for per-task ordered
// delivery.
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

// ── In-process payloads (never on Kafka) ───────────────────────────────────

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

// WorkspaceCreatedData is the body of the Identity→Workspace provisioning
// call made when a workspace is created, so the Workspace service establishes
// the repo binding and seeds default rules.
type WorkspaceCreatedData struct {
	WorkspaceID   identity.ID `json:"workspace_id"`
	Name          string      `json:"name"`
	RepoSource    string      `json:"repo_source,omitempty"`
	DefaultBranch string      `json:"default_branch,omitempty"`
}

// McpCreatedData is dispatched in-process when an MCP definition is created so
// Resources projects it into a connection row.
type McpCreatedData struct {
	McpServerID identity.ID       `json:"mcp_server_id"`
	WorkspaceID identity.ID       `json:"workspace_id"`
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	Args        []string          `json:"args"`
	Env         map[string]string `json:"env"`
}

// McpDeletedData is dispatched in-process when an MCP definition is deleted.
type McpDeletedData struct {
	McpServerID identity.ID `json:"mcp_server_id"`
	WorkspaceID identity.ID `json:"workspace_id"`
}

// AuditRecordedData carries a workspace-level admin action to the audit log
// (in-process inside the Identity service).
type AuditRecordedData struct {
	WorkspaceID identity.ID `json:"workspace_id"`
	ActorName   string      `json:"actor_name"`
	ActorID     identity.ID `json:"actor_id,omitempty"`
	Action      string      `json:"action"`
	ActionKind  string      `json:"action_kind,omitempty"`
	Target      string      `json:"target,omitempty"`
	IP          string      `json:"ip,omitempty"`
}
