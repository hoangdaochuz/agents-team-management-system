// Package tasks holds the shared-kernel kanban DTOs: Task, Project, and
// Feedback, plus their enumerations. Field names mirror the frontend's
// declared API contract (snake_case JSON).
package tasks

import "github.com/aaks/server/internal/contracts/identity"

// ID re-exports the shared identifier for convenience within this domain.
type ID = identity.ID

// ISOTime re-exports the shared timestamp alias.
type ISOTime = identity.ISOTime

// TaskStatus is a kanban column: backlog | doing | review | done | blocked | cancelled | stopped.
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

// TaskType is a kanban item kind: "task" | "story" | "bug" | "epic".
type TaskType string

// Priority is a kanban priority: "highest" | "high" | "medium" | "low".
type Priority string

// Project mirrors frontend Project.
type Project struct {
	ID            ID                `json:"id"`
	WorkspaceID   ID                `json:"workspace_id"`
	Name          string            `json:"name"`
	RepoSource    string            `json:"repo_source"`
	RepoType      identity.RepoType `json:"repo_type"`
	ClonedPath    string            `json:"cloned_path"`
	DefaultBranch string            `json:"default_branch"`
	CreatedAt     ISOTime           `json:"created_at"`
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

// Feedback mirrors frontend Feedback (human comments on a task).
type Feedback struct {
	ID        ID      `json:"id"`
	TaskID    ID      `json:"task_id"`
	Author    string  `json:"author"` // always "user"
	Body      string  `json:"body"`
	CreatedAt ISOTime `json:"created_at"`
}
