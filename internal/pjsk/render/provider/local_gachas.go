package provider

import (
	"fmt"
	"sort"
	"sync"

	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ===========================================================================
// localGachaProvider
// ===========================================================================

type localGachaProvider struct {
	store *localStore

	gachaOnce sync.Once
	gachaAll  []*masterdata.Gacha
	gachaByID map[int]*masterdata.Gacha
	gachaErr  error

	cardOnce sync.Once
	cardByID map[int]*masterdata.Card
	cardErr  error
}

func (p *localGachaProvider) ensureGachas() error {
	p.gachaOnce.Do(func() {
		items, err := loadJSON[masterdata.Gacha](p.store, "gachas.json")
		if err != nil {
			p.gachaErr = err
			return
		}
		p.gachaByID = make(map[int]*masterdata.Gacha, len(items))
		p.gachaAll = make([]*masterdata.Gacha, 0, len(items))
		for i := range items {
			g := &items[i]
			p.gachaByID[g.ID] = g
			p.gachaAll = append(p.gachaAll, g)
		}
		sort.Slice(p.gachaAll, func(i, j int) bool {
			if p.gachaAll[i].StartAt == p.gachaAll[j].StartAt {
				return p.gachaAll[i].ID > p.gachaAll[j].ID
			}
			return p.gachaAll[i].StartAt > p.gachaAll[j].StartAt
		})
	})
	return p.gachaErr
}

func (p *localGachaProvider) ensureCards() error {
	p.cardOnce.Do(func() {
		items, err := loadJSON[masterdata.Card](p.store, "cards.json")
		if err != nil {
			p.cardErr = err
			return
		}
		p.cardByID = make(map[int]*masterdata.Card, len(items))
		for i := range items {
			p.cardByID[items[i].ID] = &items[i]
		}
	})
	return p.cardErr
}

func (p *localGachaProvider) GetByID(id int) (*masterdata.Gacha, error) {
	if id == 0 {
		return nil, fmt.Errorf("gacha id is required")
	}
	if err := p.ensureGachas(); err != nil {
		return nil, err
	}
	g, ok := p.gachaByID[id]
	if !ok {
		return nil, fmt.Errorf("gacha %d not found", id)
	}
	return common.CloneGacha(g), nil
}

func (p *localGachaProvider) GetAll() []*masterdata.Gacha {
	if err := p.ensureGachas(); err != nil {
		return nil
	}
	return common.CloneGachaList(p.gachaAll)
}

func (p *localGachaProvider) GetCardByID(id int) (*masterdata.Card, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid card id")
	}
	if err := p.ensureCards(); err != nil {
		return nil, err
	}
	card, ok := p.cardByID[id]
	if !ok {
		return nil, fmt.Errorf("card %d not found", id)
	}
	c := *card
	return &c, nil
}
