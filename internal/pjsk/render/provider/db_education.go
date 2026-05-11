package provider

import (
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	renderregion "haruki-cloud/internal/pjsk/region"
)

type dbEducationProvider struct {
	client *sekaiDB.Client
	region renderregion.Value
	store  *localStore
	once   sync.Once

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

	levelMu               sync.RWMutex
	characterLevels       []*CharacterLevel
	characterLevelsLoaded bool

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

	missionMu                    sync.RWMutex
	characterMissionsByCharacter map[int][]*CharacterMission
	characterMissionGroupsByID   map[int][]*CharacterMissionParameterGroup
	leaderRequirements           []LeaderMissionRequirement
	leaderMaxPlayLimit           int
	leaderMissionsLoaded         bool

	gateMu      sync.RWMutex
	gateByID    map[int]map[int]*MysekaiGateLevel
	gatesLoaded bool

	shopMu      sync.RWMutex
	shopByBoxID map[int]*ShopItem
	shopItems   []*ShopItem
	shopsLoaded bool
}

func (p *dbEducationProvider) init() {
	p.once.Do(func() {
		p.rewardsByChar = make(map[int][]*ChallengeReward)
		p.boxByID = make(map[int]*ResourceBox)
		p.boxByPurpose = make(map[string]map[int]*ResourceBox)
		p.areaByID = make(map[int]*AreaItem)
		p.areaLevelsByItem = make(map[int][]*AreaItemLevel)
		p.areaLevelByItem = make(map[int]map[int]*AreaItemLevel)
		p.rankByChar = make(map[int]map[int]*CharacterRank)
		p.stylesByGameID = make(map[int]*GameCharacterStyle)
		p.characterMissionsByCharacter = make(map[int][]*CharacterMission)
		p.characterMissionGroupsByID = make(map[int][]*CharacterMissionParameterGroup)
		p.gateByID = make(map[int]map[int]*MysekaiGateLevel)
		p.shopByBoxID = make(map[int]*ShopItem)
		p.shopItems = nil
	})
}
