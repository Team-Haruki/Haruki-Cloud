package card

import (
	"context"
	json "github.com/bytedance/sonic"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	"haruki-cloud/internal/pjsk/render/snapshot"
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
		p.lastFilter = new(*filter)
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

func (p *adapterTestEventProvider) GetWorldBloomChapterRankingRewardRanges(_ context.Context, eventID, gameCharacterID int) ([]masterdata.WorldBloomChapterRankingRewardRange, error) {
	return nil, nil
}

func (p *adapterTestEventProvider) GetRankingHonorRewards(_ context.Context, eventID int) ([]masterdata.EventRankingHonorReward, error) {
	return nil, nil
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

	result, err := adapter.FilterCards(&PjskCardQueryInfo{
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

	_, err := adapter.FilterCards(&PjskCardQueryInfo{BanCharID: 5, BanSeq: 2})
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

func TestProviderAdapterBuildsMaxProfileMySekaiData(t *testing.T) {
	root := t.TempDir()
	writeAdapterProviderJSON(t, filepath.Join(root, "mysekaiGateLevels.json"), []map[string]any{
		{"id": 1001, "mysekaiGateId": 1, "level": 40, "powerBonusRate": 4.0},
		{"id": 2001, "mysekaiGateId": 2, "level": 35, "powerBonusRate": 3.5},
		{"id": 5001, "mysekaiGateId": 5, "level": 10, "powerBonusRate": 1.0},
	})
	writeAdapterProviderJSON(t, filepath.Join(root, "mysekaiFixtures.json"), []map[string]any{
		{"id": 1, "mysekaiFixtureGameCharacterGroupPerformanceBonusId": 10},
		{"id": 2, "mysekaiFixtureGameCharacterGroupPerformanceBonusId": 20},
		{"id": 3, "mysekaiFixtureGameCharacterGroupPerformanceBonusId": 30},
		{"id": 4, "mysekaiFixtureGameCharacterGroupPerformanceBonusId": 40},
	})
	writeAdapterProviderJSON(t, filepath.Join(root, "mysekaiFixtureGameCharacterGroupPerformanceBonuses.json"), []map[string]any{
		{"id": 10, "mysekaiFixtureGameCharacterGroupId": 100, "bonusRate": 60},
		{"id": 20, "mysekaiFixtureGameCharacterGroupId": 100, "bonusRate": 50},
		{"id": 30, "mysekaiFixtureGameCharacterGroupId": 200, "bonusRate": 6},
		{"id": 40, "mysekaiFixtureGameCharacterGroupId": 200, "bonusRate": 3},
	})
	writeAdapterProviderJSON(t, filepath.Join(root, "mysekaiFixtureGameCharacterGroups.json"), []map[string]any{
		{"id": 100, "groupId": 1, "gameCharacterId": 1},
		{"id": 200, "groupId": 2, "gameCharacterId": 5},
	})

	adapter := NewProviderAdapter(provider.NewLocalProvider(root, renderregion.JP))

	if got := adapter.GetMaxProfileMysekaiGates(); !reflect.DeepEqual(got, []snapshot.RawUserMysekaiGate{
		{MysekaiGateID: 1, MysekaiGateLevel: 40},
		{MysekaiGateID: 2, MysekaiGateLevel: 35},
		{MysekaiGateID: 5, MysekaiGateLevel: 10},
	}) {
		t.Fatalf("unexpected max profile mysekai gates: %+v", got)
	}

	if got := adapter.GetMaxProfileMysekaiFixtureBonuses(); !reflect.DeepEqual(got, []snapshot.RawUserFixtureBonus{
		{GameCharacterID: 1, TotalBonusRate: 100},
		{GameCharacterID: 5, TotalBonusRate: 9},
	}) {
		t.Fatalf("unexpected max profile fixture bonuses: %+v", got)
	}
}

func writeAdapterProviderJSON(t *testing.T, path string, value any) {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
