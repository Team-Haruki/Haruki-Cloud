package costume

import "testing"

func TestCostumePartLabelBranches(t *testing.T) {
	for partType, want := range map[string]string{
		"head": "饰品",
		"hair": "发型",
		"body": "服装",
	} {
		if got := costumePartLabel(partType); got != want {
			t.Fatalf("costumePartLabel(%q) = %q, want %q", partType, got, want)
		}
	}
}

func TestRefreshFallbackHeadPackagePathBranches(t *testing.T) {
	role := preview3DCharacterEntry{CharacterID: 1, Unit: "idol"}
	state := preview3DComboState{
		role:            role,
		headID:          10,
		headPackagePath: "stale",
		registry: &preview3DRegistry{parts: []preview3DPartEntry{
			{Costume3DID: 10, PartType: "head_optional", CharacterID: 1, Unit: "idol", Status: "empty", PackagePath: "package/head"},
		}},
	}
	state.refreshFallbackHeadPackagePath()
	if state.headPackagePath != "package/head" {
		t.Fatalf("matching fallback package = %q", state.headPackagePath)
	}

	state.headID = 11
	state.refreshFallbackHeadPackagePath()
	if state.headPackagePath != "" {
		t.Fatalf("non-matching fallback package = %q", state.headPackagePath)
	}
}

func TestDefaultHeadCandidateLessBranches(t *testing.T) {
	exact := preview3DPartEntry{Costume3DID: 20, Unit: "idol", ColorID: 2}
	other := preview3DPartEntry{Costume3DID: 10, Unit: "other", ColorID: 1}
	if !defaultHeadCandidateLess(exact, other, "idol") {
		t.Fatal("exact unit should sort first")
	}

	original := preview3DPartEntry{Costume3DID: 30, Unit: "idol", ColorID: 1}
	recolor := preview3DPartEntry{Costume3DID: 10, Unit: "idol", ColorID: 2}
	if !defaultHeadCandidateLess(original, recolor, "idol") {
		t.Fatal("original color should sort first")
	}

	lowerID := preview3DPartEntry{Costume3DID: 1, Unit: "idol", ColorID: 1}
	higherID := preview3DPartEntry{Costume3DID: 2, Unit: "idol", ColorID: 1}
	if !defaultHeadCandidateLess(lowerID, higherID, "idol") {
		t.Fatal("lower costume id should break ties")
	}
}
