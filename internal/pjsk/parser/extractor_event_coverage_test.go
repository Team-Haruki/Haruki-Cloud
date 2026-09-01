package parser

import (
	"haruki-cloud/internal/testutil"
	"regexp"
	"testing"
	"time"
)

func TestExtractorFeatureBranches(t *testing.T) {
	ext := NewExtractor(map[string]int{"miku": 21, "hatsune miku": 21})
	{
		got := ext.ExtractCharacter("HATSUNE MIKU card")
		{
			testutil.Require(t, got.Found, "unexpected character extraction: %+v", got)
			testutil.Require(t, !(got.Value != 21), "unexpected character extraction: %+v", got)
			testutil.Require(t, !(got.Remaining != "card"), "unexpected character extraction: %+v", got)
		}
	}
	{

		got := ext.ExtractCharacter("unknown")
		{
			testutil.Require(t, !(got.Found), "unexpected missing character extraction: %+v", got)
			testutil.Require(t, !(got.Remaining != "unknown"), "unexpected missing character extraction: %+v", got)
		}
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
			{
				testutil.Require(t, tc.got.Found, "unexpected extraction: %+v, want %q", tc.got, tc.want)
				testutil.Require(t, !(tc.got.Value != tc.want), "unexpected extraction: %+v, want %q", tc.got, tc.want)
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
		testutil.Require(t, !(got.Found), "unexpected match: %+v", got)

	}
	{

		got := ext.ExtractHelp(" -HELP ")
		{
			testutil.Require(t, got.Found, "unexpected help extraction: %+v", got)
			testutil.Require(t, !(got.Remaining != ""), "unexpected help extraction: %+v", got)
		}
	}
	{

		got := ext.ExtractHelp("help")
		testutil.Require(t, !(got.Found), "unexpected help match: %+v", got)
	}
	{

		got := ext.ExtractVerbose("card --verbose list")
		{
			testutil.Require(t, got.Found, "unexpected verbose extraction: %+v", got)
			testutil.Require(t, !(got.Remaining != "card list"), "unexpected verbose extraction: %+v", got)
		}
	}
	{

		got := ext.ExtractVerbose("card")
		testutil.Require(t, !(got.Found), "unexpected verbose match: %+v", got)
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
		{
			testutil.Require(t, got.Found, "ExtractYear(%q) = %+v, want %d", tc.input, got, tc.want)
			testutil.Require(t, !(got.Value != tc.want), "ExtractYear(%q) = %+v, want %d", tc.input, got, tc.want)
		}

	}
	{
		got := ext.ExtractYear("event")
		testutil.Require(t, !(got.Found), "unexpected year match: %+v", got)
	}
	{

		got := ext.ExtractID(" 123 ")
		{
			testutil.Require(t, got.Found, "unexpected id extraction: %+v", got)
			testutil.Require(t, !(got.Value != 123), "unexpected id extraction: %+v", got)
			testutil.Require(t, !(got.Remaining != ""), "unexpected id extraction: %+v", got)
		}
	}
	{

		got := ext.ExtractID("12x")
		testutil.Require(t, !(got.Found), "unexpected id match: %+v", got)
	}

}

func TestExtractorLowLevelBranches(t *testing.T) {
	rules := buildRules(map[string]string{"ab": "short", "abc": "long", "中文": "cn"})
	{
		got := extractByRules("ABC rest", rules)
		{
			testutil.Require(t, got.Found, "longest ASCII rule was not preferred: %+v", got)
			testutil.Require(t, !(got.Value != "long"), "longest ASCII rule was not preferred: %+v", got)
		}
	}
	{

		got := extractByRules("x中文y", rules)
		{
			testutil.Require(t, got.Found, "unicode rule did not match: %+v", got)
			testutil.Require(t, !(got.Value != "cn"), "unicode rule did not match: %+v", got)
		}
	}
	{

		got := extractByRules("zabz", rules)
		testutil.Require(t, !(got.Found), "ASCII word boundary should prevent match: %+v", got)
	}

	re := regexp.MustCompile(`@(\d+)`)
	value, remaining, ok := extractFirstMatch("x @123 y", re, "@")
	{
		testutil.Require(t, ok, "unexpected subgroup extraction: value=%q remaining=%q ok=%v", value, remaining, ok)
		testutil.Require(t, !(value != "@123"), "unexpected subgroup extraction: value=%q remaining=%q ok=%v", value, remaining, ok)
		testutil.Require(t, !(remaining != "x  y"), "unexpected subgroup extraction: value=%q remaining=%q ok=%v", value, remaining, ok)
	}
	{

		value, remaining, ok := extractFirstMatch("none", re, "@")
		{
			testutil.Require(t, !(ok), "unexpected missing subgroup extraction: %q %q %v", value, remaining, ok)
			testutil.Require(t, !(value != ""), "unexpected missing subgroup extraction: %q %q %v", value, remaining, ok)
			testutil.Require(t, !(remaining != "none"), "unexpected missing subgroup extraction: %q %q %v", value, remaining, ok)
		}
	}
	{

		_, _, ok := extractUIDIndexArg("prefixu2 suffix")
		testutil.RequireArgs(t, !(ok), "embedded uid index should not match")
	}
	{
		testutil.RequireArgs(t, isUIDIndexTokenBoundary("a", 0), "edge indexes should be boundaries")
		testutil.RequireArgs(t, isUIDIndexTokenBoundary("a", 1), "edge indexes should be boundaries")
	}

}

func TestEventParserCoversIdentifierSequenceAndFilterBranches(t *testing.T) {
	var nilParser *EventParser
	{
		_, ok := nilParser.CharacterIDByNickname("miku")
		testutil.RequireArgs(t, !(ok), "nil parser should not resolve nicknames")
	}

	parser := NewEventParser(map[string]int{"": 99, "miku": 21, "miku long": 22, "rin": 22})
	{
		id, ok := parser.CharacterIDByNickname(" MIKU ")
		{
			testutil.Require(t, ok, "unexpected nickname result: %d %v", id, ok)
			testutil.Require(t, !(id != 21), "unexpected nickname result: %d %v", id, ok)
		}
	}

	valid := []string{
		"event123", "456", "miku7",
		"next", "下期", "prev", "perv", "上", "current", "curr", "今", "+2", "-3",
		"2024年", "24年", "去年", "今年",
		"marathon", "cheerful", "wl", "worldlink2", "cool",
		"miku rin", "miku箱", "rinban", "blend", "only vbs", "仅25h",
	}
	for _, input := range valid {
		{
			info, err := parser.Parse(input)
			{
				testutil.Require(t, !(err != nil), "Parse(%q) = %+v, %v", input, info, err)
				testutil.Require(t, !(info == nil), "Parse(%q) = %+v, %v", input, info, err)
			}
		}

	}

	invalid := []string{
		"", "eventx", "unknown-token", "only", "only unknown",
		"blend vbs", "vbs blend", "only blend", "world0", "worldx",
	}
	for _, input := range invalid {
		{
			info, err := parser.Parse(input)
			{
				testutil.Require(t, !(err == nil), "Parse(%q) unexpectedly succeeded: %+v", input, info)
				testutil.Require(t, !(info != nil), "Parse(%q) unexpectedly succeeded: %+v", input, info)
			}
		}

	}
	{

		turn, ok := parseEventWorldBloomTurn("WORLD3")
		{
			testutil.Require(t, ok, "unexpected world bloom turn: %d %v", turn, ok)
			testutil.Require(t, !(turn != 3), "unexpected world bloom turn: %d %v", turn, ok)
		}
	}

	for _, input := range []string{"wl", "wl0", "world-nope", "other2"} {
		{
			_, ok := parseEventWorldBloomTurn(input)
			testutil.Require(t, !(ok), "unexpected world bloom turn match for %q", input)
		}

	}
	{
		got, ok := stripEventOnlyUnitPrefix("onlyvbs")
		{
			testutil.Require(t, ok, "unexpected only prefix: %q %v", got, ok)
			testutil.Require(t, !(got != "vbs"), "unexpected only prefix: %q %v", got, ok)
		}
	}
	{

		got, ok := stripEventOnlyUnitPrefix("only")
		{
			testutil.Require(t, !(ok), "unexpected empty only prefix: %q %v", got, ok)
			testutil.Require(t, !(got != "only"), "unexpected empty only prefix: %q %v", got, ok)
		}
	}
	{

		got := normalizeEventToken("  WORLD  Link  ")
		testutil.Require(t, !(got != "worldlink"), "unexpected normalized token: %q", got)
	}

}
