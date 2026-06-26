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

	selection, err := registry.resolve("jp", 33002)
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

	selection, err := registry.resolve("jp", 33002)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if selection.HairCostume3DID != 33021 {
		t.Fatalf("expected resolver to skip missing default role and use hair 33021, got %d", selection.HairCostume3DID)
	}
}
