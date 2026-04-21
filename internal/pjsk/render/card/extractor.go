package card

import (
	"haruki-cloud/internal/pjsk/queryrule"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func extractByRules(text string, rules []queryrule.StringRule) ExtractResult[string] {
	return extractByRulesWithOptions(text, rules, true)
}

func extractByRulesWithOptions(text string, rules []queryrule.StringRule, allowSingleRuneNonASCII bool) ExtractResult[string] {
	for _, rule := range rules {
		if !allowSingleRuneNonASCII && rule.SingleRuneNonASCII {
			continue
		}
		if rule.Re.MatchString(text) {
			remaining := rule.Re.ReplaceAllString(text, "")
			return ExtractResult[string]{Value: rule.Value, Remaining: strings.TrimSpace(remaining), Found: true}
		}
	}
	return ExtractResult[string]{Remaining: text}
}

func extractIntSliceByRules(text string, rules []queryrule.IntSliceRule) ExtractResult[[]int] {
	for _, rule := range rules {
		if rule.Re.MatchString(text) {
			remaining := rule.Re.ReplaceAllString(text, "")
			return ExtractResult[[]int]{
				Value:     append([]int(nil), rule.Values...),
				Remaining: strings.TrimSpace(remaining),
				Found:     true,
			}
		}
	}
	return ExtractResult[[]int]{Remaining: text}
}

func NewExtractor(nicknames map[string]int) *Extractor {
	return &Extractor{
		nicknames:    sanitizeCharacterNicknames(nicknames),
		banNicknames: sanitizeCharacterNicknames(defaultNicknames),
	}
}

func (e *Extractor) ExtractCharacter(text string) ExtractResult[int] {
	textLower := strings.ToLower(text)
	bestNickname := ""
	bestID := 0
	bestIndex := -1
	for nickname, id := range e.nicknames {
		if index := strings.Index(textLower, nickname); index != -1 && len(nickname) > len(bestNickname) {
			bestNickname = nickname
			bestID = id
			bestIndex = index
		}
	}
	if bestNickname == "" {
		return ExtractResult[int]{Remaining: text}
	}
	re := regexp.MustCompile("(?i)" + regexp.QuoteMeta(bestNickname))
	remaining := re.ReplaceAllString(text, "")
	prefixTightlyJoin := false
	suffixTightlyJoin := false
	if bestIndex > 0 {
		if prev, _ := utf8.DecodeLastRuneInString(text[:bestIndex]); prev != utf8.RuneError && !isLooseSeparator(prev) {
			prefixTightlyJoin = true
		}
	}
	matchEnd := bestIndex + len(bestNickname)
	if matchEnd >= 0 && matchEnd < len(text) {
		if next, _ := utf8.DecodeRuneInString(text[matchEnd:]); next != utf8.RuneError && !isLooseSeparator(next) {
			suffixTightlyJoin = true
		}
	}
	return ExtractResult[int]{
		Value:             bestID,
		Remaining:         strings.TrimSpace(remaining),
		Found:             true,
		PrefixTightlyJoin: prefixTightlyJoin,
		SuffixTightlyJoin: suffixTightlyJoin,
	}
}

var cardRarityAliasGroups = map[string][]string{
	"rarity_4": {"4x", "4"},
	"rarity_3": {"3x", "3"},
	"rarity_2": {"2x", "2"},
	"rarity_1": {"1x", "1"},
}

var rarityRules = queryrule.BuildStringRulesFromGroups(queryrule.MergeAliasGroups(
	queryrule.RarityAliasGroups(),
	cardRarityAliasGroups,
))

func (e *Extractor) ExtractRarity(text string) ExtractResult[string] {
	return extractByRules(text, rarityRules)
}

var cardAttrAliasGroups = map[string][]string{
	"happy": {"黄"},
}

var attrRules = queryrule.BuildStringRulesFromGroups(queryrule.MergeAliasGroups(
	queryrule.AttributeAliasGroups(),
	cardAttrAliasGroups,
))

func (e *Extractor) ExtractAttribute(text string) ExtractResult[string] {
	return extractByRules(text, attrRules)
}

func (e *Extractor) ExtractAttributeWithoutSingleRune(text string) ExtractResult[string] {
	return extractByRulesWithOptions(text, attrRules, false)
}

var skillRules = queryrule.BuildStringRulesFromGroups(queryrule.SkillAliasGroups())

var detailSkillRules = queryrule.BuildIntSliceRules(map[string][]int{
	"大分": {4},
	"p分": {11},
	"判分": {13},
	"血分": {12},
	"组分": {15, 16, 17, 18, 19},
	"团分": {15, 16, 17, 18, 19},
})

func (e *Extractor) ExtractSkill(text string) ExtractResult[string] {
	return extractByRules(text, skillRules)
}

func (e *Extractor) ExtractDetailedSkillIDs(text string) ExtractResult[[]int] {
	return extractIntSliceByRules(text, detailSkillRules)
}

var cardSupplyAliasGroups = map[string][]string{
	SupplyCollab:  {"联动限定", "联动", "collab"},
	SupplyBFes:    {"bfes限定", "bfes"},
	SupplyCFes:    {"cfes限定", "cfes"},
	SupplyWL:      {"worldlink限定", "wl限定"},
	SupplyLimited: {"期间限定"},
	SupplyNormal:  {"非限定"},
}

var supplyRules = queryrule.BuildStringRulesFromGroups(queryrule.MergeAliasGroups(
	queryrule.SupplyAliasGroups(),
	cardSupplyAliasGroups,
))

func (e *Extractor) ExtractSupply(text string) ExtractResult[string] {
	return extractByRules(text, supplyRules)
}

var unitRules = queryrule.BuildStringRules(map[string]string{
	"light_sound":    "light_sound",
	"ln":             "light_sound",
	"idol":           "idol",
	"mmj":            "idol",
	"street":         "street",
	"vbs":            "street",
	"theme_park":     "theme_park",
	"ws":             "theme_park",
	"school_refusal": "school_refusal",
	"25h":            "school_refusal",
	"25时":            "school_refusal",
	"25":             "school_refusal",
	"piapro":         "piapro",
	"vs":             "piapro",
	"v":              "piapro",
})

func (e *Extractor) ExtractUnit(text string) ExtractResult[string] {
	return extractByRules(text, unitRules)
}

var vsUnitRules = queryrule.BuildStringRules(map[string]string{
	"lnvs":  "light_sound",
	"lnv":   "light_sound",
	"mmjvs": "idol",
	"mmjv":  "idol",
	"vbsvs": "street",
	"vbsv":  "street",
	"wsvs":  "theme_park",
	"wsv":   "theme_park",
	"25hvs": "school_refusal",
	"25hv":  "school_refusal",
	"25时vs": "school_refusal",
	"25时v":  "school_refusal",
	"25vs":  "school_refusal",
	"25v":   "school_refusal",
})

func (e *Extractor) ExtractVSUnit(text string) ExtractResult[string] {
	return extractByRules(text, vsUnitRules)
}

var ocUnitRules = queryrule.BuildStringRules(map[string]string{
	"lnoc":  "light_sound",
	"纯ln":   "light_sound",
	"mmjoc": "idol",
	"纯mmj":  "idol",
	"vbsoc": "street",
	"纯vbs":  "street",
	"wsoc":  "theme_park",
	"纯ws":   "theme_park",
	"25hoc": "school_refusal",
	"纯25h":  "school_refusal",
	"25时oc": "school_refusal",
	"纯25时":  "school_refusal",
	"25oc":  "school_refusal",
	"纯25":   "school_refusal",
	"vsoc":  "piapro",
	"voc":   "piapro",
	"纯vs":   "piapro",
	"纯v":    "piapro",
})

func (e *Extractor) ExtractOCUnit(text string) ExtractResult[string] {
	return extractByRules(text, ocUnitRules)
}

func isLooseSeparator(ch rune) bool {
	switch ch {
	case ' ', '\t', '\n', '\r', ',', '，', '.', '。', '/', '\\', '-', '_', '+', '&', '＆', '|', '｜', '(', ')', '（', '）', '[', ']', '【', '】':
		return true
	default:
		return false
	}
}

var reEventID = regexp.MustCompile(`(?i)\bevent(\d+)\b`)

func (e *Extractor) ExtractEventID(text string) ExtractResult[int] {
	matches := reEventID.FindStringSubmatch(text)
	if len(matches) < 2 {
		return ExtractResult[int]{Remaining: text}
	}
	value, err := strconv.Atoi(matches[1])
	if err != nil || value <= 0 {
		return ExtractResult[int]{Remaining: text}
	}
	remaining := reEventID.ReplaceAllString(text, "")
	return ExtractResult[int]{Value: value, Remaining: strings.TrimSpace(remaining), Found: true}
}

func (e *Extractor) ExtractBanEvent(text string) ExtractResult[BanEventRef] {
	bestToken := ""
	best := BanEventRef{}
	for nickname, id := range e.banNicknames {
		for seq := 1; seq <= 9; seq++ {
			token := nickname + strconv.Itoa(seq)
			lowerText := strings.ToLower(text)
			index := strings.Index(lowerText, strings.ToLower(token))
			if index == -1 {
				continue
			}
			if !hasLooseBoundaries(text, index, index+len(token)) {
				continue
			}
			if len(token) <= len(bestToken) {
				continue
			}
			bestToken = token
			best = BanEventRef{CharacterID: id, Sequence: seq}
		}
	}
	if bestToken == "" {
		return ExtractResult[BanEventRef]{Remaining: text}
	}
	lowerText := strings.ToLower(text)
	index := strings.Index(lowerText, strings.ToLower(bestToken))
	remaining := strings.TrimSpace(text[:index] + text[index+len(bestToken):])
	return ExtractResult[BanEventRef]{Value: best, Remaining: remaining, Found: true}
}

func hasLooseBoundaries(text string, start, end int) bool {
	if start < 0 || end < start || end > len(text) {
		return false
	}
	if start > 0 {
		if prev, _ := utf8.DecodeLastRuneInString(text[:start]); prev != utf8.RuneError && !isLooseSeparator(prev) {
			return false
		}
	}
	if end < len(text) {
		if next, _ := utf8.DecodeRuneInString(text[end:]); next != utf8.RuneError && !isLooseSeparator(next) {
			return false
		}
	}
	return true
}

var (
	reYearFull  = regexp.MustCompile(`(20\d{2})年?`)
	reYearShort = regexp.MustCompile(`(\d{2})年`)
)

func (e *Extractor) ExtractYear(text string) ExtractResult[int] {
	if matches := reYearFull.FindStringSubmatch(text); len(matches) > 1 {
		year, _ := strconv.Atoi(matches[1])
		remaining := reYearFull.ReplaceAllString(text, "")
		return ExtractResult[int]{Value: year, Remaining: strings.TrimSpace(remaining), Found: true}
	}
	if matches := reYearShort.FindStringSubmatch(text); len(matches) > 1 {
		year, _ := strconv.Atoi("20" + matches[1])
		remaining := reYearShort.ReplaceAllString(text, "")
		return ExtractResult[int]{Value: year, Remaining: strings.TrimSpace(remaining), Found: true}
	}
	if strings.Contains(text, "去年") {
		remaining := strings.Replace(text, "去年", "", 1)
		return ExtractResult[int]{Value: time.Now().Year() - 1, Remaining: strings.TrimSpace(remaining), Found: true}
	}
	if strings.Contains(text, "前年") {
		remaining := strings.Replace(text, "前年", "", 1)
		return ExtractResult[int]{Value: time.Now().Year() - 2, Remaining: strings.TrimSpace(remaining), Found: true}
	}
	if strings.Contains(text, "今年") {
		remaining := strings.Replace(text, "今年", "", 1)
		return ExtractResult[int]{Value: time.Now().Year(), Remaining: strings.TrimSpace(remaining), Found: true}
	}
	return ExtractResult[int]{Remaining: text}
}
