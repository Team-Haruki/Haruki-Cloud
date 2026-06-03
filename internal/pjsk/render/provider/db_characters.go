package provider

import (
	"context"
	"fmt"
	"strings"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/gamecharacter"
	"haruki-cloud/database/sekai/gamecharacterunit"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type dbCharacterProvider struct {
	client *sekaiDB.Client
	region renderregion.Value
	once   sync.Once

	charMu    sync.RWMutex
	charCache map[int]*masterdata.Character

	unitMu    sync.RWMutex
	unitCache map[int]*masterdata.GameCharacterUnit

	colorMu    sync.RWMutex
	colorCache map[int]string
}

func (p *dbCharacterProvider) init() {
	p.once.Do(func() {
		p.charCache = make(map[int]*masterdata.Character)
		p.unitCache = make(map[int]*masterdata.GameCharacterUnit)
		p.colorCache = make(map[int]string)
	})
}

func (p *dbCharacterProvider) GetByID(ctx context.Context, id int) (*masterdata.Character, error) {
	if id == 0 {
		return nil, fmt.Errorf("character id is required")
	}
	p.init()

	p.charMu.RLock()
	if cached, ok := p.charCache[id]; ok {
		p.charMu.RUnlock()
		return new(*cached), nil
	}
	p.charMu.RUnlock()

	entity, err := p.client.Gamecharacter.Query().
		Where(gamecharacter.ServerRegionEQ(p.region.String()), gamecharacter.GameIDEQ(int64(id))).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("query character %d: %w", id, err)
	}
	model := &masterdata.Character{
		ID:        id,
		FirstName: entity.FirstName,
		GivenName: entity.GivenName,
		Unit:      entity.Unit,
	}
	p.charMu.Lock()
	p.charCache[id] = model
	p.charMu.Unlock()
	return new(*model), nil
}

func (p *dbCharacterProvider) GetColorCode(ctx context.Context, id int) (string, bool) {
	if id == 0 {
		return "", false
	}
	p.init()

	p.colorMu.RLock()
	if value, ok := p.colorCache[id]; ok {
		p.colorMu.RUnlock()
		return value, value != ""
	}
	p.colorMu.RUnlock()

	entity, err := p.client.Gamecharacterunit.Query().
		Where(gamecharacterunit.ServerRegionEQ(p.region.String()), gamecharacterunit.GameCharacterIDEQ(int64(id))).
		Order(gamecharacterunit.ByGameID()).
		First(ctx)
	if err != nil {
		return "", false
	}
	value := strings.TrimSpace(entity.ColorCode)
	p.colorMu.Lock()
	p.colorCache[id] = value
	p.colorMu.Unlock()
	return value, value != ""
}

func (p *dbCharacterProvider) GetGameCharacterUnit(ctx context.Context, id int) (*masterdata.GameCharacterUnit, error) {
	if id == 0 {
		return nil, fmt.Errorf("game character unit id is required")
	}
	p.init()

	p.unitMu.RLock()
	if cached, ok := p.unitCache[id]; ok {
		p.unitMu.RUnlock()
		return new(*cached), nil
	}
	p.unitMu.RUnlock()

	entity, err := p.client.Gamecharacterunit.Query().
		Where(gamecharacterunit.ServerRegionEQ(p.region.String()), gamecharacterunit.GameIDEQ(int64(id))).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("query game character unit %d: %w", id, err)
	}
	model := &masterdata.GameCharacterUnit{
		ID:              int(entity.GameID),
		GameCharacterID: int(entity.GameCharacterID),
		Unit:            entity.Unit,
		ColorCode:       entity.ColorCode,
	}
	p.unitMu.Lock()
	p.unitCache[id] = model
	p.unitMu.Unlock()
	return new(*model), nil
}
