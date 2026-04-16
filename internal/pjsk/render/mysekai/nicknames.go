package mysekai

import (
	"strings"

	"haruki-cloud/internal/pjsk/render/common"
)

// Package-local aliases for readability at the many in-package call sites.
// The canonical definitions live in render/common.
var (
	defaultNicknames       = common.DefaultNicknames
	cloneNicknames         = common.CloneNicknames
	normalizeNicknameQuery = common.NormalizeNicknameQuery
)

func ResolveNicknameCharacterID(query string) (int, bool) {
	normalized := normalizeNicknameQuery(query)
	if normalized == "" {
		return 0, false
	}
	if characterID, ok := defaultNicknames[normalized]; ok {
		return characterID, true
	}
	for _, token := range strings.Fields(strings.ToLower(strings.TrimSpace(query))) {
		if characterID, ok := defaultNicknames[normalizeNicknameQuery(token)]; ok {
			return characterID, true
		}
	}
	return 0, false
}
