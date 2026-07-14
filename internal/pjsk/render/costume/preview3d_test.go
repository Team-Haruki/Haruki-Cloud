package costume

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/shamaton/msgpack/v3"
)

type preview3DRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn preview3DRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func compactRegistryBytes(t *testing.T, payload any) []byte {
	t.Helper()
	packed, err := msgpack.MarshalAsArray(payload)
	if err != nil {
		t.Fatalf("marshal compact registry: %v", err)
	}
	var compressed bytes.Buffer
	writer := brotli.NewWriter(&compressed)
	if _, err := writer.Write(packed); err != nil {
		t.Fatalf("compress compact registry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close compact registry writer: %v", err)
	}
	return compressed.Bytes()
}

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

func TestPreview3DRegistryResolveFillsMissingPartsFromSelectedGroup(t *testing.T) {
	registry := &preview3DRegistry{
		characters: []preview3DCharacterEntry{{
			Character3DID:   5,
			CharacterID:     20,
			Unit:            "school_refusal",
			BodyCostume3DID: 99001,
			HeadCostume3DID: 99011,
			HairCostume3DID: 99021,
			Status:          "available",
		}},
		parts: []preview3DPartEntry{
			{Costume3DID: 33001, PartType: "body", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33002, PartType: "body", CharacterID: 20, Unit: "school_refusal", ColorID: 2, Costume3DGroupID: 330, Status: "available"},
			{Costume3DID: 33011, PartType: "head", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, BaseSourceKey: "group-330-head", Status: "available"},
			{Costume3DID: 33012, PartType: "head", CharacterID: 20, Unit: "school_refusal", ColorID: 2, Costume3DGroupID: 330, BaseSourceKey: "group-330-head", Status: "available"},
			{Costume3DID: 33021, PartType: "hair", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Costume3DGroupID: 330, Status: "available"},
		},
	}

	for _, test := range []struct {
		name   string
		input  int
		bodyID int
		headID int
		hairID int
	}{
		{name: "body color 2", input: 33002, bodyID: 33002, headID: 33012, hairID: 33021},
		{name: "head color 2", input: 33012, bodyID: 33002, headID: 33012, hairID: 33021},
		{name: "hair color 1", input: 33021, bodyID: 33001, headID: 33011, hairID: 33021},
	} {
		t.Run(test.name, func(t *testing.T) {
			selection, err := registry.resolve("jp", test.input)
			if err != nil {
				t.Fatalf("resolve failed: %v", err)
			}
			if selection.BodyCostume3DID != test.bodyID || selection.HeadCostume3DID != test.headID || selection.HairCostume3DID != test.hairID {
				t.Fatalf("expected same-group tuple body=%d head=%d hair=%d, got %+v", test.bodyID, test.headID, test.hairID, selection)
			}
		})
	}
}

func TestPreview3DRegistryResolveKeepsOfficialHeadOptionalPackagePath(t *testing.T) {
	const headPackagePath = "parts/_sources/head_optional/0033/a02"
	registry := &preview3DRegistry{
		characters: []preview3DCharacterEntry{
			{Character3DID: 1, CharacterID: 1, Unit: "light_sound", BodyCostume3DID: 2, HeadCostume3DID: 1, HairCostume3DID: 201, Status: "available"},
			{Character3DID: 43, CharacterID: 1, Unit: "light_sound", BodyCostume3DID: 35002, HeadCostume3DID: 35001, HairCostume3DID: 201, Status: "available"},
		},
		parts: []preview3DPartEntry{
			{Costume3DID: 35002, Costume3DGroupID: 35002, PartType: "body", CharacterID: 1, Unit: "light_sound", ColorID: 1, PackagePath: "parts/body/0035/0002", Status: "available"},
			{Costume3DID: 35001, Costume3DGroupID: 35001, PartType: "head_optional", CharacterID: 1, Unit: "light_sound", ColorID: 1, HeadCostume3DAssetbundleType: "head_only", PackagePath: headPackagePath, Status: "available"},
			{Costume3DID: 201, PartType: "hair", CharacterID: 1, Unit: "light_sound", ColorID: 1, PackagePath: "parts/hair/0002/0001", Status: "available"},
		},
	}

	selection, err := registry.resolve("jp", 35002)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if selection.HeadCostume3DID != 35001 || selection.HeadPackagePath != headPackagePath {
		t.Fatalf("official head-only source was lost: %+v", selection)
	}
}

