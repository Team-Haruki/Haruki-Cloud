package provider

import (
	"context"

	"haruki-cloud/database/sekai/areaitem"
	"haruki-cloud/database/sekai/areaitemlevel"
	"haruki-cloud/database/sekai/characterrank"
	"haruki-cloud/database/sekai/level"
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

func (p *dbEducationProvider) GetCharacterLevels(ctx context.Context) []*CharacterLevel {
	if !p.ensureCharacterLevelsLoaded(ctx) {
		return nil
	}

	p.levelMu.RLock()
	defer p.levelMu.RUnlock()
	return cloneEdCharacterLevels(p.characterLevels)
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

func (p *dbEducationProvider) ensureCharacterLevelsLoaded(ctx context.Context) bool {
	p.init()
	p.levelMu.RLock()
	if p.characterLevelsLoaded {
		p.levelMu.RUnlock()
		return true
	}
	p.levelMu.RUnlock()

	p.levelMu.Lock()
	defer p.levelMu.Unlock()

	if p.characterLevelsLoaded {
		return true
	}

	items, err := p.client.Level.Query().
		Where(level.ServerRegionEQ(p.region.String()), level.LevelTypeEQ("character")).
		Order(level.ByLevel()).
		All(ctx)
	if err != nil {
		return false
	}
	p.characterLevels = make([]*CharacterLevel, 0, len(items))
	for _, item := range items {
		if item.Level <= 0 {
			continue
		}
		p.characterLevels = append(p.characterLevels, &CharacterLevel{
			Level:    int(item.Level),
			TotalExp: int(item.TotalExp),
		})
	}
	p.characterLevelsLoaded = true
	return true
}

func cloneEdAreaItem(source *AreaItem) *AreaItem {
	if source == nil {
		return nil
	}
	return new(*source)
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
		out = append(out, new(*item))
	}
	return out
}

func cloneEdAreaItemLevel(source *AreaItemLevel) *AreaItemLevel {
	if source == nil {
		return nil
	}
	return new(*source)
}

func cloneEdCharacterRank(source *CharacterRank) *CharacterRank {
	if source == nil {
		return nil
	}
	return new(*source)
}

func cloneEdCharacterLevels(source []*CharacterLevel) []*CharacterLevel {
	if len(source) == 0 {
		return nil
	}
	out := make([]*CharacterLevel, 0, len(source))
	for _, item := range source {
		if item == nil {
			continue
		}
		out = append(out, new(*item))
	}
	return out
}
