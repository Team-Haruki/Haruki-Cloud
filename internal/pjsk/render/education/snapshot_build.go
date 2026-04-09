package education

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
)

const areaCoinMaterialID = -1

const (
	areaTreeAreaID   = 11
	areaFlowerAreaID = 13
)

var areaFilterUnitAreaIDs = map[string]int{
	"light_sound":    5,
	"idol":           7,
	"street":         8,
	"theme_park":     9,
	"school_refusal": 10,
}

var piaproCharacterIDs = map[int]struct{}{
	21: {},
	22: {},
	23: {},
	24: {},
	25: {},
	26: {},
}

var powerBonusUnitOrder = []string{
	"light_sound",
	"idol",
	"street",
	"theme_park",
	"school_refusal",
	"piapro",
}

var powerBonusAttrOrder = []string{
	"cute",
	"cool",
	"pure",
	"happy",
	"mysterious",
}

var gateUnitByID = map[int]string{
	1: "light_sound",
	2: "idol",
	3: "street",
	4: "theme_park",
	5: "school_refusal",
}

type resolvedSnapshotContext struct {
	region   renderregion.Value
	source   DataSource
	snapshot userdata.Snapshot
	raw      *userdata.RawUserData
	profile  *drawing.DetailedProfileCardRequest
}

func (c *Controller) BuildPowerBonusDetailRequestFromSnapshot(query PowerBonusQuery) (*drawing.PowerBonusDetailRequest, error) {
	ctx, err := c.resolveSnapshotContext(query.Region, query.Profile, query.Snapshot)
	if err != nil {
		return nil, err
	}

	charaBonuses := make(map[int]*drawing.CharacterBonus, 26)
	for charID := 1; charID <= 26; charID++ {
		charaBonuses[charID] = &drawing.CharacterBonus{
			CharaID:       charID,
			CharaIconPath: c.characterIconPath(charID),
		}
	}
	unitBonuses := make(map[string]*drawing.UnitBonus, len(powerBonusUnitOrder))
	for _, unit := range powerBonusUnitOrder {
		unitBonuses[unit] = &drawing.UnitBonus{
			Unit:         unit,
			UnitIconPath: c.unitIconPath(unit),
		}
	}
	attrBonuses := make(map[string]*drawing.AttrBonus, len(powerBonusAttrOrder))
	for _, attr := range powerBonusAttrOrder {
		attrBonuses[attr] = &drawing.AttrBonus{
			Attr:         attr,
			AttrIconPath: c.attrIconPath(attr),
		}
	}

	for _, area := range ctx.raw.UserAreas {
		for _, item := range area.AreaItems {
			level := ctx.source.GetAreaItemLevel(item.AreaItemID, item.Level)
			if level == nil {
				continue
			}
			if level.TargetGameCharacterID > 0 {
				if bonus, ok := charaBonuses[level.TargetGameCharacterID]; ok {
					bonus.AreaItem += level.Power1BonusRate
				}
			}
			if normalized := normalizeUnit(level.TargetUnit); normalized != "" {
				if bonus, ok := unitBonuses[normalized]; ok {
					bonus.AreaItem += level.Power1BonusRate
				}
			}
			if attr := normalizeAttr(level.TargetCardAttr); attr != "" {
				if bonus, ok := attrBonuses[attr]; ok {
					bonus.AreaItem += level.Power1BonusRate
				}
			}
		}
	}

	for _, character := range ctx.raw.UserCharacters {
		rank := ctx.source.GetCharacterRank(character.CharacterID, character.CharacterRank)
		if rank == nil {
			continue
		}
		if bonus, ok := charaBonuses[character.CharacterID]; ok {
			bonus.Rank += rank.Power1BonusRate
		}
	}

	for _, fixture := range ctx.raw.UserMysekaiFixtureGameCharacterPerformanceBonuses {
		if bonus, ok := charaBonuses[fixture.GameCharacterID]; ok {
			bonus.Fixture += fixture.TotalBonusRate * 0.1
		}
	}

	maxGateBonus := 0.0
	for _, gate := range ctx.raw.UserMysekaiGates {
		level := ctx.source.GetMysekaiGateLevel(gate.MysekaiGateID, gate.MysekaiGateLevel)
		if level == nil {
			continue
		}
		if bonus, ok := unitBonuses[gateUnitByID[gate.MysekaiGateID]]; ok {
			bonus.Gate += level.PowerBonusRate
		}
		if level.PowerBonusRate > maxGateBonus {
			maxGateBonus = level.PowerBonusRate
		}
	}
	if vsBonus, ok := unitBonuses["piapro"]; ok {
		vsBonus.Gate += maxGateBonus
	}

	charaList := make([]drawing.CharacterBonus, 0, len(charaBonuses))
	for charID := 1; charID <= 26; charID++ {
		bonus := charaBonuses[charID]
		bonus.Total = bonus.AreaItem + bonus.Rank + bonus.Fixture
		charaList = append(charaList, *bonus)
	}

	unitList := make([]drawing.UnitBonus, 0, len(powerBonusUnitOrder))
	for _, unit := range powerBonusUnitOrder {
		bonus := unitBonuses[unit]
		bonus.Total = bonus.AreaItem + bonus.Gate
		unitList = append(unitList, *bonus)
	}

	attrList := make([]drawing.AttrBonus, 0, len(powerBonusAttrOrder))
	for _, attr := range powerBonusAttrOrder {
		bonus := attrBonuses[attr]
		bonus.Total = bonus.AreaItem
		attrList = append(attrList, *bonus)
	}

	return c.BuildPowerBonusDetailRequest(drawing.PowerBonusDetailRequest{
		Profile:      *ctx.profile,
		CharaBonuses: charaList,
		UnitBonuses:  unitList,
		AttrBonuses:  attrList,
	})
}

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

