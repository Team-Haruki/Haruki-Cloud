package sekai

import (
	"fmt"
	rendercard "haruki-cloud/internal/pjsk/render/card"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func validateDeckUniqueIDs(values []int, limit int, label string) error {
	if len(values) == 0 {
		return fmt.Errorf("%s不能为空", label)
	}
	if len(values) > limit {
		return fmt.Errorf("%s数量不能超过%d个", label, limit)
	}
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			return fmt.Errorf("%s不能重复", label)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func resolveDeckStrategyField(field string, index int, fields []string, keywords []string) (string, int) {
	for _, keyword := range keywords {
		if strings.Contains(field, keyword) {
			if strategy := resolveDeckStrategyValue(field); strategy != "" {
				return strategy, 1
			}
			if field == keyword && index+1 < len(fields) {
				if strategy := resolveDeckStrategyValue(fields[index+1]); strategy != "" {
					return strategy, 2
				}
			}
		}
	}
	return "", 0
}

func resolveDeckStrategyValue(raw string) string {
	switch {
	case containsDeckKeyword(raw, deckMaxKeywords):
		return "max"
	case containsDeckKeyword(raw, deckMinKeywords):
		return "min"
	case containsDeckKeyword(raw, deckAverageKeywords):
		return "average"
	default:
		return ""
	}
}

func resolveDeckCharacterToken(token string) (int, string) {
	raw := strings.TrimSpace(token)
	if raw == "" {
		return 0, ""
	}
	if charID, ok := rendercard.ResolveDefaultCharacterNickname(raw); ok {
		return charID, ""
	}
	return 0, raw
}

func extractDeckKeywordNumber(field string, keywords []string, parserFn func(string) (int, error)) (int, bool, error) {
	for _, keyword := range keywords {
		if !strings.Contains(field, keyword) {
			continue
		}
		raw := strings.TrimSpace(strings.Replace(field, keyword, "", 1))
		if raw == "" {
			return 0, false, nil
		}
		value, err := parserFn(strings.TrimSuffix(raw, "%"))
		return value, true, err
	}
	return 0, false, nil
}

func extractDeckKeywordNumberFromFields(fields []string, index int, keywords []string, parserFn func(string) (int, error)) (int, int, bool, error) {
	if index < 0 || index >= len(fields) {
		return 0, 0, false, nil
	}
	if value, ok, err := extractDeckKeywordNumber(fields[index], keywords, parserFn); ok {
		return value, 1, true, err
	}
	if index+1 >= len(fields) {
		return 0, 0, false, nil
	}

	current := strings.TrimSpace(fields[index])
	next := strings.TrimSpace(fields[index+1])
	for _, keyword := range keywords {
		switch {
		case current == keyword:
			value, err := parserFn(strings.TrimSuffix(next, "%"))
			return value, 2, true, err
		case next == keyword:
			value, err := parserFn(strings.TrimSuffix(current, "%"))
			return value, 2, true, err
		}
	}
	return 0, 0, false, nil
}

func parseDeckInt(raw string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(raw))
}

func parseDeckBonusInt(raw string) (int, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimSuffix(cleaned, "%")
	cleaned = strings.TrimSuffix(cleaned, "％")
	cleaned = strings.TrimSpace(strings.ReplaceAll(cleaned, "加成", ""))
	return strconv.Atoi(strings.TrimSpace(cleaned))
}

func looksLikeDeckNumericToken(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	for _, ch := range raw {
		if ch >= '0' && ch <= '9' {
			return true
		}
	}
	return false
}

func containsDeckKeyword(args string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(args, keyword) {
			return true
		}
	}
	return false
}

func removeDeckKeywordOnce(args string, keywords []string) string {
	for _, keyword := range keywords {
		if strings.Contains(args, keyword) {
			return normalizeDeckSpaces(strings.Replace(args, keyword, "", 1))
		}
	}
	return normalizeDeckSpaces(args)
}

func normalizeDeckSpaces(args string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(strings.TrimSpace(args)), " "))
}

func deckLeadingDigits(raw string) int {
	var digits strings.Builder
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			break
		}
		digits.WriteRune(ch)
	}
	if digits.Len() == 0 {
		return 0
	}
	value, err := strconv.Atoi(digits.String())
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func deckCharacterUnit(charID int) string {
	switch {
	case charID >= 1 && charID <= 4:
		return "light_sound"
	case charID >= 5 && charID <= 8:
		return "idol"
	case charID >= 9 && charID <= 12:
		return "street"
	case charID >= 13 && charID <= 16:
		return "theme_park"
	case charID >= 17 && charID <= 20:
		return "school_refusal"
	case charID >= 21 && charID <= 26:
		return "piapro"
	default:
		return ""
	}
}

func newDeckUnitAliasRules() []deckAliasRule {
	aliases := make(map[string]string, len(educationAreaUnitAliases))
	for alias, unit := range educationAreaUnitAliases {
		aliases[alias] = unit
	}

	keys := make([]string, 0, len(aliases))
	for key := range aliases {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})

	rules := make([]deckAliasRule, 0, len(keys))
	for _, key := range keys {
		pattern := "(?i)"
		if isDeckASCIIAlias(key) {
			pattern += `\b` + regexp.QuoteMeta(key) + `\b`
		} else {
			pattern += regexp.QuoteMeta(key)
		}
		rules = append(rules, deckAliasRule{
			re:   regexp.MustCompile(pattern),
			unit: aliases[key],
		})
	}
	return rules
}

func isDeckASCIIAlias(raw string) bool {
	for _, ch := range raw {
		if ch > 127 {
			return false
		}
	}
	return true
}

func intPtr(value int) *int {
	return &value
}
