package provider

import (
	"encoding/json"
	"strings"
	"sync"
)

// ===========================================================================
// localEducationProvider
// ===========================================================================

type localEducationProvider struct {
	store *localStore

	rewardOnce    sync.Once
	rewardsByChar map[int][]*ChallengeReward
	rewardErr     error

	boxOnce      sync.Once
	boxByID      map[int]*ResourceBox
	boxByPurpose map[string]map[int]*ResourceBox
	boxErr       error

	areaOnce         sync.Once
	areaByID         map[int]*AreaItem
	areaLevelsByItem map[int][]*AreaItemLevel
	areaLevelByItem  map[int]map[int]*AreaItemLevel
	areaErr          error

	rankOnce   sync.Once
	rankByChar map[int]map[int]*CharacterRank
	rankErr    error

	gateOnce sync.Once
	gateByID map[int]map[int]*MysekaiGateLevel
	gateErr  error

	shopOnce    sync.Once
	shopByBoxID map[int]*ShopItem
	shopErr     error
}

func (p *localEducationProvider) ensureRewards() error {
	p.rewardOnce.Do(func() {
		items, err := loadJSON[ChallengeReward](p.store, "challengeLiveHighScoreRewards.json")
		if err != nil {
			p.rewardErr = err
			return
		}
		p.rewardsByChar = make(map[int][]*ChallengeReward)
		for i := range items {
			p.rewardsByChar[items[i].CharacterID] = append(
				p.rewardsByChar[items[i].CharacterID], &items[i])
		}
	})
	return p.rewardErr
}

func (p *localEducationProvider) ensureResourceBoxes() error {
	p.boxOnce.Do(func() {
		items, err := loadJSON[ResourceBox](p.store, "resourceBoxes.json")
		if err != nil {
			p.boxErr = err
			return
		}
		p.boxByID = make(map[int]*ResourceBox, len(items))
		p.boxByPurpose = make(map[string]map[int]*ResourceBox)
		for i := range items {
			box := &items[i]
			p.boxByID[box.ID] = box
			if _, ok := p.boxByPurpose[box.ResourceBoxPurpose]; !ok {
				p.boxByPurpose[box.ResourceBoxPurpose] = make(map[int]*ResourceBox)
			}
			p.boxByPurpose[box.ResourceBoxPurpose][box.ID] = box
		}
	})
	return p.boxErr
}

func (p *localEducationProvider) ensureAreaItems() error {
	p.areaOnce.Do(func() {
		items, err := loadJSON[AreaItem](p.store, "areaItems.json")
		if err != nil {
			p.areaErr = err
			return
		}
		p.areaByID = make(map[int]*AreaItem, len(items))
		for i := range items {
			p.areaByID[items[i].ID] = &items[i]
		}

		levels, err := loadJSON[AreaItemLevel](p.store, "areaItemLevels.json")
		if err != nil {
			p.areaErr = err
			return
		}
		p.areaLevelsByItem = make(map[int][]*AreaItemLevel)
		p.areaLevelByItem = make(map[int]map[int]*AreaItemLevel)
		for i := range levels {
			lv := &levels[i]
			p.areaLevelsByItem[lv.AreaItemID] = append(p.areaLevelsByItem[lv.AreaItemID], lv)
			if _, ok := p.areaLevelByItem[lv.AreaItemID]; !ok {
				p.areaLevelByItem[lv.AreaItemID] = make(map[int]*AreaItemLevel)
			}
			p.areaLevelByItem[lv.AreaItemID][lv.Level] = lv
		}
	})
	return p.areaErr
}

func (p *localEducationProvider) ensureCharacterRanks() error {
	p.rankOnce.Do(func() {
		items, err := loadJSON[localCharacterRankJSON](p.store, "characterRanks.json")
		if err != nil {
			p.rankErr = err
			return
		}
		p.rankByChar = make(map[int]map[int]*CharacterRank)
		for _, item := range items {
			rank := &CharacterRank{
				CharacterID:     item.CharacterID,
				Rank:            item.CharacterRank,
				Power1BonusRate: item.Power1BonusRate,
			}
			if _, ok := p.rankByChar[rank.CharacterID]; !ok {
				p.rankByChar[rank.CharacterID] = make(map[int]*CharacterRank)
			}
			p.rankByChar[rank.CharacterID][rank.Rank] = rank
		}
	})
	return p.rankErr
}

func (p *localEducationProvider) ensureGateLevels() error {
	p.gateOnce.Do(func() {
		items, err := loadJSON[localMysekaiGateLevelJSON](p.store, "mysekaiGateLevels.json")
		if err != nil {
			p.gateErr = err
			return
		}
		p.gateByID = make(map[int]map[int]*MysekaiGateLevel)
		for _, item := range items {
			level := &MysekaiGateLevel{
				GateID:         item.MysekaiGateID,
				Level:          item.Level,
				PowerBonusRate: item.PowerBonusRate,
			}
			if _, ok := p.gateByID[level.GateID]; !ok {
				p.gateByID[level.GateID] = make(map[int]*MysekaiGateLevel)
			}
			p.gateByID[level.GateID][level.Level] = level
		}
	})
	return p.gateErr
}

