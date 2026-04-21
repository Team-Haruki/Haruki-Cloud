package queryrule

import (
	"regexp"
	"sort"
	"unicode/utf8"
)

type StringRule struct {
	Re                 *regexp.Regexp
	Value              string
	SingleRuneNonASCII bool
}

type IntSliceRule struct {
	Re     *regexp.Regexp
	Values []int
}

func BuildStringRules(items map[string]string) []StringRule {
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

	rules := make([]StringRule, 0, len(keys))
	for _, key := range keys {
		isASCII := IsASCII(key)
		pattern := "(?i)"
		if isASCII {
			pattern += `\b` + regexp.QuoteMeta(key) + `\b`
		} else {
			pattern += regexp.QuoteMeta(key)
		}
		rules = append(rules, StringRule{
			Re:                 regexp.MustCompile(pattern),
			Value:              items[key],
			SingleRuneNonASCII: !isASCII && utf8.RuneCountInString(key) == 1,
		})
	}
	return rules
}

func BuildStringRulesFromGroups(groups map[string][]string) []StringRule {
	return BuildStringRules(FlattenAliasGroups(groups))
}

func BuildIntSliceRules(items map[string][]int) []IntSliceRule {
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

	rules := make([]IntSliceRule, 0, len(keys))
	for _, key := range keys {
		pattern := "(?i)"
		if IsASCII(key) {
			pattern += `\b` + regexp.QuoteMeta(key) + `\b`
		} else {
			pattern += regexp.QuoteMeta(key)
		}
		rules = append(rules, IntSliceRule{
			Re:     regexp.MustCompile(pattern),
			Values: append([]int(nil), items[key]...),
		})
	}
	return rules
}

func CloneAliasGroups(src map[string][]string) map[string][]string {
	dst := make(map[string][]string, len(src))
	for value, aliases := range src {
		dst[value] = append([]string(nil), aliases...)
	}
	return dst
}

func MergeAliasGroups(groups ...map[string][]string) map[string][]string {
	size := 0
	for _, group := range groups {
		size += len(group)
	}

	merged := make(map[string][]string, size)
	for _, group := range groups {
		for value, aliases := range group {
			merged[value] = append(merged[value], aliases...)
		}
	}
	return merged
}

func FlattenAliasGroups(groups map[string][]string) map[string]string {
	flat := make(map[string]string)
	for value, aliases := range groups {
		for _, alias := range aliases {
			flat[alias] = value
		}
	}
	return flat
}

func IsASCII(raw string) bool {
	for _, ch := range raw {
		if ch > 127 {
			return false
		}
	}
	return true
}

var rarityAliasGroups = map[string][]string{
	"rarity_4":        {"4星", "4star", "四星"},
	"rarity_3":        {"3星", "3star", "三星"},
	"rarity_2":        {"2星", "2star", "二星"},
	"rarity_1":        {"1星", "1star", "一星"},
	"rarity_birthday": {"生日", "birthday"},
}

func RarityAliasGroups() map[string][]string {
	return CloneAliasGroups(rarityAliasGroups)
}

var attributeAliasGroups = map[string][]string{
	"cute":       {"cute", "可爱", "粉花", "粉", "pink"},
	"cool":       {"cool", "帅气", "蓝星", "蓝", "blue"},
	"pure":       {"pure", "纯真", "绿草", "草", "绿", "green"},
	"happy":      {"happy", "快乐", "橙心", "橙", "orange"},
	"mysterious": {"mysterious", "神秘", "紫月", "紫", "purple"},
}

func AttributeAliasGroups() map[string][]string {
	return CloneAliasGroups(attributeAliasGroups)
}

var skillAliasGroups = map[string][]string{
	"score_up":      {"分卡", "分"},
	"judgment_up":   {"判卡", "判定", "判"},
	"life_recovery": {"奶卡", "回复", "奶"},
}

func SkillAliasGroups() map[string][]string {
	return CloneAliasGroups(skillAliasGroups)
}

var supplyAliasGroups = map[string][]string{
	"festival": {"fes"},
	"limited":  {"限定", "limit"},
	"normal":   {"常驻", "非限"},
	"birthday": {"生日"},
}

func SupplyAliasGroups() map[string][]string {
	return CloneAliasGroups(supplyAliasGroups)
}
