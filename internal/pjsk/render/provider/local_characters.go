package provider

import (
	"fmt"
	"strings"
	"sync"

	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ===========================================================================
// localCharacterProvider
// ===========================================================================

type localCharacterProvider struct {
	store *localStore

	charOnce sync.Once
	charByID map[int]*masterdata.Character
	charErr  error

	unitOnce  sync.Once
	unitByID  map[int]*masterdata.GameCharacterUnit
	colorByID map[int]string
	unitErr   error
}

func (p *localCharacterProvider) ensureCharacters() error {
	p.charOnce.Do(func() {
		items, err := loadJSON[localGameCharacterJSON](p.store, "gameCharacters.json")
		if err != nil {
			p.charErr = err
			return
		}
		p.charByID = make(map[int]*masterdata.Character, len(items))
		for _, item := range items {
			p.charByID[item.ID] = &masterdata.Character{
				ID:        item.ID,
				FirstName: item.FirstName,
				GivenName: item.GivenName,
				Unit:      item.Unit,
			}
		}
	})
	return p.charErr
}

func (p *localCharacterProvider) ensureUnits() error {
	p.unitOnce.Do(func() {
		items, err := loadJSON[masterdata.GameCharacterUnit](p.store, "gameCharacterUnits.json")
		if err != nil {
			p.unitErr = err
			return
		}
		p.unitByID = make(map[int]*masterdata.GameCharacterUnit, len(items))
		p.colorByID = make(map[int]string, len(items))
		for i := range items {
			p.unitByID[items[i].ID] = &items[i]
			p.colorByID[items[i].ID] = strings.TrimSpace(items[i].ColorCode)
		}
	})
	return p.unitErr
}

func (p *localCharacterProvider) GetByID(id int) (*masterdata.Character, error) {
	if id == 0 {
		return nil, fmt.Errorf("character id is required")
	}
	if err := p.ensureCharacters(); err != nil {
		return nil, err
	}
	ch, ok := p.charByID[id]
	if !ok {
		return nil, fmt.Errorf("character %d not found", id)
	}
	return common.CloneCharacter(ch), nil
}

func (p *localCharacterProvider) GetColorCode(id int) (string, bool) {
	if id == 0 {
		return "", false
	}
	if err := p.ensureUnits(); err != nil {
		return "", false
	}
	v, ok := p.colorByID[id]
	return v, ok && v != ""
}

func (p *localCharacterProvider) GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error) {
	if id == 0 {
		return nil, fmt.Errorf("game character unit id is required")
	}
	if err := p.ensureUnits(); err != nil {
		return nil, err
	}
	u, ok := p.unitByID[id]
	if !ok {
		return nil, fmt.Errorf("game character unit %d not found", id)
	}
	return common.CloneGameCharacterUnit(u), nil
}
