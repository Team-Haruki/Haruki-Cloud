package provider

import (
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"

	"golang.org/x/sync/singleflight"
)

type dbCardProvider struct {
	client     *sekaiDB.Client
	region     renderregion.Value
	characters *dbCharacterProvider
	skills     *dbSkillProvider
	once       sync.Once

	// allCards is the region's card index in release order; every lookup and
	// filter is answered from it once loaded. cardCache holds the same
	// models by id plus single cards fetched before the index was loaded.
	cardMu         sync.RWMutex
	allCards       []*masterdata.Card
	cardGeneration uint64
	cardLoads      singleflight.Group
	cardCache      map[int]*masterdata.Card

	episodeMu      sync.RWMutex
	episodesByCard map[int][]*masterdata.CardEpisode
	episodesLoaded bool
	episodeLoads   singleflight.Group

	// supplies maps card_supply_id to its normalised supply type and
	// eventCards maps event_id to the ids of its event cards; both are the
	// small region tables the card filters join against.
	supplies   dbMasterIndex[map[int]string]
	eventCards dbMasterIndex[map[int][]int]

	worldLink3Mu     sync.RWMutex
	worldLink3ByCard map[int]bool
	worldLink3Loaded bool
	worldLink3Loads  singleflight.Group

	gachaMu     sync.RWMutex
	gachaByCard map[int]*masterdata.Gacha
	gachaCache  map[int]*masterdata.Gacha

	costumeMu     sync.RWMutex
	costumeByCard map[int][]*masterdata.Costume3d
}

func (p *dbCardProvider) init() {
	p.once.Do(func() {
		p.cardCache = make(map[int]*masterdata.Card)
		p.episodesByCard = make(map[int][]*masterdata.CardEpisode)
		p.worldLink3ByCard = make(map[int]bool)
		p.gachaByCard = make(map[int]*masterdata.Gacha)
		p.gachaCache = make(map[int]*masterdata.Gacha)
		p.costumeByCard = make(map[int][]*masterdata.Costume3d)
	})
}
