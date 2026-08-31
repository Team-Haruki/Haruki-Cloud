package education

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
)

const maxAreaItemShopTimestampMs int64 = 1<<63 - 1

type areaItemBuildOptions struct {
	region         renderregion.Value
	source         DataSource
	query          AreaItemQuery
	userAreaLevels map[int]int
	userMaterials  map[int]int
	profile        *drawing.DetailedProfileCardRequest
	hasProfile     bool
	showFull       bool
	nowMs          int64
}

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

func (c *Controller) BuildAreaItemUpgradeMaterialsRequestFull(query AreaItemQuery) (*drawing.AreaItemUpgradeMaterialsRequest, error) {
	finishBuild := commandtrace.MeasureOperation(c.traceContext(), "payload.build")
	defer finishBuild()
	if c == nil || c.sources == nil {
		return nil, fmt.Errorf("education controller is not initialized")
	}
	if !hasAreaItemFilter(query) {
		return nil, fmt.Errorf("area item full query requires a filter")
	}

	region := c.sources.ResolveRegion(query.Region)
	source, ok := c.sources.SourceForRegion(region)
	if !ok {
		return nil, fmt.Errorf("education data source is not configured")
	}

	query.ShowFull = true
	return c.buildAreaItemUpgradeMaterialsRequest(areaItemBuildOptions{
		region:         region,
		source:         source,
		query:          query,
		userAreaLevels: map[int]int{},
		userMaterials:  map[int]int{},
		showFull:       true,
		nowMs:          maxAreaItemShopTimestampMs,
	})
}

func (c *Controller) BuildAreaItemUpgradeMaterialsRequestFromSnapshot(query AreaItemQuery) (*drawing.AreaItemUpgradeMaterialsRequest, error) {
	finishBuild := commandtrace.MeasureOperation(c.traceContext(), "payload.build")
	defer finishBuild()
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

	nowMs := ctx.raw.Now
	if nowMs <= 0 {
		nowMs = time.Now().UnixMilli()
	}
	return c.buildAreaItemUpgradeMaterialsRequest(areaItemBuildOptions{
		region:         ctx.region,
		source:         ctx.source,
		query:          query,
		userAreaLevels: userAreaLevels,
		userMaterials:  userMaterials,
		profile:        ctx.profile,
		hasProfile:     true,
		nowMs:          nowMs,
	})
}

func (c *Controller) buildAreaItemUpgradeMaterialsRequest(opts areaItemBuildOptions) (*drawing.AreaItemUpgradeMaterialsRequest, error) {
	itemIDs := c.resolveAreaItemIDs(opts.source, opts.userAreaLevels, opts.query)
	if len(itemIDs) == 0 {
		return nil, fmt.Errorf("area item masterdata is not available")
	}

	nowMs := opts.nowMs
	if nowMs <= 0 {
		nowMs = time.Now().UnixMilli()
	}
	levelShopItems := c.resolveAreaItemShopItems(opts.source, itemIDs, nowMs)
	releasedLevelCaps := c.resolveReleasedAreaItemLevelCaps(opts.source, itemIDs, levelShopItems)
	if opts.showFull {
		releasedLevelCaps = nil
	}

	states, minCurrentLevel := c.buildAreaItemRenderStates(opts, itemIDs, levelShopItems, releasedLevelCaps)
	areaItems := c.buildAreaItemDrawingRows(opts, states, minCurrentLevel)

	return c.BuildAreaItemUpgradeMaterialsRequest(drawing.AreaItemUpgradeMaterialsRequest{
		Profile:    opts.profile,
		AreaItems:  areaItems,
		HasProfile: opts.hasProfile,
	})
}

func (c *Controller) buildAreaItemRenderStates(
	opts areaItemBuildOptions,
	itemIDs []int,
	levelShopItems map[int]map[int]*ShopItem,
	releasedLevelCaps map[int]int,
) ([]areaItemRenderState, int) {
	states := make([]areaItemRenderState, 0, len(itemIDs))
	minCurrentLevel := 0
	if !opts.showFull {
		minCurrentLevel = -1
	}
	for _, itemID := range itemIDs {
		state, ok := c.newAreaItemRenderState(opts, itemID, levelShopItems[itemID], releasedLevelCaps[itemID])
		if !ok {
			continue
		}
		if !opts.showFull && (minCurrentLevel == -1 || state.currentLevel < minCurrentLevel) {
			minCurrentLevel = state.currentLevel
		}
		states = append(states, state)
	}
	if minCurrentLevel < 0 {
		minCurrentLevel = 0
	}
	return states, minCurrentLevel
}

