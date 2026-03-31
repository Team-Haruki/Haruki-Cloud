package provider

import "haruki-cloud/internal/pjsk/render/masterdata"

// CharacterProvider exposes character and game-character-unit queries.
type CharacterProvider interface {
	GetByID(id int) (*masterdata.Character, error)
	GetColorCode(id int) (string, bool)
	GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error)
}
