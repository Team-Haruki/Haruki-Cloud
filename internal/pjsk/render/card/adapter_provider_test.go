package card

import (
	"context"
	"strings"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
)

type adapterTestCardProvider struct {
	lastFilter   *provider.CardFilter
	filterResult []*masterdata.Card
	filterErr    error
}

func (p *adapterTestCardProvider) GetByID(_ context.Context, id int) (*masterdata.Card, error) {
	return nil, nil
}

func (p *adapterTestCardProvider) GetByCharacterAndSeq(_ context.Context, characterID, seq int) (*masterdata.Card, error) {
	return nil, nil
}

func (p *adapterTestCardProvider) Filter(_ context.Context, filter *provider.CardFilter) ([]*masterdata.Card, error) {
	if filter != nil {
		cp := *filter
		p.lastFilter = &cp
	}
	return p.filterResult, p.filterErr
}

func (p *adapterTestCardProvider) GetSupplyType(_ context.Context, card *masterdata.Card) string {
	return ""
}

func (p *adapterTestCardProvider) GetGachaByCardID(_ context.Context, cardID int) (*masterdata.Gacha, error) {
	return nil, nil
}

func (p *adapterTestCardProvider) GetCostume3dsByCardID(_ context.Context, cardID int) ([]*masterdata.Costume3d, error) {
	return nil, nil
}

func (p *adapterTestCardProvider) GetUnitByCardID(_ context.Context, cardID int) (string, error) {
	return "", nil
}

type adapterTestEventProvider struct {
	banEvents []*masterdata.Event
}

func (p *adapterTestEventProvider) GetByID(_ context.Context, id int) (*masterdata.Event, error) {
	return nil, nil
}

func (p *adapterTestEventProvider) GetByCardID(_ context.Context, cardID int) (*masterdata.Event, error) {
	return nil, nil
}

func (p *adapterTestEventProvider) GetAll(_ context.Context) []*masterdata.Event {
	return nil
}

func (p *adapterTestEventProvider) GetCards(_ context.Context, eventID int) ([]*masterdata.Card, error) {
	return nil, nil
}

func (p *adapterTestEventProvider) GetBannerCharacterID(_ context.Context, eventID int) (int, error) {
	return 0, nil
}

func (p *adapterTestEventProvider) GetDeckBonuses(_ context.Context, eventID int) ([]*masterdata.EventDeckBonus, error) {
	return nil, nil
}

func (p *adapterTestEventProvider) GetBanEvents(_ context.Context, charID int) []*masterdata.Event {
	return p.banEvents
}

func (p *adapterTestEventProvider) GetWorldBloomChapters(_ context.Context, eventID int) []*masterdata.WorldBloom {
	return nil
}

type adapterTestMasterDataProvider struct {
	cards  provider.CardProvider
	events provider.EventProvider
}

func (p *adapterTestMasterDataProvider) Region() renderregion.Value { return renderregion.JP }

func (p *adapterTestMasterDataProvider) Cards() provider.CardProvider { return p.cards }

func (p *adapterTestMasterDataProvider) Characters() provider.CharacterProvider { return nil }

func (p *adapterTestMasterDataProvider) Skills() provider.SkillProvider { return nil }

func (p *adapterTestMasterDataProvider) Events() provider.EventProvider { return p.events }

func (p *adapterTestMasterDataProvider) Musics() provider.MusicProvider { return nil }

func (p *adapterTestMasterDataProvider) Gachas() provider.GachaProvider { return nil }

func (p *adapterTestMasterDataProvider) Honors() provider.HonorProvider { return nil }

func (p *adapterTestMasterDataProvider) Stamps() provider.StampProvider { return nil }

func (p *adapterTestMasterDataProvider) VLives() provider.VLiveProvider { return nil }

func (p *adapterTestMasterDataProvider) Education() provider.EducationProvider { return nil }

func (p *adapterTestMasterDataProvider) PlayerFrames() provider.PlayerFrameProvider { return nil }

func (p *adapterTestMasterDataProvider) MySekai() provider.MySekaiProvider { return nil }

func TestProviderAdapterFilterCardsPassesMainUnitAndResolvesBanEvent(t *testing.T) {
	cardProvider := &adapterTestCardProvider{
		filterResult: []*masterdata.Card{{ID: 1001}},
	}
	events := &adapterTestEventProvider{
		banEvents: []*masterdata.Event{
			{ID: 111},
			{ID: 222},
		},
	}
	adapter := NewProviderAdapter(&adapterTestMasterDataProvider{
		cards:  cardProvider,
		events: events,
	})

	result, err := adapter.FilterCards(&CardQueryInfo{
		MainUnit:    "piapro",
		SupportUnit: "none",
		BanCharID:   5,
		BanSeq:      2,
	})
	if err != nil {
		t.Fatalf("FilterCards() error = %v", err)
	}
	if len(result) != 1 || result[0].ID != 1001 {
		t.Fatalf("unexpected filter result: %+v", result)
	}
	if cardProvider.lastFilter == nil {
		t.Fatal("expected provider filter to be forwarded")
	}
	if cardProvider.lastFilter.MainUnit != "piapro" || cardProvider.lastFilter.SupportUnit != "none" {
		t.Fatalf("unexpected unit filter: %+v", cardProvider.lastFilter)
	}
	if cardProvider.lastFilter.EventID != 222 {
		t.Fatalf("expected ban event to resolve to event 222, got %+v", cardProvider.lastFilter)
	}
}

func TestProviderAdapterFilterCardsRejectsOutOfRangeBanEvent(t *testing.T) {
	cardProvider := &adapterTestCardProvider{}
	adapter := NewProviderAdapter(&adapterTestMasterDataProvider{
		cards: cardProvider,
		events: &adapterTestEventProvider{
			banEvents: []*masterdata.Event{{ID: 111}},
		},
	})

	_, err := adapter.FilterCards(&CardQueryInfo{BanCharID: 5, BanSeq: 2})
	if err == nil {
		t.Fatal("expected out-of-range ban event query to fail")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("unexpected error: %v", err)
	}
	if cardProvider.lastFilter != nil {
		t.Fatalf("provider filter should not be called on invalid ban event: %+v", cardProvider.lastFilter)
	}
}