func (c *Controller) newAreaItemRenderState(opts areaItemBuildOptions, itemID int, shopLevels map[int]*ShopItem, releasedCap int) (areaItemRenderState, bool) {
	master := opts.source.GetAreaItem(itemID)
	levels := opts.source.GetAreaItemLevels(itemID)
	if master == nil || len(levels) == 0 {
		return areaItemRenderState{}, false
	}
	levelMap := make(map[int]*AreaItemLevel, len(levels))
	for _, level := range levels {
		if level != nil {
			levelMap[level.Level] = level
		}
	}
	currentLevel, maxVisibleLevel := areaItemVisibleLevels(opts.showFull, opts.userAreaLevels[itemID], releasedCap, levels)
	if maxVisibleLevel <= 0 {
		return areaItemRenderState{}, false
	}
	return areaItemRenderState{
		itemID:          itemID,
		master:          master,
		levels:          levels,
		levelMap:        levelMap,
		shopLevels:      shopLevels,
		currentLevel:    currentLevel,
		maxVisibleLevel: maxVisibleLevel,
		targetIconPath:  c.areaItemTargetIcon(levels),
	}, true
}

func areaItemVisibleLevels(showFull bool, currentLevel, releasedCap int, levels []*AreaItemLevel) (int, int) {
	if showFull {
		return 0, maxAreaItemLevel(levels)
	}
	if releasedCap > 0 && currentLevel > releasedCap {
		currentLevel = releasedCap
	}
	return currentLevel, maxInt(currentLevel, releasedCap)
}

func (c *Controller) buildAreaItemDrawingRows(opts areaItemBuildOptions, states []areaItemRenderState, minCurrentLevel int) []drawing.AreaItemInfo {
	result := make([]drawing.AreaItemInfo, 0, len(states))
	for _, state := range states {
		areaItem := drawing.AreaItemInfo{
			ItemID:       state.itemID,
			CurrentLevel: state.currentLevel,
			ItemIconPath: assets.ResolveRegionAssetPath(c.assets, opts.region.String(),
				filepath.Join("areaitem", state.master.AssetbundleName, state.master.AssetbundleName+".png")),
			Levels: c.buildAreaItemLevelRows(opts, state, minCurrentLevel),
		}
		if state.targetIconPath != "" {
			areaItem.TargetIconPath = &state.targetIconPath
		}
		result = append(result, areaItem)
	}
	return result
}

func (c *Controller) buildAreaItemLevelRows(opts areaItemBuildOptions, state areaItemRenderState, minCurrentLevel int) []drawing.AreaItemLevel {
	result := make([]drawing.AreaItemLevel, 0, state.maxVisibleLevel-minCurrentLevel)
	sumMaterials := make(map[int]int)
	for level := minCurrentLevel + 1; level <= state.maxVisibleLevel; level++ {
		result = append(result, c.buildAreaItemLevelRow(opts, state, level, sumMaterials))
	}
	return result
}

func (c *Controller) buildAreaItemLevelRow(opts areaItemBuildOptions, state areaItemRenderState, level int, sumMaterials map[int]int) drawing.AreaItemLevel {
	levelMaster := state.levelMap[level]
	if levelMaster == nil {
		return drawing.AreaItemLevel{Level: level, Materials: []drawing.AreaItemMaterial{}}
	}
	row := drawing.AreaItemLevel{
		Level:      level,
		Bonus:      levelMaster.Power1BonusRate,
		CanUpgrade: true,
		Materials:  []drawing.AreaItemMaterial{},
	}
	if level <= state.currentLevel {
		return row
	}
	shopItem := state.shopLevels[level]
	if shopItem == nil {
		row.CanUpgrade = false
		return row
	}
	row.Materials = c.buildAreaItemMaterials(opts, shopItem, sumMaterials, &row.CanUpgrade)
	return row
}

func (c *Controller) buildAreaItemMaterials(opts areaItemBuildOptions, shopItem *ShopItem, sumMaterials map[int]int, canUpgrade *bool) []drawing.AreaItemMaterial {
	result := make([]drawing.AreaItemMaterial, 0, len(shopItem.Costs))
	for _, cost := range shopItem.Costs {
		materialID := cost.ResourceID
		if strings.EqualFold(cost.ResourceType, "coin") {
			materialID = areaCoinMaterialID
		}
		sumMaterials[materialID] += cost.Quantity
		haveQuantity := opts.userMaterials[materialID]
		isEnough := !opts.hasProfile || haveQuantity >= sumMaterials[materialID]
		if !isEnough {
			*canUpgrade = false
		}
		result = append(result, drawing.AreaItemMaterial{
			MaterialID:       materialID,
			MaterialIconPath: c.materialIconPath(cost.ResourceType, materialID),
			Quantity:         cost.Quantity,
			HaveQuantity:     haveQuantity,
			SumQuantity:      sumMaterials[materialID],
			IsEnough:         isEnough,
		})
	}
	return result
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

	itemSet := areaItemIDSet(itemIDs)
	result := make(map[int]map[int]*ShopItem, len(itemIDs))
	for _, box := range source.GetResourceBoxesByPurpose("shop_item") {
		addResourceBoxAreaItemShopItems(source, box, itemSet, result, nowMs)
	}

	c.fillAreaItemShopItemsByShopSequence(source, itemIDs, result, nowMs)
	return result
}