func (p *localEducationProvider) ensureShopItems() error {
	p.shopOnce.Do(func() {
		items, err := loadJSON[localShopItemJSON](p.store, "shopItems.json")
		if err != nil {
			p.shopErr = err
			return
		}
		p.shopByBoxID = make(map[int]*ShopItem, len(items))
		for _, item := range items {
			entry := &ShopItem{
				ID:            item.ID,
				ResourceBoxID: item.ResourceBoxID,
			}
			if len(item.Costs) > 0 {
				var rawCosts []struct {
					Cost ShopItemCost `json:"cost"`
				}
				if err := json.Unmarshal(item.Costs, &rawCosts); err == nil {
					entry.Costs = make([]ShopItemCost, 0, len(rawCosts))
					for _, raw := range rawCosts {
						entry.Costs = append(entry.Costs, raw.Cost)
					}
				}
			}
			p.shopByBoxID[entry.ResourceBoxID] = entry
		}
	})
	return p.shopErr
}

func (p *localEducationProvider) GetChallengeRewardsByCharacter(charID int) []*ChallengeReward {
	if charID <= 0 {
		return nil
	}
	if err := p.ensureRewards(); err != nil {
		return nil
	}
	return cloneEdChallengeRewards(p.rewardsByChar[charID])
}

func (p *localEducationProvider) GetResourceBoxByPurpose(purpose string, id int) *ResourceBox {
	if id <= 0 {
		return nil
	}
	if err := p.ensureResourceBoxes(); err != nil {
		return nil
	}
	if strings.TrimSpace(purpose) == "" {
		return cloneEdResourceBox(p.boxByID[id])
	}
	if purposeMap, ok := p.boxByPurpose[purpose]; ok {
		return cloneEdResourceBox(purposeMap[id])
	}
	return nil
}

func (p *localEducationProvider) GetResourceBoxesByPurpose(purpose string) []*ResourceBox {
	if err := p.ensureResourceBoxes(); err != nil {
		return nil
	}
	if strings.TrimSpace(purpose) == "" {
		items := make([]*ResourceBox, 0, len(p.boxByID))
		for _, item := range p.boxByID {
			items = append(items, cloneEdResourceBox(item))
		}
		return items
	}
	purposeMap, ok := p.boxByPurpose[purpose]
	if !ok {
		return nil
	}
	items := make([]*ResourceBox, 0, len(purposeMap))
	for _, item := range purposeMap {
		items = append(items, cloneEdResourceBox(item))
	}
	return items
}

func (p *localEducationProvider) GetAreaItems() []*AreaItem {
	if err := p.ensureAreaItems(); err != nil {
		return nil
	}
	items := make([]*AreaItem, 0, len(p.areaByID))
	for _, item := range p.areaByID {
		items = append(items, cloneEdAreaItem(item))
	}
	return items
}

func (p *localEducationProvider) GetAreaItem(id int) *AreaItem {
	if id <= 0 {
		return nil
	}
	if err := p.ensureAreaItems(); err != nil {
		return nil
	}
	return cloneEdAreaItem(p.areaByID[id])
}

func (p *localEducationProvider) GetAreaItemLevels(areaItemID int) []*AreaItemLevel {
	if areaItemID <= 0 {
		return nil
	}
	if err := p.ensureAreaItems(); err != nil {
		return nil
	}
	return cloneEdAreaItemLevels(p.areaLevelsByItem[areaItemID])
}

func (p *localEducationProvider) GetAreaItemLevel(areaItemID, level int) *AreaItemLevel {
	if areaItemID <= 0 || level <= 0 {
		return nil
	}
	if err := p.ensureAreaItems(); err != nil {
		return nil
	}
	if levels, ok := p.areaLevelByItem[areaItemID]; ok {
		return cloneEdAreaItemLevel(levels[level])
	}
	return nil
}

func (p *localEducationProvider) GetCharacterRank(characterID, rank int) *CharacterRank {
	if characterID <= 0 || rank <= 0 {
		return nil
	}
	if err := p.ensureCharacterRanks(); err != nil {
		return nil
	}
	if ranks, ok := p.rankByChar[characterID]; ok {
		return cloneEdCharacterRank(ranks[rank])
	}
	return nil
}

func (p *localEducationProvider) GetMysekaiGateLevel(gateID, level int) *MysekaiGateLevel {
	if gateID <= 0 || level <= 0 {
		return nil
	}
	if err := p.ensureGateLevels(); err != nil {
		return nil
	}
	if levels, ok := p.gateByID[gateID]; ok {
		return cloneEdMysekaiGateLevel(levels[level])
	}
	return nil
}

func (p *localEducationProvider) GetShopItemByResourceBoxID(resourceBoxID int) *ShopItem {
	if resourceBoxID <= 0 {
		return nil
	}
	if err := p.ensureShopItems(); err != nil {
		return nil
	}
	return cloneEdShopItem(p.shopByBoxID[resourceBoxID])
}
