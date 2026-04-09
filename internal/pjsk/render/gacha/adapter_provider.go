package gacha

import (
	"context"
	"fmt"
	"sort"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
)

// ProviderAdapter bridges provider.MasterDataProvider to gacha.DataSource.
type ProviderAdapter struct {
	provider.ProviderAdapterBase
}

func NewProviderAdapter(p provider.MasterDataProvider) *ProviderAdapter {
	return &ProviderAdapter{ProviderAdapterBase: provider.NewProviderAdapterBase(p)}
}

func (a *ProviderAdapter) WithContext(ctx context.Context) DataSource {
	if a == nil {
		return nil
	}
	return &ProviderAdapter{ProviderAdapterBase: a.CloneWithContext(ctx)}
}

func (a *ProviderAdapter) GetGachaByID(id int) (*masterdata.Gacha, error) {
	return a.P.Gachas().GetByID(id)
}

func (a *ProviderAdapter) GetGachaByEventID(eventID int) (*masterdata.Gacha, error) {
	if eventID == 0 {
		return nil, fmt.Errorf("event id is required")
	}

	cards, err := a.P.Cards().Filter(&provider.CardFilter{EventID: eventID})
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
	return a.P.Cards().GetGachaByCardID(cards[idx].ID)
}

func (a *ProviderAdapter) GetGachas() []*masterdata.Gacha {
	return a.P.Gachas().GetAll()
}

func (a *ProviderAdapter) GetCardByID(id int) (*masterdata.Card, error) {
	return a.P.Cards().GetByID(id)
}