func TestPreview3DRegistryResolveRejectsIndependentGroupHeadSources(t *testing.T) {
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
			{Costume3DID: 33002, Costume3DGroupID: 330, PartType: "body", CharacterID: 20, Unit: "school_refusal", ColorID: 2, Status: "available"},
			{Costume3DID: 33011, Costume3DGroupID: 330, PartType: "head", CharacterID: 20, Unit: "school_refusal", ColorID: 1, PackagePath: "parts/head/default", Status: "available"},
			{Costume3DID: 33012, Costume3DGroupID: 330, PartType: "head", CharacterID: 20, Unit: "school_refusal", ColorID: 2, PackagePath: "parts/head/color-2", Status: "available"},
			{Costume3DID: 33013, Costume3DGroupID: 330, PartType: "head_optional", CharacterID: 20, Unit: "school_refusal", ColorID: 2, PackagePath: "parts/head_optional/color-2", Status: "available"},
			{Costume3DID: 33021, Costume3DGroupID: 330, PartType: "hair", CharacterID: 20, Unit: "school_refusal", ColorID: 1, Status: "available"},
		},
	}

	_, err := registry.resolve("jp", 33002)
	if err == nil || !strings.Contains(err.Error(), "group head source is ambiguous") {
		t.Fatalf("group head resolution must reject independent head slots, got %v", err)
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
		Character3DID:        5,
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
		Character3DID:        6,
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
		Character3DID:        5,
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
		Character3DID:   17,
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
		Character3DID:        17,
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

func TestPreview3DRegistryResolveComboUsesOutfitCharacterAndColor(t *testing.T) {
	registry := &preview3DRegistry{
		characters: []preview3DCharacterEntry{
			{Character3DID: 1, CharacterID: 1, Unit: "light_sound", BodyCostume3DID: 2, HeadCostume3DID: 1, HairCostume3DID: 201, Status: "available"},
			{Character3DID: 2, CharacterID: 2, Unit: "light_sound", BodyCostume3DID: 4, HeadCostume3DID: 3, HairCostume3DID: 202, Status: "available"},
		},
		parts: []preview3DPartEntry{
			{Costume3DID: 934002, PartType: "body", CharacterID: 1, Unit: "light_sound", ColorID: 1, Costume3DGroupID: 934001, OutfitID: 934, Status: "planned"},
			{Costume3DID: 934004, PartType: "body", CharacterID: 1, Unit: "light_sound", ColorID: 2, Costume3DGroupID: 934001, OutfitID: 934, Status: "planned"},
			{Costume3DID: 934022, PartType: "body", CharacterID: 2, Unit: "light_sound", ColorID: 1, Costume3DGroupID: 934002, OutfitID: 934, Status: "planned"},
			{Costume3DID: 934024, PartType: "body", CharacterID: 2, Unit: "light_sound", ColorID: 2, Costume3DGroupID: 934002, OutfitID: 934, Status: "planned"},
			{Costume3DID: 1, PartType: "head_optional", CharacterID: 1, Unit: "light_sound", ColorID: 1, Status: "empty"},
			{Costume3DID: 11001, PartType: "head_optional", CharacterID: 1, Unit: "light_sound", ColorID: 1, Costume3DGroupID: 11001, AccessoryID: 11001, BaseSourceKey: "accessory-11", PackagePath: "parts/_sources/head_optional/accessory-11", Status: "planned"},
			{Costume3DID: 201, PartType: "hair", CharacterID: 1, Unit: "light_sound", ColorID: 1, Status: "planned"},
		},
	}

	selection, err := registry.resolveCombo("jp", ComboQuery{Character3DID: 1, OutfitID: 934, OutfitColorID: 2, AccessoryID: 11001, AccessoryColorID: 1}, "sig")
	if err != nil {
		t.Fatalf("resolve normalized outfit failed: %v", err)
	}
	if selection.RoleID != "1:light_sound" || selection.BodyCostume3DID != 934004 {
		t.Fatalf("expected character 1 color 2 body, got %+v", selection)
	}
	if selection.HeadCostume3DID != 11001 {
		t.Fatalf("expected accessory 11001, got %+v", selection)
	}
	if _, err := registry.resolveCombo("jp", ComboQuery{Character3DID: 2, AccessoryID: 11001, AccessoryColorID: 1}, "sig"); err == nil || !strings.Contains(err.Error(), "accessory not usable") {
		t.Fatalf("expected character-exclusive accessory to be rejected, got %v", err)
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

	selection, err := registry.resolveCombo("en", ComboQuery{Character3DID: 5, HairCostume3DID: 205}, "sig")
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

func TestPreview3DRegistryResolveComboDefaultsMissingPartsFromOutfitGroup(t *testing.T) {
	const groupHeadPackagePath = "parts/head/0797/0035/idol"
	registry := &preview3DRegistry{
		characters: []preview3DCharacterEntry{
			{Character3DID: 5, CharacterID: 5, Unit: "idol", BodyCostume3DID: 10, HeadCostume3DID: 105, HairCostume3DID: 205, Status: "available"},
		},
		parts: []preview3DPartEntry{
			{Costume3DID: 9, PartType: "head_optional", CharacterID: 5, Unit: "idol", ColorID: 1, Status: "empty"},
			{Costume3DID: 10, PartType: "body", CharacterID: 5, Unit: "idol", ColorID: 1, Status: "planned"},
			{Costume3DID: 105, PartType: "head", CharacterID: 5, Unit: "idol", ColorID: 1, Status: "planned"},
			{Costume3DID: 205, PartType: "hair", CharacterID: 5, Unit: "idol", ColorID: 1, Status: "planned"},
			{Costume3DID: 797033, PartType: "head", CharacterID: 5, Unit: "idol", ColorID: 1, Costume3DGroupID: 797005, BaseSourceKey: "outfit-797-head", Status: "planned"},
			{Costume3DID: 797035, PartType: "head", CharacterID: 5, Unit: "idol", ColorID: 2, Costume3DGroupID: 797005, BaseSourceKey: "outfit-797-head", PackagePath: groupHeadPackagePath, Status: "planned"},
			{Costume3DID: 797036, PartType: "body", CharacterID: 5, Unit: "idol", ColorID: 2, Costume3DGroupID: 797005, OutfitID: 797, Status: "planned"},
			{Costume3DID: 797061, PartType: "hair", CharacterID: 5, Unit: "idol", ColorID: 1, Costume3DGroupID: 797005, Status: "planned"},
		},
	}

	selection, err := registry.resolveCombo("jp", ComboQuery{
		Character3DID: 5,
		OutfitID:      797,
		OutfitColorID: 2,
	}, "sig")
	if err != nil {
		t.Fatalf("resolve combo failed: %v", err)
	}
	if selection.BodyCostume3DID != 797036 || selection.HeadCostume3DID != 797035 || selection.HairCostume3DID != 797061 {
		t.Fatalf("expected outfit group defaults, got %+v", selection)
	}
	if selection.HeadPackagePath != groupHeadPackagePath {
		t.Fatalf("expected same-color group head package, got %+v", selection)
	}
}

func TestPreview3DRegistryResolveComboDefaultsMissingPartsFromAnyAnchorGroup(t *testing.T) {
	registry := &preview3DRegistry{
		partRegistryVersion: 2,
		characters: []preview3DCharacterEntry{
			{Character3DID: 5, CharacterID: 5, Unit: "idol", BodyCostume3DID: 10, HeadCostume3DID: 105, HairCostume3DID: 205, Status: "available"},
		},
		parts: []preview3DPartEntry{
			{Costume3DID: 9, PartType: "head_optional", CharacterID: 5, Unit: "idol", ColorID: 1, Status: "empty"},
			{Costume3DID: 10, PartType: "body", CharacterID: 5, Unit: "idol", ColorID: 1, Status: "planned"},
			{Costume3DID: 105, PartType: "head", CharacterID: 5, Unit: "idol", ColorID: 1, Status: "planned"},
			{Costume3DID: 205, PartType: "hair", CharacterID: 5, Unit: "idol", ColorID: 1, Status: "planned"},
			{Costume3DID: 142033, PartType: "head", CharacterID: 5, Unit: "idol", ColorID: 1, Costume3DGroupID: 142005, PackagePath: "parts/head/0142/0033/idol", Status: "planned"},
			{Costume3DID: 142034, PartType: "body", CharacterID: 5, Unit: "idol", ColorID: 1, Costume3DGroupID: 142005, Status: "planned"},
			{Costume3DID: 142161, PartType: "hair", CharacterID: 5, Unit: "idol", ColorID: 1, Costume3DGroupID: 142005, Status: "planned"},
			{Costume3DID: 900001, PartType: "head", CharacterID: 5, Unit: "idol", ColorID: 1, Costume3DGroupID: 900005, AccessoryID: 900005, PackagePath: "parts/head/0900/0001/idol", Status: "planned"},
			{Costume3DID: 900004, PartType: "body", CharacterID: 5, Unit: "idol", ColorID: 1, Costume3DGroupID: 900005, Status: "planned"},
			{Costume3DID: 900061, PartType: "hair", CharacterID: 5, Unit: "idol", ColorID: 1, Costume3DGroupID: 900005, Status: "planned"},
		},
	}

	tests := []struct {
		name  string
		query ComboQuery
		body  int
		head  int
		hair  int
	}{
		{name: "hair", query: ComboQuery{Character3DID: 5, HairCostume3DID: 142161}, body: 142034, head: 142033, hair: 142161},
		{name: "accessory", query: ComboQuery{Character3DID: 5, AccessoryID: 900005, AccessoryColorID: 1}, body: 900004, head: 900001, hair: 900061},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selection, err := registry.resolveCombo("jp", tt.query, "sig")
			if err != nil {
				t.Fatalf("resolve combo failed: %v", err)
			}
			if selection.BodyCostume3DID != tt.body || selection.HeadCostume3DID != tt.head || selection.HairCostume3DID != tt.hair {
				t.Fatalf("expected anchor group defaults body=%d head=%d hair=%d, got %+v", tt.body, tt.head, tt.hair, selection)
			}
		})
	}
}

func TestPreview3DRegistryResolveComboUsesExplicitHairDefaultHead(t *testing.T) {
	const defaultHeadPackagePath = "parts/head/0142/0033/idol"
	registry := &preview3DRegistry{
		characters: []preview3DCharacterEntry{
			{Character3DID: 5, CharacterID: 5, Unit: "idol", BodyCostume3DID: 10, HeadCostume3DID: 105, HairCostume3DID: 205, Status: "available"},
		},
		parts: []preview3DPartEntry{
			{Costume3DID: 9, PartType: "head_optional", CharacterID: 5, Unit: "idol", ColorID: 1, PackagePath: "parts/head_optional/9/idol", Status: "empty"},
			{Costume3DID: 10, PartType: "body", CharacterID: 5, Unit: "idol", ColorID: 1, Status: "planned"},
			{Costume3DID: 105, PartType: "head", CharacterID: 5, Unit: "idol", ColorID: 1, Status: "planned"},
			{Costume3DID: 205, PartType: "hair", CharacterID: 5, Unit: "idol", ColorID: 1, Status: "planned"},
			{Costume3DID: 797035, PartType: "head", CharacterID: 5, Unit: "idol", ColorID: 2, Costume3DGroupID: 797005, BaseSourceKey: "outfit-797-head", PackagePath: "parts/head/0797/0035/idol", Status: "planned"},
			{Costume3DID: 797036, PartType: "body", CharacterID: 5, Unit: "idol", ColorID: 2, Costume3DGroupID: 797005, OutfitID: 797, Status: "planned"},
			{Costume3DID: 797061, PartType: "hair", CharacterID: 5, Unit: "idol", ColorID: 1, Costume3DGroupID: 797005, Status: "planned"},
			{Costume3DID: 142033, PartType: "head", CharacterID: 5, Unit: "idol", ColorID: 1, Costume3DGroupID: 142005, BaseSourceKey: "hair-142-head", PackagePath: defaultHeadPackagePath, Status: "planned"},
			{Costume3DID: 142035, PartType: "head", CharacterID: 5, Unit: "idol", ColorID: 2, Costume3DGroupID: 142005, BaseSourceKey: "hair-142-head", PackagePath: "parts/head/0142/0035/idol", Status: "planned"},
			{Costume3DID: 142161, PartType: "hair", CharacterID: 5, Unit: "idol", ColorID: 1, Costume3DGroupID: 142005, Status: "planned"},
			{Costume3DID: 900001, PartType: "head", CharacterID: 5, Unit: "idol", ColorID: 1, Costume3DGroupID: 900005, PackagePath: "parts/head/explicit/idol", Status: "planned"},
		},
		rules: []preview3DCompatibilityRule{
			{Unit: "idol", HeadCostume3DID: 142033, HairCostume3DID: 142161, State: "available", IsDefault: true},
			{Unit: "idol", HeadCostume3DID: 142035, HairCostume3DID: 142161, State: "available", IsDefault: true},
			{Unit: "idol", HeadCostume3DID: 9, HairCostume3DID: 142161, State: "available"},
		},
	}

	selection, err := registry.resolveCombo("jp", ComboQuery{
		Character3DID: 5,
		OutfitID:      797,
		OutfitColorID: 2,
		HairID:        2,
	}, "sig")
	if err != nil {
		t.Fatalf("resolve combo failed: %v", err)
	}
	if selection.BodyCostume3DID != 797036 || selection.HairCostume3DID != 142161 {
		t.Fatalf("expected requested outfit and hair, got %+v", selection)
	}
	if selection.HeadCostume3DID != 142033 || selection.HeadPackagePath != defaultHeadPackagePath {
		t.Fatalf("expected explicit hair's color-1 default head, got %+v", selection)
	}

	selection, err = registry.resolveCombo("jp", ComboQuery{
		Character3DID:        5,
		OutfitID:             797,
		OutfitColorID:        2,
		HairID:               2,
		AccessoryCostume3DID: 900001,
	}, "sig")
	if err != nil {
		t.Fatalf("resolve combo with explicit accessory failed: %v", err)
	}
	if selection.HeadCostume3DID != 900001 {
		t.Fatalf("expected explicit accessory to override the hair default, got %+v", selection)
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
		Character3DID:        5,
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
		Character3DID:   5,
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

func TestResolveQueryPreviewPathUsesRequested3DRole(t *testing.T) {
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/runtime/character3d-index.json":
			fmt.Fprint(w, `{"entries":[
				{"character3dId":22,"characterId":21,"unit":"idol","bodyCostume3dId":9001,"headCostume3dId":9002,"hairCostume3dId":9003,"status":"available"},
				{"character3dId":23,"characterId":21,"unit":"light_sound","bodyCostume3dId":9101,"headCostume3dId":9102,"hairCostume3dId":9103,"status":"available"}
			]}`)
		case "/runtime/parts/part-registry.json":
			fmt.Fprint(w, `{"entries":[
				{"costume3dId":9001,"partType":"body","characterId":21,"unit":"idol","colorId":1,"outfitId":1,"status":"available"},
				{"costume3dId":9002,"partType":"head","characterId":21,"unit":"idol","colorId":1,"status":"available"},
				{"costume3dId":9003,"partType":"hair","characterId":21,"unit":"idol","colorId":1,"status":"available"},
				{"costume3dId":9101,"partType":"body","characterId":21,"unit":"light_sound","colorId":1,"outfitId":1,"status":"available"},
				{"costume3dId":9102,"partType":"head","characterId":21,"unit":"light_sound","colorId":1,"status":"available"},
				{"costume3dId":9103,"partType":"hair","characterId":21,"unit":"light_sound","colorId":1,"status":"available"}
			]}`)
		case "/runtime/parts/head-hair-compatibility.json":
			fmt.Fprint(w, `{"rules":[
				{"unit":"idol","headCostume3dId":9002,"hairCostume3dId":9003,"state":"available"},
				{"unit":"light_sound","headCostume3dId":9102,"hairCostume3dId":9103,"state":"available"}
			]}`)
		default:
			t.Fatalf("unexpected engine request: %s", r.URL.Path)
		}
	}))
	defer engine.Close()

	service := NewPreview3DService(Preview3DConfig{Enabled: true, EngineBaseURL: engine.URL})
	previewPath, err := service.ResolveQueryPreviewPath(context.Background(), "jp", 9101, Query{
		OutfitID:      1,
		Character3DID: 23,
		ColorID:       1,
	})
	if err != nil {
		t.Fatalf("ResolveQueryPreviewPath failed: %v", err)
	}
	if !strings.Contains(previewPath, "c21_light_sound_b9101") {
		t.Fatalf("expected role 23 light_sound preview, got %q", previewPath)
	}
}

