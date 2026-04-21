package parser

import (
	"haruki-cloud/internal/pjsk/queryrule"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func extractByRules(text string, rules []queryrule.StringRule) ExtractResult[string] {
	for _, rule := range rules {
		if rule.Re.MatchString(text) {
			remaining := rule.Re.ReplaceAllString(text, "")
			return ExtractResult[string]{Value: rule.Value, Remaining: strings.TrimSpace(remaining), Found: true}
		}
	}
	return ExtractResult[string]{Value: "", Remaining: text, Found: false}
}

// Extractor 通用特征提取器 — see types.go for struct definition.

var supportedRegions = []string{"jp", "en", "cn", "tw", "kr"}

// NewExtractor 创建提取器
func NewExtractor(nicknames map[string]int) *Extractor {
	return &Extractor{
		nicknames: nicknames,
	}
}

// ExtractResult — see types.go for struct definition.

// ExtractCharacter 提取角色 ID
func (e *Extractor) ExtractCharacter(text string) ExtractResult[int] {
	textLower := strings.ToLower(text)

	var bestNickname string
	var bestID int

	for nickname, id := range e.nicknames {
		if strings.Contains(textLower, nickname) {
			if len(nickname) > len(bestNickname) {
				bestNickname = nickname
				bestID = id
			}
		}
	}

	if bestNickname != "" {
		re := regexp.MustCompile("(?i)" + regexp.QuoteMeta(bestNickname))
		remaining := re.ReplaceAllString(text, "")
		return ExtractResult[int]{Value: bestID, Remaining: strings.TrimSpace(remaining), Found: true}
	}

	return ExtractResult[int]{Value: 0, Remaining: text, Found: false}
}

// --- 稀有度提取 ---

var rarityRules = queryrule.BuildStringRulesFromGroups(queryrule.RarityAliasGroups())

func (e *Extractor) ExtractRarity(text string) ExtractResult[string] {
	return extractByRules(text, rarityRules)
}

func (e *Extractor) ExtractRegionPrefix(text string) ExtractResult[string] {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "/") {
		return ExtractResult[string]{Value: "", Remaining: text, Found: false}
	}
	afterSlash := trimmed[1:]
	lower := strings.ToLower(afterSlash)
	for _, region := range supportedRegions {
		if strings.HasPrefix(lower, region) {
			nextIdx := len(region)
			if len(afterSlash) > nextIdx && isAsciiLetter(afterSlash[nextIdx]) {
				continue
			}
			afterRegion := afterSlash[nextIdx:]
			afterRegion = strings.TrimLeftFunc(afterRegion, func(r rune) bool {
				return r == ' ' || r == '\t' || r == '/'
			})
			remaining := "/" + afterRegion
			return ExtractResult[string]{Value: region, Remaining: remaining, Found: true}
		}
	}
	return ExtractResult[string]{Value: "", Remaining: text, Found: false}
}

