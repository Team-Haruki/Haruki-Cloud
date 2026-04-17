package education

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/drawing"
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

	nowMs := ctx.raw.Now
	if nowMs <= 0 {
		nowMs = time.Now().UnixMilli()
	}
	levelShopItems := c.resolveAreaItemShopItems(ctx.source, itemIDs, nowMs)
	releasedLevelCaps := c.resolveReleasedAreaItemLevelCaps(ctx.source, itemIDs, levelShopItems)

	type areaItemRenderState struct {
		itemID          int
		master          *AreaItem
		levels          []*AreaItemLevel
		levelMap        map[int]*AreaItemLevel
		shopLevels      map[int]*ShopItem
		currentLevel    int
		maxVisibleLevel int
		targetIconPath  string
	}

	states := make([]areaItemRenderState, 0, len(itemIDs))
	minCurrentLevel := -1
	for _, itemID := range itemIDs {
		master := ctx.source.GetAreaItem(itemID)
		levels := ctx.source.GetAreaItemLevels(itemID)
		if master == nil || len(levels) == 0 {
			continue
		}

		levelMap := make(map[int]*AreaItemLevel, len(levels))
		for _, level := range levels {
			if level == nil {
				continue
			}
			levelMap[level.Level] = level
		}

		currentLevel := userAreaLevels[itemID]
		if releasedCap := releasedLevelCaps[itemID]; releasedCap > 0 && currentLevel > releasedCap {
			currentLevel = releasedCap
		}

		maxVisibleLevel := currentLevel
		if releasedCap := releasedLevelCaps[itemID]; releasedCap > maxVisibleLevel {
			maxVisibleLevel = releasedCap
		}
		if maxVisibleLevel <= 0 {
			continue
		}
		if minCurrentLevel == -1 || currentLevel < minCurrentLevel {
			minCurrentLevel = currentLevel
		}

		states = append(states, areaItemRenderState{
			itemID:          itemID,
			master:          master,
			levels:          levels,
			levelMap:        levelMap,
			shopLevels:      levelShopItems[itemID],
			currentLevel:    currentLevel,
			maxVisibleLevel: maxVisibleLevel,
			targetIconPath:  c.areaItemTargetIcon(levels),
		})
	}
	if minCurrentLevel < 0 {
		minCurrentLevel = 0
	}

	areaItems := make([]drawing.AreaItemInfo, 0, len(states))
	for _, state := range states {
		sumMaterials := make(map[int]int)
		levelInfos := make([]drawing.AreaItemLevel, 0, state.maxVisibleLevel-minCurrentLevel)
		for level := minCurrentLevel + 1; level <= state.maxVisibleLevel; level++ {
			levelMaster := state.levelMap[level]
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

			if level > state.currentLevel {
				shopItem := state.shopLevels[level]
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
			}

			levelInfos = append(levelInfos, row)
		}

		areaItem := drawing.AreaItemInfo{
			ItemID:       state.itemID,
			CurrentLevel: state.currentLevel,
			ItemIconPath: assets.ResolveRegionAssetPath(c.assets, ctx.region.String(),
				filepath.Join("areaitem", state.master.AssetbundleName, state.master.AssetbundleName+".png")),
			Levels: levelInfos,
		}
		if state.targetIconPath != "" {
			areaItem.TargetIconPath = &state.targetIconPath
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

func (c *Controller) resolveAreaItemShopItems(source DataSource, itemIDs []int, nowMs int64) map[int]map[int]*ShopItem {
	if nowMs <= 0 {
		nowMs = time.Now().UnixMilli()
	}

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
		if shopItem.StartAt > 0 && shopItem.StartAt > nowMs {
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

	c.fillAreaItemShopItemsByShopSequence(source, itemIDs, result, nowMs)
	return result
}

var areaItemShopIDByAreaID = map[int]int{
	5:  5,
	7:  6,
	8:  7,
	9:  8,
	10: 9,
	11: 10,
	13: 11,
}

func (c *Controller) fillAreaItemShopItemsByShopSequence(
	source DataSource,
	itemIDs []int,
	result map[int]map[int]*ShopItem,
	nowMs int64,
) {
	if len(itemIDs) == 0 {
		return
	}

	targetsByShopID := make(map[int][]areaItemShopTarget)
	for _, itemID := range itemIDs {
		item := source.GetAreaItem(itemID)
		if item == nil {
			continue
		}

		shopID := areaItemShopIDByAreaID[item.AreaID]
		if shopID <= 0 {
			continue
		}

		levels := sortedAreaItemLevels(source.GetAreaItemLevels(itemID))
		if len(levels) == 0 {
			continue
		}

		targetsByShopID[shopID] = append(targetsByShopID[shopID], areaItemShopTarget{
			itemID:       itemID,
			sortedLevels: levels,
		})
	}
	if len(targetsByShopID) == 0 {
		return
	}

	shopItemsByShopID := make(map[int][]*ShopItem)
	for _, item := range source.GetShopItems() {
		if item == nil || item.ShopID <= 0 {
			continue
		}
		if item.StartAt > 0 && item.StartAt > nowMs {
			continue
		}
		shopItemsByShopID[item.ShopID] = append(shopItemsByShopID[item.ShopID], item)
	}
	if len(shopItemsByShopID) == 0 {
		return
	}

	for shopID, targets := range targetsByShopID {
		shopItems := shopItemsByShopID[shopID]
		if len(shopItems) == 0 || len(targets) == 0 {
			continue
		}
		if len(shopItems) < len(targets) || len(shopItems)%len(targets) != 0 {
			continue
		}

		sort.Slice(targets, func(i, j int) bool {
			return targets[i].itemID < targets[j].itemID
		})
		sort.Slice(shopItems, func(i, j int) bool {
			if shopItems[i].Seq != shopItems[j].Seq {
				return shopItems[i].Seq < shopItems[j].Seq
			}
			return shopItems[i].ID < shopItems[j].ID
		})

		blockSize := len(shopItems) / len(targets)
		offset := 0
		for _, target := range targets {
			if _, ok := result[target.itemID]; !ok {
				result[target.itemID] = make(map[int]*ShopItem)
			}

			levelsToMap := blockSize
			if levelsToMap > len(target.sortedLevels) {
				levelsToMap = len(target.sortedLevels)
			}
			for idx := 0; idx < levelsToMap; idx++ {
				level := target.sortedLevels[idx]
				if result[target.itemID][level] == nil {
					result[target.itemID][level] = shopItems[offset+idx]
				}
			}
			offset += blockSize
		}
	}
}

func sortedAreaItemLevels(levels []*AreaItemLevel) []int {
	if len(levels) == 0 {
		return nil
	}

	levelSet := make(map[int]struct{}, len(levels))
	result := make([]int, 0, len(levels))
	for _, level := range levels {
		if level == nil || level.Level <= 0 {
			continue
		}
		if _, exists := levelSet[level.Level]; exists {
			continue
		}
		levelSet[level.Level] = struct{}{}
		result = append(result, level.Level)
	}
	sort.Ints(result)
	return result
}

func (c *Controller) resolveReleasedAreaItemLevelCaps(source DataSource, itemIDs []int, levelShopItems map[int]map[int]*ShopItem) map[int]int {
	if len(itemIDs) == 0 || len(levelShopItems) == 0 {
		return nil
	}

	result := make(map[int]int, len(itemIDs))
	for _, itemID := range itemIDs {
		shopLevels := levelShopItems[itemID]
		if len(shopLevels) == 0 {
			continue
		}
		result[itemID] = releasedAreaItemLevelCap(source.GetAreaItemLevels(itemID), shopLevels)
	}
	return result
}

func releasedAreaItemLevelCap(levels []*AreaItemLevel, shopLevels map[int]*ShopItem) int {
	if len(levels) == 0 || len(shopLevels) == 0 {
		return 0
	}

	levelSet := make(map[int]struct{}, len(levels))
	for _, level := range levels {
		if level == nil || level.Level <= 0 {
			continue
		}
		levelSet[level.Level] = struct{}{}
	}
	if _, ok := levelSet[1]; !ok {
		return 0
	}

	releasedMaxLevel := 1
	for level := 2; ; level++ {
		if _, ok := levelSet[level]; !ok {
			break
		}
		if shopLevels[level] == nil {
			break
		}
		releasedMaxLevel = level
	}
	return releasedMaxLevel
}
