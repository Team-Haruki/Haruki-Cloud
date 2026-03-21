package card

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type dictRule struct {
	re  *regexp.Regexp
	val string
}

func buildRules(items map[string]string) []dictRule {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
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
			re:  regexp.MustCompile(pattern),
			val: items[key],
		})
	}
	return rules
}

func extractByRules(text string, rules []dictRule) ExtractResult[string] {
	for _, rule := range rules {
		if rule.re.MatchString(text) {
			remaining := rule.re.ReplaceAllString(text, "")
			return ExtractResult[string]{Value: rule.val, Remaining: strings.TrimSpace(remaining), Found: true}
		}
	}
	return ExtractResult[string]{Remaining: text}
}

type Extractor struct {
	nicknames map[string]int
}

type ExtractResult[T any] struct {
	Value     T
	Remaining string
	Found     bool
}

func NewExtractor(nicknames map[string]int) *Extractor {
	return &Extractor{nicknames: cloneNicknames(nicknames)}
}

func (e *Extractor) ExtractCharacter(text string) ExtractResult[int] {
	textLower := strings.ToLower(text)
	bestNickname := ""
	bestID := 0
	for nickname, id := range e.nicknames {
		if strings.Contains(textLower, nickname) && len(nickname) > len(bestNickname) {
			bestNickname = nickname
			bestID = id
		}
	}
	if bestNickname == "" {
		return ExtractResult[int]{Remaining: text}
	}
	re := regexp.MustCompile("(?i)" + regexp.QuoteMeta(bestNickname))
	remaining := re.ReplaceAllString(text, "")
	return ExtractResult[int]{Value: bestID, Remaining: strings.TrimSpace(remaining), Found: true}
}

var rarityRules = buildRules(map[string]string{
	"4星": "rarity_4", "4star": "rarity_4", "四星": "rarity_4",
	"3星": "rarity_3", "3star": "rarity_3", "三星": "rarity_3",
	"2星": "rarity_2", "2star": "rarity_2", "二星": "rarity_2",
	"1星": "rarity_1", "1star": "rarity_1", "一星": "rarity_1",
	"生日": "rarity_birthday", "birthday": "rarity_birthday",
})

func (e *Extractor) ExtractRarity(text string) ExtractResult[string] {
	return extractByRules(text, rarityRules)
}

var attrRules = buildRules(map[string]string{
	"cute": "cute", "可爱": "cute", "粉": "cute",
	"cool": "cool", "帅气": "cool", "蓝": "cool",
	"pure": "pure", "纯真": "pure", "草": "pure", "绿": "pure",
	"happy": "happy", "快乐": "happy", "橙": "happy",
	"mysterious": "mysterious", "神秘": "mysterious", "紫": "mysterious",
})

func (e *Extractor) ExtractAttribute(text string) ExtractResult[string] {
	return extractByRules(text, attrRules)
}

const (
	SupplyNormal   = "normal"
	SupplyLimited  = "limited"
	SupplyFes      = "festival"
	SupplyBirthday = "birthday"
)

var skillRules = buildRules(map[string]string{
	"p分": "perfect_score_up",
	"大分": "great_score_up",
	"分":  "score_up",
	"判定": "judgment_accuracy_up",
	"判":  "judgment_accuracy_up",
	"回复": "life_recovery",
	"奶":  "life_recovery",
})

func (e *Extractor) ExtractSkill(text string) ExtractResult[string] {
	return extractByRules(text, skillRules)
}

var supplyRules = buildRules(map[string]string{
	"fes": "festival",
	"限定":  "limited", "limit": "limited",
	"常驻": "normal", "非限": "normal",
	"生日": "birthday",
})

func (e *Extractor) ExtractSupply(text string) ExtractResult[string] {
	return extractByRules(text, supplyRules)
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
	if strings.Contains(text, "今年") {
		remaining := strings.Replace(text, "今年", "", 1)
		return ExtractResult[int]{Value: time.Now().Year(), Remaining: strings.TrimSpace(remaining), Found: true}
	}
	return ExtractResult[int]{Remaining: text}
}