func isAsciiLetter(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

// --- 属性提取 ---

var attrRules = queryrule.BuildStringRulesFromGroups(queryrule.AttributeAliasGroups())

func (e *Extractor) ExtractAttribute(text string) ExtractResult[string] {
	return extractByRules(text, attrRules)
}

// --- 技能提取 ---

var parserSkillAliasGroups = map[string][]string{
	"perfect_score_up": {"p分"},
	"great_score_up":   {"大分"},
	"judgment_up":      {"判分"},
}

var skillRules = queryrule.BuildStringRulesFromGroups(queryrule.MergeAliasGroups(
	queryrule.SkillAliasGroups(),
	parserSkillAliasGroups,
))

func (e *Extractor) ExtractSkill(text string) ExtractResult[string] {
	return extractByRules(text, skillRules)
}

// --- 限定类型提取 ---

var supplyRules = queryrule.BuildStringRulesFromGroups(queryrule.SupplyAliasGroups())

func (e *Extractor) ExtractSupply(text string) ExtractResult[string] {
	return extractByRules(text, supplyRules)
}

// --- 区服提取 ---

var reRegion = regexp.MustCompile(`(?i)-r\s+([a-zA-Z]{2})`)

func (e *Extractor) ExtractRegion(text string) ExtractResult[string] {
	if matches := reRegion.FindStringSubmatch(text); len(matches) > 1 {
		region := strings.ToLower(matches[1])
		remaining := reRegion.ReplaceAllString(text, "")
		return ExtractResult[string]{Value: region, Remaining: strings.TrimSpace(remaining), Found: true}
	}
	return ExtractResult[string]{Value: "", Remaining: text, Found: false}
}

var rePreview = regexp.MustCompile(`(?i)(^|\s+)(-p|--preview)(\s+|$)`)

func (e *Extractor) ExtractPreview(text string) ExtractResult[bool] {
	if matches := rePreview.FindStringSubmatch(text); len(matches) > 0 {
		remaining := rePreview.ReplaceAllString(text, " ")
		return ExtractResult[bool]{Value: true, Remaining: strings.TrimSpace(remaining), Found: true}
	}
	return ExtractResult[bool]{Value: false, Remaining: text, Found: false}
}

var reHelp = regexp.MustCompile(`(?i)(^|\s+)(-h|--help|帮助)(\s+|$)`)

func (e *Extractor) ExtractHelp(text string) ExtractResult[bool] {
	if matches := reHelp.FindStringSubmatch(text); len(matches) > 0 {
		remaining := reHelp.ReplaceAllString(text, " ")
		return ExtractResult[bool]{Value: true, Remaining: strings.TrimSpace(remaining), Found: true}
	}
	return ExtractResult[bool]{Value: false, Remaining: text, Found: false}
}

var reVerbose = regexp.MustCompile(`(?i)(^|\s+)(-v|--verbose)(\s+|$)`)

func (e *Extractor) ExtractVerbose(text string) ExtractResult[bool] {
	if matches := reVerbose.FindStringSubmatch(text); len(matches) > 0 {
		remaining := reVerbose.ReplaceAllString(text, " ")
		return ExtractResult[bool]{Value: true, Remaining: strings.TrimSpace(remaining), Found: true}
	}
	return ExtractResult[bool]{Value: false, Remaining: text, Found: false}
}

var (
	reUIDIndex = regexp.MustCompile(`(?i)u(\d{1,2})`)
	reUID      = regexp.MustCompile(`\d{14,20}`)
	reQQAt     = regexp.MustCompile(`@(\d{9,13})`)
)

// ExtractUid extracts account selector arguments from text.
//
// Supported forms:
// - u[i]  -> u1, u2 ... (while avoiding matches like "mu1")
// - uid   -> 14-20 digit game uid
// - @qq   -> @123456789
//
// It mirrors the legacy parser order: first remove u[i], then uid, then @qq.
// The last matched selector wins and Remaining contains the fully stripped text.
func (e *Extractor) ExtractUid(text string) ExtractResult[string] {
	remaining := strings.TrimSpace(text)
	value := ""
	found := false

	if matched, next, ok := extractUIDIndexArg(remaining); ok {
		value = matched
		remaining = next
		found = true
	}
	if matched, next, ok := extractFirstMatch(remaining, reUID, ""); ok {
		value = matched
		remaining = next
		found = true
	}
	if matched, next, ok := extractFirstMatch(remaining, reQQAt, "@"); ok {
		value = matched
		remaining = next
		found = true
	}

	return ExtractResult[string]{
		Value:     value,
		Remaining: strings.TrimSpace(remaining),
		Found:     found,
	}
}

func extractUIDIndexArg(text string) (string, string, bool) {
	indices := reUIDIndex.FindAllStringSubmatchIndex(text, -1)
	for _, idx := range indices {
		if len(idx) < 4 {
			continue
		}
		start, end := idx[0], idx[1]
		if start > 0 {
			prev := text[start-1]
			if prev == 'm' || prev == 'M' {
				continue
			}
		}
		if end < len(text) && text[end] >= '0' && text[end] <= '9' {
			continue
		}

		digitStart, digitEnd := idx[2], idx[3]
		value := "u" + text[digitStart:digitEnd]
		remaining := strings.TrimSpace(text[:start] + text[end:])
		return value, remaining, true
	}
	return "", text, false
}

func extractFirstMatch(text string, re *regexp.Regexp, prefix string) (string, string, bool) {
	idx := re.FindStringSubmatchIndex(text)
	if len(idx) == 0 {
		return "", text, false
	}

	start, end := idx[0], idx[1]
	valueStart, valueEnd := start, end
	if len(idx) >= 4 {
		valueStart, valueEnd = idx[2], idx[3]
	}

	value := prefix + text[valueStart:valueEnd]
	remaining := strings.TrimSpace(text[:start] + text[end:])
	return value, remaining, true
}

func (e *Extractor) ExtractYear(text string) ExtractResult[int] {
	return e.extractYearInternal(text)
}

var (
	reYearFull  = regexp.MustCompile(`(20\d{2})年?`)
	reYearShort = regexp.MustCompile(`(\d{2})年`)
)

func (e *Extractor) extractYearInternal(text string) ExtractResult[int] {
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
		year := time.Now().Year() - 1
		remaining := strings.Replace(text, "去年", "", 1)
		return ExtractResult[int]{Value: year, Remaining: strings.TrimSpace(remaining), Found: true}
	}

	if strings.Contains(text, "今年") {
		year := time.Now().Year()
		remaining := strings.Replace(text, "今年", "", 1)
		return ExtractResult[int]{Value: year, Remaining: strings.TrimSpace(remaining), Found: true}
	}

	return ExtractResult[int]{Value: 0, Remaining: text, Found: false}
}

var reID = regexp.MustCompile(`^\s*(\d+)\s*$`)

// ExtractID 提取纯数字 ID
func (e *Extractor) ExtractID(text string) ExtractResult[int] {
	if matches := reID.FindStringSubmatch(text); len(matches) > 1 {
		id, _ := strconv.Atoi(matches[1])
		return ExtractResult[int]{Value: id, Remaining: "", Found: true}
	}
	return ExtractResult[int]{Value: 0, Remaining: text, Found: false}
}
