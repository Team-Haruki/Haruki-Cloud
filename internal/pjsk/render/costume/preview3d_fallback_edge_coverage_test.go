package costume

import "testing"

func TestPreview3DDefaultCompatibilitySelectionEdges(t *testing.T) {
	testPreview3DDefaultHairSelection(t)
	testPreview3DDefaultHeadSelection(t)
	testPreview3DDefaultRulePredicates(t)
}

func testPreview3DDefaultHairSelection(t *testing.T) {
	t.Helper()
	registry := &preview3DRegistry{rules: []preview3DCompatibilityRule{
		{Unit: "idol", HeadCostume3DID: 9, HairCostume3DID: 99, IsDefault: true},
		{Unit: "other", HeadCostume3DID: 10, HairCostume3DID: 40, IsDefault: true},
		{Unit: "idol", HeadCostume3DID: 10, HairCostume3DID: 30, State: "not_available", IsDefault: true},
		{Unit: "", HeadCostume3DID: 10, HairCostume3DID: 22, IsDefault: true},
		{Unit: "idol", HeadCostume3DID: 10, HairCostume3DID: 21, State: "default_hint"},
		{Unit: "idol", HeadCostume3DID: 10, HairCostume3DID: 20, IsDefault: true},
	}}
	if hairID, ok := registry.defaultHairForHead("idol", 10); !ok || hairID != 20 {
		t.Fatalf("default exact-unit hair = %d, %v", hairID, ok)
	}
	if hairID, ok := registry.defaultHairForHead("", 10); !ok || hairID != 22 {
		t.Fatalf("default unit-neutral hair = %d, %v", hairID, ok)
	}
	if _, ok := registry.defaultHairForHead("idol", 999); ok {
		t.Fatal("missing default hair resolved")
	}
}

func testPreview3DDefaultHeadSelection(t *testing.T) {
	t.Helper()
	role := preview3DCharacterEntry{CharacterID: 1, Unit: "idol"}
	registry := &preview3DRegistry{
		parts: []preview3DPartEntry{
			{Costume3DID: 100, PartType: "head", CharacterID: 1, Unit: "", ColorID: 1, Status: "available"},
			{Costume3DID: 101, PartType: "head", CharacterID: 1, Unit: "idol", ColorID: 2, Status: "available"},
		},
		rules: []preview3DCompatibilityRule{
			{Unit: "", HeadCostume3DID: 100, HairCostume3DID: 20, IsDefault: true},
			{Unit: "idol", HeadCostume3DID: 101, HairCostume3DID: 20, IsDefault: true},
		},
	}
	if part, ok, err := registry.defaultHeadForHair(role, 20); err != nil || !ok || part.Costume3DID != 101 {
		t.Fatalf("default exact-unit head = %+v, %v, %v", part, ok, err)
	}
	if _, ok, err := registry.defaultHeadForHair(role, 999); err != nil || ok {
		t.Fatalf("missing default head = %v, %v", ok, err)
	}
}

func testPreview3DDefaultRulePredicates(t *testing.T) {
	t.Helper()
	role := preview3DCharacterEntry{Unit: "idol"}
	if defaultHeadRuleMatches(preview3DCompatibilityRule{Unit: "idol", HairCostume3DID: 2}, role, 1) {
		t.Fatal("wrong hair matched default rule")
	}
	if defaultHeadRuleMatches(preview3DCompatibilityRule{Unit: "idol", HairCostume3DID: 1}, role, 1) {
		t.Fatal("non-default rule matched")
	}
	if defaultHeadRuleMatches(preview3DCompatibilityRule{Unit: "other", HairCostume3DID: 1, IsDefault: true}, role, 1) {
		t.Fatal("wrong unit matched default rule")
	}
	if !defaultHeadRuleMatches(preview3DCompatibilityRule{HairCostume3DID: 1, IsDefault: true}, role, 1) {
		t.Fatal("unit-neutral default rule did not match")
	}
	left := preview3DPartEntry{Costume3DID: 2, Unit: "idol"}
	right := preview3DPartEntry{Costume3DID: 1, Unit: ""}
	if !defaultHeadOptionalLess(left, right, "idol") || defaultHeadOptionalLess(right, left, "idol") {
		t.Fatal("exact optional head ordering mismatch")
	}
	if !defaultHeadOptionalLess(preview3DPartEntry{Costume3DID: 1, Unit: "a"}, preview3DPartEntry{Costume3DID: 2, Unit: "b"}, "idol") {
		t.Fatal("lexical optional head ordering mismatch")
	}
	if !defaultHeadOptionalLess(preview3DPartEntry{Costume3DID: 1, Unit: "idol"}, preview3DPartEntry{Costume3DID: 2, Unit: "idol"}, "idol") {
		t.Fatal("optional head ID tie-break mismatch")
	}
}

func TestPreview3DHeadHairFallbackEdges(t *testing.T) {
	role := preview3DCharacterEntry{CharacterID: 1, Unit: "idol"}
	registry := &preview3DRegistry{
		parts: []preview3DPartEntry{
			{Costume3DID: 30, PartType: "hair", CharacterID: 1, Unit: "idol", Status: "available"},
			{Costume3DID: 40, PartType: "head_optional", CharacterID: 1, Unit: "idol", Status: "empty"},
		},
		rules: []preview3DCompatibilityRule{
			{Unit: "idol", HeadCostume3DID: 10, HairCostume3DID: 20, State: "not_available"},
			{Unit: "idol", HeadCostume3DID: 10, HairCostume3DID: 30, IsDefault: true},
		},
	}
	if head, hair, err := registry.applyHeadHairFallback(role, "head", 10, 20, "selected"); err != nil || head != 10 || hair != 30 {
		t.Fatalf("head-side fallback = %d/%d, %v", head, hair, err)
	}
	registry.parts[0].Status = "missing"
	if head, hair, err := registry.applyHeadHairFallback(role, "hair", 10, 20, "selected"); err != nil || head != 40 || hair != 20 {
		t.Fatalf("hair-side fallback = %d/%d, %v", head, hair, err)
	}
	if head, hair, err := registry.applyHeadHairFallback(role, "none", 10, 20, "selected"); err == nil || head != 10 || hair != 20 {
		t.Fatalf("blocked fallback error = %d/%d, %v", head, hair, err)
	}
	if head, hair, err := registry.applyHeadHairFallback(role, "auto", 11, 20, "selected"); err != nil || head != 11 || hair != 20 {
		t.Fatalf("unblocked pair changed = %d/%d, %v", head, hair, err)
	}
	if _, ok := registry.headSideFallback(role, "none", 10); ok {
		t.Fatal("unsupported head fallback mode succeeded")
	}
	if _, ok := registry.hairSideFallback(role, "head", 20); ok {
		t.Fatal("unsupported hair fallback mode succeeded")
	}
}
