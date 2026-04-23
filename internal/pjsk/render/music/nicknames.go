package music

import (
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
)

var defaultBanCharacterNicknames = buildDefaultBanCharacterNicknames()

func buildDefaultBanCharacterNicknames() map[string]int {
	result := make(map[string]int, len(assets.CharacterIDToNickname))
	for characterID, nickname := range assets.CharacterIDToNickname {
		result[nickname] = characterID
	}
	return result
}

// Package-local alias for readability at the in-package call sites.
var cloneNicknames = common.CloneNicknames
