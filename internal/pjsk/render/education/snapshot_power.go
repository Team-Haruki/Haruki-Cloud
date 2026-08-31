package education

import (
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

func (c *Controller) BuildPowerBonusDetailRequestFromSnapshot(query PowerBonusQuery) (*drawing.PowerBonusDetailRequest, error) {
	finishBuild := commandtrace.MeasureOperation(c.traceContext(), "payload.build")
	defer finishBuild()
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

	bonuses := c.newPowerBonusState()
	bonuses.applyAreaItems(ctx.source, ctx.raw.UserAreas, releasedLevelCaps)
	bonuses.applyCharacterRanks(ctx.source, ctx.raw.UserCharacters)
	bonuses.applyFixtures(ctx.raw.UserMysekaiFixtureGameCharacterPerformanceBonuses)
	bonuses.applyGates(ctx.source, ctx.raw.UserMysekaiGates)

	return c.BuildPowerBonusDetailRequest(drawing.PowerBonusDetailRequest{
		Profile:      *ctx.profile,
		CharaBonuses: bonuses.characterList(),
		UnitBonuses:  bonuses.unitList(),
		AttrBonuses:  bonuses.attrList(),
	})
}

type powerBonusState struct {
	characters map[int]*drawing.CharacterBonus
	units      map[string]*drawing.UnitBonus
	attrs      map[string]*drawing.AttrBonus
}

func (c *Controller) newPowerBonusState() *powerBonusState {
	state := &powerBonusState{
		characters: make(map[int]*drawing.CharacterBonus, 26),
		units:      make(map[string]*drawing.UnitBonus, len(powerBonusUnitOrder)),
		attrs:      make(map[string]*drawing.AttrBonus, len(powerBonusAttrOrder)),
	}
	for charID := 1; charID <= 26; charID++ {
		state.characters[charID] = &drawing.CharacterBonus{CharaID: charID, CharaIconPath: c.characterIconPath(charID)}
	}
	for _, unit := range powerBonusUnitOrder {
		state.units[unit] = &drawing.UnitBonus{Unit: unit, UnitIconPath: c.unitIconPath(unit)}
	}
	for _, attr := range powerBonusAttrOrder {
		state.attrs[attr] = &drawing.AttrBonus{Attr: attr, AttrIconPath: c.attrIconPath(attr)}
	}
	return state
}

func (s *powerBonusState) applyAreaItems(source DataSource, areas []snapshot.RawUserArea, releasedLevelCaps map[int]int) {
	for _, area := range areas {
		for _, item := range area.AreaItems {
			itemLevel := minReleasedAreaItemLevel(item.Level, releasedLevelCaps[item.AreaItemID])
			level := source.GetAreaItemLevel(item.AreaItemID, itemLevel)
			if level != nil {
				s.applyAreaItemLevel(level)
			}
		}
	}
}

func minReleasedAreaItemLevel(level, releasedCap int) int {
	if releasedCap > 0 && level > releasedCap {
		return releasedCap
	}
	return level
}

func (s *powerBonusState) applyAreaItemLevel(level *AreaItemLevel) {
	if bonus := s.characters[level.TargetGameCharacterID]; bonus != nil {
		bonus.AreaItem += level.Power1BonusRate
	}
	if bonus := s.units[normalizeUnit(level.TargetUnit)]; bonus != nil {
		bonus.AreaItem += level.Power1BonusRate
	}
	if bonus := s.attrs[normalizeAttr(level.TargetCardAttr)]; bonus != nil {
		bonus.AreaItem += level.Power1BonusRate
	}
}

func (s *powerBonusState) applyCharacterRanks(source DataSource, characters []snapshot.RawUserCharacter) {
	for _, character := range characters {
		rank := source.GetCharacterRank(character.CharacterID, character.CharacterRank)
		bonus := s.characters[character.CharacterID]
		if rank != nil && bonus != nil {
			bonus.Rank += rank.Power1BonusRate
		}
	}
}

func (s *powerBonusState) applyFixtures(fixtures []snapshot.RawUserFixtureBonus) {
	for _, fixture := range fixtures {
		if bonus := s.characters[fixture.GameCharacterID]; bonus != nil {
			bonus.Fixture += fixture.TotalBonusRate * 0.1
		}
	}
}

func (s *powerBonusState) applyGates(source DataSource, gates []snapshot.RawUserMysekaiGate) {
	maximum := 0.0
	for _, gate := range gates {
		level := source.GetMysekaiGateLevel(gate.MysekaiGateID, gate.MysekaiGateLevel)
		if level == nil {
			continue
		}
		if bonus := s.units[gateUnitByID[gate.MysekaiGateID]]; bonus != nil {
			bonus.Gate += level.PowerBonusRate
		}
		maximum = max(maximum, level.PowerBonusRate)
	}
	if bonus := s.units["piapro"]; bonus != nil {
		bonus.Gate += maximum
	}
}

func (s *powerBonusState) characterList() []drawing.CharacterBonus {
	result := make([]drawing.CharacterBonus, 0, len(s.characters))
	for charID := 1; charID <= 26; charID++ {
		bonus := s.characters[charID]
		bonus.Total = bonus.AreaItem + bonus.Rank + bonus.Fixture
		result = append(result, *bonus)
	}
	return result
}

func (s *powerBonusState) unitList() []drawing.UnitBonus {
	result := make([]drawing.UnitBonus, 0, len(powerBonusUnitOrder))
	for _, unit := range powerBonusUnitOrder {
		bonus := s.units[unit]
		bonus.Total = bonus.AreaItem + bonus.Gate
		result = append(result, *bonus)
	}
	return result
}

func (s *powerBonusState) attrList() []drawing.AttrBonus {
	result := make([]drawing.AttrBonus, 0, len(powerBonusAttrOrder))
	for _, attr := range powerBonusAttrOrder {
		bonus := s.attrs[attr]
		bonus.Total = bonus.AreaItem
		result = append(result, *bonus)
	}
	return result
}
