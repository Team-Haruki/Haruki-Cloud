package card

import (
	"slices"
	"testing"
)

func TestLooksLikeSingleCardQuerySupportsIDAndNicknameSequence(t *testing.T) {
	if !LooksLikeSingleCardQuery("1001") {
		t.Fatal("expected card id query to be treated as single-card query")
	}
	if !LooksLikeSingleCardQuery("-1") {
		t.Fatal("expected global latest-card query to be treated as single-card query")
	}
	if !LooksLikeSingleCardQuery("mnr-1") {
		t.Fatal("expected nickname sequence query to be treated as single-card query")
	}
	if LooksLikeSingleCardQuery("mnr 4星") {
		t.Fatal("did not expect filter query to be treated as single-card query")
	}
}

func TestParserStrictFilterRejectsBareCardID(t *testing.T) {
	parser := NewParser(defaultNicknames)
	if _, err := parser.ParseStrictFilter("1001"); err == nil {
		t.Fatal("expected strict filter parser to reject bare card id")
	}
}

func TestParserPreferFilterTreats25AsUnitFilter(t *testing.T) {
	parser := NewParser(defaultNicknames)

	info, err := parser.ParseStrictFilter("25")
	if err != nil {
		t.Fatalf("ParsePreferFilter() error = %v", err)
	}
	if info.Type != QueryTypeFilter {
		t.Fatalf("expected filter query, got %+v", info)
	}
	if info.Unit != "school_refusal" {
		t.Fatalf("unexpected unit filter: %+v", info)
	}
}

func TestParserPreferFilterTreatsBare4AsRarityFilter(t *testing.T) {
	parser := NewParser(defaultNicknames)

	info, err := parser.ParseStrictFilter("4")
	if err != nil {
		t.Fatalf("ParsePreferFilter() error = %v", err)
	}
	if info.Type != QueryTypeFilter {
		t.Fatalf("expected filter query, got %+v", info)
	}
	if info.Rarity != "rarity_4" {
		t.Fatalf("unexpected rarity filter: %+v", info)
	}
}

func TestParserExtractsAdvancedCardFilters(t *testing.T) {
	parser := NewParser(defaultNicknames)

	info, err := parser.Parse("event123 mmjv fes 25年")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if info.Type != QueryTypeFilter {
		t.Fatalf("unexpected query type: %+v", info)
	}
	if info.EventID != 123 {
		t.Fatalf("unexpected event filter: %+v", info)
	}
	if info.MainUnit != "piapro" || info.SupportUnit != "idol" {
		t.Fatalf("unexpected unit filter: %+v", info)
	}
	if info.SupplyType != SupplyFes {
		t.Fatalf("unexpected supply filter: %+v", info)
	}
	if info.Year != 2025 {
		t.Fatalf("unexpected year filter: %+v", info)
	}
}

func TestParserExtractsBanEventOCUnitSkillAndAttr(t *testing.T) {
	parser := NewParser(defaultNicknames)

	info, err := parser.Parse("mnr1 纯v 判分 蓝星")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if info.Type != QueryTypeFilter {
		t.Fatalf("unexpected query type: %+v", info)
	}
	if info.BanCharID != 5 || info.BanSeq != 1 {
		t.Fatalf("unexpected ban event filter: %+v", info)
	}
	if info.MainUnit != "piapro" || info.SupportUnit != "none" {
		t.Fatalf("unexpected unit filter: %+v", info)
	}
	if info.SkillType != "judgment_up" {
		t.Fatalf("unexpected skill filter: %+v", info)
	}
	if info.Attr != "cool" {
		t.Fatalf("unexpected attr filter: %+v", info)
	}
}

func TestParserSupportsLunabotSkillAliases(t *testing.T) {
	parser := NewParser(defaultNicknames)

	tests := []struct {
		query     string
		skillType string
		skillIDs  []int
	}{
		{query: "判卡", skillType: "judgment_up"},
		{query: "分卡", skillType: "score_up"},
		{query: "奶卡", skillType: "life_recovery"},
		{query: "血分", skillIDs: []int{12}},
		{query: "团分", skillIDs: []int{15, 16, 17, 18, 19}},
	}

	for _, tt := range tests {
		info, err := parser.ParseStrictFilter(tt.query)
		if err != nil {
			t.Fatalf("ParsePreferFilter(%q) error = %v", tt.query, err)
		}
		if info.Type != QueryTypeFilter {
			t.Fatalf("expected filter query for %q, got %+v", tt.query, info)
		}
		if info.SkillType != tt.skillType {
			t.Fatalf("unexpected skill type for %q: got=%q want=%q", tt.query, info.SkillType, tt.skillType)
		}
		if !slices.Equal(info.SkillIDs, tt.skillIDs) {
			t.Fatalf("unexpected skill IDs for %q: got=%v want=%v", tt.query, info.SkillIDs, tt.skillIDs)
		}
	}
}

