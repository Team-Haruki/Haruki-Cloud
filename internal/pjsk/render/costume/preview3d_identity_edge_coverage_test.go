package costume

import "testing"

func TestPreview3DAccessoryIdentityValidationEdges(t *testing.T) {
	base := preview3DPartEntry{
		Costume3DID:      1001,
		PartType:         "head",
		Costume3DGroupID: 2000,
		ColorID:          2,
		BaseSourceKey:    "source-a",
		AccessoryID:      10,
	}
	if err := validateNonAccessoryIdentity(preview3DPartEntry{PartType: "body"}); err != nil {
		t.Fatalf("empty non-accessory identity error = %v", err)
	}
	if err := validateNonAccessoryIdentity(preview3DPartEntry{PartType: "body", AccessoryID: 1}); err == nil {
		t.Fatal("non-accessory identity error = nil")
	}
	if err := validateAccessoryWithoutOriginal(preview3DPartEntry{PartType: "head"}); err != nil {
		t.Fatalf("empty accessory identity error = %v", err)
	}
	if err := validateAccessoryWithoutOriginal(preview3DPartEntry{PartType: "head", AccessoryID: 1}); err == nil {
		t.Fatal("orphan accessory identity error = nil")
	}

	assertPreview3DSourceIdentityEdges(t, base)
	assertPreview3DIdentityUtilityEdges(t)
}

func assertPreview3DSourceIdentityEdges(t *testing.T, base preview3DPartEntry) {
	t.Helper()
	valid := accessoryIdentityValidator{originalIDBySource: map[string]int{"source-a": 10}}
	if err := valid.validateSourceBackedPart(base); err != nil {
		t.Fatalf("valid source-backed part error = %v", err)
	}
	withoutID := base
	withoutID.AccessoryID = 0
	if err := valid.validateSourceBackedPart(withoutID); err == nil {
		t.Fatal("missing source-backed accessory id error = nil")
	}
	withoutSource := base
	withoutSource.BaseSourceKey = ""
	if err := valid.validateSourceBackedPart(withoutSource); err == nil {
		t.Fatal("missing source-backed base source error = nil")
	}
	unknownSource := base
	unknownSource.BaseSourceKey = "unknown"
	if err := valid.validateSourceBackedPart(unknownSource); err == nil {
		t.Fatal("unknown original source error = nil")
	}
	mismatched := base
	mismatched.AccessoryID = 11
	if err := valid.validateSourceBackedPart(mismatched); err == nil {
		t.Fatal("mismatched accessory id error = nil")
	}
	multiple := accessoryIdentityValidator{
		sourceByFamily:     map[string]string{preview3DAccessoryFamily(base): "source-b"},
		originalIDBySource: map[string]int{"source-a": 10, "source-b": 20},
	}
	if err := multiple.validateSourceBackedPart(base); err == nil {
		t.Fatal("multiple original sources error = nil")
	}
}

func assertPreview3DIdentityUtilityEdges(t *testing.T) {
	t.Helper()
	if err := missingAccessoryIdentityError(""); err != nil {
		t.Fatalf("empty missing identity error = %v", err)
	}
	if err := missingAccessoryIdentityError("source-a"); err == nil {
		t.Fatal("named missing identity error = nil")
	}
	if got := onlyAccessorySource(nil); got != "" {
		t.Fatalf("onlyAccessorySource(nil) = %q", got)
	}
	if got := onlyAccessorySource(map[string]struct{}{"source-a": {}}); got != "source-a" {
		t.Fatalf("onlyAccessorySource(single) = %q", got)
	}
	values := map[string]int{"source": 10}
	setMinimumPositive(values, "source", 0)
	setMinimumPositive(values, "source", 20)
	setMinimumPositive(values, "source", 5)
	setMinimumPositive(values, "new", 7)
	if values["source"] != 5 || values["new"] != 7 {
		t.Fatalf("setMinimumPositive values = %#v", values)
	}
}

func TestPreview3DComboRoleCandidateDedupAndOrdering(t *testing.T) {
	registry := &preview3DRegistry{characters: []preview3DCharacterEntry{
		{Character3DID: 7, CharacterID: 2, Unit: "b"},
		{Character3DID: 7, CharacterID: 1, Unit: "z"},
		{Character3DID: 7, CharacterID: 1, Unit: "a"},
		{Character3DID: 7, CharacterID: 1, Unit: "a"},
		{Character3DID: 8, CharacterID: 0, Unit: "ignored"},
	}}
	candidates := registry.comboRoleCandidates(ComboQuery{Character3DID: 7})
	if len(candidates) != 3 {
		t.Fatalf("candidate count = %d, want 3", len(candidates))
	}
	if candidates[0].CharacterID != 1 || candidates[0].Unit != "a" || candidates[1].Unit != "z" || candidates[2].CharacterID != 2 {
		t.Fatalf("candidate ordering = %+v", candidates)
	}
}
