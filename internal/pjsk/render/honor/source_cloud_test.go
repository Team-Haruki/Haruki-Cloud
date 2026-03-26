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
		HonorRarity:     json.RawMessage(`"low"`),
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
