package costume

import "testing"

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
	if want := "pjsk3d_" + signature + "_jp_c20_school_refusal_g330_cl1_b33001_h33011_r33021_o0"; body.ImageID != want {
		t.Fatalf("unexpected body image id: got %q want %q", body.ImageID, want)
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
