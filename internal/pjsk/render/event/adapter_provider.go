package event

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
)

// ProviderAdapter bridges provider.MasterDataProvider to event.DataSource.
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

func (a *ProviderAdapter) GetEventByID(id int) (*masterdata.Event, error) {
	return a.P.Events().GetByID(id)
}

func (a *ProviderAdapter) GetEventByCardID(cardID int) (*masterdata.Event, error) {
	return a.P.Events().GetByCardID(cardID)
}

func (a *ProviderAdapter) GetEvents() []*masterdata.Event {
	return a.P.Events().GetAll()
}

func (a *ProviderAdapter) GetEventCards(eventID int) ([]*masterdata.Card, error) {
	return a.P.Events().GetCards(eventID)
}

func (a *ProviderAdapter) GetEventBannerCharacterID(eventID int) (int, error) {
	return a.P.Events().GetBannerCharacterID(eventID)
}

func (a *ProviderAdapter) GetEventDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error) {
	return a.P.Events().GetDeckBonuses(eventID)
}

func (a *ProviderAdapter) GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error) {
	return a.P.Characters().GetGameCharacterUnit(id)
}

func (a *ProviderAdapter) GetBanEvents(charID int) []*masterdata.Event {
	return a.P.Events().GetBanEvents(charID)
}

func (a *ProviderAdapter) GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom {
	return a.P.Events().GetWorldBloomChapters(eventID)
}

func (a *ProviderAdapter) GetCharacterByID(id int) (*masterdata.Character, error) {
	return a.P.Characters().GetByID(id)
}
