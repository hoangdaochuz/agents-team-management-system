// Package workspaces holds the shared-kernel organization/workspace DTOs and
// the Gateway-composed Session. Field names mirror the frontend's declared API
// contract (snake_case JSON).
package workspaces

import "github.com/aaks/server/internal/contracts/identity"

// ID re-exports the shared identifier for convenience within this domain.
type ID = identity.ID

// ISOTime re-exports the shared timestamp alias.
type ISOTime = identity.ISOTime

// Session mirrors frontend Session, assembled by the Gateway from Auth (user)
// and Orgs (memberships).
type Session struct {
	User              identity.User `json:"user"`
	Workspaces        []Workspace   `json:"workspaces"`
	ActiveWorkspaceID ID            `json:"active_workspace_id,omitempty"`
}

// Organization mirrors frontend Organization.
type Organization struct {
	ID             ID                 `json:"id"`
	Name           string             `json:"name"`
	Subdomain      string             `json:"subdomain,omitempty"`
	Plan           identity.Plan      `json:"plan"`
	WorkspaceCount int                `json:"workspace_count"`
	SeatsUsed      int                `json:"seats_used"`
	SeatsTotal     int                `json:"seats_total"`
	Status         identity.OrgStatus `json:"status"`
	CreatedAt      ISOTime            `json:"created_at"`
}

// Workspace mirrors frontend Workspace. agent_count / open_task_count are
// derived counts composed by the Gateway; role is the current user's role.
type Workspace struct {
	ID            ID            `json:"id"`
	Name          string        `json:"name"`
	RepoSource    string        `json:"repo_source,omitempty"`
	DefaultBranch string        `json:"default_branch,omitempty"`
	Glyph         string        `json:"glyph,omitempty"`
	Description   string        `json:"description,omitempty"`
	AgentCount    *int          `json:"agent_count,omitempty"`
	OpenTaskCount *int          `json:"open_task_count,omitempty"`
	Role          identity.Role `json:"role"`
	CreatedAt     ISOTime       `json:"created_at"`
}

// Member mirrors frontend Member.
type Member struct {
	ID               ID                    `json:"id"`
	User             MemberUser            `json:"user"`
	Role             identity.Role         `json:"role"`
	Status           identity.MemberStatus `json:"status"`
	LastActiveAt     *ISOTime              `json:"last_active_at,omitempty"`
	IsServiceAccount *bool                 `json:"is_service_account,omitempty"`
}

// MemberUser is the nested user identity in a Member.
type MemberUser struct {
	ID    ID     `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Invite mirrors frontend Invite.
type Invite struct {
	ID          ID            `json:"id"`
	Email       string        `json:"email"`
	Name        string        `json:"name,omitempty"`
	Role        identity.Role `json:"role"`
	RequestedAt ISOTime       `json:"requested_at"`
}