func TestPreview3DRegistryResolveNamedHairUsesRequested3DRole(t *testing.T) {
	registry := &preview3DRegistry{
		characters: []preview3DCharacterEntry{
			{Character3DID: 22, CharacterID: 21, Unit: "idol", BodyCostume3DID: 9001, HeadCostume3DID: 9002, HairCostume3DID: 9003, Status: "available"},
			{Character3DID: 23, CharacterID: 21, Unit: "light_sound", BodyCostume3DID: 9101, HeadCostume3DID: 9102, HairCostume3DID: 9103, Status: "available"},
		},
		parts: []preview3DPartEntry{
			{Costume3DID: 9001, PartType: "body", CharacterID: 21, Unit: "idol", ColorID: 1, Status: "available"},
			{Costume3DID: 9002, PartType: "head", CharacterID: 21, Unit: "idol", ColorID: 1, Status: "available"},
			{Costume3DID: 9003, PartType: "hair", CharacterID: 21, Unit: "idol", ColorID: 1, Status: "available"},
			{Costume3DID: 9101, PartType: "body", CharacterID: 21, Unit: "light_sound", ColorID: 1, Status: "available"},
			{Costume3DID: 9102, PartType: "head", CharacterID: 21, Unit: "light_sound", ColorID: 1, Status: "available"},
			{Costume3DID: 9103, PartType: "hair", CharacterID: 21, Unit: "light_sound", ColorID: 1, Status: "available"},
			{Costume3DID: 9203, PartType: "hair", CharacterID: 21, ColorID: 1, Status: "available"},
		},
	}

	selection, err := registry.resolveQuery("jp", 9203, Query{
		Query:            "Shared Hair",
		ExpectedPartType: "hair",
		Character3DID:    23,
		ColorID:          1,
	}, "sig")
	if err != nil {
		t.Fatalf("resolveQuery failed: %v", err)
	}
	if selection.Unit != "light_sound" || selection.HairCostume3DID != 9203 {
		t.Fatalf("unexpected named hair selection: %+v", selection)
	}
}

