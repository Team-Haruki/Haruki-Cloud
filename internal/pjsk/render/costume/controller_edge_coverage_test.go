package costume

import (
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestCostumeAccessoryRepresentativeEdges(t *testing.T) {
	fallback := controllerCoverageCostume(1, "head")
	local := controllerCoverageCostume(2, "head")
	remote := controllerCoverageCostume(3, "head")
	source := &controllerCoverageSource{costumes: map[int]*masterdata.Costume3d{3: remote}}
	if got := resolveCostumeAccessoryRepresentative(source, map[int]*masterdata.Costume3d{2: local}, 2, fallback, true); got != local {
		t.Fatalf("local representative = %+v", got)
	}
	if got := resolveCostumeAccessoryRepresentative(source, nil, 3, fallback, true); got != remote {
		t.Fatalf("registry representative = %+v", got)
	}
	if got := resolveCostumeAccessoryRepresentative(source, nil, 4, fallback, true); got != fallback {
		t.Fatalf("fallback representative = %+v", got)
	}
	if got := resolveCostumeAccessoryRepresentative(nil, nil, 3, fallback, true); got != fallback {
		t.Fatalf("nil-source representative = %+v", got)
	}
}

func TestCostumeSourceCardsAndRoleProjectionEdges(t *testing.T) {
	source := &controllerCoverageSource{
		characters:  map[int]*masterdata.Character{1: {ID: 1}},
		sourceCards: map[int][]int{1: {10}},
	}
	controller := &Controller{}
	if got, err := controller.sourceCardsForCostumes(source, []*masterdata.Costume3d{nil, {ID: 0}}); err != nil || len(got) != 0 {
		t.Fatalf("empty source cards = %+v, %v", got, err)
	}
	got, err := controller.sourceCardsForCostumes(source, []*masterdata.Costume3d{{ID: 1}, {ID: 1}})
	if err != nil || len(got[1]) != 1 || got[1][0] != 10 {
		t.Fatalf("deduplicated source cards = %+v, %v", got, err)
	}
	controller.apply3DRoleToCostumeBasic(source, nil, 1)
	basic := &drawing.CostumeBasic{}
	controller.apply3DRoleToCostumeBasic(source, basic, 0)
	if basic.CharacterID != 0 {
		t.Fatalf("invalid role changed basic: %+v", basic)
	}
	controller.apply3DRoleToCostumeBasic(source, basic, 1)
	if basic.CharacterID != 1 {
		t.Fatalf("valid role projection = %+v", basic)
	}
}

func TestNamedLookupMikuSuffixEdges(t *testing.T) {
	plain := namedLookupParseState{nameFields: []string{"costume"}}
	plain.applyMikuUnitSuffix()
	if len(plain.nameFields) != 1 {
		t.Fatalf("plain suffix changed fields: %+v", plain)
	}
	state := namedLookupParseState{
		nameFields: []string{"costume", "ln"},
		roleAlias:  character3DAliasSelection{characterID: 21},
	}
	state.applyMikuUnitSuffix()
	if state.roleAlias.unit != "light_sound" || len(state.nameFields) != 1 {
		t.Fatalf("Miku unit suffix = %+v", state)
	}
	state.nameFields = []string{"costume", "unknown"}
	state.roleAlias.unit = ""
	state.applyMikuUnitSuffix()
	if state.roleAlias.unit != "" || len(state.nameFields) != 2 {
		t.Fatalf("unknown suffix changed state: %+v", state)
	}
}

func TestCharacter3DAliasSelectionEdges(t *testing.T) {
	selection := character3DAliasSelection{}
	if err := selection.setCharacter(21); err != nil {
		t.Fatalf("set character: %v", err)
	}
	if err := selection.setCharacter(22); err == nil {
		t.Fatal("duplicate character unexpectedly accepted")
	}
	selection.setUnit("light_sound")
	selection.setUnit("idol")
	if !selection.conflictingUnit {
		t.Fatalf("conflicting unit not recorded: %+v", selection)
	}
	role := 0
	if err := selection.apply(&role); err == nil {
		t.Fatal("conflicting alias selection unexpectedly applied")
	}
	empty := character3DAliasSelection{}
	if err := empty.apply(&role); err != nil || role != 0 {
		t.Fatalf("empty alias selection = %d, %v", role, err)
	}
}

func TestLookupPendingRoleEdges(t *testing.T) {
	state := lookupQueryParseState{query: Query{}, partType: "body", pendingRole: true}
	if err := state.applyPendingRole("miku"); err != nil || state.roleAlias.characterID != 21 || state.pendingRole {
		t.Fatalf("alias pending role = %+v, %v", state, err)
	}
	state = lookupQueryParseState{query: Query{}, partType: "body", pendingRole: true}
	if err := state.applyPendingRole("bad"); err == nil {
		t.Fatal("invalid pending role unexpectedly accepted")
	}
	state = lookupQueryParseState{query: Query{Character3DID: 1}, partType: "body", pendingRole: true}
	if err := state.applyPendingRole("2"); err == nil {
		t.Fatal("duplicate pending role unexpectedly accepted")
	}
}

func TestLookupLabeledIDEdges(t *testing.T) {
	for _, tc := range []struct {
		part  string
		label string
	}{
		{part: "head", label: "outfit"},
		{part: "body", label: "accessory"},
		{part: "body", label: "hair"},
	} {
		state := lookupQueryParseState{partType: tc.part}
		if err := state.applyLabeledID(tc.label, 1, tc.label); err == nil {
			t.Errorf("mismatched %s/%s label unexpectedly accepted", tc.part, tc.label)
		}
	}
	state := lookupQueryParseState{partType: "body"}
	if err := state.applyLabeledID("color", 2, "color2"); err != nil {
		t.Fatalf("color label: %v", err)
	}
	if err := state.applyLabeledID("outfit_color", 3, "color3"); err == nil {
		t.Fatal("duplicate color unexpectedly accepted")
	}
	if err := state.applyLabeledID("unknown", 1, "unknown1"); err == nil || !strings.Contains(err.Error(), "unknown1") {
		t.Fatalf("unknown label error = %v", err)
	}
}

func TestComboExplicitIDStateEdges(t *testing.T) {
	state := comboQueryParseState{pending: "outfit"}
	if err := state.applyExplicitID("1", 1); err != nil || state.parsed.OutfitID != 1 || state.pending != "" {
		t.Fatalf("pending outfit assignment = %+v, %v", state, err)
	}
	state.lastColorTarget = "outfit"
	if err := state.applyExplicitID("2", 2); err != nil || state.parsed.OutfitColorID != 2 || state.lastColorTarget != "" {
		t.Fatalf("outfit color assignment = %+v, %v", state, err)
	}
	if err := state.applyExplicitID("3", 3); err == nil {
		t.Fatal("unlabeled explicit ID unexpectedly accepted")
	}
}
