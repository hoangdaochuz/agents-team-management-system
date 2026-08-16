package domain

import (
	"context"
	"time"

	"github.com/aaks/server/internal/contracts/identity"
)

// SessionRepository is the session aggregate port. Sessions are httpOnly
// cookies (the SPA sends no auth header); the token is the only handle.
type SessionRepository interface {
	Create(ctx context.Context, userID identity.ID, ttl time.Duration) (string, error)
	User(ctx context.Context, token string) (User, error)
	Delete(ctx context.Context, token string) error
	CountActiveUsers24h(ctx context.Context) (int, error)
}