func TestPreview3DRegistrySeparatesExclusiveAndSharedAccessoryIDs(t *testing.T) {
	role := preview3DCharacterEntry{
		Character3DID: 2,
		CharacterID:   2,
		Unit:          "light_sound",
	}
	registry := &preview3DRegistry{parts: []preview3DPartEntry{
		{
			Costume3DID:      797001,
			Costume3DGroupID: 797001,
			PartType:         "head_optional",
			CharacterID:      1,
			Unit:             "light_sound",
			ColorID:          1,
			BaseSourceKey:    "shared",
			AccessoryID:      797,
			Status:           "available",
		},
		{
			Costume3DID:                  797009,
			Costume3DGroupID:             797002,
			PartType:                     "head",
			CharacterID:                  2,
			Unit:                         "light_sound",
			ColorID:                      1,
			BaseSourceKey:                "exclusive",
			HeadCostume3DAssetbundleType: "head_and_hair",
			AccessoryID:                  797,
			Status:                       "available",
		},
		{
			Costume3DID:                  797161,
			Costume3DGroupID:             797021,
			PartType:                     "head_optional",
			CharacterID:                  2,
			Unit:                         "light_sound",
			ColorID:                      1,
			BaseSourceKey:                "shared",
			HeadCostume3DAssetbundleType: "head_only",
			AccessoryID:                  797,
			Status:                       "available",
		},
		{
			Costume3DID:                  797162,
			Costume3DGroupID:             797021,
			PartType:                     "head_optional",
			CharacterID:                  2,
			Unit:                         "light_sound",
			ColorID:                      2,
			BaseSourceKey:                "shared-color-2",
			HeadCostume3DAssetbundleType: "head_only",
			AccessoryID:                  797,
			Status:                       "available",
		},
		{
			Costume3DID:                  797165,
			Costume3DGroupID:             797021,
			PartType:                     "head_optional",
			CharacterID:                  2,
			Unit:                         "piapro",
			ColorID:                      2,
			BaseSourceKey:                "shared-color-piapro",
			HeadCostume3DAssetbundleType: "head_only",
			Status:                       "available",
		},
	}}

	shared, ok := registry.accessoryPartForRole(797001, 1, role)
	if !ok || shared.Costume3DID != 797161 {
		t.Fatalf("shared accessory resolved to wrong raw part: %+v ok=%v", shared, ok)
	}
	exclusive, ok := registry.accessoryPartForRole(797002, 1, role)
	if !ok || exclusive.Costume3DID != 797009 {
		t.Fatalf("exclusive accessory resolved to wrong raw part: %+v ok=%v", exclusive, ok)
	}
	sharedColor, ok := registry.accessoryPartForRole(797001, 2, role)
	if !ok || sharedColor.Costume3DID != 797162 {
		t.Fatalf("shared accessory color did not inherit its original source id: %+v ok=%v", sharedColor, ok)
	}
	if collapsed, ok := registry.accessoryPartForRole(797, 1, role); ok {
		t.Fatalf("legacy collapsed id must not select either component: %+v", collapsed)
	}
	ids := registry.accessoryIDsForRole(role)
	if !slices.Equal(ids[797009], []int{797002}) || !slices.Equal(ids[797161], []int{797001}) || !slices.Equal(ids[797162], []int{797001}) {
		t.Fatalf("unexpected raw-to-public accessory ids: %+v", ids)
	}
	piaproIDs := registry.accessoryIDsForRole(preview3DCharacterEntry{Character3DID: 21, CharacterID: 2, Unit: "piapro"})
	if !slices.Equal(piaproIDs[797165], []int{797001}) {
		t.Fatalf("v1 color-only cross-unit alias did not inherit the unique original group source: %+v", piaproIDs)
	}
}

