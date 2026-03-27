package education

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/areaitem"
	"haruki-cloud/database/sekai/areaitemlevel"
	"haruki-cloud/database/sekai/challengelivehighscorereward"
	"haruki-cloud/database/sekai/characterrank"
	"haruki-cloud/database/sekai/mysekaigatelevel"
	"haruki-cloud/database/sekai/resourceboxe"
	"haruki-cloud/database/sekai/shopitem"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type CloudSource struct {
	client      *sekaiDB.Client
	region      renderregion.Value
	queryRegion renderregion.Value

	rewardMu      sync.RWMutex
	rewardsByChar map[int][]*ChallengeReward
	rewardsLoaded bool

	boxMu        sync.RWMutex
	boxByID      map[int]*ResourceBox
	boxByPurpose map[string]map[int]*ResourceBox
	boxesLoaded  bool

	areaMu           sync.RWMutex
	areaByID         map[int]*AreaItem
	areaLevelsByItem map[int][]*AreaItemLevel
	areaLevelByItem  map[int]map[int]*AreaItemLevel
	areaMasterLoaded bool

	rankMu      sync.RWMutex
	rankByChar  map[int]map[int]*CharacterRank
	ranksLoaded bool

	gateMu      sync.RWMutex
	gateByID    map[int]map[int]*MysekaiGateLevel
	gatesLoaded bool

	shopMu      sync.RWMutex
	shopByBoxID map[int]*ShopItem
	shopsLoaded bool
}

func NewCloudSource(client *sekaiDB.Client, defaultRegion renderregion.Value) *CloudSource {
	if client == nil {
		return nil
	}
	region := renderregion.WithDefault(defaultRegion)
	return &CloudSource{
		client:           client,
		region:           region,
		queryRegion:      region,
		rewardsByChar:    make(map[int][]*ChallengeReward),
		boxByID:          make(map[int]*ResourceBox),
		boxByPurpose:     make(map[string]map[int]*ResourceBox),
		areaByID:         make(map[int]*AreaItem),
		areaLevelsByItem: make(map[int][]*AreaItemLevel),
		areaLevelByItem:  make(map[int]map[int]*AreaItemLevel),
		rankByChar:       make(map[int]map[int]*CharacterRank),
		gateByID:         make(map[int]map[int]*MysekaiGateLevel),
		shopByBoxID:      make(map[int]*ShopItem),
	}
}

func (c *CloudSource) DefaultRegion() renderregion.Value {
	return c.region
}

func (c *CloudSource) GetChallengeRewardsByCharacter(charID int) []*ChallengeReward {
	if charID <= 0 {
		return nil
	}

	c.rewardMu.RLock()
	if c.rewardsLoaded {
		out := cloneChallengeRewards(c.rewardsByChar[charID])
		c.rewardMu.RUnlock()
		return out
	}
	c.rewardMu.RUnlock()

	c.rewardMu.Lock()
	defer c.rewardMu.Unlock()

	if !c.rewardsLoaded {
		items, err := c.client.Challengelivehighscorereward.Query().
			Where(challengelivehighscorereward.ServerRegionEQ(c.queryRegion.String())).
			All(context.Background())
		if err != nil {
			return nil
		}
		for _, item := range items {
			reward := &ChallengeReward{
				ID:            int(item.GameID),
				CharacterID:   int(item.CharacterID),
				HighScore:     int(item.HighScore),
				ResourceBoxID: int(item.ResourceBoxID),
			}
			c.rewardsByChar[reward.CharacterID] = append(c.rewardsByChar[reward.CharacterID], reward)
		}
		c.rewardsLoaded = true
	}

	return cloneChallengeRewards(c.rewardsByChar[charID])
}

func (c *CloudSource) GetResourceBoxByPurpose(purpose string, id int) *ResourceBox {
	if id <= 0 {
		return nil
	}
	if !c.ensureResourceBoxesLoaded() {
		return nil
	}

	c.boxMu.RLock()
	defer c.boxMu.RUnlock()

	if strings.TrimSpace(purpose) == "" {
		return cloneResourceBox(c.boxByID[id])
	}
	if purposeMap, ok := c.boxByPurpose[purpose]; ok {
		return cloneResourceBox(purposeMap[id])
	}
	return nil
}

func (c *CloudSource) GetResourceBoxesByPurpose(purpose string) []*ResourceBox {
	if !c.ensureResourceBoxesLoaded() {
		return nil
	}

	c.boxMu.RLock()
	defer c.boxMu.RUnlock()

	if strings.TrimSpace(purpose) == "" {
		items := make([]*ResourceBox, 0, len(c.boxByID))
		for _, item := range c.boxByID {
			items = append(items, cloneResourceBox(item))
		}
		return items
	}

	purposeMap, ok := c.boxByPurpose[purpose]
	if !ok {
		return nil
	}
	items := make([]*ResourceBox, 0, len(purposeMap))
	for _, item := range purposeMap {
		items = append(items, cloneResourceBox(item))
	}
	return items
}

