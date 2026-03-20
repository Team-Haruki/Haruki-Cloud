package event

import (
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type DataSource interface {
	DefaultRegion() renderregion.Value
	GetEventByID(id int) (*masterdata.Event, error)
	GetEventByCardID(cardID int) (*masterdata.Event, error)
	GetEvents() []*masterdata.Event
	GetEventCards(eventID int) ([]*masterdata.Card, error)
	GetEventBannerCharacterID(eventID int) (int, error)
	GetEventDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error)
	GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error)
	GetBanEvents(charID int) []*masterdata.Event
	GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom
	GetCharacterByID(id int) (*masterdata.Character, error)
}
