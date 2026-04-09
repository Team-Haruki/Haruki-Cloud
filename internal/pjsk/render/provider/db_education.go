package provider

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/areaitem"
	"haruki-cloud/database/sekai/areaitemlevel"
	"haruki-cloud/database/sekai/bond"
	"haruki-cloud/database/sekai/challengelivehighscorereward"
	"haruki-cloud/database/sekai/charactermissionv2parametergroup"
	"haruki-cloud/database/sekai/characterrank"
	"haruki-cloud/database/sekai/gamecharacterunit"
	"haruki-cloud/database/sekai/level"
	"haruki-cloud/database/sekai/mysekaigatelevel"
	"haruki-cloud/database/sekai/resourceboxe"
	"haruki-cloud/database/sekai/shopitem"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type dbEducationProvider struct {
	client *sekaiDB.Client
	region renderregion.Value

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

	bondMu      sync.RWMutex
	bonds       []*Bond
	bondLevels  []*BondLevel
	bondsLoaded bool

	styleMu        sync.RWMutex
	stylesByGameID map[int]*GameCharacterStyle
	stylesLoaded   bool

	missionMu            sync.RWMutex
	leaderRequirements   []LeaderMissionRequirement
	leaderMaxPlayLimit   int
	leaderMissionsLoaded bool

	gateMu      sync.RWMutex
	gateByID    map[int]map[int]*MysekaiGateLevel
	gatesLoaded bool

	shopMu      sync.RWMutex
	shopByBoxID map[int]*ShopItem
	shopsLoaded bool
}

func (p *dbEducationProvider) init() {
	if p.rewardsByChar == nil {
		p.rewardsByChar = make(map[int][]*ChallengeReward)
	}
	if p.boxByID == nil {
		p.boxByID = make(map[int]*ResourceBox)
	}
	if p.boxByPurpose == nil {
		p.boxByPurpose = make(map[string]map[int]*ResourceBox)
	}
	if p.areaByID == nil {
		p.areaByID = make(map[int]*AreaItem)
	}
	if p.areaLevelsByItem == nil {
		p.areaLevelsByItem = make(map[int][]*AreaItemLevel)
	}
	if p.areaLevelByItem == nil {
		p.areaLevelByItem = make(map[int]map[int]*AreaItemLevel)
	}
	if p.rankByChar == nil {
		p.rankByChar = make(map[int]map[int]*CharacterRank)
	}
	if p.stylesByGameID == nil {
		p.stylesByGameID = make(map[int]*GameCharacterStyle)
	}
	if p.gateByID == nil {
		p.gateByID = make(map[int]map[int]*MysekaiGateLevel)
	}
	if p.shopByBoxID == nil {
		p.shopByBoxID = make(map[int]*ShopItem)
	}
}

func (p *dbEducationProvider) GetChallengeRewardsByCharacter(charID int) []*ChallengeReward {
	return p.getChallengeRewardsByCharacter(nil, charID)
}

func (p *dbEducationProvider) getChallengeRewardsByCharacter(ctx context.Context, charID int) []*ChallengeReward {
	if charID <= 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.init()

	p.rewardMu.RLock()
	if p.rewardsLoaded {
		out := cloneEdChallengeRewards(p.rewardsByChar[charID])
		p.rewardMu.RUnlock()
		return out
	}
	p.rewardMu.RUnlock()

	p.rewardMu.Lock()
	defer p.rewardMu.Unlock()

	if !p.rewardsLoaded {
		items, err := p.client.Challengelivehighscorereward.Query().
			Where(challengelivehighscorereward.ServerRegionEQ(p.region.String())).
			All(ctx)
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
			p.rewardsByChar[reward.CharacterID] = append(p.rewardsByChar[reward.CharacterID], reward)
		}
		p.rewardsLoaded = true
	}

	return cloneEdChallengeRewards(p.rewardsByChar[charID])
}

func (p *dbEducationProvider) GetResourceBoxByPurpose(purpose string, id int) *ResourceBox {
	return p.getResourceBoxByPurpose(nil, purpose, id)
}

func (p *dbEducationProvider) getResourceBoxByPurpose(ctx context.Context, purpose string, id int) *ResourceBox {
	if id <= 0 {
		return nil
	}
	if !p.ensureResourceBoxesLoaded(ctx) {
		return nil
	}

	p.boxMu.RLock()
	defer p.boxMu.RUnlock()

	if strings.TrimSpace(purpose) == "" {
		return cloneEdResourceBox(p.boxByID[id])
	}
	if purposeMap, ok := p.boxByPurpose[purpose]; ok {
		return cloneEdResourceBox(purposeMap[id])
	}
	return nil
}

func (p *dbEducationProvider) GetResourceBoxesByPurpose(purpose string) []*ResourceBox {
	return p.getResourceBoxesByPurpose(nil, purpose)
}

