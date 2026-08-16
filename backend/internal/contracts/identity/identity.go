// Package identity holds the shared-kernel identity types: base identifiers,
// multi-tenant enumerations, and the User / signup-request DTOs. Field names
// mirror the frontend's declared API contract (snake_case JSON).
package identity

import "time"

// ID is a UUID string identifier.
type ID = string

// ISOTime values are RFC3339 timestamps; time.Time marshals to RFC3339 by default.
type ISOTime = time.Time

// ── Enumerations (mirror frontend/src/api/types.ts) ────────────────────────

// RepoType is a project repository source kind: "path" | "url".
type RepoType string

const (
	RepoTypePath = RepoType("path")
	RepoTypeURL  = RepoType("url")
)

// Provider is an LLM provider: "openai" | "anthropic" | "gemini" | "glm".
type Provider string

// Role is a workspace membership role: "owner" | "admin" | "member".
type Role string

const (
	RoleOwner  = Role("owner")
	RoleAdmin  = Role("admin")
	RoleMember = Role("member")
)

// MemberStatus is a membership lifecycle state: "active" | "invited" | "suspended".
type MemberStatus string

const (
	MemberActive    = MemberStatus("active")
	MemberInvited   = MemberStatus("invited")
	MemberSuspended = MemberStatus("suspended")
)

// Plan is an organization plan: "free" | "team" | "pro" | "enterprise".
type Plan string

const (
	PlanFree       = Plan("free")
	PlanTeam       = Plan("team")
	PlanPro        = Plan("pro")
	PlanEnterprise = Plan("enterprise")
)

// OrgStatus is an organization state: "active" | "trial" | "suspended".
type OrgStatus string

const (
	OrgActive    = OrgStatus("active")
	OrgTrial     = OrgStatus("trial")
	OrgSuspended = OrgStatus("suspended")
)

// SignupState is a signup request state: "pending" | "approved" | "declined".
type SignupState string

const (
	SignupPending  = SignupState("pending")
	SignupApproved = SignupState("approved")
	SignupDeclined = SignupState("declined")
)

// ── Identity DTOs ──────────────────────────────────────────────────────────

// User mirrors frontend User (identity + role in the active workspace).
type User struct {
	ID           ID      `json:"id"`
	Name         string  `json:"name"`
	Email        string  `json:"email"`
	Avatar       string  `json:"avatar,omitempty"`
	Role         Role    `json:"role"`
	IsSuperadmin *bool   `json:"is_superadmin,omitempty"`
	CreatedAt    ISOTime `json:"created_at"`
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

// ProviderKey exposes provider metadata only — the API key never leaves Settings.
type ProviderKey struct {
	Provider  Provider `json:"provider"`
	CreatedAt ISOTime  `json:"created_at"`
}
