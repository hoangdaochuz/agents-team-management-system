package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/tasks"
)

// Project is the project aggregate (wire DTO as domain type, D7).
type Project = tasks.Project

// CreateInput is the body of POST /projects.
type CreateInput struct {
	Name          string            `json:"name"`
	RepoSource    string            `json:"repo_source"`
	RepoType      identity.RepoType `json:"repo_type"`
	DefaultBranch string            `json:"default_branch,omitempty"`
}

// UpdateInput applies non-nil fields of the body of PUT /projects.
type UpdateInput struct {
	Name          *string            `json:"name,omitempty"`
	RepoSource    *string            `json:"repo_source,omitempty"`
	RepoType      *identity.RepoType `json:"repo_type,omitempty"`
	DefaultBranch *string            `json:"default_branch,omitempty"`
	ClonedPath    *string            `json:"cloned_path,omitempty"`
}

// ProjectRepository is the project aggregate port. Every read is scoped to the
// caller's workspace set and every mutation rejects rows outside it, so a
// tenant can never observe or touch another tenant's projects.
type ProjectRepository interface {
	List(ctx context.Context, ws []identity.ID) ([]Project, error)
	Get(ctx context.Context, id identity.ID, ws []identity.ID) (Project, error)
	Create(ctx context.Context, workspaceID identity.ID, in CreateInput) (Project, error)
	Update(ctx context.Context, id identity.ID, ws []identity.ID, in UpdateInput) (Project, error)
	Delete(ctx context.Context, id identity.ID, ws []identity.ID) error
}