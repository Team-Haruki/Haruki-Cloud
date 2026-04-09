package education

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/utils/drawing"
)

func (c *Controller) BuildAreaItemUpgradeMaterialsRequestFromSnapshot(query AreaItemQuery) (*drawing.AreaItemUpgradeMaterialsRequest, error) {
	ctx, err := c.resolveSnapshotContext(query.Region, query.Profile, query.Snapshot)
	if err != nil {
		return nil, err
	}

	userAreaLevels := make(map[int]int)
	for _, area := range ctx.raw.UserAreas {
		for _, item := range area.AreaItems {
			if item.AreaItemID <= 0 {
				continue
			}
			if item.Level > userAreaLevels[item.AreaItemID] {
				userAreaLevels[item.AreaItemID] = item.Level
			}
		}
	}
	if len(userAreaLevels) == 0 {
		return nil, fmt.Errorf("user snapshot is missing area item data")
	}

	userMaterials := map[int]int{
		areaCoinMaterialID: ctx.raw.UserGamedata.Coin,
	}
	for _, item := range ctx.raw.UserMaterials {
		if item.MaterialID <= 0 {
			continue
		}
		userMaterials[item.MaterialID] = item.Quantity
	}

	itemIDs := c.resolveAreaItemIDs(ctx.source, userAreaLevels, query)
	if len(itemIDs) == 0 {
		return nil, fmt.Errorf("area item masterdata is not available")
	}

	levelShopItems := c.resolveAreaItemShopItems(ctx.source, itemIDs)
	minCurrentLevel := 0
	if len(itemIDs) > 0 {
		minCurrentLevel = -1
		for _, itemID := range itemIDs {
			level := userAreaLevels[itemID]
			if minCurrentLevel == -1 || level < minCurrentLevel {
				minCurrentLevel = level
			}
		}
		if minCurrentLevel < 0 {
			minCurrentLevel = 0
		}
	}
	areaItems := make([]drawing.AreaItemInfo, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		master := ctx.source.GetAreaItem(itemID)
		levels := ctx.source.GetAreaItemLevels(itemID)
		if master == nil || len(levels) == 0 {
			continue
		}

		levelMap := make(map[int]*AreaItemLevel, len(levels))
		maxLevel := 0
		for _, level := range levels {
			if level == nil {
				continue
			}
			levelMap[level.Level] = level
			if level.Level > maxLevel {
				maxLevel = level.Level
			}
		}
		if maxLevel == 0 {
			continue
		}

		currentLevel := userAreaLevels[itemID]
		sumMaterials := make(map[int]int)
		levelInfos := make([]drawing.AreaItemLevel, 0, maxLevel-minCurrentLevel)
		for level := minCurrentLevel + 1; level <= maxLevel; level++ {
			levelMaster := levelMap[level]
			if levelMaster == nil {
				levelInfos = append(levelInfos, drawing.AreaItemLevel{Level: level, Materials: []drawing.AreaItemMaterial{}})
				continue
			}

			row := drawing.AreaItemLevel{
				Level:      level,
				Bonus:      levelMaster.Power1BonusRate,
				CanUpgrade: true,
				Materials:  []drawing.AreaItemMaterial{},
			}

			if level > currentLevel {
				shopItem := levelShopItems[itemID][level]
				if shopItem == nil {
					row.CanUpgrade = false
				} else {
					row.Materials = make([]drawing.AreaItemMaterial, 0, len(shopItem.Costs))
					for _, cost := range shopItem.Costs {
						materialID := cost.ResourceID
						if strings.EqualFold(cost.ResourceType, "coin") {
							materialID = areaCoinMaterialID
						}
						sumMaterials[materialID] += cost.Quantity
						haveQuantity := userMaterials[materialID]
						isEnough := haveQuantity >= sumMaterials[materialID]
						if !isEnough {
							row.CanUpgrade = false
						}
						row.Materials = append(row.Materials, drawing.AreaItemMaterial{
							MaterialID:       materialID,
							MaterialIconPath: c.materialIconPath(cost.ResourceType, materialID),
							Quantity:         cost.Quantity,
							HaveQuantity:     haveQuantity,
							SumQuantity:      sumMaterials[materialID],
							IsEnough:         isEnough,
						})
					}
				}
			} else {
				row.CanUpgrade = true
			}

			levelInfos = append(levelInfos, row)
		}

		targetIconPath := c.areaItemTargetIcon(levels)
		areaItem := drawing.AreaItemInfo{
			ItemID:       itemID,
			CurrentLevel: currentLevel,
			ItemIconPath: assets.ResolveRegionAssetPath(c.assets, ctx.region.String(),
				filepath.Join("areaitem", master.AssetbundleName, master.AssetbundleName+".png")),
			Levels: levelInfos,
		}
		if targetIconPath != "" {
			areaItem.TargetIconPath = &targetIconPath
		}
		areaItems = append(areaItems, areaItem)
	}

	return c.BuildAreaItemUpgradeMaterialsRequest(drawing.AreaItemUpgradeMaterialsRequest{
		Profile:    ctx.profile,
		AreaItems:  areaItems,
		HasProfile: true,
	})
}

func (c *Controller) resolveAreaItemIDs(source DataSource, userAreaLevels map[int]int, query AreaItemQuery) []int {
	if !hasAreaItemFilter(query) {
		itemIDs := make([]int, 0, len(userAreaLevels))
		for itemID := range userAreaLevels {
			if len(source.GetAreaItemLevels(itemID)) == 0 {
				continue
			}
			itemIDs = append(itemIDs, itemID)
		}
		sort.Ints(itemIDs)
		return itemIDs
	}

	filterUnit := normalizeUnit(query.Unit)
	filterAttr := normalizeAttr(query.Attr)
	filterPiapro := filterUnit == "piapro"
	if filterPiapro {
		filterUnit = ""
	}

	matched := make([]int, 0, 16)
	for _, item := range source.GetAreaItems() {
		if item == nil || item.ID <= 0 {
			continue
		}
		levels := source.GetAreaItemLevels(item.ID)
		if len(levels) == 0 {
			continue
		}
		if areaItemMatchesFilter(item, levels, filterUnit, filterAttr, query.Cid, query.Tree, query.Flower, filterPiapro) {
			matched = append(matched, item.ID)
		}
	}
	sort.Ints(matched)
	return matched
}

func (c *Controller) resolveAreaItemShopItems(source DataSource, itemIDs []int) map[int]map[int]*ShopItem {
	itemSet := make(map[int]struct{}, len(itemIDs))
	for _, itemID := range itemIDs {
		itemSet[itemID] = struct{}{}
	}

	result := make(map[int]map[int]*ShopItem, len(itemIDs))
	for _, box := range source.GetResourceBoxesByPurpose("shop_item") {
		if box == nil {
			continue
		}
		shopItem := source.GetShopItemByResourceBoxID(box.ID)
		if shopItem == nil {
			continue
		}
		for _, detail := range box.Details {
			if !strings.EqualFold(detail.ResourceType, "area_item") || detail.ResourceID <= 0 || detail.ResourceLevel <= 0 {
				continue
			}
			if _, ok := itemSet[detail.ResourceID]; !ok {
				continue
			}
			if _, ok := result[detail.ResourceID]; !ok {
				result[detail.ResourceID] = make(map[int]*ShopItem)
			}
			if _, exists := result[detail.ResourceID][detail.ResourceLevel]; !exists {
				result[detail.ResourceID][detail.ResourceLevel] = shopItem
			}
		}
	}
	return result
}
