package provider

import (
	"fmt"
	"sort"

	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ===========================================================================
// localGachaProvider
// ===========================================================================

type gachaIndex struct {
	all  []*masterdata.Gacha
	byID map[int]*masterdata.Gacha
}

type localGachaProvider struct {
	store  *localStore
	gachas lazyValue[gachaIndex]
	cards  lazyValue[map[int]*masterdata.Card]
}

func (p *localGachaProvider) ensureGachas() error {
	return p.gachas.init(func() (gachaIndex, error) {
		items, err := loadJSON[masterdata.Gacha](p.store, "gachas.json")
		if err != nil {
			return gachaIndex{}, err
		}
		idx := gachaIndex{
			byID: make(map[int]*masterdata.Gacha, len(items)),
			all:  make([]*masterdata.Gacha, 0, len(items)),
		}
		for i := range items {
			g := &items[i]
			idx.byID[g.ID] = g
			idx.all = append(idx.all, g)
		}
		sort.Slice(idx.all, func(i, j int) bool {
			if idx.all[i].StartAt == idx.all[j].StartAt {
				return idx.all[i].ID > idx.all[j].ID
			}
			return idx.all[i].StartAt > idx.all[j].StartAt
		})
		return idx, nil
	})
}

func (p *localGachaProvider) ensureCards() error {
	return p.cards.init(func() (map[int]*masterdata.Card, error) {
		items, err := loadJSON[masterdata.Card](p.store, "cards.json")
		if err != nil {
			return nil, err
		}
		byID := make(map[int]*masterdata.Card, len(items))
		for i := range items {
			byID[items[i].ID] = &items[i]
		}
		return byID, nil
	})
}

func (p *localGachaProvider) GetByID(id int) (*masterdata.Gacha, error) {
	if id == 0 {
		return nil, fmt.Errorf("gacha id is required")
	}
	if err := p.ensureGachas(); err != nil {
		return nil, err
	}
	g, ok := p.gachas.v().byID[id]
	if !ok {
		return nil, fmt.Errorf("gacha %d not found", id)
	}
	return common.CloneGacha(g), nil
}

func (p *localGachaProvider) GetAll() []*masterdata.Gacha {
	if err := p.ensureGachas(); err != nil {
		return nil
	}
	return common.CloneGachaList(p.gachas.v().all)
}

func (p *localGachaProvider) GetCardByID(id int) (*masterdata.Card, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid card id")
	}
	if err := p.ensureCards(); err != nil {
		return nil, err
	}
	card, ok := p.cards.v()[id]
	if !ok {
		return nil, fmt.Errorf("card %d not found", id)
	}
	c := *card
	return &c, nil
}
