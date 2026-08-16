package messaging

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/services/admin/internal/application"
)

// AuditRecordedHandler appends workspace admin actions emitted by other
// services to the audit log (audit.recorded). Idempotent: every event appends
// a fresh row, and at-least-once redelivery is tolerated by the append-only
// design.
type AuditRecordedHandler struct{ App *application.App }

func (h AuditRecordedHandler) Topics() []string { return []string{events.TopicAuditRecorded} }

func (h AuditRecordedHandler) Handle(ctx context.Context, msg events.EventEnvelope) error {
	return events.Forward(ctx, msg, h.App.RecordAudit)
}