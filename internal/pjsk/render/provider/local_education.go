package provider

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
