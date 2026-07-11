package costume

import (
	"context"
	"encoding/json"
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

	selection, err := registry.resolve("jp", 33002)
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

	_, err := registry.resolve("jp", 33002)
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

	selection, err := registry.resolve("jp", 33002)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if selection.HairCostume3DID != 33021 {
		t.Fatalf("expected resolver to skip missing default role and use hair 33021, got %d", selection.HairCostume3DID)
	}
}

func TestPreview3DRegistryResolveUsesInputCostumeID(t *testing.T) {
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
	body, err := registry.resolve("jp", 33001)
	if err != nil {
		t.Fatalf("resolve body failed: %v", err)
	}
	head, err := registry.resolve("jp", 33011)
	if err != nil {
		t.Fatalf("resolve head failed: %v", err)
	}
	color2, err := registry.resolve("jp", 33002)
	if err != nil {
		t.Fatalf("resolve color2 failed: %v", err)
	}

	if body.ImageID == head.ImageID {
		t.Fatalf("different input costume ids should not share image id: body=%q head=%q", body.ImageID, head.ImageID)
	}
	if body.ImageID == color2.ImageID {
		t.Fatalf("different input costume ids should not share image id: %q", body.ImageID)
	}
	if want := "pjsk3d_jp_c20_school_refusal_i33001_b33001_h33011_r33021_o0"; body.ImageID != want {
		t.Fatalf("unexpected body image id: got %q want %q", body.ImageID, want)
	}
	if want := "pjsk3d_jp_c20_school_refusal_i33011_b33001_h33011_r33021_o0"; head.ImageID != want {
		t.Fatalf("unexpected head image id: got %q want %q", head.ImageID, want)
	}
}

func TestPreview3DRegistryResolveImageIDIncludesRegion(t *testing.T) {
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

	jp, err := registry.resolve("jp", 33001)
	if err != nil {
		t.Fatalf("resolve jp failed: %v", err)
	}
	cn, err := registry.resolve("cn", 33001)
	if err != nil {
		t.Fatalf("resolve cn failed: %v", err)
	}
	if jp.ImageID == cn.ImageID {
		t.Fatalf("same tuple should not share image id across regions: jp=%q cn=%q", jp.ImageID, cn.ImageID)
	}
	if !strings.HasPrefix(jp.ImageID, "pjsk3d_jp_") {
		t.Fatalf("jp image id should include region: %q", jp.ImageID)
	}
	if !strings.HasPrefix(cn.ImageID, "pjsk3d_cn_") {
		t.Fatalf("cn image id should include region: %q", cn.ImageID)
	}
}

func TestPreview3DRegistryResolveRejectsBlockedHeadHairPair(t *testing.T) {
	registry := preview3DCompatibilityTestRegistry([]preview3DCompatibilityRule{
		{Unit: "school_refusal", HeadCostume3DID: 53129, HairCostume3DID: 33021, State: "not_available"},
	})

	_, err := registry.resolve("jp", 53129)
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

	selection, err := registry.resolve("jp", 53129)
	if err != nil {
		t.Fatalf("available patterns should not reject unlisted head/hair pairs: %v", err)
	}
	if selection.HeadCostume3DID != 53129 || selection.HairCostume3DID != 33021 {
		t.Fatalf("unexpected resolved tuple: %+v", selection)
	}
}

