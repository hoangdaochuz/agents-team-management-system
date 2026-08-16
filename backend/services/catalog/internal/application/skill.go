package application

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
	"github.com/aaks/server/services/catalog/internal/domain"
)

// ListSkills returns the skills visible in the caller's workspace set, newest
// first (empty set yields an empty list — fail closed).
func (a *App) ListSkills(ctx context.Context, ws []identity.ID) ([]resources.Skill, error) {
	out, err := a.repo.Skills.List(ctx, ws)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []resources.Skill{}
	}
	return out, nil
}

// GetSkill returns one skill scoped to the caller's workspace set.
func (a *App) GetSkill(ctx context.Context, id identity.ID, ws []identity.ID) (resources.Skill, error) {
	return a.repo.Skills.Get(ctx, id, ws)
}

// CreateSkill inserts a skill and publishes skill.created after the commit.
func (a *App) CreateSkill(ctx context.Context, workspaceID identity.ID, in domain.SkillCreate) (resources.Skill, error) {
	var out resources.Skill
	err := a.uow.Do(ctx, func(tx *Tx) error {
		var err error
		out, err = tx.Skills.Create(ctx, workspaceID, in)
		return err
	})
	if err != nil {
		return resources.Skill{}, err
	}
	a.pub.Publish(ctx, events.TopicSkillCreated, events.SkillCreatedData{
		SkillID: out.ID, WorkspaceID: out.WorkspaceID,
	}, out.WorkspaceID)
	return out, nil
}

// UpdateSkill patches a skill scoped to the caller's workspace set.
func (a *App) UpdateSkill(ctx context.Context, id identity.ID, ws []identity.ID, in domain.SkillUpdate) (resources.Skill, error) {
	return a.repo.Skills.Update(ctx, id, ws, in)
}

// DeleteSkill removes a skill and publishes skill.deleted after the commit.
func (a *App) DeleteSkill(ctx context.Context, id identity.ID, ws []identity.ID) error {
	err := a.uow.Do(ctx, func(tx *Tx) error {
		return tx.Skills.Delete(ctx, id, ws)
	})
	if err != nil {
		return err
	}
	a.pub.Publish(ctx, events.TopicSkillDeleted, events.SkillDeletedData{SkillID: id}, "")
	return nil
}

// ListWorkspaceSkills lists the skills of exactly one workspace (scoped path).
func (a *App) ListWorkspaceSkills(ctx context.Context, workspaceID identity.ID) ([]resources.Skill, error) {
	out, err := a.repo.Skills.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []resources.Skill{}
	}
	return out, nil
}

// SetSkillEnabled toggles the workspace-level enable state (scoped path).
func (a *App) SetSkillEnabled(ctx context.Context, workspaceID, id identity.ID, enabled bool) (resources.Skill, error) {
	return a.repo.Skills.SetEnabled(ctx, workspaceID, id, enabled)
}