func (p *dbEducationProvider) getResourceBoxesByPurpose(ctx context.Context, purpose string) []*ResourceBox {
	if !p.ensureResourceBoxesLoaded(ctx) {
		return nil
	}

	p.boxMu.RLock()
	defer p.boxMu.RUnlock()

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

func (p *dbEducationProvider) GetAreaItem(id int) *AreaItem {
	return p.getAreaItem(nil, id)
}

func (p *dbEducationProvider) getAreaItem(ctx context.Context, id int) *AreaItem {
	if id <= 0 || !p.ensureAreaMasterLoaded(ctx) {
		return nil
	}

	p.areaMu.RLock()
	defer p.areaMu.RUnlock()
	return cloneEdAreaItem(p.areaByID[id])
}

func (p *dbEducationProvider) GetAreaItems() []*AreaItem {
	return p.getAreaItems(nil)
}

func (p *dbEducationProvider) getAreaItems(ctx context.Context) []*AreaItem {
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

func (p *dbEducationProvider) GetAreaItemLevels(areaItemID int) []*AreaItemLevel {
	return p.getAreaItemLevels(nil, areaItemID)
}

func (p *dbEducationProvider) getAreaItemLevels(ctx context.Context, areaItemID int) []*AreaItemLevel {
	if areaItemID <= 0 || !p.ensureAreaMasterLoaded(ctx) {
		return nil
	}

	p.areaMu.RLock()
	defer p.areaMu.RUnlock()
	return cloneEdAreaItemLevels(p.areaLevelsByItem[areaItemID])
}

func (p *dbEducationProvider) GetAreaItemLevel(areaItemID, level int) *AreaItemLevel {
	return p.getAreaItemLevel(nil, areaItemID, level)
}

func (p *dbEducationProvider) getAreaItemLevel(ctx context.Context, areaItemID, level int) *AreaItemLevel {
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

func (p *dbEducationProvider) GetCharacterRank(characterID, rank int) *CharacterRank {
	return p.getCharacterRank(nil, characterID, rank)
}

func (p *dbEducationProvider) getCharacterRank(ctx context.Context, characterID, rank int) *CharacterRank {
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

func (p *dbEducationProvider) GetBonds() []*Bond {
	return p.getBonds(nil)
}

func (p *dbEducationProvider) getBonds(ctx context.Context) []*Bond {
	if !p.ensureBondMasterLoaded(ctx) {
		return nil
	}

	p.bondMu.RLock()
	defer p.bondMu.RUnlock()
	return cloneEdBonds(p.bonds)
}

func (p *dbEducationProvider) GetBondLevels() []*BondLevel {
	return p.getBondLevels(nil)
}

func (p *dbEducationProvider) getBondLevels(ctx context.Context) []*BondLevel {
	if !p.ensureBondMasterLoaded(ctx) {
		return nil
	}

	p.bondMu.RLock()
	defer p.bondMu.RUnlock()
	return cloneEdBondLevels(p.bondLevels)
}

func (p *dbEducationProvider) GetGameCharacterStyle(gameID int) *GameCharacterStyle {
	return p.getGameCharacterStyle(nil, gameID)
}

func (p *dbEducationProvider) getGameCharacterStyle(ctx context.Context, gameID int) *GameCharacterStyle {
	if gameID <= 0 || !p.ensureGameCharacterStylesLoaded(ctx) {
		return nil
	}

	p.styleMu.RLock()
	defer p.styleMu.RUnlock()
	return cloneEdGameCharacterStyle(p.stylesByGameID[gameID])
}

func (p *dbEducationProvider) GetLeaderMissionRequirements() ([]LeaderMissionRequirement, int) {
	return p.getLeaderMissionRequirements(nil)
}

func (p *dbEducationProvider) getLeaderMissionRequirements(ctx context.Context) ([]LeaderMissionRequirement, int) {
	if !p.ensureLeaderMissionsLoaded(ctx) {
		return nil, 0
	}

	p.missionMu.RLock()
	defer p.missionMu.RUnlock()
	return cloneEdLeaderMissionRequirements(p.leaderRequirements), p.leaderMaxPlayLimit
}

func (p *dbEducationProvider) GetMysekaiGateLevel(gateID, level int) *MysekaiGateLevel {
	return p.getMysekaiGateLevel(nil, gateID, level)
}

func (p *dbEducationProvider) getMysekaiGateLevel(ctx context.Context, gateID, level int) *MysekaiGateLevel {
	if gateID <= 0 || level <= 0 || !p.ensureGateLevelsLoaded(ctx) {
		return nil
	}

	p.gateMu.RLock()
	defer p.gateMu.RUnlock()
	if levels, ok := p.gateByID[gateID]; ok {
		return cloneEdMysekaiGateLevel(levels[level])
	}
	return nil
}

func (p *dbEducationProvider) GetShopItemByResourceBoxID(resourceBoxID int) *ShopItem {
	return p.getShopItemByResourceBoxID(nil, resourceBoxID)
}

func (p *dbEducationProvider) getShopItemByResourceBoxID(ctx context.Context, resourceBoxID int) *ShopItem {
	if resourceBoxID <= 0 || !p.ensureShopItemsLoaded(ctx) {
		return nil
	}

	p.shopMu.RLock()
	defer p.shopMu.RUnlock()
	return cloneEdShopItem(p.shopByBoxID[resourceBoxID])
}

func (p *dbEducationProvider) ensureResourceBoxesLoaded(ctx context.Context) bool {
	p.init()
	if ctx == nil {
		ctx = context.Background()
	}

	p.boxMu.RLock()
	if p.boxesLoaded {
		p.boxMu.RUnlock()
		return true
	}
	p.boxMu.RUnlock()

	p.boxMu.Lock()
	defer p.boxMu.Unlock()

	if p.boxesLoaded {
		return true
	}

	items, err := p.client.Resourceboxe.Query().
		Where(resourceboxe.ServerRegionEQ(p.region.String())).
		All(ctx)
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
		p.boxByID[box.ID] = box
		if _, ok := p.boxByPurpose[box.ResourceBoxPurpose]; !ok {
			p.boxByPurpose[box.ResourceBoxPurpose] = make(map[int]*ResourceBox)
		}
		p.boxByPurpose[box.ResourceBoxPurpose][box.ID] = box
	}
	p.boxesLoaded = true
	return true
}

func (p *dbEducationProvider) ensureAreaMasterLoaded(ctx context.Context) bool {
	p.init()
	if ctx == nil {
		ctx = context.Background()
	}

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
	if ctx == nil {
		ctx = context.Background()
	}

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

func (p *dbEducationProvider) ensureBondMasterLoaded(ctx context.Context) bool {
	p.init()
	if ctx == nil {
		ctx = context.Background()
	}

	p.bondMu.RLock()
	if p.bondsLoaded {
		p.bondMu.RUnlock()
		return true
	}
	p.bondMu.RUnlock()

	p.bondMu.Lock()
	defer p.bondMu.Unlock()

	if p.bondsLoaded {
		return true
	}

	items, err := p.client.Bond.Query().
		Where(bond.ServerRegionEQ(p.region.String())).
		All(ctx)
	if err != nil {
		return false
	}
	p.bonds = make([]*Bond, 0, len(items))
	for _, item := range items {
		p.bonds = append(p.bonds, &Bond{
			GroupID:      int(item.GroupID),
			CharacterID1: int(item.CharacterId1),
			CharacterID2: int(item.CharacterId2),
		})
	}

	levels, err := p.client.Level.Query().
		Where(level.ServerRegionEQ(p.region.String()), level.LevelTypeEQ("bonds")).
		All(ctx)
	if err != nil {
		return false
	}
	p.bondLevels = make([]*BondLevel, 0, len(levels))
	for _, item := range levels {
		p.bondLevels = append(p.bondLevels, &BondLevel{
			Level:    int(item.Level),
			TotalExp: int(item.TotalExp),
		})
	}

	p.bondsLoaded = true
	return true
}

func (p *dbEducationProvider) ensureGameCharacterStylesLoaded(ctx context.Context) bool {
	p.init()
	if ctx == nil {
		ctx = context.Background()
	}

	p.styleMu.RLock()
	if p.stylesLoaded {
		p.styleMu.RUnlock()
		return true
	}
	p.styleMu.RUnlock()

	p.styleMu.Lock()
	defer p.styleMu.Unlock()

	if p.stylesLoaded {
		return true
	}

	items, err := p.client.Gamecharacterunit.Query().
		Where(gamecharacterunit.ServerRegionEQ(p.region.String())).
		All(ctx)
	if err != nil {
		return false
	}
	for _, item := range items {
		p.stylesByGameID[int(item.GameID)] = &GameCharacterStyle{
			GameID:      int(item.GameID),
			CharacterID: int(item.GameCharacterID),
			ColorCode:   strings.TrimSpace(item.ColorCode),
		}
	}

	p.stylesLoaded = true
	return true
}

func (p *dbEducationProvider) ensureLeaderMissionsLoaded(ctx context.Context) bool {
	p.init()
	if ctx == nil {
		ctx = context.Background()
	}

	p.missionMu.RLock()
	if p.leaderMissionsLoaded {
		p.missionMu.RUnlock()
		return true
	}
	p.missionMu.RUnlock()

	p.missionMu.Lock()
	defer p.missionMu.Unlock()

	if p.leaderMissionsLoaded {
		return true
	}

	items, err := p.client.Charactermissionv2Parametergroup.Query().
		Where(
			charactermissionv2parametergroup.ServerRegionEQ(p.region.String()),
			charactermissionv2parametergroup.GameIDIn(1, 101),
		).
		Order(charactermissionv2parametergroup.ByGameID(), charactermissionv2parametergroup.BySeq()).
		All(ctx)
	if err != nil {
		return false
	}
	p.leaderRequirements = make([]LeaderMissionRequirement, 0)
	for _, item := range items {
		switch item.GameID {
		case 1:
			if requirement := int(item.Requirement); requirement > p.leaderMaxPlayLimit {
				p.leaderMaxPlayLimit = requirement
			}
		case 101:
			p.leaderRequirements = append(p.leaderRequirements, LeaderMissionRequirement{
				Seq:         int(item.Seq),
				Requirement: int(item.Requirement),
			})
		}
	}

	p.leaderMissionsLoaded = true
	return true
}

func (p *dbEducationProvider) ensureGateLevelsLoaded(ctx context.Context) bool {
	p.init()
	if ctx == nil {
		ctx = context.Background()
	}

	p.gateMu.RLock()
	if p.gatesLoaded {
		p.gateMu.RUnlock()
		return true
	}
	p.gateMu.RUnlock()

	p.gateMu.Lock()
	defer p.gateMu.Unlock()

	if p.gatesLoaded {
		return true
	}

	items, err := p.client.Mysekaigatelevel.Query().
		Where(mysekaigatelevel.ServerRegionEQ(p.region.String())).
		All(ctx)
	if err != nil {
		return false
	}
	for _, item := range items {
		level := &MysekaiGateLevel{
			GateID:         int(item.MysekaiGateID),
			Level:          int(item.Level),
			PowerBonusRate: item.PowerBonusRate,
		}
		if _, ok := p.gateByID[level.GateID]; !ok {
			p.gateByID[level.GateID] = make(map[int]*MysekaiGateLevel)
		}
		p.gateByID[level.GateID][level.Level] = level
	}
	p.gatesLoaded = true
	return true
}

func (p *dbEducationProvider) ensureShopItemsLoaded(ctx context.Context) bool {
	p.init()
	if ctx == nil {
		ctx = context.Background()
	}

	p.shopMu.RLock()
	if p.shopsLoaded {
		p.shopMu.RUnlock()
		return true
	}
	p.shopMu.RUnlock()

	p.shopMu.Lock()
	defer p.shopMu.Unlock()

	if p.shopsLoaded {
		return true
	}

	items, err := p.client.Shopitem.Query().
		Where(shopitem.ServerRegionEQ(p.region.String())).
		All(ctx)
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
		p.shopByBoxID[shopEntry.ResourceBoxID] = shopEntry
	}
	p.shopsLoaded = true
	return true
}

func cloneEdChallengeRewards(source []*ChallengeReward) []*ChallengeReward {
	if len(source) == 0 {
		return nil
	}
	out := make([]*ChallengeReward, 0, len(source))
	for _, item := range source {
		if item == nil {
			continue
		}
		c := *item
		out = append(out, &c)
	}
	return out
}

func cloneEdResourceBox(source *ResourceBox) *ResourceBox {
	if source == nil {
		return nil
	}
	c := *source
	c.Details = append([]ResourceBoxDetail(nil), source.Details...)
	return &c
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

func cloneEdBonds(source []*Bond) []*Bond {
	if len(source) == 0 {
		return nil
	}
	out := make([]*Bond, 0, len(source))
	for _, item := range source {
		if item == nil {
			continue
		}
		c := *item
		out = append(out, &c)
	}
	return out
}

func cloneEdBondLevels(source []*BondLevel) []*BondLevel {
	if len(source) == 0 {
		return nil
	}
	out := make([]*BondLevel, 0, len(source))
	for _, item := range source {
		if item == nil {
			continue
		}
		c := *item
		out = append(out, &c)
	}
	return out
}

func cloneEdGameCharacterStyle(source *GameCharacterStyle) *GameCharacterStyle {
	if source == nil {
		return nil
	}
	c := *source
	return &c
}

func cloneEdLeaderMissionRequirements(source []LeaderMissionRequirement) []LeaderMissionRequirement {
	if len(source) == 0 {
		return nil
	}
	return append([]LeaderMissionRequirement(nil), source...)
}

func cloneEdMysekaiGateLevel(source *MysekaiGateLevel) *MysekaiGateLevel {
	if source == nil {
		return nil
	}
	c := *source
	return &c
}

func cloneEdShopItem(source *ShopItem) *ShopItem {
	if source == nil {
		return nil
	}
	c := *source
	c.Costs = append([]ShopItemCost(nil), source.Costs...)
	return &c
}
