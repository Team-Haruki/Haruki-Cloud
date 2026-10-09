package mysekai

import (
	"errors"
	"fmt"
	"sort"

	"haruki-cloud/internal/pjsk/drawing"
)

// The client uses !isBought for each offered blueprint. purchaseLimit in
// master data does not replace that server-provided state or apply per item.
func (c *Controller) buildBlueprintShopGroups(query ShopQuery, merged map[string]any, rules []map[string]any, passActive bool, resolver mysekaiResourceResolver) ([]drawing.MysekaiShopGroup, error) {
	if len(rules) == 0 {
		if len(nestedList(merged, "userMysekaiBlueprintShopItems")) > 0 || query.ShopType == "blueprint" {
			return nil, shopMasterdataMissing(errors.New("mysekai shop masterdata missing blueprint rules"))
		}
		return nil, nil
	}
	lineup, err := shopSnapshotList(merged, "userMysekaiBlueprintShopItems")
	if err != nil {
		return nil, err
	}
	ownedRows, err := shopSnapshotList(merged, "userMysekaiBlueprints")
	if err != nil {
		return nil, err
	}
	ownedIDs := map[int]bool{}
	for _, raw := range ownedRows {
		row, _ := raw.(map[string]any)
		ownedIDs[intNumber(row["mysekaiBlueprintId"], 0)] = true
	}
	blueprints := c.masterdata.loadMapByID("mysekaiBlueprints.json")
	fixtures := c.masterdata.loadMapByID("mysekaiFixtures.json")
	ruleByType := map[string]map[string]any{}
	for _, rule := range rules {
		ruleByType[stringValue(rule["mysekaiBlueprintShopItemLotteryType"])] = rule
	}
	rows := make([]map[string]any, 0, len(lineup))
	for _, raw := range lineup {
		if row, ok := raw.(map[string]any); ok {
			rows = append(rows, row)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return intNumber(rows[i]["seq"], 0) < intNumber(rows[j]["seq"], 0) })
	groups := map[string]*drawing.MysekaiShopGroup{}
	for _, row := range rows {
		period := stringValue(row["mysekaiBlueprintShopItemLotteryType"])
		rule := ruleByType[period]
		if (period != "daily" && period != "weekly") || rule == nil {
			return nil, shopMasterdataMissing(fmt.Errorf("mysekai shop masterdata missing blueprint rule %s", period))
		}
		id := intNumber(row["mysekaiBlueprintId"], 0)
		blueprint := blueprints[id]
		fixture := fixtures[intNumber(blueprint["craftTargetId"], 0)]
		if blueprint == nil || fixture == nil {
			return nil, shopMasterdataMissing(fmt.Errorf("mysekai shop masterdata missing blueprint %d", id))
		}
		bought := boolValue(row["isBought"])
		available := passActive && !bought
		if !query.ShowAll && !available {
			continue
		}
		owned := ownedIDs[id]
		count := 0
		if bought {
			count = 1
		}
		remaining := 1 - count
		name := stringValue(fixture["name"])
		_, jewelImage := resolver.resolve("jewel", 0)
		item := drawing.MysekaiShopItem{
			ID: id, Name: &name, Quantity: 1,
			ImagePath:         drawing.AssetPath(fixtureThumbnailPath(func(p string) string { return c.regionPath(resolver.region, p) }, fixture)),
			Costs:             []drawing.MysekaiShopCost{{ImagePath: drawing.AssetPath(jewelImage), Quantity: intNumber(rule["consumeJewelQuantity"], 0)}},
			ExchangeLimitType: period, ExchangeLimitValue: new(1), ExchangedCount: &count,
			Owned: &owned, IsBought: &bought, Available: &available, RemainingCount: &remaining,
		}
		shopType := "blueprint_" + period
		group := groups[shopType]
		if group == nil {
			title := mysekaiShopTypeTitles[shopType]
			group = &drawing.MysekaiShopGroup{ShopType: shopType, Title: &title}
			groups[shopType] = group
		}
		group.Items = append(group.Items, item)
	}
	result := make([]drawing.MysekaiShopGroup, 0, len(groups))
	for _, group := range groups {
		result = append(result, *group)
	}
	return result, nil
}

// Keep status visible on Drawing versions predating the optional state fields.
// Decorate only the outgoing request so the builder retains the resource name.
func decorateShopRequest(request *drawing.MysekaiShopRequest) {
	for gi := range request.Shops {
		for ii := range request.Shops[gi].Items {
			item := &request.Shops[gi].Items[ii]
			name := fmt.Sprintf("ID %d", item.ID)
			if item.Name != nil && *item.Name != "" {
				name = *item.Name
			}
			if item.Owned != nil {
				if *item.Owned {
					name += "【已持有】"
				} else {
					name += "【未持有】"
				}
			}
			if item.MaterialCapacityCount != nil && *item.MaterialCapacityCount == 0 {
				name += "【材料仓库已满】"
			}
			if request.PassActive != nil && !*request.PassActive {
				name += "【需通行证】"
			}
			if item.IsBought != nil && *item.IsBought {
				name += "【本期已购买】"
			}
			if item.RemainingCount != nil && item.IsBought == nil {
				name += fmt.Sprintf("【剩余%d次】", *item.RemainingCount)
			}
			item.Name = &name
		}
	}
}
