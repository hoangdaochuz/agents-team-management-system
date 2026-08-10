package contracts

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

	// ── Facts (Agent-Runner -> consumers) ─────────────────────────────────
	TopicStep          = "step"           // one topic; messages carry task_id + run_id
	TopicRunCompleted  = "run.completed"
	TopicFinding       = "finding"
	TopicVerdict       = "verdict"
	TopicPrOpened      = "pr.opened"

	// ── Task state (Task svc -> any consumer) ─────────────────────────────
	TopicTaskStatusChanged = "task.status-changed"
)

// AllTopics returns every topic the system uses, for auto-creation / validation.
func AllTopics() []string {
	return []string{
		TopicTaskRunRequested,
		TopicTaskReviewRequested,
		TopicTaskStopRequested,
		TopicStep,
		TopicRunCompleted,
		TopicFinding,
		TopicVerdict,
		TopicPrOpened,
		TopicTaskStatusChanged,
	}
}

// EventEnvelope wraps every Kafka message. Key fields support idempotency and
// partitioning: TaskID is the partition key; EventID lets consumers dedup on
// at-least-once redelivery.
type EventEnvelope struct {
	EventID   string      `json:"event_id"`          // unique id of this event (for dedup)
	EventType string      `json:"event_type"`        // discriminator; matches the topic
	TaskID    ID          `json:"task_id,omitempty"` // partition key + correlation
	RunID     ID          `json:"run_id,omitempty"`
	OccurredAt ISOTime    `json:"occurred_at"`
	Data      interface{} `json:"data"`
}

// ── Command payloads ───────────────────────────────────────────────────────

// RunRequestedData requests the runner start an implementer run.
type RunRequestedData struct {
	TaskID     ID    `json:"task_id"`
	AgentID    ID    `json:"agent_id"`
	ProjectID  ID    `json:"project_id"`
	RoundNo    int   `json:"round_no"`
	ModelOverride string `json:"model_override,omitempty"`
}

// ReviewRequestedData requests the runner start a reviewer run.
type ReviewRequestedData struct {
	TaskID    ID `json:"task_id"`
	AgentID   ID `json:"agent_id"`
	RunID     ID `json:"run_id"` // the implementer run to review
	RoundNo   int `json:"round_no"`
}

// StopRequestedData requests the runner abort an in-flight run.
type StopRequestedData struct {
	TaskID ID `json:"task_id"`
	RunID  ID `json:"run_id,omitempty"`
}

// ── Fact payloads ──────────────────────────────────────────────────────────

// StepData carries a single agent step for realtime streaming + persistence.
type StepData struct {
	Step Step `json:"step"`
}

// RunCompletedData is emitted when a run terminates (done/aborted/stopped).
type RunCompletedData struct {
	TaskID     ID        `json:"task_id"`
	RunID      ID        `json:"run_id"`
	Role       RunRole   `json:"role"`
	Status     RunStatus `json:"status"`
	RoundNo    int       `json:"round_no"`
	TokenUsage int       `json:"token_usage"`
	Error      string    `json:"error,omitempty"`
}

// FindingData carries a reviewer finding.
type FindingData struct {
	Finding Finding `json:"finding"`
}

// VerdictData is the reviewer's verdict on a run.
type VerdictData struct {
	TaskID    ID              `json:"task_id"`
	RunID     ID              `json:"run_id"`
	RoundNo   int             `json:"round_no"`
	Decision  VerdictDecision `json:"decision"`
	Summary   string          `json:"summary,omitempty"`
}

// PrOpenedData is emitted when a PR is created from a task branch.
type PrOpenedData struct {
	TaskID ID     `json:"task_id"`
	RunID  ID     `json:"run_id,omitempty"`
	URL    string `json:"url"`
}

// TaskStatusChangedData is published on every authoritative task status change.
type TaskStatusChangedData struct {
	TaskID   ID         `json:"task_id"`
	From     TaskStatus `json:"from"`
	To       TaskStatus `json:"to"`
	RoundNo  int        `json:"round_no"`
}
