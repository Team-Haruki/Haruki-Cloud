package provider

import (
	"sync"
	"time"

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

	cardMu       sync.RWMutex
	cardCache    map[int]*masterdata.Card
	cardCachedAt map[int]time.Time

	episodeMu        sync.RWMutex
	episodesByCard   map[int][]*masterdata.CardEpisode
	episodesLoaded   bool
	episodesLoadedAt time.Time
	episodeLoads     singleflight.Group

	supplyMu   sync.RWMutex
	supplyByID map[int]string

	worldLink3Mu       sync.RWMutex
	worldLink3ByCard   map[int]bool
	worldLink3Loaded   bool
	worldLink3LoadedAt time.Time
	worldLink3Loads    singleflight.Group

	gachaMu     sync.RWMutex
	gachaByCard map[int]*masterdata.Gacha
	gachaCache  map[int]*masterdata.Gacha

	costumeMu     sync.RWMutex
	costumeByCard map[int][]*masterdata.Costume3d
}

func (p *dbCardProvider) init() {
	p.once.Do(func() {
		p.cardCache = make(map[int]*masterdata.Card)
		p.cardCachedAt = make(map[int]time.Time)
		p.episodesByCard = make(map[int][]*masterdata.CardEpisode)
		p.supplyByID = make(map[int]string)
		p.worldLink3ByCard = make(map[int]bool)
		p.gachaByCard = make(map[int]*masterdata.Gacha)
		p.gachaCache = make(map[int]*masterdata.Gacha)
		p.costumeByCard = make(map[int][]*masterdata.Costume3d)
	})
}
