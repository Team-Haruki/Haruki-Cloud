package card

import "testing"

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

func TestParserPreferFilterTreats25AsUnitFilter(t *testing.T) {
	parser := NewParser(defaultNicknames)

	info, err := parser.ParsePreferFilter("25")
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

	info, err := parser.ParsePreferFilter("4")
	if err != nil {
		t.Fatalf("ParsePreferFilter() error = %v", err)
	}
	if info.Type != QueryTypeFilter {
		t.Fatalf("expected filter query, got %+v", info)
	}
	if info.Rarity != "rarity_4" {
		t.Fatalf("unexpected rarity filter: %+v", info)
	}
	if LooksLikeSingleCardQueryPreferFilter("4") {
		t.Fatal("did not expect bare 4 list query to be treated as single-card query")
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
	}{
		{query: "判卡", skillType: "judgment_up"},
		{query: "分卡", skillType: "score_up"},
		{query: "奶卡", skillType: "life_recovery"},
	}

	for _, tt := range tests {
		info, err := parser.ParsePreferFilter(tt.query)
		if err != nil {
			t.Fatalf("ParsePreferFilter(%q) error = %v", tt.query, err)
		}
		if info.Type != QueryTypeFilter {
			t.Fatalf("expected filter query for %q, got %+v", tt.query, info)
		}
		if info.SkillType != tt.skillType {
			t.Fatalf("unexpected skill type for %q: got=%q want=%q", tt.query, info.SkillType, tt.skillType)
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
