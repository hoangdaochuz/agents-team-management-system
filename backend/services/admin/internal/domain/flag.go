package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/admin"
)

// FeatureFlag is the sysadmin feature-flag aggregate (wire DTO as domain type,
// D7).
type FeatureFlag = admin.FeatureFlag

// FlagRepository is the feature-flag aggregate port.
type FlagRepository interface {
	List(ctx context.Context) ([]FeatureFlag, error)
	SetEnabled(ctx context.Context, key string, enabled bool) (FeatureFlag, error)
}
