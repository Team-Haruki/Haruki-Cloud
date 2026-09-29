package mysekai

import "fmt"

// The game counts ordinary materials against one shared warehouse capacity.
// Character and birthday-party materials may be received beyond that limit.
func (c *Controller) shopMaterialCapacity(merged map[string]any, contents []ShopResource, resolver mysekaiResourceResolver) (*int, error) {
	quantity := 0
	for _, resource := range contents {
		if resource.ResourceType != "mysekai_material" {
			continue
		}
		row := resolver.materials[resource.ResourceID]
		if row == nil {
			return nil, fmt.Errorf("mysekai shop masterdata missing material %d", resource.ResourceID)
		}
		kind := stringValue(row["mysekaiMaterialType"])
		if kind != "game_character" && kind != "birthday_party" {
			quantity += max(0, resource.Quantity)
		}
	}
	if quantity == 0 {
		return nil, nil
	}
	gamedata, ok := merged["userMysekaiGamedata"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mysekai shop snapshot missing userMysekaiGamedata")
	}
	possession, ok := merged["userMysekaiMaterialPossession"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mysekai shop snapshot missing userMysekaiMaterialPossession")
	}
	level := intNumber(gamedata["mysekaiMaterialPossessionLevel"], 0)
	for _, row := range c.masterdata.loadList("mysekaiMaterialPossessions.json") {
		if intNumber(row["level"], -1) == level {
			remaining := max(0, intNumber(row["possessionLimit"], 0)-intNumber(possession["quantity"], 0)) / quantity
			return &remaining, nil
		}
	}
	return nil, fmt.Errorf("mysekai shop masterdata missing material possession level %d", level)
}
