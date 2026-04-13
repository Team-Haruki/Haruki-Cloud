package education

import (
	"time"

	"haruki-cloud/utils/drawing"
)

func (c *Controller) BuildPowerBonusDetailRequestFromSnapshot(query PowerBonusQuery) (*drawing.PowerBonusDetailRequest, error) {
	ctx, err := c.resolveSnapshotContext(query.Region, query.Profile, query.Snapshot)
	if err != nil {
		return nil, err
	}

	nowMs := ctx.raw.Now
	if nowMs <= 0 {
		nowMs = time.Now().UnixMilli()
	}
	userAreaLevels := collectUserAreaItemLevels(ctx.raw.UserAreas)
	itemIDs := make([]int, 0, len(userAreaLevels))
	for itemID := range userAreaLevels {
		itemIDs = append(itemIDs, itemID)
	}
	releasedLevelCaps := c.resolveReleasedAreaItemLevelCaps(
		ctx.source,
		itemIDs,
		c.resolveAreaItemShopItems(ctx.source, itemIDs, nowMs),
	)

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
			itemLevel := item.Level
			if releasedCap := releasedLevelCaps[item.AreaItemID]; releasedCap > 0 && itemLevel > releasedCap {
				itemLevel = releasedCap
			}
			level := ctx.source.GetAreaItemLevel(item.AreaItemID, itemLevel)
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