func (c *Controller) BuildBondsRequestFromSnapshot(query BondsQuery) (*drawing.BondsRequest, error) {
	ctx, err := c.resolveSnapshotContext(query.Region, query.Profile, query.Snapshot)
	if err != nil {
		return nil, err
	}

	bondsMaster := ctx.source.GetBonds()
	if len(bondsMaster) == 0 {
		return nil, fmt.Errorf("bond masterdata is not available")
	}

	type bondPair struct {
		CharID1 int
		CharID2 int
	}
	groupToPair := make(map[int]bondPair, len(bondsMaster))
	for _, item := range bondsMaster {
		if item == nil || item.GroupID <= 0 {
			continue
		}
		groupToPair[item.GroupID] = bondPair{CharID1: item.CharacterID1, CharID2: item.CharacterID2}
	}

	type userBondEntry struct {
		BondsGroupID int
		Rank         int
		Exp          int
	}
	userBondByGroupID := make(map[int]userBondEntry, len(ctx.raw.UserBonds))
	for _, item := range ctx.raw.UserBonds {
		userBondByGroupID[item.BondsGroupID] = userBondEntry{
			BondsGroupID: item.BondsGroupID,
			Rank:         item.Rank,
			Exp:          item.Exp,
		}
	}

	charRankMap := make(map[int]int, len(ctx.raw.UserCharacters))
	for _, item := range ctx.raw.UserCharacters {
		charRankMap[item.CharacterID] = item.CharacterRank
	}

	levelTotalExp := make(map[int]int)
	maxLevel := 0
	for _, item := range ctx.source.GetBondLevels() {
		if item == nil || item.Level <= 0 {
			continue
		}
		levelTotalExp[item.Level] = item.TotalExp
		if item.Level > maxLevel {
			maxLevel = item.Level
		}
	}

	selectedPairs := make([]bondPair, 0, len(ctx.raw.UserBonds))
	selectedState := make([]userBondEntry, 0, len(ctx.raw.UserBonds))
	requiredCharIDs := make(map[int]struct{}, len(ctx.raw.UserBonds)*2)
	if query.Cid > 0 {
		for _, master := range bondsMaster {
			if master == nil {
				continue
			}
			pair := bondPair{CharID1: master.CharacterID1, CharID2: master.CharacterID2}
			if pair.CharID1 != query.Cid && pair.CharID2 != query.Cid {
				continue
			}
			if pair.CharID1 != query.Cid {
				pair.CharID1, pair.CharID2 = pair.CharID2, pair.CharID1
			}
			selectedPairs = append(selectedPairs, pair)
			selectedState = append(selectedState, userBondByGroupID[master.GroupID])
			requiredCharIDs[pair.CharID1] = struct{}{}
			requiredCharIDs[pair.CharID2] = struct{}{}
		}
	} else {
		for _, item := range ctx.raw.UserBonds {
			pair, ok := groupToPair[item.BondsGroupID]
			if !ok {
				continue
			}
			selectedPairs = append(selectedPairs, pair)
			selectedState = append(selectedState, userBondByGroupID[item.BondsGroupID])
			requiredCharIDs[pair.CharID1] = struct{}{}
			requiredCharIDs[pair.CharID2] = struct{}{}
		}
	}

	charStyles := make(map[int]*GameCharacterStyle, len(requiredCharIDs))
	for charID := range requiredCharIDs {
		if style := ctx.source.GetGameCharacterStyle(charID); style != nil {
			charStyles[charID] = style
		}
	}

	resolveCharIcon := func(gameID int) string {
		if style, ok := charStyles[gameID]; ok && style.CharacterID > 0 {
			return c.characterIconPath(style.CharacterID)
		}
		return c.characterIconPath(gameID)
	}
	resolveBondSortCharacterID := func(gameID int) int {
		if style, ok := charStyles[gameID]; ok && style.CharacterID > 0 {
			return style.CharacterID
		}
		return gameID
	}

	bonds := make([]drawing.BondInfo, 0, len(selectedPairs))
	userMaxLevel := 0
	for idx, pair := range selectedPairs {
		state := selectedState[idx]
		if state.Rank > userMaxLevel {
			userMaxLevel = state.Rank
		}

		info := drawing.BondInfo{
			CharaID1:       pair.CharID1,
			CharaID2:       pair.CharID2,
			CharaIconPath1: resolveCharIcon(pair.CharID1),
			CharaIconPath2: resolveCharIcon(pair.CharID2),
			CharaRank1:     charRankMap[pair.CharID1],
			CharaRank2:     charRankMap[pair.CharID2],
			BondLevel:      state.Rank,
			HasBond:        state.BondsGroupID != 0,
			Color1:         defaultBondColor(),
			Color2:         defaultBondColor(),
		}
		if style, ok := charStyles[pair.CharID1]; ok {
			info.Color1 = parseBondColorCode(style.ColorCode)
		}
		if style, ok := charStyles[pair.CharID2]; ok {
			info.Color2 = parseBondColorCode(style.ColorCode)
		}
		if state.Rank > 0 && state.Rank < maxLevel {
			currentTotalExp, okCurrent := levelTotalExp[state.Rank]
			nextTotalExp, okNext := levelTotalExp[state.Rank+1]
			if okCurrent && okNext {
				needExp := nextTotalExp - currentTotalExp - state.Exp
				if needExp < 0 {
					needExp = 0
				}
				info.NeedExp = &needExp
			}
		}
		bonds = append(bonds, info)
	}

	if query.Cid > 0 {
		deduped := make([]drawing.BondInfo, 0, len(bonds))
		indexByDisplayRight := make(map[int]int, len(bonds))
		betterBondInfo := func(current, candidate drawing.BondInfo) bool {
			if candidate.BondLevel != current.BondLevel {
				return candidate.BondLevel > current.BondLevel
			}
			if candidate.HasBond != current.HasBond {
				return candidate.HasBond
			}
			rightCurrent := resolveBondSortCharacterID(current.CharaID2)
			rightCandidate := resolveBondSortCharacterID(candidate.CharaID2)
			if rightCandidate != rightCurrent {
				return rightCandidate < rightCurrent
			}
			return candidate.CharaID2 < current.CharaID2
		}
		for _, bond := range bonds {
			displayRight := resolveBondSortCharacterID(bond.CharaID2)
			if idx, ok := indexByDisplayRight[displayRight]; ok {
				if betterBondInfo(deduped[idx], bond) {
					deduped[idx] = bond
				}
				continue
			}
			indexByDisplayRight[displayRight] = len(deduped)
			deduped = append(deduped, bond)
		}
		bonds = deduped
	}
	if maxLevel == 0 {
		maxLevel = userMaxLevel
	}

	sort.Slice(bonds, func(i, j int) bool {
		if query.Cid > 0 {
			if bonds[i].BondLevel != bonds[j].BondLevel {
				return bonds[i].BondLevel > bonds[j].BondLevel
			}
			if bonds[i].HasBond != bonds[j].HasBond {
				return bonds[i].HasBond
			}
			rightI := resolveBondSortCharacterID(bonds[i].CharaID2)
			rightJ := resolveBondSortCharacterID(bonds[j].CharaID2)
			if rightI != rightJ {
				return rightI < rightJ
			}
			if bonds[i].CharaID2 != bonds[j].CharaID2 {
				return bonds[i].CharaID2 < bonds[j].CharaID2
			}
			return bonds[i].CharaID1 < bonds[j].CharaID1
		}
		if bonds[i].BondLevel != bonds[j].BondLevel {
			return bonds[i].BondLevel > bonds[j].BondLevel
		}
		if bonds[i].CharaID1 != bonds[j].CharaID1 {
			return bonds[i].CharaID1 < bonds[j].CharaID1
		}
		return bonds[i].CharaID2 < bonds[j].CharaID2
	})

	return c.BuildBondsRequest(drawing.BondsRequest{
		Profile:  *ctx.profile,
		Bonds:    bonds,
		MaxLevel: maxLevel,
	})
}

