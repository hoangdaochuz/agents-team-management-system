package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
)

// User is a user row with its password hash and active flag (internal only).
// Its wire shape is the shared kernel DTO (D7: no divergence, so no private
// type is introduced).
type User struct {
	identity.User
	PasswordHash string
	Active       bool
}

// UserRepository is the user aggregate port (ISP: per-aggregate).
type UserRepository interface {
	GetByEmail(ctx context.Context, email string) (User, error)
	Get(ctx context.Context, id identity.ID) (User, error)
	Create(ctx context.Context, name, email, passwordHash string, superadmin bool) (identity.User, error)
	Activate(ctx context.Context, id identity.ID) error
	ActivateByEmail(ctx context.Context, email string) error
}