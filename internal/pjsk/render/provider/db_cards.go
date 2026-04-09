package provider

import (
	"context"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type dbCardProvider struct {
	client     *sekaiDB.Client
	region     renderregion.Value
	characters *dbCharacterProvider
	skills     *dbSkillProvider

	cardMu    sync.RWMutex
	cardCache map[int]*masterdata.Card

	supplyMu   sync.RWMutex
	supplyByID map[int]string

	gachaMu     sync.RWMutex
	gachaByCard map[int]*masterdata.Gacha
	gachaCache  map[int]*masterdata.Gacha

	costumeMu     sync.RWMutex
	costumeByCard map[int][]*masterdata.Costume3d
}

func (p *dbCardProvider) init() {
	if p.cardCache == nil {
		p.cardCache = make(map[int]*masterdata.Card)
	}
	if p.supplyByID == nil {
		p.supplyByID = make(map[int]string)
	}
	if p.gachaByCard == nil {
		p.gachaByCard = make(map[int]*masterdata.Gacha)
	}
	if p.gachaCache == nil {
		p.gachaCache = make(map[int]*masterdata.Gacha)
	}
	if p.costumeByCard == nil {
		p.costumeByCard = make(map[int][]*masterdata.Costume3d)
	}
}

func cardContextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
