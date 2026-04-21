package card

import (
	"unicode/utf8"

	"haruki-cloud/internal/pjsk/render/common"
)

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
		if isReservedCardAttributeAlias(normalized) {
			continue
		}
		if isReservedCardAttributeFragmentAlias(normalized) {
			continue
		}
		result[normalized] = value
	}
	return result
}

var reservedCardAttributeAliases = map[string]struct{}{
	"cute": {}, "可爱": {}, "粉花": {}, "粉": {}, "pink": {},
	"cool": {}, "帅气": {}, "蓝星": {}, "蓝": {}, "blue": {},
	"pure": {}, "纯真": {}, "纯洁": {}, "绿草": {}, "草": {}, "绿": {}, "green": {},
	"happy": {}, "快乐": {}, "橙心": {}, "橙": {}, "黄": {}, "orange": {},
	"mysterious": {}, "神秘": {}, "紫月": {}, "紫": {}, "purple": {},
}

func isReservedCardAttributeAlias(text string) bool {
	_, ok := reservedCardAttributeAliases[text]
	return ok
}

var reservedCardAttributeFragmentAliases = buildReservedCardAttributeFragmentAliases()

func buildReservedCardAttributeFragmentAliases() map[string]struct{} {
	result := make(map[string]struct{})
	for alias := range reservedCardAttributeAliases {
		if utf8.RuneCountInString(alias) <= 1 || isASCIIOnly(alias) {
			continue
		}
		for _, r := range alias {
			result[string(r)] = struct{}{}
		}
	}
	return result
}

func isReservedCardAttributeFragmentAlias(text string) bool {
	if utf8.RuneCountInString(text) != 1 || isASCIIOnly(text) {
		return false
	}
	_, ok := reservedCardAttributeFragmentAliases[text]
	return ok
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

func isASCIIOnly(text string) bool {
	for _, ch := range text {
		if ch > 127 {
			return false
		}
	}
	return text != ""
}
