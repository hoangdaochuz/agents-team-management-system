package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
)

// Skill is the skill aggregate (wire DTO as domain type, D7).
type Skill = resources.Skill

// SkillCreate carries the fields of a new skill (domain value object; JSON
// decoding happens in the interface layer).
type SkillCreate struct {
	Name        string
	Description string
	BodyMd      string
	Trigger     string
}

// SkillUpdate carries the optional fields of a skill patch; nil means "leave
// unchanged".
type SkillUpdate struct {
	Name          *string
	Description   *string
	BodyMd        *string
	ResourcesPath *string
	Enabled       *bool
	Trigger       *string
	StepCount     *int
}

// SkillRepository is the skill aggregate port.
type SkillRepository interface {
	List(ctx context.Context, ws []identity.ID) ([]Skill, error)
	Get(ctx context.Context, id identity.ID, ws []identity.ID) (Skill, error)
	Create(ctx context.Context, workspaceID identity.ID, in SkillCreate) (Skill, error)
	Update(ctx context.Context, id identity.ID, ws []identity.ID, in SkillUpdate) (Skill, error)
	Delete(ctx context.Context, id identity.ID, ws []identity.ID) error
	ListByWorkspace(ctx context.Context, workspaceID identity.ID) ([]Skill, error)
	SetEnabled(ctx context.Context, workspaceID, id identity.ID, enabled bool) (Skill, error)
}