func areaItemIDSet(itemIDs []int) map[int]struct{} {
	result := make(map[int]struct{}, len(itemIDs))
	for _, itemID := range itemIDs {
		result[itemID] = struct{}{}
	}
	return result
}

func addResourceBoxAreaItemShopItems(source DataSource, box *ResourceBox, itemSet map[int]struct{}, result map[int]map[int]*ShopItem, nowMs int64) {
	if box == nil {
		return
	}
	shopItem := source.GetShopItemByResourceBoxID(box.ID)
	if shopItem == nil || shopItem.StartAt > 0 && shopItem.StartAt > nowMs {
		return
	}
	for _, detail := range box.Details {
		addResourceBoxAreaItemShopLevel(detail, shopItem, itemSet, result)
	}
}

func addResourceBoxAreaItemShopLevel(detail ResourceBoxDetail, shopItem *ShopItem, itemSet map[int]struct{}, result map[int]map[int]*ShopItem) {
	if !strings.EqualFold(detail.ResourceType, "area_item") || detail.ResourceID <= 0 || detail.ResourceLevel <= 0 {
		return
	}
	if _, ok := itemSet[detail.ResourceID]; !ok {
		return
	}
	if result[detail.ResourceID] == nil {
		result[detail.ResourceID] = make(map[int]*ShopItem)
	}
	if result[detail.ResourceID][detail.ResourceLevel] == nil {
		result[detail.ResourceID][detail.ResourceLevel] = shopItem
	}
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
	targetsByShopID := areaItemShopTargets(source, itemIDs)
	if len(targetsByShopID) == 0 {
		return
	}
	shopItemsByShopID := activeShopItemsByShopID(source, nowMs)
	for shopID, targets := range targetsByShopID {
		fillAreaItemShopSequenceBlock(result, targets, shopItemsByShopID[shopID])
	}
}

func areaItemShopTargets(source DataSource, itemIDs []int) map[int][]areaItemShopTarget {
	result := make(map[int][]areaItemShopTarget)
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

		result[shopID] = append(result[shopID], areaItemShopTarget{
			itemID:       itemID,
			sortedLevels: levels,
		})
	}
	return result
}

func activeShopItemsByShopID(source DataSource, nowMs int64) map[int][]*ShopItem {
	result := make(map[int][]*ShopItem)
	for _, item := range source.GetShopItems() {
		if item == nil || item.ShopID <= 0 {
			continue
		}
		if item.StartAt > 0 && item.StartAt > nowMs {
			continue
		}
		result[item.ShopID] = append(result[item.ShopID], item)
	}
	return result
}

func fillAreaItemShopSequenceBlock(result map[int]map[int]*ShopItem, targets []areaItemShopTarget, shopItems []*ShopItem) {
	if len(targets) == 0 || len(shopItems) < len(targets) || len(shopItems)%len(targets) != 0 {
		return
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
	for index, target := range targets {
		fillAreaItemShopTarget(result, target, shopItems, index*blockSize, blockSize)
	}
}

func fillAreaItemShopTarget(result map[int]map[int]*ShopItem, target areaItemShopTarget, shopItems []*ShopItem, offset, blockSize int) {
	if result[target.itemID] == nil {
		result[target.itemID] = make(map[int]*ShopItem)
	}
	levelsToMap := blockSize
	if len(target.sortedLevels) < levelsToMap {
		levelsToMap = len(target.sortedLevels)
	}
	for index := range levelsToMap {
		level := target.sortedLevels[index]
		if result[target.itemID][level] == nil {
			result[target.itemID][level] = shopItems[offset+index]
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

func maxAreaItemLevel(levels []*AreaItemLevel) int {
	maxLevel := 0
	for _, level := range levels {
		if level == nil {
			continue
		}
		if level.Level > maxLevel {
			maxLevel = level.Level
		}
	}
	return maxLevel
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
