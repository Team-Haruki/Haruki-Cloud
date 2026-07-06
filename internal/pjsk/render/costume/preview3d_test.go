package costume

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPreview3DRegistryResolveSkipsMissingGroupParts(t *testing.T) {
	registry := &preview3DRegistry{
		characters: []preview3DCharacterEntry{{
			Character3DID:   5,
			CharacterID:     20,
			Unit:            "school_refusal",
			BodyCostume3DID: 33001,
			HeadCostume3DID: 33011,
			HairCostume3DID: 33021,
			Status:          "available",
		}},
		parts: []preview3DPartEntry{
			{Costume3DID: 33001, PartType: "body", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33002, PartType: "body", CharacterID: 20, Unit: "school_refusal", ColorID: 2, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33011, PartType: "head", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33012, PartType: "head", CharacterID: 20, Unit: "school_refusal", ColorID: 2, Costume3DGroupID: 330, Status: "missing"},
			{Costume3DID: 33021, PartType: "hair", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
		},
	}

	selection, err := registry.resolve("jp", 33002, preview3DCacheSignature("test", 700, 500, 2, "capture"))
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if selection.HeadCostume3DID != 33011 {
		t.Fatalf("expected resolver to skip missing same-color head 33012 and use 33011, got %d", selection.HeadCostume3DID)
	}
}

func TestPreview3DRegistryResolveReportsMissingRuntimePackageDetails(t *testing.T) {
	registry := &preview3DRegistry{
		parts: []preview3DPartEntry{{
			Costume3DID: 33002,
			PartType:    "body",
			CharacterID: 20,
			Unit:        "school_refusal",
			Status:      "missing",
			PackagePath: "parts/body/33002/school_refusal",
			BundlePath:  "live_pv/model/characterv2/body/0033/0002.bundle",
			Warnings:    []string{"body bundle not found"},
		}},
	}

	_, err := registry.resolve("jp", 33002, "sig")
	if err == nil {
		t.Fatalf("expected missing runtime package error")
	}
	for _, want := range []string{
		"missing runtime package",
		"packagePath=parts/body/33002/school_refusal",
		"bundlePath=live_pv/model/characterv2/body/0033/0002.bundle",
		"warning=body bundle not found",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in error: %v", want, err)
		}
	}
}

func TestPreview3DRegistryResolveSkipsMissingDefaultRoles(t *testing.T) {
	registry := &preview3DRegistry{
		characters: []preview3DCharacterEntry{
			{
				Character3DID:   1,
				CharacterID:     20,
				Unit:            "school_refusal",
				BodyCostume3DID: 99901,
				HeadCostume3DID: 99911,
				HairCostume3DID: 99921,
				Status:          "missing",
			},
			{
				Character3DID:   5,
				CharacterID:     20,
				Unit:            "school_refusal",
				BodyCostume3DID: 33001,
				HeadCostume3DID: 33011,
				HairCostume3DID: 33021,
				Status:          "available",
			},
		},
		parts: []preview3DPartEntry{
			{Costume3DID: 33001, PartType: "body", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33002, PartType: "body", CharacterID: 20, Unit: "school_refusal", ColorID: 2, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33011, PartType: "head", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33021, PartType: "hair", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 331, Status: "available"},
		},
	}

	selection, err := registry.resolve("jp", 33002, preview3DCacheSignature("test", 700, 500, 2, "capture"))
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if selection.HairCostume3DID != 33021 {
		t.Fatalf("expected resolver to skip missing default role and use hair 33021, got %d", selection.HairCostume3DID)
	}
}

