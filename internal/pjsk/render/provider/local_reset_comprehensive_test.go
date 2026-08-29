package provider

import (
	"errors"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

var errResetTestDirtyCache = errors.New("dirty cache")

type resetTestLazyCheck struct {
	name  string
	reset func() bool
}

func resetTestMarkLazyDirty[T any](checks *[]resetTestLazyCheck, name string, value *lazyValue[T]) {
	value.loaded = true
	value.err = errResetTestDirtyCache
	*checks = append(*checks, resetTestLazyCheck{
		name: name,
		reset: func() bool {
			return !value.loaded && value.err == nil
		},
	})
}

func resetTestRequireLazyCachesReset(t *testing.T, checks []resetTestLazyCheck) {
	t.Helper()
	for _, check := range checks {
		if !check.reset() {
			t.Errorf("lazy cache %s was not reset", check.name)
		}
	}
}

func resetTestDirtyStore(name string) *localStore {
	store := newLocalStore(name)
	store.cache["cached.json"] = []byte("cached")
	return store
}

func resetTestRequireStoreReset(t *testing.T, name string, store *localStore) {
	t.Helper()
	if store == nil || store.cache == nil || len(store.cache) != 0 {
		t.Errorf("local store %s was not reset: %#v", name, store)
	}
}

func resetTestRequireEmptyMap[K comparable, V any](t *testing.T, name string, value map[K]V) {
	t.Helper()
	if value == nil || len(value) != 0 {
		t.Errorf("cache map %s was not reset: %#v", name, value)
	}
}

func resetTestMarkLocalCardCaches(checks *[]resetTestLazyCheck, prefix string, provider *localCardProvider) {
	resetTestMarkLazyDirty(checks, prefix+".cards", &provider.cards)
	resetTestMarkLazyDirty(checks, prefix+".eventCards", &provider.eventCards)
	resetTestMarkLazyDirty(checks, prefix+".episodes", &provider.episodes)
	resetTestMarkLazyDirty(checks, prefix+".supplies", &provider.supplies)
	resetTestMarkLazyDirty(checks, prefix+".gachas", &provider.gachas)
	resetTestMarkLazyDirty(checks, prefix+".costumes", &provider.costumes)
}

func resetTestMarkLocalEventCaches(checks *[]resetTestLazyCheck, prefix string, provider *localEventProvider) {
	resetTestMarkLazyDirty(checks, prefix+".events", &provider.events)
	resetTestMarkLazyDirty(checks, prefix+".eventCards", &provider.eventCards)
	resetTestMarkLazyDirty(checks, prefix+".deckBonus", &provider.deckBonus)
	resetTestMarkLazyDirty(checks, prefix+".worldBloom", &provider.worldBloom)
	resetTestMarkLazyDirty(checks, prefix+".worldBloomChapterRankingRewardRanges", &provider.worldBloomChapterRankingRewardRanges)
}

func resetTestMarkLocalMusicCaches(checks *[]resetTestLazyCheck, prefix string, provider *localMusicProvider) {
	resetTestMarkLazyDirty(checks, prefix+".musics", &provider.musics)
	resetTestMarkLazyDirty(checks, prefix+".diffs", &provider.diffs)
	resetTestMarkLazyDirty(checks, prefix+".vocals", &provider.vocals)
	resetTestMarkLazyDirty(checks, prefix+".tags", &provider.tags)
	resetTestMarkLazyDirty(checks, prefix+".outside", &provider.outside)
	resetTestMarkLazyDirty(checks, prefix+".eventMusics", &provider.eventMusics)
	resetTestMarkLazyDirty(checks, prefix+".limited", &provider.limited)
}

func TestLocalProviderResetMasterdataCacheClearsEveryCache(t *testing.T) {
	provider := NewLocalProvider(t.TempDir(), renderregion.Unknown)
	provider.store.cache["cards.json"] = []byte("cached")
	originalStoreCache := provider.store.cache

	checks := make([]resetTestLazyCheck, 0, 50)
	resetTestMarkLazyDirty(&checks, "characters.chars", &provider.characters.chars)
	resetTestMarkLazyDirty(&checks, "characters.units", &provider.characters.units)
	resetTestMarkLazyDirty(&checks, "skills.skills", &provider.skills.skills)
	resetTestMarkLocalCardCaches(&checks, "cards", provider.cards)
	resetTestMarkLocalEventCaches(&checks, "events", provider.events)
	resetTestMarkLocalMusicCaches(&checks, "musics", provider.musics)
	resetTestMarkLazyDirty(&checks, "gachas.gachas", &provider.gachas.gachas)
	resetTestMarkLazyDirty(&checks, "gachas.cards", &provider.gachas.cards)
	resetTestMarkLazyDirty(&checks, "gachas.ceils", &provider.gachas.ceils)
	resetTestMarkLazyDirty(&checks, "costumes.costumes", &provider.costumes.costumes)
	resetTestMarkLazyDirty(&checks, "honors.honors", &provider.honors.honors)
	resetTestMarkLazyDirty(&checks, "honors.groups", &provider.honors.groups)
	resetTestMarkLazyDirty(&checks, "honors.bondsHonors", &provider.honors.bondsHonors)
	resetTestMarkLazyDirty(&checks, "honors.bondsHonorWords", &provider.honors.bondsHonorWords)
	resetTestMarkLazyDirty(&checks, "honors.gcu", &provider.honors.gcu)
	resetTestMarkLazyDirty(&checks, "honors.birthday", &provider.honors.birthday)
	resetTestMarkLazyDirty(&checks, "honors.eventHonors", &provider.honors.eventHonors)
	resetTestMarkLazyDirty(&checks, "stamps.stamps", &provider.stamps.stamps)
	resetTestMarkLazyDirty(&checks, "vlives.lives", &provider.vlives.lives)
	resetTestMarkLazyDirty(&checks, "education.boxes", &provider.education.boxes)
	resetTestMarkLazyDirty(&checks, "education.rewards", &provider.education.rewards)
	resetTestMarkLazyDirty(&checks, "education.areas", &provider.education.areas)
	resetTestMarkLazyDirty(&checks, "education.ranks", &provider.education.ranks)
	resetTestMarkLazyDirty(&checks, "education.bonds", &provider.education.bonds)
	resetTestMarkLazyDirty(&checks, "education.styles", &provider.education.styles)
	resetTestMarkLazyDirty(&checks, "education.missions", &provider.education.missions)
	resetTestMarkLazyDirty(&checks, "education.gates", &provider.education.gates)
	resetTestMarkLazyDirty(&checks, "education.shops", &provider.education.shops)
	resetTestMarkLazyDirty(&checks, "playerFrames.frames", &provider.playerFrames.frames)
	resetTestMarkLazyDirty(&checks, "playerFrames.groups", &provider.playerFrames.groups)

	provider.ResetMasterdataCache()

	resetTestRequireStoreReset(t, "provider", provider.store)
	if len(originalStoreCache) != 1 {
		t.Fatalf("ResetCache mutated the old map instead of replacing it: %#v", originalStoreCache)
	}
	resetTestRequireLazyCachesReset(t, checks)

	// A reset is deliberately idempotent so reload hooks can safely fan out.
	provider.ResetMasterdataCache()
	resetTestRequireStoreReset(t, "provider after second reset", provider.store)
	resetTestRequireLazyCachesReset(t, checks)
}

func TestDatabaseProviderResetMasterdataCacheClearsAllCachesAndFallbacks(t *testing.T) {
	dirtyAt := time.Now()

	cards := &dbCardProvider{}
	cards.init()
	cards.cardCache[1] = &masterdata.Card{ID: 1}
	cards.cardCachedAt[1] = dirtyAt
	cards.episodesByCard[1] = []*masterdata.CardEpisode{{ID: 1}}
	cards.episodesLoaded = true
	cards.episodesLoadedAt = dirtyAt
	cards.supplyByID[1] = "limited"
	cards.worldLink3ByCard[1] = true
	cards.worldLink3Loaded = true
	cards.worldLink3LoadedAt = dirtyAt
	cards.gachaByCard[1] = &masterdata.Gacha{ID: 1}
	cards.gachaCache[1] = &masterdata.Gacha{ID: 1}
	cards.costumeByCard[1] = []*masterdata.Costume3d{{ID: 1}}

	characters := &dbCharacterProvider{}
	characters.init()
	characters.charCache[1] = &masterdata.Character{ID: 1}
	characters.unitCache[1] = &masterdata.GameCharacterUnit{ID: 1}
	characters.colorCache[1] = "#ffffff"

	skills := &dbSkillProvider{}
	skills.init()
	skills.cache[1] = &masterdata.Skill{ID: 1}
	skills.cacheLoadedAt[1] = dirtyAt
	skills.allLoaded = true
	skills.allLoadedAt = dirtyAt

	gachas := &dbGachaProvider{}
	gachas.init()
	gachas.gachaCache[1] = &masterdata.Gacha{ID: 1}
	gachas.gachas = []*masterdata.Gacha{{ID: 1}}
	gachas.cardCache[1] = &masterdata.Card{ID: 1}
	gachas.ceilCache[1] = "ceil"

	stamps := &dbStampProvider{loaded: true, stamps: []masterdata.Stamp{{ID: 1}}}

	frames := &dbPlayerFrameProvider{}
	frames.init()
	frames.frameCache[1] = &masterdata.PlayerFrame{ID: 1}
	frames.groupCache[1] = &masterdata.PlayerFrameGroup{ID: 1}

	musicStore := resetTestDirtyStore("music")
	musicLocal := &localMusicProvider{store: musicStore}
	localChecks := make([]resetTestLazyCheck, 0, 24)
	resetTestMarkLocalMusicCaches(&localChecks, "music fallback", musicLocal)
	musics := &dbMusicProvider{local: musicLocal}
	musics.init()
	musics.musicByID[1] = &masterdata.Music{ID: 1}
	musics.musicList = []*masterdata.Music{{ID: 1}}
	musics.outsideByID[1] = "outside"
	musics.localizedByID[1] = []string{"localized"}
	musics.difficultiesByID[1] = []*masterdata.MusicDifficulty{{ID: 1}}
	musics.limitedByMusic = map[int][]*masterdata.LimitedTimeMusic{1: {{ID: 1}}}
	musics.limitedLoaded = true
	musics.limitedLoadedAt = dirtyAt

	eventStore := resetTestDirtyStore("event")
	eventFallbackCards := &localCardProvider{store: eventStore}
	resetTestMarkLocalCardCaches(&localChecks, "event fallback cards", eventFallbackCards)
	eventLocal := &localEventProvider{store: eventStore, cards: eventFallbackCards}
	resetTestMarkLocalEventCaches(&localChecks, "event fallback", eventLocal)
	events := &dbEventProvider{store: eventStore, local: eventLocal}
	events.init()
	events.eventCache[1] = &masterdata.Event{ID: 1}
	events.cardCache[1] = &masterdata.Card{ID: 1}
	events.unitCache[1] = "idol"
	events.supplyCache[1] = "limited"

	educationStore := resetTestDirtyStore("education")
	education := &dbEducationProvider{store: educationStore}
	education.init()
	education.rewardsByChar[1] = []*ChallengeReward{{}}
	education.rewardsLoaded = true
	education.boxByID[1] = &ResourceBox{}
	education.boxByPurpose["purpose"] = map[int]*ResourceBox{1: {}}
	education.boxesLoaded = true
	education.areaByID[1] = &AreaItem{}
	education.areaLevelsByItem[1] = []*AreaItemLevel{{}}
	education.areaLevelByItem[1] = map[int]*AreaItemLevel{1: {}}
	education.areaMasterLoaded = true
	education.characterLevels = []*CharacterLevel{{}}
	education.characterLevelsLoaded = true
	education.rankByChar[1] = map[int]*CharacterRank{1: {}}
	education.ranksLoaded = true
	education.bonds = []*Bond{{}}
	education.bondLevels = []*BondLevel{{}}
	education.bondsLoaded = true
	education.stylesByGameID[1] = &GameCharacterStyle{}
	education.stylesLoaded = true
	education.characterMissionsByCharacter[1] = []*CharacterMission{{}}
	education.characterMissionGroupsByID[1] = []*CharacterMissionParameterGroup{{}}
	education.leaderRequirements = []LeaderMissionRequirement{{}}
	education.leaderMaxPlayLimit = 10
	education.leaderMissionsLoaded = true
	education.gateByID[1] = map[int]*MysekaiGateLevel{1: {}}
	education.gatesLoaded = true
	education.shopByBoxID[1] = &ShopItem{}
	education.shopItems = []*ShopItem{{}}
	education.shopsLoaded = true

	honorStore := resetTestDirtyStore("honor")
	honors := &dbHonorProvider{store: honorStore}
	honors.init()
	honors.honorCache[1] = &masterdata.Honor{ID: 1}
	honors.honorMissing[2] = struct{}{}
	honors.groupCache[1] = &masterdata.HonorGroup{ID: 1}
	honors.bondsCache[1] = &masterdata.BondsHonor{ID: 1}
	honors.bondsMissing[2] = struct{}{}
	honors.bondsWordCache[1] = &masterdata.BondsHonorWord{ID: 1}
	honors.bondsWordLoaded = true
	honors.gcuCache[1] = &masterdata.GameCharacterUnit{ID: 1}
	honors.birthdayByGroup[1] = honorBirthdayAssets{background: "background"}
	honors.birthdayChars = append(honors.birthdayChars, nil)
	honors.birthdayLoaded = true
	honors.eventByHonorID[1] = 1
	honors.eventByHonorLoaded = true

	mysekaiStore := resetTestDirtyStore("mysekai")
	mysekai := &dbMySekaiProvider{
		local:       &localMySekaiProvider{store: mysekaiStore},
		lists:       map[string][]map[string]any{"list": {{"id": 1}}},
		mapsByID:    map[string]map[int]map[string]any{"map": {1: {"id": 1}}},
		unavailable: map[string]struct{}{"missing": {}},
	}

	provider := &DatabaseProvider{
		cards:        cards,
		characters:   characters,
		skills:       skills,
		gachas:       gachas,
		stamps:       stamps,
		playerFrames: frames,
		musics:       musics,
		events:       events,
		education:    education,
		honors:       honors,
		mysekai:      mysekai,
	}

	provider.ResetMasterdataCache()

	resetTestRequireEmptyMap(t, "cards.cardCache", cards.cardCache)
	resetTestRequireEmptyMap(t, "cards.cardCachedAt", cards.cardCachedAt)
	resetTestRequireEmptyMap(t, "cards.episodesByCard", cards.episodesByCard)
	resetTestRequireEmptyMap(t, "cards.supplyByID", cards.supplyByID)
	resetTestRequireEmptyMap(t, "cards.worldLink3ByCard", cards.worldLink3ByCard)
	resetTestRequireEmptyMap(t, "cards.gachaByCard", cards.gachaByCard)
	resetTestRequireEmptyMap(t, "cards.gachaCache", cards.gachaCache)
	resetTestRequireEmptyMap(t, "cards.costumeByCard", cards.costumeByCard)
	if cards.episodesLoaded || !cards.episodesLoadedAt.IsZero() || cards.worldLink3Loaded || !cards.worldLink3LoadedAt.IsZero() {
		t.Errorf("card bulk-cache state was not reset")
	}

	resetTestRequireEmptyMap(t, "characters.charCache", characters.charCache)
	resetTestRequireEmptyMap(t, "characters.unitCache", characters.unitCache)
	resetTestRequireEmptyMap(t, "characters.colorCache", characters.colorCache)

	resetTestRequireEmptyMap(t, "skills.cache", skills.cache)
	resetTestRequireEmptyMap(t, "skills.cacheLoadedAt", skills.cacheLoadedAt)
	if skills.allLoaded || !skills.allLoadedAt.IsZero() {
		t.Errorf("skill bulk-cache state was not reset")
	}

	resetTestRequireEmptyMap(t, "gachas.gachaCache", gachas.gachaCache)
	resetTestRequireEmptyMap(t, "gachas.cardCache", gachas.cardCache)
	resetTestRequireEmptyMap(t, "gachas.ceilCache", gachas.ceilCache)
	if gachas.gachas != nil {
		t.Errorf("gacha list was not reset: %#v", gachas.gachas)
	}
	if stamps.loaded || stamps.stamps != nil {
		t.Errorf("stamp cache was not reset: loaded=%t stamps=%#v", stamps.loaded, stamps.stamps)
	}
	resetTestRequireEmptyMap(t, "frames.frameCache", frames.frameCache)
	resetTestRequireEmptyMap(t, "frames.groupCache", frames.groupCache)

	resetTestRequireStoreReset(t, "music fallback", musicStore)
	resetTestRequireEmptyMap(t, "musics.musicByID", musics.musicByID)
	resetTestRequireEmptyMap(t, "musics.outsideByID", musics.outsideByID)
	resetTestRequireEmptyMap(t, "musics.localizedByID", musics.localizedByID)
	resetTestRequireEmptyMap(t, "musics.difficultiesByID", musics.difficultiesByID)
	resetTestRequireEmptyMap(t, "musics.limitedByMusic", musics.limitedByMusic)
	if musics.musicList != nil || musics.limitedLoaded || !musics.limitedLoadedAt.IsZero() {
		t.Errorf("music list/bulk-cache state was not reset")
	}

	resetTestRequireStoreReset(t, "event fallback", eventStore)
	resetTestRequireEmptyMap(t, "events.eventCache", events.eventCache)
	resetTestRequireEmptyMap(t, "events.cardCache", events.cardCache)
	resetTestRequireEmptyMap(t, "events.unitCache", events.unitCache)
	resetTestRequireEmptyMap(t, "events.supplyCache", events.supplyCache)
	resetTestRequireLazyCachesReset(t, localChecks)

	resetTestRequireStoreReset(t, "education fallback", educationStore)
	resetTestRequireEmptyMap(t, "education.rewardsByChar", education.rewardsByChar)
	resetTestRequireEmptyMap(t, "education.boxByID", education.boxByID)
	resetTestRequireEmptyMap(t, "education.boxByPurpose", education.boxByPurpose)
	resetTestRequireEmptyMap(t, "education.areaByID", education.areaByID)
	resetTestRequireEmptyMap(t, "education.areaLevelsByItem", education.areaLevelsByItem)
	resetTestRequireEmptyMap(t, "education.areaLevelByItem", education.areaLevelByItem)
	resetTestRequireEmptyMap(t, "education.rankByChar", education.rankByChar)
	resetTestRequireEmptyMap(t, "education.stylesByGameID", education.stylesByGameID)
	resetTestRequireEmptyMap(t, "education.characterMissionsByCharacter", education.characterMissionsByCharacter)
	resetTestRequireEmptyMap(t, "education.characterMissionGroupsByID", education.characterMissionGroupsByID)
	resetTestRequireEmptyMap(t, "education.gateByID", education.gateByID)
	resetTestRequireEmptyMap(t, "education.shopByBoxID", education.shopByBoxID)
	if education.rewardsLoaded || education.boxesLoaded || education.areaMasterLoaded ||
		education.characterLevelsLoaded || education.ranksLoaded || education.bondsLoaded ||
		education.stylesLoaded || education.leaderMissionsLoaded || education.gatesLoaded || education.shopsLoaded {
		t.Errorf("education loaded flags were not reset")
	}
	if education.characterLevels != nil || education.bonds != nil || education.bondLevels != nil ||
		education.leaderRequirements != nil || education.shopItems != nil || education.leaderMaxPlayLimit != 0 {
		t.Errorf("education slice/scalar caches were not reset")
	}

	resetTestRequireStoreReset(t, "honor fallback", honorStore)
	resetTestRequireEmptyMap(t, "honors.honorCache", honors.honorCache)
	resetTestRequireEmptyMap(t, "honors.honorMissing", honors.honorMissing)
	resetTestRequireEmptyMap(t, "honors.groupCache", honors.groupCache)
	resetTestRequireEmptyMap(t, "honors.bondsCache", honors.bondsCache)
	resetTestRequireEmptyMap(t, "honors.bondsMissing", honors.bondsMissing)
	resetTestRequireEmptyMap(t, "honors.bondsWordCache", honors.bondsWordCache)
	resetTestRequireEmptyMap(t, "honors.gcuCache", honors.gcuCache)
	resetTestRequireEmptyMap(t, "honors.birthdayByGroup", honors.birthdayByGroup)
	resetTestRequireEmptyMap(t, "honors.eventByHonorID", honors.eventByHonorID)
	if honors.bondsWordLoaded || honors.birthdayLoaded || honors.eventByHonorLoaded || honors.birthdayChars != nil {
		t.Errorf("honor loaded/slice state was not reset")
	}

	resetTestRequireStoreReset(t, "mysekai fallback", mysekaiStore)
	resetTestRequireEmptyMap(t, "mysekai.lists", mysekai.lists)
	resetTestRequireEmptyMap(t, "mysekai.mapsByID", mysekai.mapsByID)
	resetTestRequireEmptyMap(t, "mysekai.unavailable", mysekai.unavailable)
}

func TestDatabaseFallbackResetWithoutLocalSources(t *testing.T) {
	musics := &dbMusicProvider{}
	musics.resetLocalMasterdataCache()
	resetTestRequireEmptyMap(t, "musics.musicByID", musics.musicByID)

	events := &dbEventProvider{}
	events.resetLocalMasterdataCache()
	resetTestRequireEmptyMap(t, "events.eventCache", events.eventCache)

	education := &dbEducationProvider{}
	education.resetLocalMasterdataCache()
	resetTestRequireEmptyMap(t, "education.boxByID", education.boxByID)

	honors := &dbHonorProvider{}
	honors.resetLocalMasterdataCache()
	resetTestRequireEmptyMap(t, "honors.honorCache", honors.honorCache)

	mysekai := &dbMySekaiProvider{}
	mysekai.resetLocalMasterdataCache()
	resetTestRequireEmptyMap(t, "mysekai.lists", mysekai.lists)
}

func TestMasterdataResetNilReceivers(t *testing.T) {
	tests := []struct {
		name  string
		reset func()
	}{
		{name: "local store", reset: func() { var value *localStore; value.ResetCache() }},
		{name: "local provider", reset: func() { var value *LocalProvider; value.ResetMasterdataCache() }},
		{name: "database provider", reset: func() { var value *DatabaseProvider; value.ResetMasterdataCache() }},
		{name: "database cards", reset: func() { var value *dbCardProvider; value.resetMasterdataCache() }},
		{name: "database characters", reset: func() { var value *dbCharacterProvider; value.resetMasterdataCache() }},
		{name: "database skills", reset: func() { var value *dbSkillProvider; value.resetMasterdataCache() }},
		{name: "database gachas", reset: func() { var value *dbGachaProvider; value.resetMasterdataCache() }},
		{name: "database stamps", reset: func() { var value *dbStampProvider; value.resetMasterdataCache() }},
		{name: "database frames", reset: func() { var value *dbPlayerFrameProvider; value.resetMasterdataCache() }},
		{name: "local characters", reset: func() { var value *localCharacterProvider; value.reset() }},
		{name: "local skills", reset: func() { var value *localSkillProvider; value.reset() }},
		{name: "local cards", reset: func() { var value *localCardProvider; value.reset() }},
		{name: "local events", reset: func() { var value *localEventProvider; value.reset() }},
		{name: "local musics", reset: func() { var value *localMusicProvider; value.reset() }},
		{name: "local gachas", reset: func() { var value *localGachaProvider; value.reset() }},
		{name: "local costumes", reset: func() { var value *localCostumeProvider; value.reset() }},
		{name: "local honors", reset: func() { var value *localHonorProvider; value.reset() }},
		{name: "local stamps", reset: func() { var value *localStampProvider; value.reset() }},
		{name: "local virtual lives", reset: func() { var value *localVLiveProvider; value.reset() }},
		{name: "local education", reset: func() { var value *localEducationProvider; value.reset() }},
		{name: "local player frames", reset: func() { var value *localPlayerFrameProvider; value.reset() }},
		{name: "database musics", reset: func() { var value *dbMusicProvider; value.resetLocalMasterdataCache() }},
		{name: "database events", reset: func() { var value *dbEventProvider; value.resetLocalMasterdataCache() }},
		{name: "database education", reset: func() { var value *dbEducationProvider; value.resetLocalMasterdataCache() }},
		{name: "database honors", reset: func() { var value *dbHonorProvider; value.resetLocalMasterdataCache() }},
		{name: "database mysekai", reset: func() { var value *dbMySekaiProvider; value.resetLocalMasterdataCache() }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.reset()
		})
	}
}
