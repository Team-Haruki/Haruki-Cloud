package gacha

import (
	"encoding/json"
	"testing"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/pjsk/render/common"
)

func TestConvertGachaEntityDecodesNestedFields(t *testing.T) {
	rarityRatesJSON, _ := json.Marshal([]map[string]any{
		{"id": 1, "groupId": 2, "cardRarityType": "rarity_4", "lotteryType": "normal", "rate": 3.0},
	})
	detailsJSON, _ := json.Marshal([]map[string]any{
		{"id": 10, "gachaId": 101, "cardId": 2001, "weight": 50, "isWish": true},
	})
	behaviorsJSON, _ := json.Marshal([]map[string]any{
		{"id": 20, "gachaId": 101, "gachaBehaviorType": "over_rarity_3_once", "costResourceType": "jewel", "costResourceQuantity": 3000, "spinCount": 10, "executeLimit": 1, "groupId": 1, "priority": 1, "resourceCategory": "currency", "gachaSpinnableType": "normal"},
	})
	pickupsJSON, _ := json.Marshal([]map[string]any{
		{"id": 30, "gachaId": 101, "cardId": 2001, "gachaPickupType": "pickup"},
	})
	informationJSON, _ := json.Marshal(map[string]any{
		"gachaId": 101, "summary": "summary", "description": "desc",
	})

	entity := &sekaiDB.Gacha{
		GameID:               101,
		GachaType:            "ceil",
		Name:                 "Test Gacha",
		Seq:                  7,
		AssetbundleName:      "ab_gacha_101",
		StartAt:              1000,
		EndAt:                2000,
		GachaCeilItemID:      88,
		GachaCardRarityRates: json.RawMessage(rarityRatesJSON),
		GachaDetails:         json.RawMessage(detailsJSON),
		GachaBehaviors:       json.RawMessage(behaviorsJSON),
		GachaPickups:         json.RawMessage(pickupsJSON),
		GachaInformation:     json.RawMessage(informationJSON),
	}

	model, err := common.ConvertGachaEntity(entity)
	if err != nil {
		t.Fatalf("convertGachaEntity failed: %v", err)
	}
	if model.ID != 101 || model.Name != "Test Gacha" {
		t.Fatalf("unexpected top-level fields: %+v", model)
	}
	if model.GachaCeilItemID == nil || *model.GachaCeilItemID != 88 {
		t.Fatalf("unexpected ceil item id: %#v", model.GachaCeilItemID)
	}
	if len(model.GachaDetails) != 1 || model.GachaDetails[0].CardID != 2001 {
		t.Fatalf("unexpected gacha details: %+v", model.GachaDetails)
	}
	if len(model.GachaBehaviors) != 1 || model.GachaBehaviors[0].ExecuteLimit == nil || *model.GachaBehaviors[0].ExecuteLimit != 1 {
		t.Fatalf("unexpected gacha behaviors: %+v", model.GachaBehaviors)
	}
	if model.GachaInformation.Summary != "summary" {
		t.Fatalf("unexpected gacha information: %+v", model.GachaInformation)
	}
}
