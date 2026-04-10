package provider

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

// EventProvider exposes event-related masterdata queries.
type EventProvider interface {
	GetByID(ctx context.Context, id int) (*masterdata.Event, error)
	GetByCardID(ctx context.Context, cardID int) (*masterdata.Event, error)
	GetAll(ctx context.Context) []*masterdata.Event
	GetCards(ctx context.Context, eventID int) ([]*masterdata.Card, error)
	GetBannerCharacterID(ctx context.Context, eventID int) (int, error)
	GetDeckBonuses(ctx context.Context, eventID int) ([]*masterdata.EventDeckBonus, error)
	GetBanEvents(ctx context.Context, charID int) []*masterdata.Event
	GetWorldBloomChapters(ctx context.Context, eventID int) []*masterdata.WorldBloom
}