func TestPreview3DRegistryResolveAllowsOfficialPresetOutsideAvailablePatterns(t *testing.T) {
	registry := preview3DCompatibilityTestRegistry([]preview3DCompatibilityRule{
		{Unit: "school_refusal", HeadCostume3DID: 33011, HairCostume3DID: 99921, State: "available"},
	})

	selection, err := registry.resolve("jp", 33001)
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

	selection, err := registry.resolve("jp", 53129)
	if err != nil {
		t.Fatalf("resolve accessory with default hair failed: %v", err)
	}
	if selection.HairCostume3DID != 99021 {
		t.Fatalf("expected default hair 99021, got %+v", selection)
	}
	if selection.HeadCostume3DID != 53129 || selection.HeadOptionalCostume3DID != nil {
		t.Fatalf("expected selected accessory to stay as the business head, got %+v", selection)
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

	selection, err := registry.resolve("jp", 99021)
	if err != nil {
		t.Fatalf("resolve hair with default accessory failed: %v", err)
	}
	if selection.HairCostume3DID != 99021 {
		t.Fatalf("expected selected hair to stay active, got %+v", selection)
	}
	if selection.HeadCostume3DID != 53129 || selection.HeadOptionalCostume3DID != nil {
		t.Fatalf("expected default empty accessory 53129 as the business head, got %+v", selection)
	}
}

func TestPreview3DRegistryResolveTreatsOnlyHeadAndHairAsHeadSlot(t *testing.T) {
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

	selection, err := registry.resolve("jp", 45033)
	if err != nil {
		t.Fatalf("resolve complete head failed: %v", err)
	}
	if selection.HeadCostume3DID != 45033 {
		t.Fatalf("expected complete head to use the main head slot, got %+v", selection)
	}
	if selection.HeadOptionalCostume3DID != nil {
		t.Fatalf("complete head types must not also fill the head_optional slot: %+v", selection.HeadOptionalCostume3DID)
	}
	if slot := preview3DPartSlot(preview3DPartEntry{PartType: "head", HeadCostume3DAssetbundleType: "head_all"}); slot != "head_optional" {
		t.Fatalf("expected head_all to remain an optional accessory contribution, got %s", slot)
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
		Unit:                 "school_refusal",
		BodyCostume3DID:      33001,
		HairCostume3DID:      33021,
		AccessoryCostume3DID: 30129,
	}, "sig")
	if err != nil {
		t.Fatalf("resolve combo failed: %v", err)
	}
	if selection.HeadCostume3DID != 30129 || selection.HeadOptionalCostume3DID != nil {
		t.Fatalf("expected the selected accessory id to remain the business head id, got %+v", selection)
	}

	selection, err = registry.resolveCombo("jp", ComboQuery{
		Unit:                 "idol",
		BodyCostume3DID:      33001,
		HairCostume3DID:      33021,
		AccessoryCostume3DID: 30129,
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

func TestPreview3DRegistryResolveComboKeepsEmptyHeadAsBusinessSelection(t *testing.T) {
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
		Unit:                 "school_refusal",
		BodyCostume3DID:      33001,
		HairCostume3DID:      33021,
		AccessoryCostume3DID: 53129,
	}, "sig")
	if err != nil {
		t.Fatalf("resolve combo failed: %v", err)
	}
	if selection.HeadCostume3DID != 53129 || selection.HeadOptionalCostume3DID != nil {
		t.Fatalf("expected empty accessory id to remain the business head selection, got %+v", selection)
	}
	if !strings.Contains(selection.ImageID, "_h53129_") || !strings.HasSuffix(selection.ImageID, "_o0") {
		t.Fatalf("expected empty accessory id in the head position, got %q", selection.ImageID)
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
		Unit:                 "school_refusal",
		BodyCostume3DID:      34,
		HairCostume3DID:      217,
		AccessoryCostume3DID: 33,
	}, "sig")
	if err != nil {
		t.Fatalf("resolve combo with empty optional failed: %v", err)
	}
	if selection.HeadCostume3DID != 33 {
		t.Fatalf("expected explicit empty optional to keep official default head 33, got %d", selection.HeadCostume3DID)
	}
	if selection.HeadOptionalCostume3DID != nil {
		t.Fatalf("did not expect a second accessory slot, got %+v", selection.HeadOptionalCostume3DID)
	}
}

func TestPreview3DRegistryResolveComboRejectsAmbiguousRoleWhenUnitIsOmitted(t *testing.T) {
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

	if _, err := registry.resolveCombo("jp", ComboQuery{BodyCostume3DID: 33001}, "sig"); err == nil {
		t.Fatal("expected ambiguous role to require more input")
	}
}

