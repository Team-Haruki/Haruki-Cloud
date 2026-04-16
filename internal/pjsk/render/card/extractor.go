package card

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type dictRule struct {
	re                 *regexp.Regexp
	val                string
	singleRuneNonASCII bool
}

func buildRules(items map[string]string) []dictRule {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] > keys[j]
	})

	rules := make([]dictRule, 0, len(keys))
	for _, key := range keys {
		isASCII := true
		for _, ch := range key {
			if ch > 127 {
				isASCII = false
				break
			}
		}
		pattern := "(?i)"
		if isASCII {
			pattern += `\b` + regexp.QuoteMeta(key) + `\b`
		} else {
			pattern += regexp.QuoteMeta(key)
		}
		rules = append(rules, dictRule{
			re:                 regexp.MustCompile(pattern),
			val:                items[key],
			singleRuneNonASCII: !isASCII && utf8.RuneCountInString(key) == 1,
		})
	}
	return rules
}

func extractByRules(text string, rules []dictRule) ExtractResult[string] {
	return extractByRulesWithOptions(text, rules, true)
}

func extractByRulesWithOptions(text string, rules []dictRule, allowSingleRuneNonASCII bool) ExtractResult[string] {
	for _, rule := range rules {
		if !allowSingleRuneNonASCII && rule.singleRuneNonASCII {
			continue
		}
		if rule.re.MatchString(text) {
			remaining := rule.re.ReplaceAllString(text, "")
			return ExtractResult[string]{Value: rule.val, Remaining: strings.TrimSpace(remaining), Found: true}
		}
	}
	return ExtractResult[string]{Remaining: text}
}

type Extractor struct {
	nicknames    map[string]int
	banNicknames map[string]int
}

type BanEventRef struct {
	CharacterID int
	Sequence    int
}

type ExtractResult[T any] struct {
	Value             T
	Remaining         string
	Found             bool
	PrefixTightlyJoin bool
	SuffixTightlyJoin bool
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

var rarityRules = buildRules(map[string]string{
	"4星": "rarity_4", "4star": "rarity_4", "4x": "rarity_4", "4": "rarity_4", "四星": "rarity_4",
	"3星": "rarity_3", "3star": "rarity_3", "3x": "rarity_3", "3": "rarity_3", "三星": "rarity_3",
	"2星": "rarity_2", "2star": "rarity_2", "2x": "rarity_2", "2": "rarity_2", "二星": "rarity_2",
	"1星": "rarity_1", "1star": "rarity_1", "1x": "rarity_1", "1": "rarity_1", "一星": "rarity_1",
	"生日": "rarity_birthday", "birthday": "rarity_birthday",
})

func (e *Extractor) ExtractRarity(text string) ExtractResult[string] {
	return extractByRules(text, rarityRules)
}

var attrRules = buildRules(map[string]string{
	"cute": "cute", "可爱": "cute", "粉花": "cute", "粉": "cute",
	"cool": "cool", "帅气": "cool", "蓝星": "cool", "蓝": "cool",
	"pure": "pure", "纯真": "pure", "绿草": "pure", "草": "pure", "绿": "pure",
	"happy": "happy", "快乐": "happy", "橙心": "happy", "橙": "happy", "黄": "happy",
	"mysterious": "mysterious", "神秘": "mysterious", "紫月": "mysterious", "紫": "mysterious",
})

func (e *Extractor) ExtractAttribute(text string) ExtractResult[string] {
	return extractByRules(text, attrRules)
}

func (e *Extractor) ExtractAttributeWithoutSingleRune(text string) ExtractResult[string] {
	return extractByRulesWithOptions(text, attrRules, false)
}

const (
	SupplyNormal   = "normal"
	SupplyLimited  = "limited"
	SupplyFes      = "festival"
	SupplyCFes     = "colorful_festival_limited"
	SupplyBFes     = "bloom_festival_limited"
	SupplyCollab   = "collaboration_limited"
	SupplyBirthday = "birthday"
)

var skillRules = buildRules(map[string]string{
	"奶卡": "life_recovery",
	"分卡": "score_up",
	"判卡": "judgment_up",
	"p分": "perfect_score_up",
	"判分": "judgment_up",
	"大分": "great_score_up",
	"分":  "score_up",
	"判定": "judgment_up",
	"判":  "judgment_up",
	"回复": "life_recovery",
	"奶":  "life_recovery",
})

func (e *Extractor) ExtractSkill(text string) ExtractResult[string] {
	return extractByRules(text, skillRules)
}

var supplyRules = buildRules(map[string]string{
	"联动":     SupplyCollab,
	"联动限定":   SupplyCollab,
	"collab": SupplyCollab,
	"bfes限定": SupplyBFes,
	"bfes":   SupplyBFes,
	"cfes限定": SupplyCFes,
	"cfes":   SupplyCFes,
	"期间限定":   SupplyLimited,
	"fes":    SupplyFes,
	"非限定":    SupplyNormal,
	"限定":     SupplyLimited,
	"limit":  SupplyLimited,
	"常驻":     SupplyNormal,
	"非限":     SupplyNormal,
	"生日":     SupplyBirthday,
})

func (e *Extractor) ExtractSupply(text string) ExtractResult[string] {
	return extractByRules(text, supplyRules)
}

var unitRules = buildRules(map[string]string{
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

var vsUnitRules = buildRules(map[string]string{
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

var ocUnitRules = buildRules(map[string]string{
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
