package provider

import (
	"encoding/json"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ===========================================================================
// localEducationProvider
// ===========================================================================

type boxIndex struct {
	byID      map[int]*ResourceBox
	byPurpose map[string]map[int]*ResourceBox
}

type areaIndex struct {
	byID         map[int]*AreaItem
	levelsByItem map[int][]*AreaItemLevel
	levelByItem  map[int]map[int]*AreaItemLevel
}

type bondData struct {
	bonds  []*Bond
	levels []*BondLevel
}

type missionData struct {
	requirements []LeaderMissionRequirement
	maxPlayLimit int
}

type localEducationProvider struct {
	store *localStore

	rewards  lazyValue[map[int][]*ChallengeReward]
	boxes    lazyValue[boxIndex]
	areas    lazyValue[areaIndex]
	ranks    lazyValue[map[int]map[int]*CharacterRank]
	bonds    lazyValue[bondData]
	styles   lazyValue[map[int]*GameCharacterStyle]
	missions lazyValue[missionData]
	gates    lazyValue[map[int]map[int]*MysekaiGateLevel]
	shops    lazyValue[map[int]*ShopItem]
}

func (p *localEducationProvider) ensureRewards() error {
	return p.rewards.init(func() (map[int][]*ChallengeReward, error) {
		items, err := loadJSON[ChallengeReward](p.store, "challengeLiveHighScoreRewards.json")
		if err != nil {
			return nil, err
		}
		byChar := make(map[int][]*ChallengeReward)
		for i := range items {
			byChar[items[i].CharacterID] = append(byChar[items[i].CharacterID], &items[i])
		}
		return byChar, nil
	})
}

func (p *localEducationProvider) ensureResourceBoxes() error {
	return p.boxes.init(func() (boxIndex, error) {
		items, err := loadJSON[ResourceBox](p.store, "resourceBoxes.json")
		if err != nil {
			return boxIndex{}, err
		}
		idx := boxIndex{
			byID:      make(map[int]*ResourceBox, len(items)),
			byPurpose: make(map[string]map[int]*ResourceBox),
		}
		for i := range items {
			box := &items[i]
			idx.byID[box.ID] = box
			if _, ok := idx.byPurpose[box.ResourceBoxPurpose]; !ok {
				idx.byPurpose[box.ResourceBoxPurpose] = make(map[int]*ResourceBox)
			}
			idx.byPurpose[box.ResourceBoxPurpose][box.ID] = box
		}
		return idx, nil
	})
}

func (p *localEducationProvider) ensureAreaItems() error {
	return p.areas.init(func() (areaIndex, error) {
		items, err := loadJSON[AreaItem](p.store, "areaItems.json")
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

		levels, err := loadJSON[AreaItemLevel](p.store, "areaItemLevels.json")
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
		items, err := loadJSON[localCharacterRankJSON](p.store, "characterRanks.json")
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

func (p *localEducationProvider) ensureBondMaster() error {
	return p.bonds.init(func() (bondData, error) {
		items, err := loadJSON[localBondJSON](p.store, "bonds.json")
		if err != nil {
			return bondData{}, err
		}
		data := bondData{bonds: make([]*Bond, 0, len(items))}
		for _, item := range items {
			data.bonds = append(data.bonds, &Bond{
				GroupID:      item.GroupID,
				CharacterID1: item.CharacterID1,
				CharacterID2: item.CharacterID2,
			})
		}

		levels, err := loadJSON[localLevelJSON](p.store, "levels.json")
		if err != nil {
			return bondData{}, err
		}
		data.levels = make([]*BondLevel, 0)
		for _, item := range levels {
			if !strings.EqualFold(item.LevelType, "bonds") || item.Level <= 0 {
				continue
			}
			data.levels = append(data.levels, &BondLevel{
				Level:    item.Level,
				TotalExp: item.TotalExp,
			})
		}
		sort.Slice(data.levels, func(i, j int) bool { return data.levels[i].Level < data.levels[j].Level })
		return data, nil
	})
}

func (p *localEducationProvider) ensureGameCharacterStyles() error {
	return p.styles.init(func() (map[int]*GameCharacterStyle, error) {
		items, err := loadJSON[masterdata.GameCharacterUnit](p.store, "gameCharacterUnits.json")
		if err != nil {
			return nil, err
		}
		byID := make(map[int]*GameCharacterStyle, len(items))
		for i := range items {
			byID[items[i].ID] = &GameCharacterStyle{
				GameID:      items[i].ID,
				CharacterID: items[i].GameCharacterID,
				ColorCode:   strings.TrimSpace(items[i].ColorCode),
			}
		}
		return byID, nil
	})
}

func (p *localEducationProvider) ensureLeaderMissionRequirements() error {
	return p.missions.init(func() (missionData, error) {
		items, err := loadJSON[localLeaderMissionRequirementJSON](p.store, "characterMissionV2ParameterGroups.json")
		if err != nil {
			return missionData{}, err
		}
		data := missionData{}
		for _, item := range items {
			switch item.GameID {
			case 1:
				if item.Requirement > data.maxPlayLimit {
					data.maxPlayLimit = item.Requirement
				}
			case 101:
				data.requirements = append(data.requirements, LeaderMissionRequirement{
					Seq:         item.Seq,
					Requirement: item.Requirement,
				})
			}
		}
		sort.Slice(data.requirements, func(i, j int) bool { return data.requirements[i].Seq < data.requirements[j].Seq })
		return data, nil
	})
}

func (p *localEducationProvider) ensureGateLevels() error {
	return p.gates.init(func() (map[int]map[int]*MysekaiGateLevel, error) {
		items, err := loadJSON[localMysekaiGateLevelJSON](p.store, "mysekaiGateLevels.json")
		if err != nil {
			return nil, err
		}
		byID := make(map[int]map[int]*MysekaiGateLevel)
		for _, item := range items {
			level := &MysekaiGateLevel{
				GateID:         item.MysekaiGateID,
				Level:          item.Level,
				PowerBonusRate: item.PowerBonusRate,
			}
			if _, ok := byID[level.GateID]; !ok {
				byID[level.GateID] = make(map[int]*MysekaiGateLevel)
			}
			byID[level.GateID][level.Level] = level
		}
		return byID, nil
	})
}

func (p *localEducationProvider) ensureShopItems() error {
	return p.shops.init(func() (map[int]*ShopItem, error) {
		items, err := loadJSON[localShopItemJSON](p.store, "shopItems.json")
		if err != nil {
			return nil, err
		}
		byBoxID := make(map[int]*ShopItem, len(items))
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
			byBoxID[entry.ResourceBoxID] = entry
		}
		return byBoxID, nil
	})
}

func (p *localEducationProvider) GetChallengeRewardsByCharacter(charID int) []*ChallengeReward {
	if charID <= 0 {
		return nil
	}
	if err := p.ensureRewards(); err != nil {
		return nil
	}
	return cloneEdChallengeRewards(p.rewards.v()[charID])
}

func (p *localEducationProvider) GetResourceBoxByPurpose(purpose string, id int) *ResourceBox {
	if id <= 0 {
		return nil
	}
	if err := p.ensureResourceBoxes(); err != nil {
		return nil
	}
	if strings.TrimSpace(purpose) == "" {
		return cloneEdResourceBox(p.boxes.v().byID[id])
	}
	if purposeMap, ok := p.boxes.v().byPurpose[purpose]; ok {
		return cloneEdResourceBox(purposeMap[id])
	}
	return nil
}

func (p *localEducationProvider) GetResourceBoxesByPurpose(purpose string) []*ResourceBox {
	if err := p.ensureResourceBoxes(); err != nil {
		return nil
	}
	if strings.TrimSpace(purpose) == "" {
		items := make([]*ResourceBox, 0, len(p.boxes.v().byID))
		for _, item := range p.boxes.v().byID {
			items = append(items, cloneEdResourceBox(item))
		}
		return items
	}
	purposeMap, ok := p.boxes.v().byPurpose[purpose]
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
	items := make([]*AreaItem, 0, len(p.areas.v().byID))
	for _, item := range p.areas.v().byID {
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
	return cloneEdAreaItem(p.areas.v().byID[id])
}

func (p *localEducationProvider) GetAreaItemLevels(areaItemID int) []*AreaItemLevel {
	if areaItemID <= 0 {
		return nil
	}
	if err := p.ensureAreaItems(); err != nil {
		return nil
	}
	return cloneEdAreaItemLevels(p.areas.v().levelsByItem[areaItemID])
}

func (p *localEducationProvider) GetAreaItemLevel(areaItemID, level int) *AreaItemLevel {
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

func (p *localEducationProvider) GetCharacterRank(characterID, rank int) *CharacterRank {
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

func (p *localEducationProvider) GetBonds() []*Bond {
	if err := p.ensureBondMaster(); err != nil {
		return nil
	}
	return cloneEdBonds(p.bonds.v().bonds)
}

func (p *localEducationProvider) GetBondLevels() []*BondLevel {
	if err := p.ensureBondMaster(); err != nil {
		return nil
	}
	return cloneEdBondLevels(p.bonds.v().levels)
}

func (p *localEducationProvider) GetGameCharacterStyle(gameID int) *GameCharacterStyle {
	if gameID <= 0 {
		return nil
	}
	if err := p.ensureGameCharacterStyles(); err != nil {
		return nil
	}
	return cloneEdGameCharacterStyle(p.styles.v()[gameID])
}

func (p *localEducationProvider) GetLeaderMissionRequirements() ([]LeaderMissionRequirement, int) {
	if err := p.ensureLeaderMissionRequirements(); err != nil {
		return nil, 0
	}
	return cloneEdLeaderMissionRequirements(p.missions.v().requirements), p.missions.v().maxPlayLimit
}

func (p *localEducationProvider) GetMysekaiGateLevel(gateID, level int) *MysekaiGateLevel {
	if gateID <= 0 || level <= 0 {
		return nil
	}
	if err := p.ensureGateLevels(); err != nil {
		return nil
	}
	if levels, ok := p.gates.v()[gateID]; ok {
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
	return cloneEdShopItem(p.shops.v()[resourceBoxID])
}

type localBondJSON struct {
	GroupID      int `json:"groupId"`
	CharacterID1 int `json:"characterId1"`
	CharacterID2 int `json:"characterId2"`
}

type localLevelJSON struct {
	LevelType string `json:"levelType"`
	Level     int    `json:"level"`
	TotalExp  int    `json:"totalExp"`
}

type localLeaderMissionRequirementJSON struct {
	GameID      int `json:"gameId"`
	Seq         int `json:"seq"`
	Requirement int `json:"requirement"`
}
