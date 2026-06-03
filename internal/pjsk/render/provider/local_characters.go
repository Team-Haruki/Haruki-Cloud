package provider

import (
	"context"
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ===========================================================================
// localCharacterProvider
// ===========================================================================

type charUnitData struct {
	byID    map[int]*masterdata.GameCharacterUnit
	colorID map[int]string
}

type localCharacterProvider struct {
	store *localStore
	chars lazyValue[map[int]*masterdata.Character]
	units lazyValue[charUnitData]
}

func (p *localCharacterProvider) ensureCharacters() error {
	return p.chars.init(func() (map[int]*masterdata.Character, error) {
		items, err := loadJSON[localGameCharacterJSON](p.store, "gameCharacters.json")
		if err != nil {
			return nil, err
		}
		byID := make(map[int]*masterdata.Character, len(items))
		for _, item := range items {
			byID[item.ID] = &masterdata.Character{
				ID:        item.ID,
				FirstName: item.FirstName,
				GivenName: item.GivenName,
				Unit:      item.Unit,
			}
		}
		return byID, nil
	})
}

func (p *localCharacterProvider) ensureUnits() error {
	return p.units.init(func() (charUnitData, error) {
		items, err := loadJSON[masterdata.GameCharacterUnit](p.store, "gameCharacterUnits.json")
		if err != nil {
			return charUnitData{}, err
		}
		data := charUnitData{
			byID:    make(map[int]*masterdata.GameCharacterUnit, len(items)),
			colorID: make(map[int]string, len(items)),
		}
		for i := range items {
			data.byID[items[i].ID] = &items[i]
			colorCode := strings.TrimSpace(items[i].ColorCode)
			if colorCode == "" {
				continue
			}
			characterID := items[i].GameCharacterID
			if characterID == 0 {
				characterID = items[i].ID
			}
			if _, exists := data.colorID[characterID]; !exists {
				data.colorID[characterID] = colorCode
			}
		}
		return data, nil
	})
}

func (p *localCharacterProvider) GetByID(_ context.Context, id int) (*masterdata.Character, error) {
	if id == 0 {
		return nil, fmt.Errorf("character id is required")
	}
	if err := p.ensureCharacters(); err != nil {
		return nil, err
	}
	ch, ok := p.chars.v()[id]
	if !ok {
		return nil, fmt.Errorf("character %d not found", id)
	}
	return common.CloneCharacter(ch), nil
}

func (p *localCharacterProvider) GetColorCode(_ context.Context, id int) (string, bool) {
	if id == 0 {
		return "", false
	}
	if err := p.ensureUnits(); err != nil {
		return "", false
	}
	v, ok := p.units.v().colorID[id]
	return v, ok && v != ""
}

func (p *localCharacterProvider) GetGameCharacterUnit(_ context.Context, id int) (*masterdata.GameCharacterUnit, error) {
	if id == 0 {
		return nil, fmt.Errorf("game character unit id is required")
	}
	if err := p.ensureUnits(); err != nil {
		return nil, err
	}
	u, ok := p.units.v().byID[id]
	if !ok {
		return nil, fmt.Errorf("game character unit %d not found", id)
	}
	return common.CloneGameCharacterUnit(u), nil
}
