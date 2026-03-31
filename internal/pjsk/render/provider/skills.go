package provider

import "haruki-cloud/internal/pjsk/render/masterdata"

// SkillProvider exposes skill masterdata queries.
type SkillProvider interface {
	GetByID(id int) (*masterdata.Skill, error)
	FormatDescription(skill *masterdata.Skill, cardCharacterID int) string
}
