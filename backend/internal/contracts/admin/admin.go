// Package admin holds the shared-kernel sysadmin DTOs: audit entries, feature
// flags, and system health/KPI snapshots. Field names mirror the frontend's
// declared API contract (snake_case JSON).
package admin

import "github.com/aaks/server/internal/contracts/identity"

// ID re-exports the shared identifier for convenience within this domain.
type ID = identity.ID

// ISOTime re-exports the shared timestamp alias.
type ISOTime = identity.ISOTime

// AuditEntry mirrors frontend AuditEntry.
type AuditEntry struct {
	ID         ID         `json:"id"`
	Actor      AuditActor `json:"actor"`
	Action     string     `json:"action"`
	ActionKind string     `json:"action_kind,omitempty"`
	Target     string     `json:"target,omitempty"`
	CreatedAt  ISOTime    `json:"created_at"`
	IP         string     `json:"ip,omitempty"`
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
	Pct    int    `json:"pct"`    // 0..100
	Status string `json:"status"` // ok | warn | down
}

// SystemHealth mirrors frontend SystemHealth.
type SystemHealth struct {
	Services []ServiceHealth `json:"services"`
}

// SystemKpis mirrors frontend SystemKpis.
type SystemKpis struct {
	Organizations    int  `json:"organizations"`
	OrgsDelta        *int `json:"orgs_delta,omitempty"`
	Workspaces       int  `json:"workspaces"`
	ActiveUsers24h   int  `json:"active_users_24h"`
	ActiveUsersDelta *int `json:"active_users_delta,omitempty"`
	OpenSeats        int  `json:"open_seats"`
	OpenSeatsDelta   *int `json:"open_seats_delta,omitempty"`
}
