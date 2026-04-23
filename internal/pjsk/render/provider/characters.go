package provider

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

// CharacterProvider exposes character and game-character-unit queries.
type CharacterProvider interface {
	GetByID(ctx context.Context, id int) (*masterdata.Character, error)
	GetColorCode(ctx context.Context, id int) (string, bool)
	GetGameCharacterUnit(ctx context.Context, id int) (*masterdata.GameCharacterUnit, error)
}