func TestPreview3DRegistryResolvesSameRawAccessoryByRoleUnit(t *testing.T) {
	registry := &preview3DRegistry{parts: []preview3DPartEntry{
		{Costume3DID: 2003001, Costume3DGroupID: 2003001, PartType: "head_optional", CharacterID: 21, Unit: "piapro", ColorID: 1, BaseSourceKey: "shared", Status: "available"},
		{Costume3DID: 2003129, Costume3DGroupID: 2003017, PartType: "head", CharacterID: 21, Unit: "idol", ColorID: 1, BaseSourceKey: "exclusive", Status: "available"},
		{Costume3DID: 2003129, Costume3DGroupID: 2003017, PartType: "head_optional", CharacterID: 21, Unit: "light_sound", ColorID: 1, BaseSourceKey: "shared", Status: "available"},
	}}

	idolIDs := registry.accessoryIDsForRole(preview3DCharacterEntry{Character3DID: 22, CharacterID: 21, Unit: "idol"})
	lightSoundIDs := registry.accessoryIDsForRole(preview3DCharacterEntry{Character3DID: 23, CharacterID: 21, Unit: "light_sound"})
	if !slices.Equal(idolIDs[2003129], []int{2003017}) {
		t.Fatalf("idol role should resolve the raw id as the exclusive accessory: %+v", idolIDs)
	}
	if !slices.Equal(lightSoundIDs[2003129], []int{2003001}) {
		t.Fatalf("light_sound role should resolve the same raw id as the shared accessory: %+v", lightSoundIDs)
	}
}