func (c *Controller) BuildLeaderCountRequestFromSnapshot(query LeaderCountQuery) (*drawing.LeaderCountRequest, error) {
	ctx, err := c.resolveSnapshotContext(query.Region, query.Profile, query.Snapshot)
	if err != nil {
		return nil, err
	}

	playCountByCharacter := make(map[int]int, 26)
	missionRequirements, maxPlayLimit := ctx.source.GetLeaderMissionRequirements()
	exCountByCharacter := make(map[int]int)
	exLevelByCharacter := make(map[int]int)
	hasPlayLiveMission := false

	for _, item := range ctx.raw.UserCharacterMissionV2s {
		if item.CharacterID <= 0 {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(item.CharacterMissionType)) {
		case "play_live":
			playCountByCharacter[item.CharacterID] = item.Progress
			hasPlayLiveMission = true
		case "play_live_ex":
			exCountByCharacter[item.CharacterID] = item.Progress
			if _, ok := exLevelByCharacter[item.CharacterID]; !ok {
				exLevelByCharacter[item.CharacterID] = 0
			}
		}
	}

	if !hasPlayLiveMission {
		for _, item := range ctx.raw.UserCharacterLiveUsageCounts {
			if item.CharacterID <= 0 || !strings.EqualFold(item.CharacterLiveUsageType, "leader") {
				continue
			}
			playCountByCharacter[item.CharacterID] = item.UsageCount
		}
	}

	for _, item := range ctx.raw.UserCharacterMissionV2Statuses {
		if item.CharacterID <= 0 || item.ParameterGroupID != 101 {
			continue
		}
		if item.Seq > exLevelByCharacter[item.CharacterID] {
			exLevelByCharacter[item.CharacterID] = item.Seq
		}
		exCountByCharacter[item.CharacterID] += leaderMissionRequirementForSeq(missionRequirements, item.Seq)
	}

	leaders := make([]drawing.LeaderCountInfo, 0, 26)
	for charID := 1; charID <= 26; charID++ {
		playCount := playCountByCharacter[charID]
		leaders = append(leaders, drawing.LeaderCountInfo{
			CharaID:       charID,
			CharaIconPath: c.characterIconPath(charID),
			PlayCount:     playCount,
			ExLevel:       exLevelByCharacter[charID],
			ExCount:       exCountByCharacter[charID],
		})
	}
	sort.SliceStable(leaders, func(i, j int) bool {
		totalI := leaders[i].PlayCount + leaders[i].ExCount
		totalJ := leaders[j].PlayCount + leaders[j].ExCount
		if totalI == totalJ {
			return leaders[i].CharaID < leaders[j].CharaID
		}
		return totalI > totalJ
	})

	maxPlay := maxPlayLimit
	if maxPlay <= 0 {
		for _, item := range leaders {
			if item.PlayCount > maxPlay {
				maxPlay = item.PlayCount
			}
		}
	}

	return c.BuildLeaderCountRequest(drawing.LeaderCountRequest{
		Profile:      *ctx.profile,
		LeaderCounts: leaders,
		MaxPlayCount: maxPlay,
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

func (c *Controller) resolveSnapshotContext(
	region renderregion.Value,
	profile *drawing.DetailedProfileCardRequest,
	snapshot userdata.Snapshot,
) (*resolvedSnapshotContext, error) {
	if c == nil || c.sources == nil {
		return nil, fmt.Errorf("education controller is not initialized")
	}

	if snapshot == nil {
		snapshot = c.snapshot
	}
	if snapshot == nil {
		return nil, fmt.Errorf("local user snapshot is not configured")
	}
	if err := snapshot.Require(); err != nil {
		return nil, err
	}

	resolvedRegion := c.sources.ResolveRegion(region)
	source, ok := c.sources.SourceForRegion(resolvedRegion)
	if !ok {
		return nil, fmt.Errorf("education data source is not configured")
	}

	raw := snapshot.RawData()
	if raw == nil {
		return nil, fmt.Errorf("user snapshot is missing raw suite data")
	}

	if profile == nil {
		profile = snapshot.DetailedProfile(resolvedRegion)
	}
	if profile == nil {
		return nil, fmt.Errorf("user snapshot is missing profile data")
	}

	return &resolvedSnapshotContext{
		region:   resolvedRegion,
		source:   source,
		snapshot: snapshot,
		raw:      raw,
		profile:  profile,
	}, nil
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
