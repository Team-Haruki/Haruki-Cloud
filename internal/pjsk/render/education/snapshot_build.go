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
	source   Source
	snapshot *userdata.Service
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

	itemIDs := make([]int, 0, len(userAreaLevels))
	lowerLevel := 0
	firstLevel := true
	for itemID, level := range userAreaLevels {
		if len(ctx.source.GetAreaItemLevels(itemID)) == 0 {
			continue
		}
		itemIDs = append(itemIDs, itemID)
		if firstLevel || level < lowerLevel {
			lowerLevel = level
			firstLevel = false
		}
	}
	sort.Ints(itemIDs)
	if len(itemIDs) == 0 {
		return nil, fmt.Errorf("area item masterdata is not available")
	}

	levelShopItems := c.resolveAreaItemShopItems(ctx.source, itemIDs)
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
		levelInfos := make([]drawing.AreaItemLevel, 0, maxLevel-lowerLevel)
		for level := lowerLevel + 1; level <= maxLevel; level++ {
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

func (c *Controller) resolveSnapshotContext(
	region renderregion.Value,
	profile *drawing.DetailedProfileCardRequest,
	snapshot *userdata.Service,
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

func (c *Controller) resolveAreaItemShopItems(source Source, itemIDs []int) map[int]map[int]*ShopItem {
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

func (c *Controller) areaItemTargetIcon(levels []*AreaItemLevel) string {
	for _, level := range levels {
		if level == nil {
			continue
		}
		if level.TargetGameCharacterID > 0 {
			return c.characterIconPath(level.TargetGameCharacterID)
		}
		if unit := normalizeUnit(level.TargetUnit); unit != "" {
			return c.unitIconPath(unit)
		}
		if attr := normalizeAttr(level.TargetCardAttr); attr != "" {
			return c.attrIconPath(attr)
		}
	}
	return ""
}

func (c *Controller) unitIconPath(unit string) string {
	icon := assets.UnitIconFilename(unit)
	if icon == "" {
		return ""
	}
	return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, icon+".png")
}

func (c *Controller) attrIconPath(attr string) string {
	attr = normalizeAttr(attr)
	if attr == "" {
		return ""
	}
	return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir,
		filepath.Join("card", fmt.Sprintf("attr_icon_%s.png", attr)))
}

func (c *Controller) materialIconPath(resourceType string, materialID int) string {
	resourceType = strings.ToLower(strings.TrimSpace(resourceType))
	if resourceType == "paid_jewel" {
		resourceType = "jewel"
	}
	switch resourceType {
	case "coin", "virtual_coin", "jewel":
		return assets.ResolveAssetPath(c.assets, "",
			filepath.Join("thumbnail", "common_material_rip", resourceType+".png"))
	case "material":
		if materialID <= 0 {
			return ""
		}
		return assets.ResolveAssetPath(c.assets, "",
			filepath.Join("thumbnail", "material_rip", fmt.Sprintf("material%d.png", materialID)))
	default:
		return ""
	}
}

func normalizeUnit(unit string) string {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "", "any":
		return ""
	case "light_sound_club":
		return "light_sound"
	case "more_more_jump":
		return "idol"
	case "vivid_bad_squad":
		return "street"
	case "wonderlands_x_showtime":
		return "theme_park"
	case "25_ji_night_cord_de":
		return "school_refusal"
	default:
		return strings.ToLower(strings.TrimSpace(unit))
	}
}

func normalizeAttr(attr string) string {
	attr = strings.ToLower(strings.TrimSpace(attr))
	if attr == "" || attr == "any" {
		return ""
	}
	return attr
}
