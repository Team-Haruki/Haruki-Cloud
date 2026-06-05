package provider

import "haruki-cloud/internal/pjsk/render/masterdata"

func (s *localStore) ResetCache() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.cache = make(map[string][]byte)
	s.mu.Unlock()
}

func (p *LocalProvider) ResetLocalMasterdataCache() {
	if p == nil {
		return
	}
	p.store.ResetCache()
	p.cards.reset()
	p.characters.reset()
	p.skills.reset()
	p.events.reset()
	p.musics.reset()
	p.gachas.reset()
	p.costumes.reset()
	p.honors.reset()
	p.stamps.reset()
	p.vlives.reset()
	p.education.reset()
	p.playerFrames.reset()
}

func (p *DatabaseProvider) ResetLocalMasterdataCache() {
	if p == nil {
		return
	}
	p.honors.resetLocalMasterdataCache()
	p.education.resetLocalMasterdataCache()
	p.musics.resetLocalMasterdataCache()
	p.events.resetLocalMasterdataCache()
	p.mysekai.resetLocalMasterdataCache()
}

func (p *localCharacterProvider) reset() {
	if p == nil {
		return
	}
	p.chars.reset()
	p.units.reset()
}

func (p *localSkillProvider) reset() {
	if p == nil {
		return
	}
	p.skills.reset()
}

func (p *localCardProvider) reset() {
	if p == nil {
		return
	}
	p.cards.reset()
	p.eventCards.reset()
	p.episodes.reset()
	p.supplies.reset()
	p.gachas.reset()
	p.costumes.reset()
}

func (p *localEventProvider) reset() {
	if p == nil {
		return
	}
	p.events.reset()
	p.eventCards.reset()
	p.deckBonus.reset()
	p.worldBloom.reset()
	p.worldBloomChapterRankingRewardRanges.reset()
	p.cards.reset()
}

func (p *localMusicProvider) reset() {
	if p == nil {
		return
	}
	p.musics.reset()
	p.diffs.reset()
	p.vocals.reset()
	p.tags.reset()
	p.outside.reset()
	p.eventMusics.reset()
	p.limited.reset()
}

func (p *localGachaProvider) reset() {
	if p == nil {
		return
	}
	p.gachas.reset()
	p.cards.reset()
	p.ceils.reset()
}

func (p *localCostumeProvider) reset() {
	if p == nil {
		return
	}
	p.costumes.reset()
}

func (p *localHonorProvider) reset() {
	if p == nil {
		return
	}
	p.honors.reset()
	p.groups.reset()
	p.bondsHonors.reset()
	p.bondsHonorWords.reset()
	p.gcu.reset()
	p.birthday.reset()
	p.eventHonors.reset()
}

func (p *localStampProvider) reset() {
	if p == nil {
		return
	}
	p.stamps.reset()
}

func (p *localVLiveProvider) reset() {
	if p == nil {
		return
	}
	p.lives.reset()
}

func (p *localEducationProvider) reset() {
	if p == nil {
		return
	}
	p.boxes.reset()
	p.rewards.reset()
	p.areas.reset()
	p.ranks.reset()
	p.bonds.reset()
	p.styles.reset()
	p.missions.reset()
	p.gates.reset()
	p.shops.reset()
}

func (p *localPlayerFrameProvider) reset() {
	if p == nil {
		return
	}
	p.frames.reset()
	p.groups.reset()
}

func (p *dbMusicProvider) resetLocalMasterdataCache() {
	if p == nil {
		return
	}
	if p.local != nil {
		p.local.store.ResetCache()
		p.local.reset()
	}
	p.init()
	p.mu.Lock()
	p.musicByID = make(map[int]*masterdata.Music)
	p.musicList = nil
	p.outsideByID = make(map[int]string)
	p.localizedByID = make(map[int][]string)
	p.difficultiesByID = make(map[int][]*masterdata.MusicDifficulty)
	p.mu.Unlock()
}

