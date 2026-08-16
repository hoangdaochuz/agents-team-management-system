package application

import (
	"context"
	"errors"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/orgs/internal/domain"
)

// RequireMember verifies active membership in the workspace, returning the
// caller's role (ErrNotMember when absent).
func (a *App) RequireMember(ctx context.Context, userID, workspaceID identity.ID) (identity.Role, error) {
	role, err := a.repo.Members.UserRoleIn(ctx, userID, workspaceID)
	if errors.Is(err, domain.ErrNotFound) {
		return "", domain.ErrNotMember
	}
	return role, err
}

// RequireAdmin is RequireMember plus an admin/owner role gate.
func (a *App) RequireAdmin(ctx context.Context, userID, workspaceID identity.ID) error {
	role, err := a.RequireMember(ctx, userID, workspaceID)
	if err != nil {
		return err
	}
	if role != identity.RoleOwner && role != identity.RoleAdmin {
		return domain.ErrForbidden
	}
	return nil
}

// ListMembers returns the active + invited members of a workspace.
func (a *App) ListMembers(ctx context.Context, workspaceID identity.ID) ([]domain.Member, error) {
	return a.repo.Members.List(ctx, workspaceID)
}

// UpdateRole changes a member's role, reverting a demotion that would leave
// the workspace without owners (last-owner invariant).
func (a *App) UpdateRole(ctx context.Context, workspaceID, memberID identity.ID, role identity.Role) (domain.Member, error) {
	if role != identity.RoleOwner && role != identity.RoleAdmin && role != identity.RoleMember {
		return domain.Member{}, errInvalidRole
	}
	target, err := a.repo.Members.SetRole(ctx, workspaceID, memberID, role)
	if err != nil {
		return domain.Member{}, err
	}
	if target.Role == identity.RoleOwner {
		return target, nil
	}
	n, err := a.repo.Members.OwnerCount(ctx, workspaceID)
	if err != nil {
		return domain.Member{}, err
	}
	if n == 0 {
		if _, err := a.repo.Members.SetRole(ctx, workspaceID, target.ID, identity.RoleOwner); err != nil {
			return domain.Member{}, err
		}
		return domain.Member{}, domain.ErrLastOwner
	}
	return target, nil
}

// Remove removes a member, refusing to remove the last owner.
func (a *App) Remove(ctx context.Context, actorID, workspaceID, memberID identity.ID) error {
	if memberID == actorID {
		return ErrSelfRemoval
	}
	members, err := a.repo.Members.List(ctx, workspaceID)
	if err != nil {
		return err
	}
	var target *domain.Member
	for i := range members {
		if members[i].ID == memberID {
			target = &members[i]
		}
	}
	if target == nil {
		return domain.ErrNotFound
	}
	if target.Role == identity.RoleOwner {
		n, err := a.repo.Members.OwnerCount(ctx, workspaceID)
		if err != nil {
			return err
		}
		if n <= 1 {
			return domain.ErrLastOwner
		}
	}
	return a.repo.Members.Remove(ctx, workspaceID, memberID)
}

var (
	errInvalidRole = errors.New("role must be owner, admin or member")
	// ErrSelfRemoval is returned when a member tries to remove themselves.
	ErrSelfRemoval = errors.New("cannot remove yourself")
)