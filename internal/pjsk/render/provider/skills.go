package provider

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

// SkillProvider exposes skill masterdata queries.
type SkillProvider interface {
	GetByID(ctx context.Context, id int) (*masterdata.Skill, error)
	FormatDescription(ctx context.Context, skill *masterdata.Skill, cardCharacterID int) string
}