func (p *dbEventProvider) resetLocalMasterdataCache() {
	if p == nil {
		return
	}
	if p.store != nil {
		p.store.ResetCache()
	}
	if p.local != nil {
		p.local.reset()
	}
	p.init()
	p.eventMu.Lock()
	p.eventCache = make(map[int]*masterdata.Event)
	p.eventMu.Unlock()
	p.cardMu.Lock()
	p.cardCache = make(map[int]*masterdata.Card)
	p.cardMu.Unlock()
	p.unitMu.Lock()
	p.unitCache = make(map[int]string)
	p.unitMu.Unlock()
	p.supplyMu.Lock()
	p.supplyCache = make(map[int]string)
	p.supplyMu.Unlock()
}

func (p *dbEducationProvider) resetLocalMasterdataCache() {
	if p == nil {
		return
	}
	p.store.ResetCache()
	p.init()

	p.rewardMu.Lock()
	p.rewardsByChar = make(map[int][]*ChallengeReward)
	p.rewardsLoaded = false
	p.rewardMu.Unlock()

	p.boxMu.Lock()
	p.boxByID = make(map[int]*ResourceBox)
	p.boxByPurpose = make(map[string]map[int]*ResourceBox)
	p.boxesLoaded = false
	p.boxMu.Unlock()

	p.areaMu.Lock()
	p.areaByID = make(map[int]*AreaItem)
	p.areaLevelsByItem = make(map[int][]*AreaItemLevel)
	p.areaLevelByItem = make(map[int]map[int]*AreaItemLevel)
	p.areaMasterLoaded = false
	p.areaMu.Unlock()

	p.levelMu.Lock()
	p.characterLevels = nil
	p.characterLevelsLoaded = false
	p.levelMu.Unlock()

	p.rankMu.Lock()
	p.rankByChar = make(map[int]map[int]*CharacterRank)
	p.ranksLoaded = false
	p.rankMu.Unlock()

	p.bondMu.Lock()
	p.bonds = nil
	p.bondLevels = nil
	p.bondsLoaded = false
	p.bondMu.Unlock()

	p.styleMu.Lock()
	p.stylesByGameID = make(map[int]*GameCharacterStyle)
	p.stylesLoaded = false
	p.styleMu.Unlock()

	p.missionMu.Lock()
	p.characterMissionsByCharacter = make(map[int][]*CharacterMission)
	p.characterMissionGroupsByID = make(map[int][]*CharacterMissionParameterGroup)
	p.leaderRequirements = nil
	p.leaderMaxPlayLimit = 0
	p.leaderMissionsLoaded = false
	p.missionMu.Unlock()

	p.gateMu.Lock()
	p.gateByID = make(map[int]map[int]*MysekaiGateLevel)
	p.gatesLoaded = false
	p.gateMu.Unlock()

	p.shopMu.Lock()
	p.shopByBoxID = make(map[int]*ShopItem)
	p.shopItems = nil
	p.shopsLoaded = false
	p.shopMu.Unlock()
}

func (p *dbHonorProvider) resetLocalMasterdataCache() {
	if p == nil {
		return
	}
	p.store.ResetCache()
	p.init()

	p.honorMu.Lock()
	p.honorCache = make(map[int]*masterdata.Honor)
	p.honorMu.Unlock()

	p.groupMu.Lock()
	p.groupCache = make(map[int]*masterdata.HonorGroup)
	p.groupMu.Unlock()

	p.bondsMu.Lock()
	p.bondsCache = make(map[int]*masterdata.BondsHonor)
	p.bondsMu.Unlock()

	p.bondsWordMu.Lock()
	p.bondsWordCache = make(map[int]*masterdata.BondsHonorWord)
	p.bondsWordLoaded = false
	p.bondsWordMu.Unlock()

	p.gcuMu.Lock()
	p.gcuCache = make(map[int]*masterdata.GameCharacterUnit)
	p.gcuMu.Unlock()

	p.birthdayMu.Lock()
	p.birthdayByGroup = make(map[int]honorBirthdayAssets)
	p.birthdayChars = nil
	p.birthdayLoaded = false
	p.birthdayMu.Unlock()

	p.eventHonorMu.Lock()
	p.eventByHonorID = make(map[int]int)
	p.eventByHonorLoaded = false
	p.eventHonorMu.Unlock()
}

func (p *dbMySekaiProvider) resetLocalMasterdataCache() {
	if p == nil || p.local == nil {
		return
	}
	p.local.store.ResetCache()
}