func TestPreview3DRegistryKeepsIndependentSourcesSeparate(t *testing.T) {
	role := preview3DCharacterEntry{
		Character3DID:   2,
		CharacterID:     2,
		Unit:            "light_sound",
		BodyCostume3DID: 100,
		HeadCostume3DID: 101,
		HairCostume3DID: 102,
		Status:          "available",
	}
	registry := &preview3DRegistry{
		partRegistryVersion: 2,
		characters:          []preview3DCharacterEntry{role},
		parts: []preview3DPartEntry{
			{Costume3DID: 100, PartType: "body", CharacterID: 2, Unit: "light_sound", ColorID: 1, Status: "available"},
			{Costume3DID: 101, PartType: "head", CharacterID: 2, Unit: "light_sound", ColorID: 1, Status: "available"},
			{Costume3DID: 102, PartType: "hair", CharacterID: 2, Unit: "light_sound", ColorID: 1, Status: "available"},
			{Costume3DID: 797009, Costume3DGroupID: 797001, PartType: "head_optional", CharacterID: 2, Unit: "light_sound", ColorID: 1, BaseSourceKey: "shared", PackagePath: "parts/_sources/head_optional/shared", AccessoryID: 797001, Status: "available"},
			{Costume3DID: 797009, Costume3DGroupID: 797002, PartType: "head", CharacterID: 2, Unit: "light_sound", ColorID: 1, BaseSourceKey: "exclusive", PackagePath: "parts/_sources/head/exclusive", AccessoryID: 797002, Status: "available"},
		},
	}

	ids := registry.accessoryIDsForRole(role)
	if !slices.Equal(ids[797009], []int{797001, 797002}) {
		t.Fatalf("same raw id must retain both independent accessory identities: %+v", ids)
	}
	catalog := registry.accessoryCatalog([]preview3DCharacterEntry{role})
	if len(catalog) != 2 || catalog[0].AccessoryID != 797001 || catalog[1].AccessoryID != 797002 {
		t.Fatalf("catalog collapsed generic and role-specific sources: %+v", catalog)
	}
	shared, sharedOK := registry.accessoryPartForRole(797001, 1, role)
	exclusive, exclusiveOK := registry.accessoryPartForRole(797002, 1, role)
	if !sharedOK || !exclusiveOK || shared.BaseSourceKey == exclusive.BaseSourceKey {
		t.Fatalf("public ids did not resolve their own sources: shared=%+v exclusive=%+v", shared, exclusive)
	}
	sharedSelection, err := registry.resolveCombo("jp", ComboQuery{Character3DID: 2, AccessoryID: 797001, AccessoryColorID: 1}, "same")
	if err != nil {
		t.Fatalf("resolve shared accessory: %v", err)
	}
	exclusiveSelection, err := registry.resolveCombo("jp", ComboQuery{Character3DID: 2, AccessoryID: 797002, AccessoryColorID: 1}, "same")
	if err != nil {
		t.Fatalf("resolve exclusive accessory: %v", err)
	}
	if sharedSelection.HeadCostume3DID != exclusiveSelection.HeadCostume3DID {
		t.Fatalf("fixture must exercise the same raw id: shared=%+v exclusive=%+v", sharedSelection, exclusiveSelection)
	}
	if sharedSelection.HeadPackagePath != shared.PackagePath || exclusiveSelection.HeadPackagePath != exclusive.PackagePath {
		t.Fatalf("independent sources were not carried into capture selections: shared=%+v exclusive=%+v", sharedSelection, exclusiveSelection)
	}
	if sharedSelection.ImageID == exclusiveSelection.ImageID {
		t.Fatalf("independent source images collapsed into one cache key: %q", sharedSelection.ImageID)
	}
	if _, err := registry.resolveCombo("jp", ComboQuery{Character3DID: 2, AccessoryCostume3DID: 797009}, ""); err == nil || !strings.Contains(err.Error(), "raw id is ambiguous") {
		t.Fatalf("raw combo must not choose one independent source, got %v", err)
	}
	if candidates := registry.legacyAccessoryIDsForRole(797, role); !slices.Equal(candidates, []int{797001, 797002}) {
		t.Fatalf("legacy accessory family must expose both independent ids: %+v", candidates)
	}
	if candidates := registry.legacyAccessoryIDsForRole(797001, role); len(candidates) != 0 {
		t.Fatalf("canonical accessory id must not be treated as legacy: %+v", candidates)
	}
	_, err = registry.resolveCombo("jp", ComboQuery{Character3DID: 2, AccessoryID: 797, AccessoryColorID: 1}, "")
	var legacyErr *LegacyAccessoryIDError
	if !errors.As(err, &legacyErr) || !slices.Equal(legacyErr.AccessoryIDs, []int{797001, 797002}) || !strings.Contains(err.Error(), "ids=[797001 797002]") {
		t.Fatalf("legacy combo id must list every independent candidate without choosing one, got %v", err)
	}
}

func TestPreview3DRegistryLegacyAccessoryIDsUseOriginalGroupFamilies(t *testing.T) {
	role := preview3DCharacterEntry{Character3DID: 2, CharacterID: 2, Unit: "light_sound"}
	registry := &preview3DRegistry{
		partRegistryVersion: 2,
		parts: []preview3DPartEntry{
			{Costume3DID: 11001, Costume3DGroupID: 11001, PartType: "head_optional", CharacterID: 2, Unit: "light_sound", ColorID: 1, BaseSourceKey: "cross-family", AccessoryID: 11001, Status: "available"},
			{Costume3DID: 12001, Costume3DGroupID: 12001, PartType: "head_optional", CharacterID: 2, Unit: "light_sound", ColorID: 1, BaseSourceKey: "cross-family", AccessoryID: 11001, Status: "available"},
		},
	}
	if err := registry.validateAccessoryIdentity(); err != nil {
		t.Fatalf("cross-family source fixture is invalid: %v", err)
	}
	if candidates := registry.legacyAccessoryIDsForRole(12, role); !slices.Equal(candidates, []int{11001}) {
		t.Fatalf("legacy id must follow each row's original group family: %+v", candidates)
	}
	if candidates := registry.legacyAccessoryIDsForRole(11001, role); len(candidates) != 0 {
		t.Fatalf("canonical accessory id must remain an exact id: %+v", candidates)
	}
}

func TestPreview3DRegistryRejectsSamePackagePathAcrossRawHeadSlots(t *testing.T) {
	registry := &preview3DRegistry{
		partRegistryVersion: 2,
		characters: []preview3DCharacterEntry{{
			Character3DID:   2,
			CharacterID:     2,
			Unit:            "light_sound",
			BodyCostume3DID: 100,
			HeadCostume3DID: 101,
			HairCostume3DID: 102,
			Status:          "available",
		}},
		parts: []preview3DPartEntry{
			{Costume3DID: 100, PartType: "body", CharacterID: 2, Unit: "light_sound", ColorID: 1, Status: "available"},
			{Costume3DID: 101, PartType: "head", CharacterID: 2, Unit: "light_sound", ColorID: 1, Status: "available"},
			{Costume3DID: 102, PartType: "hair", CharacterID: 2, Unit: "light_sound", ColorID: 1, Status: "available"},
			{Costume3DID: 700, PartType: "head", CharacterID: 2, Unit: "light_sound", ColorID: 1, BaseSourceKey: "same", PackagePath: "parts/_sources/head/same", Status: "available"},
			{Costume3DID: 700, PartType: "head_optional", CharacterID: 2, Unit: "light_sound", ColorID: 1, BaseSourceKey: "same", PackagePath: "parts/_sources/head/same", Status: "available"},
		},
	}

	_, err := registry.resolveCombo("jp", ComboQuery{Character3DID: 2, AccessoryCostume3DID: 700}, "")
	if err == nil || !strings.Contains(err.Error(), "head raw id is ambiguous") {
		t.Fatalf("raw head selection must keep head slots distinct even when package paths match, got %v", err)
	}
}