func TestParserSupportsLunabotCharacterAliases(t *testing.T) {
	parser := NewParser(defaultNicknames)

	tests := []struct {
		query       string
		characterID int
	}{
		{query: "tks-1", characterID: 13},
		{query: "khn-1", characterID: 9},
		{query: "akt-1", characterID: 11},
		{query: "青柳冬弥-1", characterID: 12},
		{query: "凤笑梦-1", characterID: 14},
		{query: "草薙宁宁-1", characterID: 15},
	}

	for _, tt := range tests {
		info, err := parser.Parse(tt.query)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", tt.query, err)
		}
		if info.Type != QueryTypeSeq || info.CharacterID != tt.characterID || info.Sequence != -1 {
			t.Fatalf("unexpected parse result for %q: %+v", tt.query, info)
		}
	}
}

func TestParserPrefersFullCharacterNameOverAttributeSubstring(t *testing.T) {
	parser := NewParser(defaultNicknames)

	info, err := parser.ParsePreferFilter("草薙宁宁")
	if err != nil {
		t.Fatalf("ParsePreferFilter(草薙宁宁) error = %v", err)
	}
	if info.Type != QueryTypeFilter {
		t.Fatalf("expected filter query, got %+v", info)
	}
	if info.CharacterID != 15 {
		t.Fatalf("unexpected character parse result: %+v", info)
	}
	if info.Attr != "" {
		t.Fatalf("did not expect attribute to be extracted from full character name: %+v", info)
	}
}

func TestParserPrefersFullCharacterNameOverMysteriousAliasSubstring(t *testing.T) {
	parser := NewParser(defaultNicknames)

	info, err := parser.ParsePreferFilter("望月穗波")
	if err != nil {
		t.Fatalf("ParsePreferFilter(望月穗波) error = %v", err)
	}
	if info.Type != QueryTypeFilter {
		t.Fatalf("expected filter query, got %+v", info)
	}
	if info.CharacterID != 3 {
		t.Fatalf("unexpected character parse result: %+v", info)
	}
	if info.Attr != "" {
		t.Fatalf("did not expect attribute to be extracted from full character name: %+v", info)
	}
}

func TestParserSupportsApprovedAliasNicknamesBeforeAttributeKeywords(t *testing.T) {
	nicknames := cloneNicknames(defaultNicknames)
	nicknames["黄桃"] = 13
	parser := NewParser(nicknames)

	info, err := parser.ParseStrictFilter("黄桃")
	if err != nil {
		t.Fatalf("ParsePreferFilter(黄桃) error = %v", err)
	}
	if info.Type != QueryTypeFilter {
		t.Fatalf("expected filter query, got %+v", info)
	}
	if info.CharacterID != 13 {
		t.Fatalf("unexpected character parse result: %+v", info)
	}
	if info.Attr != "" {
		t.Fatalf("did not expect happy attribute to be extracted from alias 黄桃: %+v", info)
	}
}

func TestParserIgnoresNumericApprovedAliases(t *testing.T) {
	nicknames := cloneNicknames(defaultNicknames)
	nicknames["2"] = 2
	parser := NewParser(nicknames)

	if _, err := parser.ParseStrictFilter("24"); err == nil {
		t.Fatal("expected numeric alias collision input to be rejected in filter mode")
	}
}

func TestParserSupportsUnitAndRarityWithoutNumericAliasHijack(t *testing.T) {
	nicknames := cloneNicknames(defaultNicknames)
	nicknames["4"] = 4
	parser := NewParser(nicknames)

	info, err := parser.ParseStrictFilter("ws 4")
	if err != nil {
		t.Fatalf("ParsePreferFilter(ws 4) error = %v", err)
	}
	if info.Type != QueryTypeFilter {
		t.Fatalf("expected filter query, got %+v", info)
	}
	if info.Unit != "theme_park" || info.Rarity != "rarity_4" {
		t.Fatalf("unexpected ws+4 parse result: %+v", info)
	}
	if info.CharacterID != 0 || info.BanCharID != 0 {
		t.Fatalf("did not expect numeric alias to hijack ws 4: %+v", info)
	}
}

func TestParserSupportsAttributeAndCharacterCombination(t *testing.T) {
	parser := NewParser(defaultNicknames)

	info, err := parser.ParseStrictFilter("绿 mnr")
	if err != nil {
		t.Fatalf("ParseStrictFilter(绿 mnr) error = %v", err)
	}
	if info.Type != QueryTypeFilter {
		t.Fatalf("expected filter query, got %+v", info)
	}
	if info.Attr != "pure" || info.CharacterID != 5 {
		t.Fatalf("unexpected attr/character parse result: %+v", info)
	}
}

