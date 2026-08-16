package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/identity"
)

// Agent is the agent aggregate (wire DTO as domain type, D7).
type Agent = agentexec.Agent

// AgentCreate carries the fields of a new agent (domain value object; JSON
// decoding happens in the interface layer).
type AgentCreate struct {
	Name               string
	Role               string
	SystemPrompt       string
	DefaultModel       string
	AllowedTools       []string
	RoleTitle          string
	Provider           identity.Provider
	Temperature        *float64
	MaxOutputTokens    *int
	AutonomyMode       agentexec.AutonomyMode
	UserPromptTemplate string
	KnowledgeSourceIDs []identity.ID
	Guardrails         *agentexec.Guardrails
}

// AgentUpdate carries the optional fields of an agent patch; nil means "leave
// unchanged". CurrentTaskID set to "" clears the running task.
type AgentUpdate struct {
	Name               *string
	Role               *string
	SystemPrompt       *string
	DefaultModel       *string
	AllowedTools       *[]string
	Status             *string
	CurrentTaskID      *string
	RoleTitle          *string
	Provider           *identity.Provider
	Temperature        *float64
	MaxOutputTokens    *int
	AutonomyMode       *agentexec.AutonomyMode
	UserPromptTemplate *string
	KnowledgeSourceIDs *[]identity.ID
	Guardrails         *agentexec.Guardrails
}

// AgentRepository is the agent aggregate port (CRUD + link tables).
type AgentRepository interface {
	List(ctx context.Context, ws []identity.ID) ([]Agent, error)
	Get(ctx context.Context, id identity.ID, ws []identity.ID) (Agent, error)
	GetUnscoped(ctx context.Context, id identity.ID) (Agent, error)
	Create(ctx context.Context, workspaceID identity.ID, in AgentCreate) (Agent, error)
	Update(ctx context.Context, id identity.ID, ws []identity.ID, in AgentUpdate) (Agent, error)
	Delete(ctx context.Context, id identity.ID, ws []identity.ID) error
	CountByWorkspace(ctx context.Context, workspaceID identity.ID) (int, error)
	LinkSkill(ctx context.Context, agentID, skillID identity.ID) error
	UnlinkSkill(ctx context.Context, agentID, skillID identity.ID) error
	LinkMcp(ctx context.Context, agentID, mcpID identity.ID) error
	UnlinkMcp(ctx context.Context, agentID, mcpID identity.ID) error
}

// CatalogProjectionRepository is the port over the projected skill/MCP
// definition rows the Agent service keeps for attachment validation (no
// service-to-service sync calls).
type CatalogProjectionRepository interface {
	SkillWorkspace(ctx context.Context, skillID identity.ID) (identity.ID, error)
	McpWorkspace(ctx context.Context, mcpID identity.ID) (identity.ID, error)
}
