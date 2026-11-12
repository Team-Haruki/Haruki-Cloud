package mysekai

import (
	"fmt"
	"strings"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/drawing"
)

type convertRecipeBoxKey struct {
	purpose string
	id      int
}

type convertRecipeCatalog struct {
	boxes        map[convertRecipeBoxKey]map[string]any
	blueprints   map[int]map[string]any
	records      map[int]map[string]any
	names        map[string]map[int]map[string]any
	resolveImage pathResolver
}

func loadConvertRecipeCatalog(source masterdataSource) convertRecipeCatalog {
	catalog := convertRecipeCatalog{
		boxes:      make(map[convertRecipeBoxKey]map[string]any),
		blueprints: source.loadMapByID("mysekaiConvertItemObtainMysekaiBlueprints.json"),
		records:    source.loadMapByID("mysekaiConvertItemObtainMysekaiMusicRecords.json"),
		names:      make(map[string]map[int]map[string]any),
	}
	// Resource box IDs are only unique within their purpose.
	for _, box := range source.loadList("resourceBoxes.json") {
		key := convertRecipeBoxKey{stringValue(box["resourceBoxPurpose"]), intNumber(box["id"], 0)}
		if key.purpose == "mysekai_convert_consumption" || key.purpose == "mysekai_convert_obtain" {
			catalog.boxes[key] = box
		}
	}
	for resourceType, filename := range map[string]string{
		"material":              "materials.json",
		"mysekai_material":      "mysekaiMaterials.json",
		"mysekai_item":          "mysekaiItems.json",
		"boost_item":            "boostItems.json",
		"practice_ticket":       "practiceTickets.json",
		"skill_practice_ticket": "skillPracticeTickets.json",
	} {
		catalog.names[resourceType] = source.loadMapByID(filename)
	}
	return catalog
}

func (catalog convertRecipeCatalog) entry(row map[string]any) convertRecipeEntry {
	title, outputs := catalog.output(row)
	categoryKey, categorySeq := convertRecipeOutputCategory(outputs)
	return convertRecipeEntry{
		categorySeq: categorySeq,
		recipe: drawing.MysekaiConvertRecipe{
			ID: intNumber(row["id"], 0), Name: title, Category: convertRecipeCategoryName(categoryKey), CategoryKey: categoryKey,
			Inputs:  catalog.boxResourceItems("mysekai_convert_consumption", intNumber(row["consumeResourceBoxId"], 0)),
			Outputs: outputs, Duration: convertRecipeDuration(row),
			RequiredLevel: max(0, intNumber(row["requiredConvertFixtureLevel"], 0)),
			Limits:        convertRecipeLimits(row),
		},
	}
}

func convertRecipeOutputCategory(outputs []drawing.MysekaiConvertResource) (string, int) {
	category := ""
	for _, resource := range outputs {
		kind := resource.ResourceType
		switch kind {
		case "mysekai_material", "mysekai_blueprint", "mysekai_music_record":
		default:
			kind = "normal"
		}
		if category != "" && category != kind {
			// A future mixed bundle remains one recipe in the general item group.
			return "normal", 10
		}
		category = kind
	}
	switch category {
	case "mysekai_material":
		return category, 20
	case "mysekai_blueprint":
		return category, 30
	case "mysekai_music_record":
		return category, 40
	default:
		return "normal", 10
	}
}

func (catalog convertRecipeCatalog) output(row map[string]any) (string, []drawing.MysekaiConvertResource) {
	id := intNumber(row["externalId"], 0)
	obtainType := stringValue(row["mysekaiConvertItemObtainType"])
	if obtainType == "resource_box" {
		output := catalog.boxResourceItems("mysekai_convert_obtain", id)
		title := stringValue(catalog.boxes[convertRecipeBoxKey{"mysekai_convert_obtain", id}]["name"])
		if title == "" {
			title = convertRecipeResourcesText(output)
		}
		return title, output
	}
	var item map[string]any
	var name, image string
	switch obtainType {
	case "mysekai_blueprint":
		item = catalog.blueprints[id]
		switch stringValue(item["mysekaiConvertItemObtainMysekaiBlueprintType"]) {
		case "random":
			name = i18n.T("convert.output.blueprint_random")
			image = "mysekai/icon/category_icon/icon_blueprint.png"
		case "mysekai_fixture_main_genre_choice":
			name = i18n.T("convert.output.blueprint_choice")
			image = "mysekai/icon/category_icon/icon_blueprint.png"
		case "white_blueprint":
			name = i18n.T("convert.output.blueprint_blank")
			image = "mysekai/thumbnail/item/item_blank_blueprint.png"
		}
	case "mysekai_music_record":
		item = catalog.records[id]
		switch stringValue(item["mysekaiConvertItemObtainMysekaiMusicRecordType"]) {
		case "random":
			name = i18n.T("convert.output.record_random")
			image = "mysekai/icon/category_icon/icon_record_jacket.png"
		case "mysekai_music_record_category_choice":
			name = i18n.T("convert.output.record_choice")
			image = "mysekai/icon/category_icon/icon_record_jacket.png"
		}
	}
	if name == "" {
		name = fmt.Sprintf("%s/%d", obtainType, id)
	}
	return name, []drawing.MysekaiConvertResource{{
		Name: name, ImagePath: catalog.imagePath(image), Quantity: convertRecipeQuantity(item["quantity"]),
		ResourceType: obtainType, ID: id,
	}}
}

