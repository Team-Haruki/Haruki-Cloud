package provider

import (
	"context"

	"haruki-cloud/database/sekai/areaitem"
	"haruki-cloud/database/sekai/areaitemlevel"
	"haruki-cloud/database/sekai/characterrank"
)

func (p *dbEducationProvider) GetAreaItem(ctx context.Context, id int) *AreaItem {
	if id <= 0 || !p.ensureAreaMasterLoaded(ctx) {
		return nil
	}

	p.areaMu.RLock()
	defer p.areaMu.RUnlock()
	return cloneEdAreaItem(p.areaByID[id])
}

func (p *dbEducationProvider) GetAreaItems(ctx context.Context) []*AreaItem {
	if !p.ensureAreaMasterLoaded(ctx) {
		return nil
	}

	p.areaMu.RLock()
	defer p.areaMu.RUnlock()

	items := make([]*AreaItem, 0, len(p.areaByID))
	for _, item := range p.areaByID {
		items = append(items, cloneEdAreaItem(item))
	}
	return items
}

func (p *dbEducationProvider) GetAreaItemLevels(ctx context.Context, areaItemID int) []*AreaItemLevel {
	if areaItemID <= 0 || !p.ensureAreaMasterLoaded(ctx) {
		return nil
	}

	p.areaMu.RLock()
	defer p.areaMu.RUnlock()
	return cloneEdAreaItemLevels(p.areaLevelsByItem[areaItemID])
}

func (p *dbEducationProvider) GetAreaItemLevel(ctx context.Context, areaItemID, level int) *AreaItemLevel {
	if areaItemID <= 0 || level <= 0 || !p.ensureAreaMasterLoaded(ctx) {
		return nil
	}

	p.areaMu.RLock()
	defer p.areaMu.RUnlock()
	if levels, ok := p.areaLevelByItem[areaItemID]; ok {
		return cloneEdAreaItemLevel(levels[level])
	}
	return nil
}

func (p *dbEducationProvider) GetCharacterRank(ctx context.Context, characterID, rank int) *CharacterRank {
	if characterID <= 0 || rank <= 0 || !p.ensureCharacterRanksLoaded(ctx) {
		return nil
	}

	p.rankMu.RLock()
	defer p.rankMu.RUnlock()
	if ranks, ok := p.rankByChar[characterID]; ok {
		return cloneEdCharacterRank(ranks[rank])
	}
	return nil
}

func (p *dbEducationProvider) ensureAreaMasterLoaded(ctx context.Context) bool {
	p.init()
	p.areaMu.RLock()
	if p.areaMasterLoaded {
		p.areaMu.RUnlock()
		return true
	}
	p.areaMu.RUnlock()

	p.areaMu.Lock()
	defer p.areaMu.Unlock()

	if p.areaMasterLoaded {
		return true
	}

	items, err := p.client.Areaitem.Query().
		Where(areaitem.ServerRegionEQ(p.region.String())).
		All(ctx)
	if err != nil {
		return false
	}
	for _, item := range items {
		p.areaByID[int(item.GameID)] = &AreaItem{
			ID:              int(item.GameID),
			AreaID:          int(item.AreaID),
			Name:            item.Name,
			AssetbundleName: item.AssetbundleName,
		}
	}

	levels, err := p.client.Areaitemlevel.Query().
		Where(areaitemlevel.ServerRegionEQ(p.region.String())).
		All(ctx)
	if err != nil {
		return false
	}
	for _, item := range levels {
		level := &AreaItemLevel{
			AreaItemID:            int(item.AreaItemID),
			Level:                 int(item.Level),
			TargetUnit:            item.TargetUnit,
			TargetCardAttr:        item.TargetCardAttr,
			TargetGameCharacterID: int(item.TargetGameCharacterID),
			Power1BonusRate:       item.Power1BonusRate,
		}
		p.areaLevelsByItem[level.AreaItemID] = append(p.areaLevelsByItem[level.AreaItemID], level)
		if _, ok := p.areaLevelByItem[level.AreaItemID]; !ok {
			p.areaLevelByItem[level.AreaItemID] = make(map[int]*AreaItemLevel)
		}
		p.areaLevelByItem[level.AreaItemID][level.Level] = level
	}

	p.areaMasterLoaded = true
	return true
}

func (p *dbEducationProvider) ensureCharacterRanksLoaded(ctx context.Context) bool {
	p.init()
	p.rankMu.RLock()
	if p.ranksLoaded {
		p.rankMu.RUnlock()
		return true
	}
	p.rankMu.RUnlock()

	p.rankMu.Lock()
	defer p.rankMu.Unlock()

	if p.ranksLoaded {
		return true
	}

	items, err := p.client.Characterrank.Query().
		Where(characterrank.ServerRegionEQ(p.region.String())).
		All(ctx)
	if err != nil {
		return false
	}
	for _, item := range items {
		rank := &CharacterRank{
			CharacterID:     int(item.CharacterID),
			Rank:            int(item.CharacterRank),
			Power1BonusRate: item.Power1BonusRate,
		}
		if _, ok := p.rankByChar[rank.CharacterID]; !ok {
			p.rankByChar[rank.CharacterID] = make(map[int]*CharacterRank)
		}
		p.rankByChar[rank.CharacterID][rank.Rank] = rank
	}
	p.ranksLoaded = true
	return true
}

func cloneEdAreaItem(source *AreaItem) *AreaItem {
	if source == nil {
		return nil
	}
	c := *source
	return &c
}

func cloneEdAreaItemLevels(source []*AreaItemLevel) []*AreaItemLevel {
	if len(source) == 0 {
		return nil
	}
	out := make([]*AreaItemLevel, 0, len(source))
	for _, item := range source {
		if item == nil {
			continue
		}
		c := *item
		out = append(out, &c)
	}
	return out
}

func cloneEdAreaItemLevel(source *AreaItemLevel) *AreaItemLevel {
	if source == nil {
		return nil
	}
	c := *source
	return &c
}

func cloneEdCharacterRank(source *CharacterRank) *CharacterRank {
	if source == nil {
		return nil
	}
	c := *source
	return &c
}
