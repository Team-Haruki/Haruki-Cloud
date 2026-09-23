package provider

import (
	"sync"
	"time"

	sekaiDB "haruki-cloud/database/sekai"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"

	"golang.org/x/sync/singleflight"
)

type dbMusicProvider struct {
	client *sekaiDB.Client
	region renderregion.Value
	events EventProvider
	once   sync.Once
	local  *localMusicProvider

	mu               sync.RWMutex
	musicByID        map[int]*masterdata.Music
	musicList        []*masterdata.Music
	outsideByID      map[int]string
	localizedByID    map[int][]string
	difficultiesByID map[int][]*masterdata.MusicDifficulty

	difficultyMu         sync.RWMutex
	difficultyLoads      singleflight.Group
	difficultyIndex      map[int][]*masterdata.MusicDifficulty
	difficultyLoadedAt   time.Time
	difficultyGeneration uint64

	// limitedtimemusic is a small region-wide table queried once per music by
	// music/rewards (~744 queries per command before the bulk index). Load it
	// whole under the shared bulk-index flight; a missing key doubles as the
	// negative cache for the common "music has no limited window" case.
	limitedMu       sync.RWMutex
	limitedLoads    singleflight.Group
	limitedByMusic  map[int][]*masterdata.LimitedTimeMusic
	limitedLoaded   bool
	limitedLoadedAt time.Time

	// JP 6.8 moved musics.categories into musiccategories. The region table
	// is loaded whole (ordered by game id) and applied to musics whose own
	// categories are empty; regions that still ship musics.categories have
	// an empty table and never need it.
	categoryMu         sync.RWMutex
	categoryLoads      singleflight.Group
	categoriesByMusic  map[int][]string
	categoriesLoaded   bool
	categoriesLoadedAt time.Time
}

func (p *dbMusicProvider) init() {
	p.once.Do(func() {
		p.musicByID = make(map[int]*masterdata.Music)
		p.outsideByID = make(map[int]string)
		p.localizedByID = make(map[int][]string)
		p.difficultiesByID = make(map[int][]*masterdata.MusicDifficulty)
	})
}