func TestPreview3DRegistryResolveComboDefaultsMissingPartsAndUsesEmptyHead(t *testing.T) {
	registry := &preview3DRegistry{
		characters: []preview3DCharacterEntry{
			{Character3DID: 5, CharacterID: 5, Unit: "idol", BodyCostume3DID: 10, HeadCostume3DID: 105, HairCostume3DID: 205, Status: "available"},
		},
		parts: []preview3DPartEntry{
			{Costume3DID: 9, PartType: "head_optional", CharacterID: 5, Unit: "idol", Status: "empty"},
			{Costume3DID: 10, PartType: "body", CharacterID: 5, Unit: "idol", Status: "planned"},
			{Costume3DID: 105, PartType: "head", CharacterID: 5, Unit: "idol", Status: "planned"},
			{Costume3DID: 205, PartType: "hair", CharacterID: 5, Unit: "idol", Status: "planned"},
		},
	}

	selection, err := registry.resolveCombo("en", ComboQuery{HairCostume3DID: 205}, "sig")
	if err != nil {
		t.Fatalf("resolve sparse combo failed: %v", err)
	}
	if selection.RoleID != "5:idol" || selection.BodyCostume3DID != 10 || selection.HairCostume3DID != 205 {
		t.Fatalf("expected role defaults, got %+v", selection)
	}
	if selection.HeadCostume3DID != 9 || selection.HeadOptionalCostume3DID != nil {
		t.Fatalf("expected empty head 9 without optional accessory, got head=%d optional=%+v", selection.HeadCostume3DID, selection.HeadOptionalCostume3DID)
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
		Unit:                 "idol",
		BodyCostume3DID:      10,
		HairCostume3DID:      205,
		AccessoryCostume3DID: 105,
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

func TestPreview3DServiceUsesRegionEngineBaseURL(t *testing.T) {
	var cnCapturePayload map[string]any
	jpRequests := atomic.Int32{}
	jpEngine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jpRequests.Add(1)
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
		default:
			t.Fatalf("unexpected jp engine request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer jpEngine.Close()

	cnEngine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && strings.HasPrefix(r.URL.Path, "/captures/") {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Path {
		case "/runtime/character3d-index.json":
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"entries":[{"character3dId":6,"characterId":21,"unit":"idol","bodyCostume3dId":44001,"headCostume3dId":44011,"hairCostume3dId":44021,"status":"available"}]}`)
		case "/runtime/parts/part-registry.json":
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"entries":[
				{"costume3dId":44001,"partType":"body","characterId":21,"unit":"idol","colorId":1,"costume3dGroupId":440,"status":"available"},
				{"costume3dId":44011,"partType":"head","characterId":21,"unit":"idol","colorId":1,"costume3dGroupId":440,"status":"available"},
				{"costume3dId":44021,"partType":"hair","characterId":21,"unit":"idol","colorId":1,"costume3dGroupId":440,"status":"available"}
			]}`)
		case "/runtime/parts/head-hair-compatibility.json":
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"rules":[{"unit":"idol","headCostume3dId":44011,"hairCostume3dId":44021,"state":"available"}]}`)
		case "/capture":
			if err := json.NewDecoder(r.Body).Decode(&cnCapturePayload); err != nil {
				t.Fatalf("decode cn capture payload: %v", err)
			}
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"ok":true}`)
		default:
			t.Fatalf("unexpected cn engine request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer cnEngine.Close()

	service := NewPreview3DService(Preview3DConfig{
		Enabled:             true,
		EngineBaseURL:       jpEngine.URL,
		EngineBaseURLs:      map[string]string{"cn": cnEngine.URL},
		CaptureCacheVersion: "test",
		CaptureExistsTTL:    -1,
	})
	if err := service.EnsurePreviewCapture(context.Background(), "cn", 44001); err != nil {
		t.Fatalf("EnsurePreviewCapture failed: %v", err)
	}
	imageID, _ := cnCapturePayload["imageId"].(string)
	if !strings.HasPrefix(imageID, "pjsk3d_cn_") {
		t.Fatalf("expected cn image id, got %q", imageID)
	}
	if jpRequests.Load() != 0 {
		t.Fatalf("cn preview should not call jp engine, requests=%d", jpRequests.Load())
	}
}

func TestPreview3DServiceRejectsMissingRegionEngineWhenMapConfigured(t *testing.T) {
	service := NewPreview3DService(Preview3DConfig{
		Enabled:        true,
		EngineBaseURL:  "http://legacy-jp-engine",
		EngineBaseURLs: map[string]string{"cn": "http://cn-engine"},
	})

	_, err := service.endpointForRegion("en")
	if err == nil {
		t.Fatal("expected missing en engine to be rejected")
	}
	if !strings.Contains(err.Error(), "region en") {
		t.Fatalf("expected region in error, got %v", err)
	}
}

func TestPreview3DServiceDoesNotFallbackWhenRegionMapContainsEmptyURL(t *testing.T) {
	service := NewPreview3DService(Preview3DConfig{
		Enabled:        true,
		EngineBaseURL:  "http://legacy-jp-engine",
		EngineBaseURLs: map[string]string{"cn": " "},
	})

	_, err := service.endpointForRegion("jp")
	if err == nil {
		t.Fatal("expected empty region map entry to disable legacy fallback")
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
		EngineBaseURLs:        map[string]string{"jp": engine.URL, "cn": engine.URL},
		CaptureMaxConcurrency: 1,
		CaptureAcquireTimeout: time.Second,
		CaptureExistsTTL:      -1,
		TemporaryCaptureTTL:   time.Hour,
	})
	jpEndpoint, err := service.endpointForRegion("jp")
	if err != nil {
		t.Fatalf("jp endpoint: %v", err)
	}
	cnEndpoint, err := service.endpointForRegion("cn")
	if err != nil {
		t.Fatalf("cn endpoint: %v", err)
	}

	selections := []preview3DSelection{
		{ImageID: "pjsk3d_a", RoleID: "1:unit", BodyCostume3DID: 1, HeadCostume3DID: 2, HairCostume3DID: 3},
		{ImageID: "pjsk3d_b", RoleID: "1:unit", BodyCostume3DID: 1, HeadCostume3DID: 2, HairCostume3DID: 4},
	}
	endpoints := []preview3DEndpoint{jpEndpoint, cnEndpoint}
	var wg sync.WaitGroup
	errs := make(chan error, len(selections))
	for index, selection := range selections {
		wg.Add(1)
		go func(endpoint preview3DEndpoint, selection preview3DSelection) {
			defer wg.Done()
			errs <- service.ensureCapture(context.Background(), endpoint, selection, "persistent")
		}(endpoints[index], selection)
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
