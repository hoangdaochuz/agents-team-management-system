package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/admin"
	"github.com/aaks/server/internal/contracts/identity"
)

// AuditRow is an audit entry with its raw actor id and workspace id (unexposed
// in the wire DTO).
type AuditRow struct {
	admin.AuditEntry
	ActorID     identity.ID
	WorkspaceID identity.ID
}

// AuditRepository is the audit-log aggregate port. Entries are persisted from
// audit.recorded events emitted by other services.
type AuditRepository interface {
	List(ctx context.Context, workspaceID identity.ID, kind string) ([]AuditRow, error)
	ListSystem(ctx context.Context, limit int) ([]AuditRow, error)
	Append(ctx context.Context, workspaceID identity.ID, actorName string, actorID identity.ID, action, kind, target, ip string) error
	Count24h(ctx context.Context, workspaceID identity.ID) (int, error)
}