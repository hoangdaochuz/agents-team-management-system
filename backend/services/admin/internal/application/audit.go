package application

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/admin/internal/domain"
)

// ListWorkspaceAudit returns a workspace's audit entries, newest first,
// optionally filtered by action kind.
func (a *App) ListWorkspaceAudit(ctx context.Context, workspaceID identity.ID, kind string) ([]domain.AuditRow, error) {
	return a.repo.Audit.List(ctx, workspaceID, kind)
}

// ListSystemAudit returns the most recent audit entries across workspaces
// (superadmin surface).
func (a *App) ListSystemAudit(ctx context.Context, limit int) ([]domain.AuditRow, error) {
	return a.repo.Audit.ListSystem(ctx, limit)
}

// CountAudit24h counts entries in a workspace over the last 24h (export stub).
func (a *App) CountAudit24h(ctx context.Context, workspaceID identity.ID) (int, error) {
	return a.repo.Audit.Count24h(ctx, workspaceID)
}

// RecordAudit persists a workspace audit event emitted by another service.
func (a *App) RecordAudit(ctx context.Context, d events.AuditRecordedData) error {
	return a.repo.Audit.Append(ctx, d.WorkspaceID, d.ActorName, d.ActorID, d.Action, d.ActionKind, d.Target, d.IP)
}