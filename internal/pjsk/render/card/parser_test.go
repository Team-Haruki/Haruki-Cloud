package card

import "testing"

func TestLooksLikeSingleCardQuerySupportsIDAndNicknameSequence(t *testing.T) {
	if !LooksLikeSingleCardQuery("1001") {
		t.Fatal("expected card id query to be treated as single-card query")
	}
	if !LooksLikeSingleCardQuery("mnr-1") {
		t.Fatal("expected nickname sequence query to be treated as single-card query")
	}
	if LooksLikeSingleCardQuery("mnr 4星") {
		t.Fatal("did not expect filter query to be treated as single-card query")
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
	if info.SkillType != "judgment_accuracy_up" {
		t.Fatalf("unexpected skill filter: %+v", info)
	}
	if info.Attr != "cool" {
		t.Fatalf("unexpected attr filter: %+v", info)
	}
}
