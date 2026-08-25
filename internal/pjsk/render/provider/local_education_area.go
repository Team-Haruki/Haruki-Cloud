package provider

import "context"

func (p *localEducationProvider) ensureAreaItems() error {
	return p.areas.init(func() (areaIndex, error) {
		items, err := p.store.loadJSON[AreaItem]("areaItems.json")
		if err != nil {
			return areaIndex{}, err
		}
		idx := areaIndex{
			byID:         make(map[int]*AreaItem, len(items)),
			levelsByItem: make(map[int][]*AreaItemLevel),
			levelByItem:  make(map[int]map[int]*AreaItemLevel),
		}
		for i := range items {
			idx.byID[items[i].ID] = &items[i]
		}

		levels, err := p.store.loadJSON[AreaItemLevel]("areaItemLevels.json")
		if err != nil {
			return areaIndex{}, err
		}
		for i := range levels {
			lv := &levels[i]
			idx.levelsByItem[lv.AreaItemID] = append(idx.levelsByItem[lv.AreaItemID], lv)
			if _, ok := idx.levelByItem[lv.AreaItemID]; !ok {
				idx.levelByItem[lv.AreaItemID] = make(map[int]*AreaItemLevel)
			}
			idx.levelByItem[lv.AreaItemID][lv.Level] = lv
		}
		return idx, nil
	})
}

func (p *localEducationProvider) ensureCharacterRanks() error {
	return p.ranks.init(func() (map[int]map[int]*CharacterRank, error) {
		items, err := p.store.loadJSON[localCharacterRankJSON]("characterRanks.json")
		if err != nil {
			return nil, err
		}
		byChar := make(map[int]map[int]*CharacterRank)
		for _, item := range items {
			rank := &CharacterRank{
				CharacterID:     item.CharacterID,
				Rank:            item.CharacterRank,
				Power1BonusRate: item.Power1BonusRate,
			}
			if _, ok := byChar[rank.CharacterID]; !ok {
				byChar[rank.CharacterID] = make(map[int]*CharacterRank)
			}
			byChar[rank.CharacterID][rank.Rank] = rank
		}
		return byChar, nil
	})
}

func (p *localEducationProvider) GetAreaItems(_ context.Context) []*AreaItem {
	if err := p.ensureAreaItems(); err != nil {
		return nil
	}
	items := make([]*AreaItem, 0, len(p.areas.v().byID))
	for _, item := range p.areas.v().byID {
		items = append(items, cloneEdAreaItem(item))
	}
	return items
}

func (p *localEducationProvider) GetAreaItem(_ context.Context, id int) *AreaItem {
	if id <= 0 {
		return nil
	}
	if err := p.ensureAreaItems(); err != nil {
		return nil
	}
	return cloneEdAreaItem(p.areas.v().byID[id])
}

func (p *localEducationProvider) GetAreaItemLevels(_ context.Context, areaItemID int) []*AreaItemLevel {
	if areaItemID <= 0 {
		return nil
	}
	if err := p.ensureAreaItems(); err != nil {
		return nil
	}
	return cloneEdAreaItemLevels(p.areas.v().levelsByItem[areaItemID])
}

func (p *localEducationProvider) GetAreaItemLevel(_ context.Context, areaItemID, level int) *AreaItemLevel {
	if areaItemID <= 0 || level <= 0 {
		return nil
	}
	if err := p.ensureAreaItems(); err != nil {
		return nil
	}
	if levels, ok := p.areas.v().levelByItem[areaItemID]; ok {
		return cloneEdAreaItemLevel(levels[level])
	}
	return nil
}

func (p *localEducationProvider) GetCharacterRank(_ context.Context, characterID, rank int) *CharacterRank {
	if characterID <= 0 || rank <= 0 {
		return nil
	}
	if err := p.ensureCharacterRanks(); err != nil {
		return nil
	}
	if ranks, ok := p.ranks.v()[characterID]; ok {
		return cloneEdCharacterRank(ranks[rank])
	}
	return nil
}