func TestParserTreatsBare25AsSchoolRefusalUnitFilter(t *testing.T) {
	parser := NewParser(defaultNicknames)

	info, err := parser.ParseStrictFilter("25")
	if err != nil {
		t.Fatalf("ParseStrictFilter(25) error = %v", err)
	}
	if info.Type != QueryTypeFilter {
		t.Fatalf("expected filter query, got %+v", info)
	}
	if info.Unit != "school_refusal" {
		t.Fatalf("unexpected unit parse result: %+v", info)
	}
	if info.CharacterID != 0 {
		t.Fatalf("did not expect 25 to resolve as character in strict filter mode: %+v", info)
	}
}

func TestParserIgnoresNumericAliasWhenParsingUnitAndRarity(t *testing.T) {
	nicknames := cloneNicknames(defaultNicknames)
	nicknames["4"] = 4
	parser := NewParser(nicknames)

	info, err := parser.ParseStrictFilter("ws 4")
	if err != nil {
		t.Fatalf("ParseStrictFilter(ws 4) error = %v", err)
	}
	if info.Type != QueryTypeFilter {
		t.Fatalf("expected filter query, got %+v", info)
	}
	if info.Unit != "theme_park" || info.Rarity != "rarity_4" {
		t.Fatalf("unexpected unit/rarity parse result: %+v", info)
	}
	if info.CharacterID != 0 || info.BanCharID != 0 {
		t.Fatalf("did not expect numeric alias to hijack unit+rarity parsing: %+v", info)
	}
}

func TestParserDoesNotTreatNicknamePlusRarityAsBanEvent(t *testing.T) {
	parser := NewParser(defaultNicknames)

	info, err := parser.ParseStrictFilter("宁宁4星")
	if err != nil {
		t.Fatalf("ParsePreferFilter(宁宁4星) error = %v", err)
	}
	if info.Type != QueryTypeFilter {
		t.Fatalf("expected filter query, got %+v", info)
	}
	if info.CharacterID != 15 {
		t.Fatalf("unexpected character parse result: %+v", info)
	}
	if info.Rarity != "rarity_4" {
		t.Fatalf("unexpected rarity parse result: %+v", info)
	}
	if info.BanCharID != 0 || info.BanSeq != 0 {
		t.Fatalf("did not expect ban event parse result: %+v", info)
	}
}

func TestParserRejectsUnparsedRarityDigits(t *testing.T) {
	parser := NewParser(defaultNicknames)

	if _, err := parser.ParseStrictFilter("4 3"); err == nil {
		t.Fatal("expected leftover rarity digits to be rejected")
	}
}

func TestParserDoesNotExtractSingleRuneAttributeWhenTightlyJoinedBeforeCharacter(t *testing.T) {
	parser := NewParser(defaultNicknames)

	info, err := parser.ParseStrictFilter("草草薙宁宁")
	if err != nil {
		t.Fatalf("ParsePreferFilter(草草薙宁宁) error = %v", err)
	}
	if info.Type != QueryTypeFilter {
		t.Fatalf("expected filter query, got %+v", info)
	}
	if info.CharacterID != 15 {
		t.Fatalf("unexpected character parse result: %+v", info)
	}
	if info.Attr != "" {
		t.Fatalf("did not expect pure attribute from tightly-joined prefix: %+v", info)
	}
}

func TestParserDoesNotExtractSingleRuneAttributeWhenTightlyJoinedBeforeCharacterAlias(t *testing.T) {
	parser := NewParser(defaultNicknames)

	info, err := parser.ParseStrictFilter("月望月穗波")
	if err != nil {
		t.Fatalf("ParsePreferFilter(月望月穗波) error = %v", err)
	}
	if info.Type != QueryTypeFilter {
		t.Fatalf("expected filter query, got %+v", info)
	}
	if info.CharacterID != 3 {
		t.Fatalf("unexpected character parse result: %+v", info)
	}
	if info.Attr != "" {
		t.Fatalf("did not expect mysterious attribute from tightly-joined prefix: %+v", info)
	}
}

func TestParserSupportsGlobalLatestCardSequence(t *testing.T) {
	parser := NewParser(defaultNicknames)

	info, err := parser.Parse("-1")
	if err != nil {
		t.Fatalf("Parse(-1) error = %v", err)
	}
	if info.Type != QueryTypeLatest || info.Sequence != -1 {
		t.Fatalf("unexpected parse result: %+v", info)
	}

	info, err = parser.ParsePreferFilter("-2")
	if err != nil {
		t.Fatalf("ParsePreferFilter(-2) error = %v", err)
	}
	if info.Type != QueryTypeLatest || info.Sequence != -2 {
		t.Fatalf("unexpected prefer-filter parse result: %+v", info)
	}
}
