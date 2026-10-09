package provider

import (
	"context"
	"errors"
	"fmt"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ErrEventCardsNotFound is returned by EventProvider.GetCards when an event
// has no cards in the master data (an event can be listed before its cards).
var ErrEventCardsNotFound = errors.New("no cards found for event")

func eventCardsNotFound(eventID int) error {
	return fmt.Errorf("%w %d", ErrEventCardsNotFound, eventID)
}

// EventProvider exposes event-related masterdata queries.
type EventProvider interface {
	GetByID(ctx context.Context, id int) (*masterdata.Event, error)
	GetByCardID(ctx context.Context, cardID int) (*masterdata.Event, error)
	GetAll(ctx context.Context) []*masterdata.Event
	GetCards(ctx context.Context, eventID int) ([]*masterdata.Card, error)
	GetRankingHonorRewards(ctx context.Context, eventID int) ([]masterdata.EventRankingHonorReward, error)
	GetBannerCharacterID(ctx context.Context, eventID int) (int, error)
	GetDeckBonuses(ctx context.Context, eventID int) ([]*masterdata.EventDeckBonus, error)
	GetBanEvents(ctx context.Context, charID int) []*masterdata.Event
	GetWorldBloomChapters(ctx context.Context, eventID int) []*masterdata.WorldBloom
	GetWorldBloomChapterRankingRewardRanges(ctx context.Context, eventID, gameCharacterID int) ([]masterdata.WorldBloomChapterRankingRewardRange, error)
}
