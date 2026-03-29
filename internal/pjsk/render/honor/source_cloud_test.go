package honor

import (
	"encoding/json"
	"testing"

	sekaiDB "haruki-cloud/database/sekai"
)

func TestConvertCloudHonorDecodesLevels(t *testing.T) {
	levelsJSON, _ := json.Marshal([]map[string]interface{}{
		{
			"level":           5,
			"honorRarity":     "high",
			"description":     "desc",
			"assetbundleName": "honor_top_001_lv5",
		},
	})
	entity := &sekaiDB.Honor{
		GameID:          101,
		GroupID:         20,
		HonorRarity:     "low",
		Name:            "Test Honor",
		AssetbundleName: "honor_top_001",
		Levels:          json.RawMessage(levelsJSON),
	}

	model, err := convertCloudHonor(entity)
	if err != nil {
		t.Fatalf("convertCloudHonor failed: %v", err)
	}
	if model.ID != 101 || model.GroupID != 20 {
		t.Fatalf("unexpected top-level fields: %+v", model)
	}
	if len(model.Levels) != 1 || model.Levels[0].Level != 5 || model.Levels[0].AssetBundleName != "honor_top_001_lv5" {
		t.Fatalf("unexpected levels: %+v", model.Levels)
	}
}

func TestBirthdayGroupMatchesCharacter(t *testing.T) {
	row := &sekaiDB.Gamecharacter{
		GameID:           6,
		FirstName:        "桐谷",
		GivenName:        "遥",
		FirstNameEnglish: "Kiratani",
		GivenNameEnglish: "Haruka",
	}

	if !birthdayGroupMatchesCharacter("HAPPY BIRTHDAY 遥 2025.10.5", row) {
		t.Fatal("expected Japanese given name to match birthday group")
	}
	if !birthdayGroupMatchesCharacter("HAPPY BIRTHDAY Haruka 2025.10.5", row) {
		t.Fatal("expected English given name to match birthday group")
	}
	if birthdayGroupMatchesCharacter("HAPPY BIRTHDAY 奏 2026.2.10", row) {
		t.Fatal("did not expect unrelated birthday group to match")
	}
}
