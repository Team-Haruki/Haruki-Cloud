package parser

import (
	"regexp"
	"testing"
	"time"
)

func TestExtractorFeatureBranches(t *testing.T) {
	ext := NewExtractor(map[string]int{"miku": 21, "hatsune miku": 21})
	if got := ext.ExtractCharacter("HATSUNE MIKU card"); !got.Found || got.Value != 21 || got.Remaining != "card" {
		t.Fatalf("unexpected character extraction: %+v", got)
	}
	if got := ext.ExtractCharacter("unknown"); got.Found || got.Remaining != "unknown" {
		t.Fatalf("unexpected missing character extraction: %+v", got)
	}

	stringCases := []struct {
		name string
		got  ExtractResult[string]
		want string
	}{
		{"rarity", ext.ExtractRarity("birthday miku"), "rarity_birthday"},
		{"attribute", ext.ExtractAttribute("cool card"), "cool"},
		{"skill", ext.ExtractSkill("判定 card"), "judgment_up"},
		{"supply", ext.ExtractSupply("限定 card"), SupplyLimited},
		{"region", ext.ExtractRegion("card -r EN"), "en"},
	}
	for _, tc := range stringCases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.got.Found || tc.got.Value != tc.want {
				t.Fatalf("unexpected extraction: %+v, want %q", tc.got, tc.want)
			}
		})
	}
	for _, got := range []ExtractResult[string]{
		ext.ExtractRarity("none"),
		ext.ExtractAttribute("none"),
		ext.ExtractSkill("none"),
		ext.ExtractSupply("none"),
		ext.ExtractRegion("none"),
	} {
		if got.Found {
			t.Fatalf("unexpected match: %+v", got)
		}
	}

	if got := ext.ExtractHelp(" -HELP "); !got.Found || got.Remaining != "" {
		t.Fatalf("unexpected help extraction: %+v", got)
	}
	if got := ext.ExtractHelp("help"); got.Found {
		t.Fatalf("unexpected help match: %+v", got)
	}
	if got := ext.ExtractVerbose("card --verbose list"); !got.Found || got.Remaining != "card list" {
		t.Fatalf("unexpected verbose extraction: %+v", got)
	}
	if got := ext.ExtractVerbose("card"); got.Found {
		t.Fatalf("unexpected verbose match: %+v", got)
	}

	year := time.Now().Year()
	for _, tc := range []struct {
		input string
		want  int
	}{
		{"2024年 event", 2024},
		{"24年 event", 2024},
		{"去年 event", year - 1},
		{"今年 event", year},
	} {
		got := ext.ExtractYear(tc.input)
		if !got.Found || got.Value != tc.want {
			t.Fatalf("ExtractYear(%q) = %+v, want %d", tc.input, got, tc.want)
		}
	}
	if got := ext.ExtractYear("event"); got.Found {
		t.Fatalf("unexpected year match: %+v", got)
	}
	if got := ext.ExtractID(" 123 "); !got.Found || got.Value != 123 || got.Remaining != "" {
		t.Fatalf("unexpected id extraction: %+v", got)
	}
	if got := ext.ExtractID("12x"); got.Found {
		t.Fatalf("unexpected id match: %+v", got)
	}
}

func TestExtractorLowLevelBranches(t *testing.T) {
	rules := buildRules(map[string]string{"ab": "short", "abc": "long", "中文": "cn"})
	if got := extractByRules("ABC rest", rules); !got.Found || got.Value != "long" {
		t.Fatalf("longest ASCII rule was not preferred: %+v", got)
	}
	if got := extractByRules("x中文y", rules); !got.Found || got.Value != "cn" {
		t.Fatalf("unicode rule did not match: %+v", got)
	}
	if got := extractByRules("zabz", rules); got.Found {
		t.Fatalf("ASCII word boundary should prevent match: %+v", got)
	}

	re := regexp.MustCompile(`@(\d+)`)
	value, remaining, ok := extractFirstMatch("x @123 y", re, "@")
	if !ok || value != "@123" || remaining != "x  y" {
		t.Fatalf("unexpected subgroup extraction: value=%q remaining=%q ok=%v", value, remaining, ok)
	}
	if value, remaining, ok := extractFirstMatch("none", re, "@"); ok || value != "" || remaining != "none" {
		t.Fatalf("unexpected missing subgroup extraction: %q %q %v", value, remaining, ok)
	}
	if _, _, ok := extractUIDIndexArg("prefixu2 suffix"); ok {
		t.Fatal("embedded uid index should not match")
	}
	if !isUIDIndexTokenBoundary("a", 0) || !isUIDIndexTokenBoundary("a", 1) {
		t.Fatal("edge indexes should be boundaries")
	}
}

func TestEventParserCoversIdentifierSequenceAndFilterBranches(t *testing.T) {
	var nilParser *EventParser
	if _, ok := nilParser.CharacterIDByNickname("miku"); ok {
		t.Fatal("nil parser should not resolve nicknames")
	}
	parser := NewEventParser(map[string]int{"": 99, "miku": 21, "miku long": 22, "rin": 22})
	if id, ok := parser.CharacterIDByNickname(" MIKU "); !ok || id != 21 {
		t.Fatalf("unexpected nickname result: %d %v", id, ok)
	}

	valid := []string{
		"event123", "456", "miku7",
		"next", "下期", "prev", "perv", "上", "current", "curr", "今", "+2", "-3",
		"2024年", "24年", "去年", "今年",
		"marathon", "cheerful", "wl", "worldlink2", "cool",
		"miku rin", "miku箱", "rinban", "blend", "only vbs", "仅25h",
	}
	for _, input := range valid {
		if info, err := parser.Parse(input); err != nil || info == nil {
			t.Fatalf("Parse(%q) = %+v, %v", input, info, err)
		}
	}

	invalid := []string{
		"", "eventx", "unknown-token", "only", "only unknown",
		"blend vbs", "vbs blend", "only blend", "world0", "worldx",
	}
	for _, input := range invalid {
		if info, err := parser.Parse(input); err == nil || info != nil {
			t.Fatalf("Parse(%q) unexpectedly succeeded: %+v", input, info)
		}
	}

	if turn, ok := parseEventWorldBloomTurn("WORLD3"); !ok || turn != 3 {
		t.Fatalf("unexpected world bloom turn: %d %v", turn, ok)
	}
	for _, input := range []string{"wl", "wl0", "world-nope", "other2"} {
		if _, ok := parseEventWorldBloomTurn(input); ok {
			t.Fatalf("unexpected world bloom turn match for %q", input)
		}
	}
	if got, ok := stripEventOnlyUnitPrefix("onlyvbs"); !ok || got != "vbs" {
		t.Fatalf("unexpected only prefix: %q %v", got, ok)
	}
	if got, ok := stripEventOnlyUnitPrefix("only"); ok || got != "only" {
		t.Fatalf("unexpected empty only prefix: %q %v", got, ok)
	}
	if got := normalizeEventToken("  WORLD  Link  "); got != "worldlink" {
		t.Fatalf("unexpected normalized token: %q", got)
	}
}
