package provider

import (
	"context"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type dbMusicProvider struct {
	client *sekaiDB.Client
	region renderregion.Value
	events EventProvider

	mu            sync.RWMutex
	musicByID     map[int]*masterdata.Music
	musicList     []*masterdata.Music
	outsideByID   map[int]string
	localizedByID map[int][]string
}

func (p *dbMusicProvider) init() {
	if p.musicByID == nil {
		p.musicByID = make(map[int]*masterdata.Music)
	}
	if p.outsideByID == nil {
		p.outsideByID = make(map[int]string)
	}
	if p.localizedByID == nil {
		p.localizedByID = make(map[int][]string)
	}
}

func musicContextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