func (c *CloudSource) GetAreaItem(id int) *AreaItem {
	if id <= 0 || !c.ensureAreaMasterLoaded() {
		return nil
	}

	c.areaMu.RLock()
	defer c.areaMu.RUnlock()
	return cloneAreaItem(c.areaByID[id])
}

func (c *CloudSource) GetAreaItemLevels(areaItemID int) []*AreaItemLevel {
	if areaItemID <= 0 || !c.ensureAreaMasterLoaded() {
		return nil
	}

	c.areaMu.RLock()
	defer c.areaMu.RUnlock()
	return cloneAreaItemLevels(c.areaLevelsByItem[areaItemID])
}

func (c *CloudSource) GetAreaItemLevel(areaItemID, level int) *AreaItemLevel {
	if areaItemID <= 0 || level <= 0 || !c.ensureAreaMasterLoaded() {
		return nil
	}

	c.areaMu.RLock()
	defer c.areaMu.RUnlock()
	if levels, ok := c.areaLevelByItem[areaItemID]; ok {
		return cloneAreaItemLevel(levels[level])
	}
	return nil
}

func (c *CloudSource) GetCharacterRank(characterID, rank int) *CharacterRank {
	if characterID <= 0 || rank <= 0 || !c.ensureCharacterRanksLoaded() {
		return nil
	}

	c.rankMu.RLock()
	defer c.rankMu.RUnlock()
	if ranks, ok := c.rankByChar[characterID]; ok {
		return cloneCharacterRank(ranks[rank])
	}
	return nil
}

func (c *CloudSource) GetMysekaiGateLevel(gateID, level int) *MysekaiGateLevel {
	if gateID <= 0 || level <= 0 || !c.ensureGateLevelsLoaded() {
		return nil
	}

	c.gateMu.RLock()
	defer c.gateMu.RUnlock()
	if levels, ok := c.gateByID[gateID]; ok {
		return cloneMysekaiGateLevel(levels[level])
	}
	return nil
}

func (c *CloudSource) GetShopItemByResourceBoxID(resourceBoxID int) *ShopItem {
	if resourceBoxID <= 0 || !c.ensureShopItemsLoaded() {
		return nil
	}

	c.shopMu.RLock()
	defer c.shopMu.RUnlock()
	return cloneShopItem(c.shopByBoxID[resourceBoxID])
}

func (c *CloudSource) ensureResourceBoxesLoaded() bool {
	c.boxMu.RLock()
	if c.boxesLoaded {
		c.boxMu.RUnlock()
		return true
	}
	c.boxMu.RUnlock()

	c.boxMu.Lock()
	defer c.boxMu.Unlock()

	if c.boxesLoaded {
		return true
	}

	items, err := c.client.Resourceboxe.Query().
		Where(resourceboxe.ServerRegionEQ(c.queryRegion.String())).
		All(context.Background())
	if err != nil {
		return false
	}
	for _, item := range items {
		box := &ResourceBox{
			ID:                 int(item.GameID),
			ResourceBoxPurpose: item.ResourceBoxPurpose,
			ResourceBoxType:    item.ResourceBoxType,
			Description:        item.Description,
		}
		if len(item.Details) > 0 {
			var details []ResourceBoxDetail
			if err := json.Unmarshal(item.Details, &details); err != nil {
				continue
			}
			box.Details = details
		}
		c.boxByID[box.ID] = box
		if _, ok := c.boxByPurpose[box.ResourceBoxPurpose]; !ok {
			c.boxByPurpose[box.ResourceBoxPurpose] = make(map[int]*ResourceBox)
		}
		c.boxByPurpose[box.ResourceBoxPurpose][box.ID] = box
	}
	c.boxesLoaded = true
	return true
}

func (c *CloudSource) ensureAreaMasterLoaded() bool {
	c.areaMu.RLock()
	if c.areaMasterLoaded {
		c.areaMu.RUnlock()
		return true
	}
	c.areaMu.RUnlock()

	c.areaMu.Lock()
	defer c.areaMu.Unlock()

	if c.areaMasterLoaded {
		return true
	}

	items, err := c.client.Areaitem.Query().
		Where(areaitem.ServerRegionEQ(c.queryRegion.String())).
		All(context.Background())
	if err != nil {
		return false
	}
	for _, item := range items {
		c.areaByID[int(item.GameID)] = &AreaItem{
			ID:              int(item.GameID),
			AreaID:          int(item.AreaID),
			Name:            item.Name,
			AssetbundleName: item.AssetbundleName,
		}
	}

	levels, err := c.client.Areaitemlevel.Query().
		Where(areaitemlevel.ServerRegionEQ(c.queryRegion.String())).
		All(context.Background())
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
		c.areaLevelsByItem[level.AreaItemID] = append(c.areaLevelsByItem[level.AreaItemID], level)
		if _, ok := c.areaLevelByItem[level.AreaItemID]; !ok {
			c.areaLevelByItem[level.AreaItemID] = make(map[int]*AreaItemLevel)
		}
		c.areaLevelByItem[level.AreaItemID][level.Level] = level
	}

	c.areaMasterLoaded = true
	return true
}

