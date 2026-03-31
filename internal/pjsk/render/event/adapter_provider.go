package event

import (
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

// ProviderAdapter bridges provider.MasterDataProvider to event.DataSource.
type ProviderAdapter struct {
	p provider.MasterDataProvider
}

func NewProviderAdapter(p provider.MasterDataProvider) *ProviderAdapter {
	return &ProviderAdapter{p: p}
}

func (a *ProviderAdapter) DefaultRegion() renderregion.Value { return a.p.Region() }

func (a *ProviderAdapter) GetEventByID(id int) (*masterdata.Event, error) {
	return a.p.Events().GetByID(id)
}

func (a *ProviderAdapter) GetEventByCardID(cardID int) (*masterdata.Event, error) {
	return a.p.Events().GetByCardID(cardID)
}

func (a *ProviderAdapter) GetEvents() []*masterdata.Event {
	return a.p.Events().GetAll()
}

func (a *ProviderAdapter) GetEventCards(eventID int) ([]*masterdata.Card, error) {
	return a.p.Events().GetCards(eventID)
}

func (a *ProviderAdapter) GetEventBannerCharacterID(eventID int) (int, error) {
	return a.p.Events().GetBannerCharacterID(eventID)
}

func (a *ProviderAdapter) GetEventDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error) {
	return a.p.Events().GetDeckBonuses(eventID)
}

func (a *ProviderAdapter) GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error) {
	return a.p.Characters().GetGameCharacterUnit(id)
}

func (a *ProviderAdapter) GetBanEvents(charID int) []*masterdata.Event {
	return a.p.Events().GetBanEvents(charID)
}

func (a *ProviderAdapter) GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom {
	return a.p.Events().GetWorldBloomChapters(eventID)
}

func (a *ProviderAdapter) GetCharacterByID(id int) (*masterdata.Character, error) {
	return a.p.Characters().GetByID(id)
}
