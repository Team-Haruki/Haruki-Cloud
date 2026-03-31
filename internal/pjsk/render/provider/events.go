package provider

import "haruki-cloud/internal/pjsk/render/masterdata"

// EventProvider exposes event-related masterdata queries.
type EventProvider interface {
	GetByID(id int) (*masterdata.Event, error)
	GetByCardID(cardID int) (*masterdata.Event, error)
	GetAll() []*masterdata.Event
	GetCards(eventID int) ([]*masterdata.Card, error)
	GetBannerCharacterID(eventID int) (int, error)
	GetDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error)
	GetBanEvents(charID int) []*masterdata.Event
	GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom
}