func (c *CloudSource) ensureCharacterRanksLoaded() bool {
	c.rankMu.RLock()
	if c.ranksLoaded {
		c.rankMu.RUnlock()
		return true
	}
	c.rankMu.RUnlock()

	c.rankMu.Lock()
	defer c.rankMu.Unlock()

	if c.ranksLoaded {
		return true
	}

	items, err := c.client.Characterrank.Query().
		Where(characterrank.ServerRegionEQ(c.queryRegion.String())).
		All(context.Background())
	if err != nil {
		return false
	}
	for _, item := range items {
		rank := &CharacterRank{
			CharacterID:     int(item.CharacterID),
			Rank:            int(item.CharacterRank),
			Power1BonusRate: item.Power1BonusRate,
		}
		if _, ok := c.rankByChar[rank.CharacterID]; !ok {
			c.rankByChar[rank.CharacterID] = make(map[int]*CharacterRank)
		}
		c.rankByChar[rank.CharacterID][rank.Rank] = rank
	}
	c.ranksLoaded = true
	return true
}

func (c *CloudSource) ensureGateLevelsLoaded() bool {
	c.gateMu.RLock()
	if c.gatesLoaded {
		c.gateMu.RUnlock()
		return true
	}
	c.gateMu.RUnlock()

	c.gateMu.Lock()
	defer c.gateMu.Unlock()

	if c.gatesLoaded {
		return true
	}

	items, err := c.client.Mysekaigatelevel.Query().
		Where(mysekaigatelevel.ServerRegionEQ(c.queryRegion.String())).
		All(context.Background())
	if err != nil {
		return false
	}
	for _, item := range items {
		level := &MysekaiGateLevel{
			GateID:         int(item.MysekaiGateID),
			Level:          int(item.Level),
			PowerBonusRate: item.PowerBonusRate,
		}
		if _, ok := c.gateByID[level.GateID]; !ok {
			c.gateByID[level.GateID] = make(map[int]*MysekaiGateLevel)
		}
		c.gateByID[level.GateID][level.Level] = level
	}
	c.gatesLoaded = true
	return true
}

func (c *CloudSource) ensureShopItemsLoaded() bool {
	c.shopMu.RLock()
	if c.shopsLoaded {
		c.shopMu.RUnlock()
		return true
	}
	c.shopMu.RUnlock()

	c.shopMu.Lock()
	defer c.shopMu.Unlock()

	if c.shopsLoaded {
		return true
	}

	items, err := c.client.Shopitem.Query().
		Where(shopitem.ServerRegionEQ(c.queryRegion.String())).
		All(context.Background())
	if err != nil {
		return false
	}
	for _, item := range items {
		shopEntry := &ShopItem{
			ID:            int(item.GameID),
			ResourceBoxID: int(item.ResourceBoxID),
		}
		if len(item.Costs) > 0 {
			var rawCosts []struct {
				Cost ShopItemCost `json:"cost"`
			}
			if err := json.Unmarshal(item.Costs, &rawCosts); err == nil {
				shopEntry.Costs = make([]ShopItemCost, 0, len(rawCosts))
				for _, raw := range rawCosts {
					shopEntry.Costs = append(shopEntry.Costs, raw.Cost)
				}
			}
		}
		c.shopByBoxID[shopEntry.ResourceBoxID] = shopEntry
	}
	c.shopsLoaded = true
	return true
}

func cloneChallengeRewards(source []*ChallengeReward) []*ChallengeReward {
	if len(source) == 0 {
		return nil
	}
	out := make([]*ChallengeReward, 0, len(source))
	for _, item := range source {
		if item == nil {
			continue
		}
		copy := *item
		out = append(out, &copy)
	}
	return out
}

func cloneResourceBox(source *ResourceBox) *ResourceBox {
	if source == nil {
		return nil
	}
	copy := *source
	copy.Details = append([]ResourceBoxDetail(nil), source.Details...)
	return &copy
}

func cloneAreaItem(source *AreaItem) *AreaItem {
	if source == nil {
		return nil
	}
	copy := *source
	return &copy
}

func cloneAreaItemLevels(source []*AreaItemLevel) []*AreaItemLevel {
	if len(source) == 0 {
		return nil
	}
	out := make([]*AreaItemLevel, 0, len(source))
	for _, item := range source {
		if item == nil {
			continue
		}
		copy := *item
		out = append(out, &copy)
	}
	return out
}

func cloneAreaItemLevel(source *AreaItemLevel) *AreaItemLevel {
	if source == nil {
		return nil
	}
	copy := *source
	return &copy
}

func cloneCharacterRank(source *CharacterRank) *CharacterRank {
	if source == nil {
		return nil
	}
	copy := *source
	return &copy
}

func cloneMysekaiGateLevel(source *MysekaiGateLevel) *MysekaiGateLevel {
	if source == nil {
		return nil
	}
	copy := *source
	return &copy
}

func cloneShopItem(source *ShopItem) *ShopItem {
	if source == nil {
		return nil
	}
	copy := *source
	copy.Costs = append([]ShopItemCost(nil), source.Costs...)
	return &copy
}