func TestPreview3DRegistryRejectsConflictingCanonicalAndRawAccessorySelectors(t *testing.T) {
	registry := &preview3DRegistry{}
	_, err := registry.resolveCombo("jp", ComboQuery{
		Character3DID:        2,
		AccessoryID:          797001,
		AccessoryCostume3DID: 797009,
	}, "same")
	if err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("conflicting accessory identities must be rejected, got %v", err)
	}
}

func TestPreview3DCaptureCarriesExactHeadPackagePath(t *testing.T) {
	var payload map[string]any
	transport := preview3DRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/capture" {
			t.Fatalf("unexpected engine request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode capture payload: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Request:    r,
		}, nil
	})

	service := NewPreview3DService(Preview3DConfig{Enabled: true, EngineBaseURL: "http://engine.test"})
	service.client = &http.Client{Transport: transport}
	endpoint, err := service.endpointForRegion("jp")
	if err != nil {
		t.Fatalf("resolve endpoint: %v", err)
	}
	selection := preview3DSelection{
		ImageID:         "pjsk3d_exact_source",
		RoleID:          "2:light_sound",
		BodyCostume3DID: 100,
		HeadCostume3DID: 797009,
		HeadPackagePath: "parts/_sources/head_optional/shared",
		HairCostume3DID: 102,
		AccessoryID:     797001,
	}
	if err := service.captureSelection(context.Background(), endpoint, selection, "persistent"); err != nil {
		t.Fatalf("capture selection: %v", err)
	}
	if got := payload["headPackagePath"]; got != selection.HeadPackagePath {
		t.Fatalf("headPackagePath was not carried to Engine: got %#v want %q", got, selection.HeadPackagePath)
	}
}

func TestPreview3DRegistryVersion2UsesAndValidatesExportedAccessoryIDs(t *testing.T) {
	registry := &preview3DRegistry{
		partRegistryVersion: 2,
		parts: []preview3DPartEntry{
			{Costume3DID: 30301, Costume3DGroupID: 303, PartType: "head_optional", CharacterID: 2, Unit: "light_sound", ColorID: 1, BaseSourceKey: "not-an-accessory", Status: "available"},
			{Costume3DID: 797001, Costume3DGroupID: 797001, PartType: "head_optional", CharacterID: 1, Unit: "light_sound", ColorID: 1, BaseSourceKey: "shared", AccessoryID: 797001, Status: "available"},
			{Costume3DID: 797161, Costume3DGroupID: 797021, PartType: "head_optional", CharacterID: 2, Unit: "light_sound", ColorID: 1, BaseSourceKey: "shared", AccessoryID: 797001, Status: "available"},
			{Costume3DID: 797162, Costume3DGroupID: 797021, PartType: "head_optional", CharacterID: 2, Unit: "light_sound", ColorID: 2, BaseSourceKey: "shared-color-2", AccessoryID: 797001, Status: "available"},
			{Costume3DID: 797164, Costume3DGroupID: 797021, PartType: "head_optional", CharacterID: 2, Unit: "piapro", ColorID: 4, BaseSourceKey: "shared", AccessoryID: 797001, Status: "available"},
			{Costume3DID: 797165, Costume3DGroupID: 797021, PartType: "head_optional", CharacterID: 2, Unit: "piapro", ColorID: 2, BaseSourceKey: "shared-color-piapro", AccessoryID: 797001, Status: "available"},
		},
	}
	if err := registry.validateAccessoryIdentity(); err != nil {
		t.Fatalf("valid v2 accessory registry rejected: %v", err)
	}
	ids := registry.accessoryIDsForRole(preview3DCharacterEntry{Character3DID: 2, CharacterID: 2, Unit: "light_sound"})
	if !slices.Equal(ids[797161], []int{797001}) || !slices.Equal(ids[797162], []int{797001}) {
		t.Fatalf("v2 registry accessory ids were not consumed: %+v", ids)
	}
	if len(ids[30301]) != 0 {
		t.Fatalf("group ids below the exporter threshold must remain unexposed: %+v", ids)
	}
	piaproIDs := registry.accessoryIDsForRole(preview3DCharacterEntry{Character3DID: 21, CharacterID: 2, Unit: "piapro"})
	if !slices.Equal(piaproIDs[797164], []int{797001}) || !slices.Equal(piaproIDs[797165], []int{797001}) {
		t.Fatalf("color-only aliases did not inherit direct or unique-group original sources: %+v", piaproIDs)
	}
}

func TestPreview3DRegistryVersion2RejectsConflictingDirectAndFamilySources(t *testing.T) {
	registry := &preview3DRegistry{
		partRegistryVersion: 2,
		parts: []preview3DPartEntry{
			{Costume3DID: 1001, Costume3DGroupID: 1001, PartType: "head_optional", CharacterID: 1, Unit: "light_sound", ColorID: 1, BaseSourceKey: "shared", AccessoryID: 1001, Status: "available"},
			{Costume3DID: 1002, Costume3DGroupID: 1002, PartType: "head_optional", CharacterID: 1, Unit: "idol", ColorID: 1, BaseSourceKey: "exclusive", AccessoryID: 1002, Status: "available"},
			{Costume3DID: 1003, Costume3DGroupID: 1002, PartType: "head_optional", CharacterID: 1, Unit: "idol", ColorID: 2, BaseSourceKey: "shared", AccessoryID: 1002, Status: "available"},
		},
	}
	if err := registry.validateAccessoryIdentity(); err == nil || !strings.Contains(err.Error(), "multiple original-color sources") {
		t.Fatalf("expected conflicting direct/family lineage to be rejected, got %v", err)
	}
}

