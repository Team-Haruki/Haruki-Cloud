package event

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
)

func NewProviderAdapter(p provider.MasterDataProvider) *ProviderAdapter {
	return &ProviderAdapter{PjskProviderAdapterBase: provider.NewProviderAdapterBase(p)}
}

func (a *ProviderAdapter) WithContext(ctx context.Context) DataSource {
	if a == nil {
		return nil
	}
	return &ProviderAdapter{PjskProviderAdapterBase: a.CloneWithContext(ctx)}
}

func (a *ProviderAdapter) GetEventByID(id int) (*masterdata.Event, error) {
	return a.P.Events().GetByID(a.Context(), id)
}

func (a *ProviderAdapter) GetEventByCardID(cardID int) (*masterdata.Event, error) {
	return a.P.Events().GetByCardID(a.Context(), cardID)
}

func (a *ProviderAdapter) GetEvents() []*masterdata.Event {
	return a.P.Events().GetAll(a.Context())
}

func (a *ProviderAdapter) GetEventCards(eventID int) ([]*masterdata.Card, error) {
	return a.P.Events().GetCards(a.Context(), eventID)
}

func (a *ProviderAdapter) GetEventBannerCharacterID(eventID int) (int, error) {
	return a.P.Events().GetBannerCharacterID(a.Context(), eventID)
}

func (a *ProviderAdapter) GetEventDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error) {
	return a.P.Events().GetDeckBonuses(a.Context(), eventID)
}

func (a *ProviderAdapter) GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error) {
	return a.P.Characters().GetGameCharacterUnit(a.Context(), id)
}

func (a *ProviderAdapter) GetBanEvents(charID int) []*masterdata.Event {
	return a.P.Events().GetBanEvents(a.Context(), charID)
}

func (a *ProviderAdapter) GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom {
	return a.P.Events().GetWorldBloomChapters(a.Context(), eventID)
}

func (a *ProviderAdapter) GetCharacterByID(id int) (*masterdata.Character, error) {
	return a.P.Characters().GetByID(a.Context(), id)
}