func TestPreview3DRegistryResolveUsesGroupColorRoleCacheKey(t *testing.T) {
	registry := &preview3DRegistry{
		characters: []preview3DCharacterEntry{{
			Character3DID:   5,
			CharacterID:     20,
			Unit:            "school_refusal",
			BodyCostume3DID: 33001,
			HeadCostume3DID: 33011,
			HairCostume3DID: 33021,
			Status:          "available",
		}},
		parts: []preview3DPartEntry{
			{Costume3DID: 33001, PartType: "body", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33002, PartType: "body", CharacterID: 20, Unit: "school_refusal", ColorID: 2, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33011, PartType: "head", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33012, PartType: "head", CharacterID: 20, Unit: "school_refusal", ColorID: 2, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33021, PartType: "hair", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
		},
	}
	signature := preview3DCacheSignature("test", 700, 500, 2, "capture")

	body, err := registry.resolve("jp", 33001, signature)
	if err != nil {
		t.Fatalf("resolve body failed: %v", err)
	}
	head, err := registry.resolve("jp", 33011, signature)
	if err != nil {
		t.Fatalf("resolve head failed: %v", err)
	}
	color2, err := registry.resolve("jp", 33002, signature)
	if err != nil {
		t.Fatalf("resolve color2 failed: %v", err)
	}

	if body.ImageID != head.ImageID {
		t.Fatalf("same group/color/role should share image id: body=%q head=%q", body.ImageID, head.ImageID)
	}
	if body.ImageID == color2.ImageID {
		t.Fatalf("different color should not share image id: %q", body.ImageID)
	}
	if want := "pjsk3d_" + signature + "_c20_school_refusal_g330_cl1_b33001_h33011_r33021_o0"; body.ImageID != want {
		t.Fatalf("unexpected body image id: got %q want %q", body.ImageID, want)
	}
}

func TestPreview3DRegistryResolveImageIDIgnoresRegion(t *testing.T) {
	registry := &preview3DRegistry{
		characters: []preview3DCharacterEntry{{
			Character3DID:   5,
			CharacterID:     20,
			Unit:            "school_refusal",
			BodyCostume3DID: 33001,
			HeadCostume3DID: 33011,
			HairCostume3DID: 33021,
			Status:          "available",
		}},
		parts: []preview3DPartEntry{
			{Costume3DID: 33001, PartType: "body", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33011, PartType: "head", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33021, PartType: "hair", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
		},
	}

	jp, err := registry.resolve("jp", 33001, "sig")
	if err != nil {
		t.Fatalf("resolve jp failed: %v", err)
	}
	cn, err := registry.resolve("cn", 33001, "sig")
	if err != nil {
		t.Fatalf("resolve cn failed: %v", err)
	}
	if jp.ImageID != cn.ImageID {
		t.Fatalf("same tuple should share image id across regions: jp=%q cn=%q", jp.ImageID, cn.ImageID)
	}
}

func TestPreview3DRegistryResolveRejectsBlockedHeadHairPair(t *testing.T) {
	registry := preview3DCompatibilityTestRegistry([]preview3DCompatibilityRule{
		{Unit: "school_refusal", HeadCostume3DID: 53129, HairCostume3DID: 33021, State: "not_available"},
	})

	_, err := registry.resolve("jp", 53129, "sig")
	if err == nil {
		t.Fatalf("expected blocked head/hair pair to be rejected")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected blocked error, got %v", err)
	}
}

func TestPreview3DRegistryResolveAllowsUnlistedHairWhenOnlyAvailablePatternsExist(t *testing.T) {
	registry := preview3DCompatibilityTestRegistry([]preview3DCompatibilityRule{
		{Unit: "school_refusal", HeadCostume3DID: 33011, HairCostume3DID: 99921, State: "available"},
	})

	selection, err := registry.resolve("jp", 53129, "sig")
	if err != nil {
		t.Fatalf("available patterns should not reject unlisted head/hair pairs: %v", err)
	}
	if selection.HeadCostume3DID != 33011 || selection.HairCostume3DID != 33021 {
		t.Fatalf("unexpected resolved tuple: %+v", selection)
	}
}

func TestPreview3DRegistryResolveAllowsOfficialPresetOutsideAvailablePatterns(t *testing.T) {
	registry := preview3DCompatibilityTestRegistry([]preview3DCompatibilityRule{
		{Unit: "school_refusal", HeadCostume3DID: 33011, HairCostume3DID: 99921, State: "available"},
	})

	selection, err := registry.resolve("jp", 33001, "sig")
	if err != nil {
		t.Fatalf("official preset should not be rejected by custom compatibility patterns: %v", err)
	}
	if selection.HeadCostume3DID != 33011 || selection.HairCostume3DID != 33021 {
		t.Fatalf("unexpected official tuple: %+v", selection)
	}
}

func TestPreview3DRegistryResolveAccessoryConflictUsesDefaultHair(t *testing.T) {
	registry := preview3DCompatibilityTestRegistry([]preview3DCompatibilityRule{
		{Unit: "school_refusal", HeadCostume3DID: 53129, HairCostume3DID: 33021, State: "not_available"},
		{Unit: "school_refusal", HeadCostume3DID: 53129, HairCostume3DID: 99021, State: "default_hint"},
	})
	registry.parts = append(registry.parts,
		preview3DPartEntry{Costume3DID: 99021, PartType: "hair", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 990, Status: "available"},
	)

	selection, err := registry.resolve("jp", 53129, "sig")
	if err != nil {
		t.Fatalf("resolve accessory with default hair failed: %v", err)
	}
	if selection.HairCostume3DID != 99021 {
		t.Fatalf("expected default hair 99021, got %+v", selection)
	}
	if selection.HeadOptionalCostume3DID == nil || *selection.HeadOptionalCostume3DID != 53129 {
		t.Fatalf("expected selected accessory to stay active, got %+v", selection.HeadOptionalCostume3DID)
	}
}

func TestPreview3DRegistryResolveHairConflictUsesDefaultAccessory(t *testing.T) {
	registry := &preview3DRegistry{
		characters: []preview3DCharacterEntry{{
			Character3DID:   5,
			CharacterID:     20,
			Unit:            "school_refusal",
			BodyCostume3DID: 33001,
			HeadCostume3DID: 33011,
			HairCostume3DID: 33021,
			Status:          "available",
		}},
		parts: []preview3DPartEntry{
			{Costume3DID: 33001, PartType: "body", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33011, PartType: "head", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 30129, PartType: "head_optional", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 990, Status: "available"},
			{Costume3DID: 53129, PartType: "head_optional", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 531, Status: "empty"},
			{Costume3DID: 33021, PartType: "hair", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 99021, PartType: "hair", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 990, Status: "available"},
		},
		rules: []preview3DCompatibilityRule{
			{Unit: "school_refusal", HeadCostume3DID: 30129, HairCostume3DID: 99021, State: "not_available"},
		},
	}

	selection, err := registry.resolve("jp", 99021, "sig")
	if err != nil {
		t.Fatalf("resolve hair with default accessory failed: %v", err)
	}
	if selection.HairCostume3DID != 99021 {
		t.Fatalf("expected selected hair to stay active, got %+v", selection)
	}
	if selection.HeadOptionalCostume3DID == nil || *selection.HeadOptionalCostume3DID != 53129 {
		t.Fatalf("expected default empty accessory 53129, got %+v", selection.HeadOptionalCostume3DID)
	}
}

func TestPreview3DRegistryResolveTreatsCompleteHeadTypesAsHeadSlot(t *testing.T) {
	registry := &preview3DRegistry{
		characters: []preview3DCharacterEntry{{
			Character3DID:   5,
			CharacterID:     20,
			Unit:            "school_refusal",
			BodyCostume3DID: 33001,
			HeadCostume3DID: 33011,
			HairCostume3DID: 33021,
			Status:          "available",
		}},
		parts: []preview3DPartEntry{
			{Costume3DID: 33001, PartType: "body", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 450, Status: "available"},
			{Costume3DID: 33021, PartType: "hair", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 450, Status: "available"},
			{Costume3DID: 45033, PartType: "head_optional", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 450, HeadCostume3DAssetbundleType: "head_and_hair", Status: "available"},
		},
	}

	selection, err := registry.resolve("jp", 45033, "sig")
	if err != nil {
		t.Fatalf("resolve complete head failed: %v", err)
	}
	if selection.HeadCostume3DID != 45033 {
		t.Fatalf("expected complete head to use the main head slot, got %+v", selection)
	}
	if selection.HeadOptionalCostume3DID != nil {
		t.Fatalf("complete head types must not also fill the head_optional slot: %+v", selection.HeadOptionalCostume3DID)
	}
}

func TestPreview3DCacheSignatureChangesWithVersionAndCamera(t *testing.T) {
	base := preview3DCacheSignature("v1", 1400, 1000, 2, "capture")
	if base == preview3DCacheSignature("v2", 1400, 1000, 2, "capture") {
		t.Fatalf("cache version should change signature")
	}
	if base == preview3DCacheSignature("v1", 1400, 1000, 2, "default") {
		t.Fatalf("camera preset should change signature")
	}
}

func preview3DCompatibilityTestRegistry(rules []preview3DCompatibilityRule) *preview3DRegistry {
	return &preview3DRegistry{
		characters: []preview3DCharacterEntry{{
			Character3DID:   5,
			CharacterID:     20,
			Unit:            "school_refusal",
			BodyCostume3DID: 33001,
			HeadCostume3DID: 33011,
			HairCostume3DID: 33021,
			Status:          "available",
		}},
		parts: []preview3DPartEntry{
			{Costume3DID: 33001, PartType: "body", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33011, PartType: "head", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33021, PartType: "hair", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 53129, PartType: "head_optional", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
		},
		rules: rules,
	}
}

func TestPreview3DRegistryResolveComboSlotsAccessoryAfterUnit(t *testing.T) {
	registry := &preview3DRegistry{
		characters: []preview3DCharacterEntry{
			{Character3DID: 5, CharacterID: 20, Unit: "school_refusal", BodyCostume3DID: 33001, HeadCostume3DID: 33011, HairCostume3DID: 33021, Status: "available"},
			{Character3DID: 6, CharacterID: 20, Unit: "idol", BodyCostume3DID: 33001, HeadCostume3DID: 33011, HairCostume3DID: 33021, Status: "available"},
		},
		parts: []preview3DPartEntry{
			{Costume3DID: 33001, PartType: "body", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33001, PartType: "body", CharacterID: 20, Unit: "idol", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33011, PartType: "head", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33011, PartType: "head", CharacterID: 20, Unit: "idol", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33021, PartType: "hair", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33021, PartType: "hair", CharacterID: 20, Unit: "idol", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 30129, PartType: "head_optional", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 30129, PartType: "head", CharacterID: 20, Unit: "idol", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
		},
		rules: []preview3DCompatibilityRule{
			{Unit: "school_refusal", HeadCostume3DID: 33011, HairCostume3DID: 33021, State: "available"},
			{Unit: "idol", HeadCostume3DID: 30129, HairCostume3DID: 33021, State: "available"},
		},
	}

	selection, err := registry.resolveCombo("jp", ComboQuery{
		Unit:                "school_refusal",
		BodyCostume3DID:     33001,
		HairCostume3DID:     33021,
		AccessoryCostumeIDs: []int{30129},
	}, "sig")
	if err != nil {
		t.Fatalf("resolve combo failed: %v", err)
	}
	if selection.HeadOptionalCostume3DID == nil || *selection.HeadOptionalCostume3DID != 30129 {
		t.Fatalf("expected accessory to resolve to head_optional, got %+v", selection.HeadOptionalCostume3DID)
	}
	if selection.HeadCostume3DID != 33011 {
		t.Fatalf("expected default head to stay 33011, got %d", selection.HeadCostume3DID)
	}

	selection, err = registry.resolveCombo("jp", ComboQuery{
		Unit:                "idol",
		BodyCostume3DID:     33001,
		HairCostume3DID:     33021,
		AccessoryCostumeIDs: []int{30129},
	}, "sig")
	if err != nil {
		t.Fatalf("resolve idol combo failed: %v", err)
	}
	if selection.HeadCostume3DID != 30129 {
		t.Fatalf("expected same id to resolve to head for idol, got %d", selection.HeadCostume3DID)
	}
	if selection.HeadOptionalCostume3DID != nil {
		t.Fatalf("did not expect optional accessory for idol, got %+v", selection.HeadOptionalCostume3DID)
	}
}

func TestPreview3DRegistryResolveComboKeepsEmptyHeadOptionalSlot(t *testing.T) {
	registry := &preview3DRegistry{
		characters: []preview3DCharacterEntry{
			{Character3DID: 5, CharacterID: 20, Unit: "school_refusal", BodyCostume3DID: 33001, HeadCostume3DID: 33011, HairCostume3DID: 33021, Status: "available"},
		},
		parts: []preview3DPartEntry{
			{Costume3DID: 33001, PartType: "body", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33011, PartType: "head", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33021, PartType: "hair", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 53129, PartType: "head_optional", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "empty"},
		},
	}

	selection, err := registry.resolveCombo("jp", ComboQuery{
		Unit:                "school_refusal",
		BodyCostume3DID:     33001,
		HairCostume3DID:     33021,
		AccessoryCostumeIDs: []int{53129},
	}, "sig")
	if err != nil {
		t.Fatalf("resolve combo failed: %v", err)
	}
	if selection.HeadOptionalCostume3DID == nil || *selection.HeadOptionalCostume3DID != 53129 {
		t.Fatalf("expected empty head_optional slot to stay selected, got %+v", selection.HeadOptionalCostume3DID)
	}
	if !strings.HasSuffix(selection.ImageID, "_o53129") {
		t.Fatalf("expected empty head_optional id in image key, got %q", selection.ImageID)
	}
}

func TestPreview3DRegistryResolveComboKeepsOfficialDefaultHeadWhenGroupHasVirtualHead(t *testing.T) {
	registry := &preview3DRegistry{
		characters: []preview3DCharacterEntry{
			{Character3DID: 17, CharacterID: 17, Unit: "school_refusal", BodyCostume3DID: 34, HeadCostume3DID: 33, HairCostume3DID: 217, Status: "available"},
		},
		parts: []preview3DPartEntry{
			{Costume3DID: 33, PartType: "head_optional", CharacterID: 17, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 17, HeadCostume3DAssetbundleType: "head_only", Status: "empty"},
			{Costume3DID: 34, PartType: "body", CharacterID: 17, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 17, Status: "planned"},
			{Costume3DID: 217, PartType: "hair", CharacterID: 17, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 217, HeadCostume3DAssetbundleType: "head_and_hair", Status: "planned"},
			{Costume3DID: 10000002, PartType: "head", CharacterID: 17, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 17, HeadCostume3DAssetbundleType: "head_and_hair", Status: "planned"},
		},
		rules: []preview3DCompatibilityRule{
			{Unit: "school_refusal", HeadCostume3DID: 10000002, HairCostume3DID: 217, State: "not_available"},
		},
	}

	selection, err := registry.resolveCombo("jp", ComboQuery{
		Unit:            "school_refusal",
		BodyCostume3DID: 34,
		HairCostume3DID: 217,
	}, "sig")
	if err != nil {
		t.Fatalf("resolve combo failed: %v", err)
	}
	if selection.HeadCostume3DID != 33 {
		t.Fatalf("expected official default head 33, got %d", selection.HeadCostume3DID)
	}
	if selection.HeadOptionalCostume3DID != nil {
		t.Fatalf("did not expect implicit empty optional in default tuple, got %+v", selection.HeadOptionalCostume3DID)
	}

	selection, err = registry.resolveCombo("jp", ComboQuery{
		Unit:                "school_refusal",
		BodyCostume3DID:     34,
		HairCostume3DID:     217,
		AccessoryCostumeIDs: []int{33},
	}, "sig")
	if err != nil {
		t.Fatalf("resolve combo with empty optional failed: %v", err)
	}
	if selection.HeadCostume3DID != 33 {
		t.Fatalf("expected explicit empty optional to keep official default head 33, got %d", selection.HeadCostume3DID)
	}
	if selection.HeadOptionalCostume3DID == nil || *selection.HeadOptionalCostume3DID != 33 {
		t.Fatalf("expected explicit empty optional 33 to stay selected, got %+v", selection.HeadOptionalCostume3DID)
	}
}

func TestPreview3DRegistryResolveComboRequiresUnitWhenAmbiguous(t *testing.T) {
	registry := &preview3DRegistry{
		characters: []preview3DCharacterEntry{
			{Character3DID: 5, CharacterID: 20, Unit: "school_refusal", BodyCostume3DID: 33001, HeadCostume3DID: 33011, HairCostume3DID: 33021, Status: "available"},
			{Character3DID: 6, CharacterID: 20, Unit: "idol", BodyCostume3DID: 33001, HeadCostume3DID: 33011, HairCostume3DID: 33021, Status: "available"},
		},
		parts: []preview3DPartEntry{
			{Costume3DID: 33001, PartType: "body", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33001, PartType: "body", CharacterID: 20, Unit: "idol", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
		},
	}

	_, err := registry.resolveCombo("jp", ComboQuery{BodyCostume3DID: 33001}, "sig")
	if err == nil {
		t.Fatalf("expected ambiguous combo to require unit")
	}
}

func TestPreview3DRegistryResolveComboDeduplicatesSameRolePresets(t *testing.T) {
	registry := &preview3DRegistry{
		characters: []preview3DCharacterEntry{
			{Character3DID: 5, CharacterID: 5, Unit: "idol", BodyCostume3DID: 10, HeadCostume3DID: 105, HairCostume3DID: 205, Status: "available"},
			{Character3DID: 59, CharacterID: 5, Unit: "idol", BodyCostume3DID: 45034, HeadCostume3DID: 45033, HairCostume3DID: 205, Status: "available"},
		},
		parts: []preview3DPartEntry{
			{Costume3DID: 10, PartType: "body", CharacterID: 5, Unit: "idol", ColorID: 1, Costume3DGroupID: 5, Status: "planned"},
			{Costume3DID: 105, PartType: "head", CharacterID: 5, Unit: "idol", ColorID: 1, Costume3DGroupID: 105, Status: "planned"},
			{Costume3DID: 205, PartType: "hair", CharacterID: 5, Unit: "idol", ColorID: 1, Costume3DGroupID: 205, Status: "planned"},
		},
	}

	selection, err := registry.resolveCombo("jp", ComboQuery{
		Unit:                "idol",
		BodyCostume3DID:     10,
		HairCostume3DID:     205,
		AccessoryCostumeIDs: []int{105},
	}, "sig")
	if err != nil {
		t.Fatalf("resolve combo failed: %v", err)
	}
	if selection.RoleID != "5:idol" {
		t.Fatalf("expected role 5:idol, got %s", selection.RoleID)
	}
}

func TestPreview3DCaptureTemporaryComboUsesExistingCapture(t *testing.T) {
	const png = "cached-combo"
	captureCalled := false
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && strings.HasPrefix(r.URL.Path, "/captures/tmp_pjsk3d_") {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/captures/tmp_pjsk3d_") {
			w.Header().Set("content-type", "image/png")
			fmt.Fprint(w, png)
			return
		}
		switch r.URL.Path {
		case "/runtime/character3d-index.json":
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"entries":[{"character3dId":5,"characterId":20,"unit":"school_refusal","bodyCostume3dId":33001,"headCostume3dId":33011,"hairCostume3dId":33021,"status":"available"}]}`)
		case "/runtime/parts/part-registry.json":
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"entries":[
				{"costume3dId":33001,"partType":"body","characterId":20,"unit":"school_refusal","colorId":1,"costume3dGroupId":330,"status":"available"},
				{"costume3dId":33011,"partType":"head","characterId":20,"unit":"school_refusal","colorId":1,"costume3dGroupId":330,"status":"available"},
				{"costume3dId":33021,"partType":"hair","characterId":20,"unit":"school_refusal","colorId":1,"costume3dGroupId":330,"status":"available"}
			]}`)
		case "/runtime/parts/head-hair-compatibility.json":
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"rules":[{"unit":"school_refusal","headCostume3dId":33011,"hairCostume3dId":33021,"state":"available"}]}`)
		case "/capture":
			captureCalled = true
			http.Error(w, "cached combo should not capture", http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected engine request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer engine.Close()

	service := NewPreview3DService(Preview3DConfig{Enabled: true, EngineBaseURL: engine.URL, CaptureCacheVersion: "test"})
	data, err := service.CaptureTemporaryCombo(context.Background(), "jp", ComboQuery{
		Unit:            "school_refusal",
		BodyCostume3DID: 33001,
		HairCostume3DID: 33021,
	})
	if err != nil {
		t.Fatalf("CaptureTemporaryCombo failed: %v", err)
	}
	if string(data) != png {
		t.Fatalf("unexpected png data: %q", string(data))
	}
	if captureCalled {
		t.Fatalf("cached combo should not call capture")
	}
}

func TestPreview3DEnsureCaptureSerializesMisses(t *testing.T) {
	var active atomic.Int32
	var maxActive atomic.Int32
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && strings.HasPrefix(r.URL.Path, "/captures/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/capture" {
			current := active.Add(1)
			for {
				previous := maxActive.Load()
				if current <= previous || maxActive.CompareAndSwap(previous, current) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		t.Fatalf("unexpected engine request: %s %s", r.Method, r.URL.Path)
	}))
	defer engine.Close()

	service := NewPreview3DService(Preview3DConfig{
		Enabled:               true,
		EngineBaseURL:         engine.URL,
		CaptureMaxConcurrency: 1,
		CaptureAcquireTimeout: time.Second,
		CaptureExistsTTL:      -1,
		TemporaryCaptureTTL:   time.Hour,
	})

	selections := []preview3DSelection{
		{ImageID: "pjsk3d_a", RoleID: "1:unit", BodyCostume3DID: 1, HeadCostume3DID: 2, HairCostume3DID: 3},
		{ImageID: "pjsk3d_b", RoleID: "1:unit", BodyCostume3DID: 1, HeadCostume3DID: 2, HairCostume3DID: 4},
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(selections))
	for _, selection := range selections {
		wg.Add(1)
		go func(selection preview3DSelection) {
			defer wg.Done()
			errs <- service.ensureCapture(context.Background(), selection, "persistent")
		}(selection)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("ensureCapture failed: %v", err)
		}
	}
	if maxActive.Load() != 1 {
		t.Fatalf("expected serialized captures, max active=%d", maxActive.Load())
	}
}
