package application

import (
	"context"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
	"github.com/aaks/server/services/agent/internal/domain"
)

// List returns the agents visible in the caller's workspace set, newest first
// (empty set yields an empty list — fail closed).
func (a *App) List(ctx context.Context, ws []identity.ID) ([]agentexec.Agent, error) {
	out, err := a.repo.Agents.List(ctx, ws)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []agentexec.Agent{}
	}
	return out, nil
}

// Get returns one agent scoped to the caller's workspace set.
func (a *App) Get(ctx context.Context, id identity.ID, ws []identity.ID) (agentexec.Agent, error) {
	return a.repo.Agents.Get(ctx, id, ws)
}

// Create inserts an agent in the caller's workspace.
func (a *App) Create(ctx context.Context, workspaceID identity.ID, in domain.AgentCreate) (agentexec.Agent, error) {
	return a.repo.Agents.Create(ctx, workspaceID, in)
}

// Update patches an agent scoped to the caller's workspace set.
func (a *App) Update(ctx context.Context, id identity.ID, ws []identity.ID, in domain.AgentUpdate) (agentexec.Agent, error) {
	return a.repo.Agents.Update(ctx, id, ws, in)
}

// Delete removes an agent scoped to the caller's workspace set.
func (a *App) Delete(ctx context.Context, id identity.ID, ws []identity.ID) error {
	return a.repo.Agents.Delete(ctx, id, ws)
}

// CountByWorkspace returns the agent count of a workspace (Gateway
// workspace-stats composition).
func (a *App) CountByWorkspace(ctx context.Context, workspaceID identity.ID) (int, error) {
	return a.repo.Agents.CountByWorkspace(ctx, workspaceID)
}

// AttachSkill links a skill to an agent, rejecting definitions from another
// workspace and unknown (unprojected) definitions.
func (a *App) AttachSkill(ctx context.Context, agentID, skillID identity.ID) error {
	agent, err := a.repo.Agents.GetUnscoped(ctx, agentID)
	if err != nil {
		return err
	}
	ws, err := a.repo.Projections.SkillWorkspace(ctx, skillID)
	if err != nil {
		return err // unknown skill: reject
	}
	if ws != agent.WorkspaceID {
		return domain.ErrCrossWorkspace
	}
	return a.repo.Agents.LinkSkill(ctx, agentID, skillID)
}

// DetachSkill removes a skill link from an agent.
func (a *App) DetachSkill(ctx context.Context, agentID, skillID identity.ID) error {
	return a.repo.Agents.UnlinkSkill(ctx, agentID, skillID)
}

// AttachMcp links an MCP server definition to an agent, rejecting definitions
// from another workspace and unknown (unprojected) definitions.
func (a *App) AttachMcp(ctx context.Context, agentID, mcpID identity.ID) error {
	agent, err := a.repo.Agents.GetUnscoped(ctx, agentID)
	if err != nil {
		return err
	}
	ws, err := a.repo.Projections.McpWorkspace(ctx, mcpID)
	if err != nil {
		return err // unknown definition: reject
	}
	if ws != agent.WorkspaceID {
		return domain.ErrCrossWorkspace
	}
	return a.repo.Agents.LinkMcp(ctx, agentID, mcpID)
}

// DetachMcp removes an MCP definition link from an agent.
func (a *App) DetachMcp(ctx context.Context, agentID, mcpID identity.ID) error {
	return a.repo.Agents.UnlinkMcp(ctx, agentID, mcpID)
}

// AgentMcpServers returns an agent's attached MCP server *definitions*
// (hydrated from Catalog) so the Runner can bridge them as tools (task 5.5).
// The agent service owns the attachment; Catalog owns the definitions. When
// the agent has no attachments, no Catalog URL is configured, or hydration
// fails, it degrades to an empty list rather than failing the run setup.
func (a *App) AgentMcpServers(ctx context.Context, agentID identity.ID) ([]resources.McpServer, error) {
	agent, err := a.repo.Agents.GetUnscoped(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if len(agent.McpIDs) == 0 || a.catalog == nil {
		return []resources.McpServer{}, nil
	}
	out, err := a.catalog.FetchMcpServers(ctx, agent.McpIDs)
	if err != nil {
		a.log.Warn("mcp definition hydration failed", "agent", agent.ID, "error", err)
		return []resources.McpServer{}, nil
	}
	return out, nil
}
