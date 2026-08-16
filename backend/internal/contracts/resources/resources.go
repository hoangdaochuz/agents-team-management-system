// Package resources holds the shared-kernel workspace-resource DTOs: skills,
// MCP servers, knowledge sources, plugins, rules, and MCP connections. Field
// names mirror the frontend's declared API contract (snake_case JSON).
package resources

import "github.com/aaks/server/internal/contracts/identity"

// ID re-exports the shared identifier for convenience within this domain.
type ID = identity.ID

// ISOTime re-exports the shared timestamp alias.
type ISOTime = identity.ISOTime

// IndexStatus is a knowledge-source indexing state:
// "indexed" | "reindexing" | "failed" | "pending".
type IndexStatus string

const (
	IndexPending    = IndexStatus("pending")
	IndexIndexed    = IndexStatus("indexed")
	IndexReindexing = IndexStatus("reindexing")
	IndexFailed     = IndexStatus("failed")
)

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
	ID          ID     `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
}

// McpConnection mirrors frontend McpConnection.
type McpConnection struct {
	ID        ID       `json:"id"`
	Name      string   `json:"name"`
	Transport string   `json:"transport"` // stdio | http
	ToolCount int      `json:"tool_count"`
	ToolNames []string `json:"tool_names,omitempty"`
	Status    string   `json:"status"` // connected | offline
}