func TestPreview3DRegistryVersion2RejectsRowsExporterCannotProduce(t *testing.T) {
	tests := []struct {
		name  string
		parts []preview3DPartEntry
		want  string
	}{
		{
			name: "positive id without base source",
			parts: []preview3DPartEntry{
				{Costume3DID: 1001, Costume3DGroupID: 1001, PartType: "head_optional", CharacterID: 1, Unit: "idol", ColorID: 1, BaseSourceKey: "source", AccessoryID: 1001, Status: "available"},
				{Costume3DID: 1002, Costume3DGroupID: 1001, PartType: "head_optional", CharacterID: 1, Unit: "idol", ColorID: 2, AccessoryID: 1001, Status: "available"},
			},
			want: "no base source",
		},
		{
			name: "source backed missing row without id",
			parts: []preview3DPartEntry{
				{Costume3DID: 1001, Costume3DGroupID: 1001, PartType: "head_optional", CharacterID: 1, Unit: "idol", ColorID: 1, BaseSourceKey: "source", Status: "missing"},
			},
			want: "has no accessoryId",
		},
		{
			name: "id on body",
			parts: []preview3DPartEntry{
				{Costume3DID: 1001, Costume3DGroupID: 1001, PartType: "body", CharacterID: 1, Unit: "idol", ColorID: 1, BaseSourceKey: "source", AccessoryID: 1001, Status: "available"},
			},
			want: "non-accessory",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := &preview3DRegistry{partRegistryVersion: 2, parts: tt.parts}
			if err := registry.validateAccessoryIdentity(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q validation failure, got %v", tt.want, err)
			}
		})
	}
}

func TestPreview3DRegistryVersion2RejectsAccessoryIDCollision(t *testing.T) {
	registry := &preview3DRegistry{
		partRegistryVersion: 2,
		parts: []preview3DPartEntry{
			{Costume3DID: 1001, Costume3DGroupID: 1001, PartType: "head", CharacterID: 1, Unit: "light_sound", ColorID: 1, BaseSourceKey: "source-a", AccessoryID: 1001, Status: "available"},
			{Costume3DID: 1002, Costume3DGroupID: 1001, PartType: "head", CharacterID: 2, Unit: "idol", ColorID: 1, BaseSourceKey: "source-b", AccessoryID: 1001, Status: "available"},
		},
	}
	if err := registry.validateAccessoryIdentity(); err == nil || !strings.Contains(err.Error(), "maps to sources") {
		t.Fatalf("expected cross-source accessory id collision to be rejected, got %v", err)
	}
}

func TestPreview3DPartRegistryPrefersCompactMessagePack(t *testing.T) {
	payload := preview3DCompactPartRegistry{
		SchemaVersion:   compactRegistrySchemaVersion,
		RegistryVersion: 2,
		Entries: []preview3DPartEntry{{
			Costume3DID:      33001,
			PartType:         "body",
			CharacterID:      20,
			Unit:             "school_refusal",
			ColorID:          1,
			Costume3DGroupID: 330,
			OutfitID:         33,
			PackagePath:      "parts/body/33001",
			Status:           "available",
		}},
	}
	compressed := compactRegistryBytes(t, payload)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept"), compactPartRegistryContentType) {
			t.Fatalf("compact registry accept header missing: %q", r.Header.Get("Accept"))
		}
		w.Header().Set("content-type", compactPartRegistryContentType)
		_, _ = w.Write(compressed)
	}))
	defer server.Close()

	service := NewPreview3DService(Preview3DConfig{Enabled: true, EngineBaseURL: server.URL})
	registry, err := service.getPartRegistry(context.Background(), preview3DEndpoint{baseURL: server.URL})
	if err != nil {
		t.Fatalf("decode compact registry: %v", err)
	}
	if registry.Version != 2 || len(registry.Entries) != 1 || registry.Entries[0].OutfitID != 33 {
		t.Fatalf("unexpected compact registry: %+v", registry)
	}
}

func TestPreview3DCompatibilityPrefersCompactMessagePack(t *testing.T) {
	payload := preview3DCompactCompatibilityRegistry{
		SchemaVersion: compactRegistrySchemaVersion,
		Rules: []preview3DCompatibilityRule{{
			Unit:            "school_refusal",
			HeadCostume3DID: 33011,
			HairCostume3DID: 33021,
			State:           "available",
			IsDefault:       true,
		}},
	}
	compressed := compactRegistryBytes(t, payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept"), compactCompatibilityContentType) {
			t.Fatalf("compact compatibility accept header missing: %q", r.Header.Get("Accept"))
		}
		w.Header().Set("content-type", compactCompatibilityContentType)
		_, _ = w.Write(compressed)
	}))
	defer server.Close()

	service := NewPreview3DService(Preview3DConfig{Enabled: true, EngineBaseURL: server.URL})
	registry, err := service.getCompatibilityRegistry(context.Background(), preview3DEndpoint{baseURL: server.URL})
	if err != nil {
		t.Fatalf("decode compact compatibility: %v", err)
	}
	if len(registry.Rules) != 1 || !registry.Rules[0].IsDefault || registry.Rules[0].HeadCostume3DID != 33011 {
		t.Fatalf("unexpected compact compatibility: %+v", registry)
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

func TestPreview3DRegistryDecodeErrorKeepsRegistryPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()
	service := NewPreview3DService(Preview3DConfig{Enabled: true, EngineBaseURL: server.URL})
	var output map[string]any
	err := service.getJSON(context.Background(), preview3DEndpoint{baseURL: server.URL}, "/runtime/parts/part-registry.json", &output)
	if err == nil || !strings.HasPrefix(err.Error(), "3d preview registry ") {
		t.Fatalf("decode failure must keep the registry error class, got %v", err)
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
