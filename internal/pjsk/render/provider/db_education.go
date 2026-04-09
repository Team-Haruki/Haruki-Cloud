package provider

import (
	"context"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
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

func educationContextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