func (catalog convertRecipeCatalog) boxResources(purpose string, id int) string {
	return convertRecipeResourcesText(catalog.boxResourceItems(purpose, id))
}

func (catalog convertRecipeCatalog) boxResourceItems(purpose string, id int) []drawing.MysekaiConvertResource {
	box := catalog.boxes[convertRecipeBoxKey{purpose, id}]
	details := convertRecipeBoxDetails(box["details"])
	resources := make([]drawing.MysekaiConvertResource, 0, len(details))
	for _, raw := range details {
		detail, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		resourceType := stringValue(detail["resourceType"])
		resourceID := intNumber(detail["resourceId"], 0)
		metadata := catalog.names[resourceType][resourceID]
		name := stringValue(metadata["name"])
		if resourceType == "coin" {
			name = i18n.T("inventory.image.resource_type.coin")
		}
		if name == "" {
			name = fmt.Sprintf("%s/%d", resourceType, resourceID)
		}
		resources = append(resources, drawing.MysekaiConvertResource{
			Name: name, ResourceType: resourceType, ID: resourceID,
			Quantity:  convertRecipeQuantity(detail["resourceQuantity"]),
			ImagePath: catalog.resourceImage(resourceType, resourceID, metadata),
		})
	}
	if len(resources) == 0 {
		resources = append(resources, drawing.MysekaiConvertResource{
			Name:         i18n.T("convert.resource.missing", i18n.Data{"Type": purpose, "ID": id}),
			ResourceType: "resource_box", ID: id,
		})
	}
	return resources
}

func (catalog convertRecipeCatalog) resourceImage(resourceType string, id int, metadata map[string]any) string {
	var relative string
	switch resourceType {
	case "coin", "virtual_coin", "jewel":
		relative = fmt.Sprintf("thumbnail/common_material/%s.png", resourceType)
	case "material":
		if id > 0 {
			relative = fmt.Sprintf("thumbnail/material/material%d.png", id)
		}
	case "boost_item":
		if id > 0 {
			relative = fmt.Sprintf("thumbnail/boost_item/boost_item%d.png", id)
		}
	case "practice_ticket", "skill_practice_ticket":
		if id > 0 {
			relative = fmt.Sprintf("thumbnail/%s/ticket%d.png", resourceType, id)
		}
	case "mysekai_material", "mysekai_item":
		if icon := stringValue(metadata["iconAssetbundleName"]); icon != "" {
			relative = fmt.Sprintf("mysekai/thumbnail/%s/%s.png", strings.TrimPrefix(resourceType, "mysekai_"), icon)
		}
	}
	return catalog.imagePath(relative)
}

func (catalog convertRecipeCatalog) imagePath(relative string) string {
	if relative == "" || catalog.resolveImage == nil {
		return relative
	}
	return catalog.resolveImage(relative)
}

func convertRecipeBoxDetails(value any) []any {
	var details []any
	switch raw := value.(type) {
	case []any:
		return raw
	case string:
		_ = decodeJSONUseNumber([]byte(raw), &details)
	case []byte:
		_ = decodeJSONUseNumber(raw, &details)
	}
	return details
}

func convertRecipeQuantity(value any) *int {
	quantity := intNumber(value, -1)
	if quantity < 0 {
		return nil
	}
	return &quantity
}

func convertRecipeResourcesText(resources []drawing.MysekaiConvertResource) string {
	parts := make([]string, 0, len(resources))
	for _, resource := range resources {
		if resource.Quantity == nil {
			parts = append(parts, i18n.T("convert.resource.quantity_unknown", i18n.Data{"Name": resource.Name}))
		} else {
			parts = append(parts, i18n.T("convert.resource.quantity", i18n.Data{"Name": resource.Name, "Quantity": i18n.Wan(float64(*resource.Quantity))}))
		}
	}
	return strings.Join(parts, " + ")
}
