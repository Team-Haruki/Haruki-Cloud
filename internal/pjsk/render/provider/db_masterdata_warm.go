package provider

import (
	"context"
	"errors"
	"fmt"
)

// WarmMasterdata loads the bulk indexes behind the hot card commands (the
// card index, the character, skill, supply, event-card and world-link
// tables it filters against, and the music difficulty index) so the first
// request after startup or a masterdata reset does not wait for them. The
// steps run one after another on the caller's goroutine; callers bound the
// concurrency across regions. Each failure is reported, and the remaining
// steps still run.
func (p *DatabaseProvider) WarmMasterdata(ctx context.Context) error {
	if p == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	steps := []struct {
		name string
		run  func(context.Context) error
	}{
		{name: "cards", run: func(ctx context.Context) error {
			if p.cards == nil {
				return nil
			}
			p.cards.init()
			_, err := p.cards.cardIndex(ctx)
			return err
		}},
		{name: "characters", run: func(ctx context.Context) error {
			if p.characters == nil {
				return nil
			}
			return p.characters.preloadAll(ctx)
		}},
		{name: "skills", run: func(ctx context.Context) error {
			if p.skills == nil {
				return nil
			}
			return p.skills.ensureAllLoaded(ctx)
		}},
		{name: "card_supplies", run: func(ctx context.Context) error {
			if p.cards == nil {
				return nil
			}
			_, err := p.cards.supplies.get(ctx, "cards.supply_type_index", p.cards.loadSupplyTypes)
			return err
		}},
		{name: "event_cards", run: func(ctx context.Context) error {
			if p.cards == nil {
				return nil
			}
			_, err := p.cards.eventCards.get(ctx, "cards.event_card_index", p.cards.loadEventCardIDs)
			return err
		}},
		{name: "world_link_cards", run: func(ctx context.Context) error {
			if p.cards == nil || p.cards.loadWorldLink3Cards(ctx) {
				return nil
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			return errors.New("world link card index did not load")
		}},
		{name: "music_difficulties", run: func(ctx context.Context) error {
			if p.musics == nil {
				return nil
			}
			p.musics.init()
			return p.musics.PreloadDifficulties(ctx)
		}},
	}
	var errs []error
	for _, step := range steps {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		if err := step.run(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", step.name, err))
		}
	}
	return errors.Join(errs...)
}
