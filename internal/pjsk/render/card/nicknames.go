package card

import "haruki-cloud/internal/pjsk/render/common"

// Package-local aliases for readability at the many in-package call sites.
// The canonical definitions live in render/common.
var (
	defaultNicknames       = common.DefaultNicknames
	cloneNicknames         = common.CloneNicknames
	normalizeNicknameQuery = common.NormalizeNicknameQuery
)

func sanitizeCharacterNicknames(items map[string]int) map[string]int {
	result := make(map[string]int, len(items))
	for key, value := range items {
		normalized := normalizeNicknameQuery(key)
		if normalized == "" || value <= 0 {
			continue
		}
		if isAllDigits(normalized) {
			continue
		}
		result[normalized] = value
	}
	return result
}

func DefaultCharacterNicknames() map[string]int {
	return cloneNicknames(defaultNicknames)
}

func ResolveDefaultCharacterNickname(query string) (int, bool) {
	target := normalizeNicknameQuery(query)
	if target == "" {
		return 0, false
	}
	for nickname, characterID := range defaultNicknames {
		if normalizeNicknameQuery(nickname) == target {
			return characterID, true
		}
	}
	return 0, false
}

func isAllDigits(text string) bool {
	if text == "" {
		return false
	}
	for _, ch := range text {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
