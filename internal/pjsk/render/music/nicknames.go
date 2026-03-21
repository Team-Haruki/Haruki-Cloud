package music

import "haruki-cloud/internal/pjsk/render/assets"

var defaultNicknames = buildDefaultNicknames()

func buildDefaultNicknames() map[string]int {
	result := make(map[string]int, len(assets.CharacterIDToNickname))
	for characterID, nickname := range assets.CharacterIDToNickname {
		result[nickname] = characterID
	}
	return result
}

func cloneNicknames(src map[string]int) map[string]int {
	result := make(map[string]int, len(src))
	for key, value := range src {
		result[key] = value
	}
	return result
}
