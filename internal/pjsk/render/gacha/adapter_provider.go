package gacha

import (
	"fmt"
	"sort"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

// ProviderAdapter bridges provider.MasterDataProvider to gacha.DataSource.
type ProviderAdapter struct {
	p provider.MasterDataProvider
}

func NewProviderAdapter(p provider.MasterDataProvider) *ProviderAdapter {
	return &ProviderAdapter{p: p}
}

func (a *ProviderAdapter) DefaultRegion() renderregion.Value { return a.p.Region() }

func (a *ProviderAdapter) GetGachaByID(id int) (*masterdata.Gacha, error) {
	return a.p.Gachas().GetByID(id)
}

func (a *ProviderAdapter) GetGachaByEventID(eventID int) (*masterdata.Gacha, error) {
	if eventID == 0 {
		return nil, fmt.Errorf("event id is required")
	}

	cards, err := a.p.Cards().Filter(&provider.CardFilter{EventID: eventID})
	if err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		return nil, fmt.Errorf("gacha not found for event: %d", eventID)
	}

	// Keep lunabot semantics: prefer the third event card to skip fes cards when present.
	sort.Slice(cards, func(i, j int) bool {
		return cards[i].ID < cards[j].ID
	})
	idx := len(cards) - 1
	if idx > 2 {
		idx = 2
	}
	return a.p.Cards().GetGachaByCardID(cards[idx].ID)
}

func (a *ProviderAdapter) GetGachas() []*masterdata.Gacha {
	return a.p.Gachas().GetAll()
}

func (a *ProviderAdapter) GetCardByID(id int) (*masterdata.Card, error) {
	return a.p.Cards().GetByID(id)
}
