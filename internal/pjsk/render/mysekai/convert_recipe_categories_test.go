package mysekai

import (
	"slices"
	"testing"
	"time"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/drawing"
)

func TestConvertRecipeCategoriesKeepAllDiamondRecipesTogether(t *testing.T) {
	data := convertRecipeTestData()
	delete(data, "mysekaiConvertItemTabs.json")
	data["mysekaiMaterials.json"] = append(data["mysekaiMaterials.json"], map[string]any{"id": 12, "name": "Diamond"})
	for _, id := range []int{10015, 20001} {
		data["resourceBoxes.json"] = append(data["resourceBoxes.json"], map[string]any{
			"id": id, "resourceBoxPurpose": "mysekai_convert_obtain", "name": "Diamond",
			"details": []any{map[string]any{"resourceType": "mysekai_material", "resourceId": 12, "resourceQuantity": 1}},
		})
	}
	variants := []struct{ id, tab, input, output, minutes, limit int }{
		{15, 2, 24, 20001, 120, 10},
		{36, 1, 32, 10015, 2640, 1},
		{62, 1, 64, 10015, 2640, 1},
		{63, 1, 94, 10015, 2640, 1},
	}
	for _, variant := range variants {
		row := convertRecipeTestRow(variant.id, variant.tab, "resource_box", variant.output)
		row["consumeResourceBoxId"] = 30000 + variant.id
		row["convertMinutes"], row["onceConvertLimit"] = variant.minutes, variant.limit
		data["mysekaiConvertItems.json"] = append(data["mysekaiConvertItems.json"], row)
		data["resourceBoxes.json"] = append(data["resourceBoxes.json"], map[string]any{
			"id": 30000 + variant.id, "resourceBoxPurpose": "mysekai_convert_consumption",
			"details": []any{
				map[string]any{"resourceType": "mysekai_material", "resourceId": variant.input, "resourceQuantity": 16},
				map[string]any{"resourceType": "coin", "resourceQuantity": 50000},
			},
		})
	}
	// Deliberately misleading tab IDs must not affect output grouping or order.
	data["mysekaiConvertItems.json"] = append(data["mysekaiConvertItems.json"],
		convertRecipeTestRow(22, 1, "mysekai_blueprint", 1),
		convertRecipeTestRow(26, 1, "mysekai_music_record", 1),
	)
	request, err := (&Controller{masterdata: data}).BuildConvertRecipeRequest(ConvertRecipeQuery{})
	if err != nil {
		t.Fatal(err)
	}
	var ids []int
	byID := make(map[int]drawing.MysekaiConvertRecipe)
	for _, recipe := range request.Recipes {
		ids = append(ids, recipe.ID)
		byID[recipe.ID] = recipe
	}
	if request.Total != 7 || !slices.Equal(ids, []int{1, 15, 36, 62, 63, 22, 26}) {
		t.Fatalf("output category order lost or recipes merged: total=%d IDs=%v", request.Total, ids)
	}
	for _, variant := range variants {
		recipe := byID[variant.id]
		if recipe.CategoryKey != "mysekai_material" || recipe.Category != i18n.T("convert.category.mysekai_material") {
			t.Errorf("diamond recipe %d retained its original tab category: %+v", variant.id, recipe)
		}
		if len(recipe.Inputs) != 2 || recipe.Inputs[0].ID != variant.input || len(recipe.Outputs) != 1 || recipe.Outputs[0].ID != 12 {
			t.Errorf("diamond recipe %d lost its distinct resources: %+v", variant.id, recipe)
		}
		if recipe.Duration != i18n.FormatDuration(time.Duration(variant.minutes)*time.Minute).String() {
			t.Errorf("diamond recipe %d lost its conversion time: %+v", variant.id, recipe)
		}
	}
	if byID[1].CategoryKey != "normal" || byID[22].CategoryKey != "mysekai_blueprint" || byID[26].CategoryKey != "mysekai_music_record" {
		t.Fatalf("output classifications did not override source tabs: %+v", byID)
	}
}

func TestConvertRecipeOutputCategoriesHandleBundles(t *testing.T) {
	for _, test := range []struct {
		name     string
		types    []string
		category string
		order    int
	}{
		{"multiple gems", []string{"material", "material", "material", "material", "material"}, "normal", 10},
		{"multiple mysekai materials", []string{"mysekai_material", "mysekai_material"}, "mysekai_material", 20},
		{"ordinary item types", []string{"boost_item", "practice_ticket", "mysekai_item"}, "normal", 10},
		{"mixed output classes", []string{"mysekai_material", "mysekai_blueprint"}, "normal", 10},
		{"reversed mixed output classes", []string{"mysekai_blueprint", "mysekai_material"}, "normal", 10},
		{"missing output", nil, "normal", 10},
		{"unknown output", []string{"future_resource_type"}, "normal", 10},
	} {
		t.Run(test.name, func(t *testing.T) {
			resources := make([]drawing.MysekaiConvertResource, 0, len(test.types))
			for _, kind := range test.types {
				resources = append(resources, drawing.MysekaiConvertResource{ResourceType: kind})
			}
			category, order := convertRecipeOutputCategory(resources)
			if category != test.category || order != test.order {
				t.Fatalf("category=%q order=%d, want %q order=%d", category, order, test.category, test.order)
			}
		})
	}
}
