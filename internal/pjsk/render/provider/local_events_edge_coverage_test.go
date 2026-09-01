package provider

import (
	"context"
	"testing"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestLocalEventProviderReadEdges(t *testing.T) {
	ctx := context.Background()
	provider, _ := newEventBehaviorFallback(t)
	if _, err := provider.GetByID(ctx, 0); err == nil {
		t.Fatal("zero event id unexpectedly resolved")
	}
	event, err := provider.GetByID(ctx, 99)
	if err != nil || event.ID != 99 {
		t.Fatalf("local event = %+v, %v", event, err)
	}
	if _, err := provider.GetByID(ctx, 404); err == nil {
		t.Fatal("missing local event unexpectedly resolved")
	}
	byCard, err := provider.GetByCardID(ctx, 999)
	if err != nil || byCard.ID != 99 {
		t.Fatalf("event by card = %+v, %v", byCard, err)
	}
	if _, err := provider.GetByCardID(ctx, 404); err == nil {
		t.Fatal("missing event card unexpectedly resolved")
	}
	if all := provider.GetAll(ctx); len(all) != 3 {
		t.Fatalf("local events = %+v", all)
	}
}

func TestLocalEventProviderRelatedDataEdges(t *testing.T) {
	ctx := context.Background()
	provider, _ := newEventBehaviorFallback(t)
	cards, err := provider.GetCards(ctx, 99)
	if err != nil || len(cards) != 1 || cards[0].ID != 999 {
		t.Fatalf("local event cards = %+v, %v", cards, err)
	}
	if _, err := provider.GetCards(ctx, 404); err == nil {
		t.Fatal("missing local event cards unexpectedly resolved")
	}
	rewards, err := provider.GetRankingHonorRewards(ctx, 11)
	if err != nil || len(rewards) != 1 || rewards[0].HonorID != 911 {
		t.Fatalf("local ranking rewards = %+v, %v", rewards, err)
	}
	if _, err := provider.GetRankingHonorRewards(ctx, 404); err == nil {
		t.Fatal("missing reward event unexpectedly resolved")
	}
	if empty, err := provider.GetDeckBonuses(ctx, 404); err != nil || empty != nil {
		t.Fatalf("missing deck bonuses = %+v, %v", empty, err)
	}
	if chapters := provider.GetWorldBloomChapters(ctx, 404); chapters != nil {
		t.Fatalf("missing chapters = %+v", chapters)
	}
}

func TestLocalEventProviderDependencyEdges(t *testing.T) {
	ctx := context.Background()
	provider := &localEventProvider{store: newLocalStore(t.TempDir())}
	if err := provider.ensureDeckBonuses(); err == nil {
		t.Fatal("missing deck bonus data unexpectedly loaded")
	}
	if err := provider.ensureWorldBlooms(); err == nil {
		t.Fatal("missing world bloom data unexpectedly loaded")
	}
	if err := provider.ensureWorldBloomChapterRankingRewardRanges(); err == nil {
		t.Fatal("missing world bloom rewards unexpectedly loaded")
	}
	if _, err := provider.GetByID(ctx, 1); err == nil {
		t.Fatal("missing event data unexpectedly loaded")
	}
	if _, err := provider.GetByCardID(ctx, 1); err == nil {
		t.Fatal("missing event-card data unexpectedly loaded")
	}
	if all := provider.GetAll(ctx); all != nil {
		t.Fatalf("missing event list = %+v", all)
	}
	if _, err := provider.GetRankingHonorRewards(ctx, 1); err == nil {
		t.Fatal("missing ranking reward data unexpectedly loaded")
	}
	if _, err := provider.GetDeckBonuses(ctx, 1); err == nil {
		t.Fatal("missing deck bonus lookup unexpectedly succeeded")
	}
}

func TestLocalEventProviderBannerAndUnitGuards(t *testing.T) {
	ctx := context.Background()
	provider, _ := newEventBehaviorFallback(t)
	if banner, err := provider.GetBannerCharacterID(ctx, 99); err != nil || banner != 9 {
		t.Fatalf("banner character = %d, %v", banner, err)
	}
	if provider.isBoxEvent(ctx, 99) {
		t.Fatal("provider without character dependency unexpectedly reported box event")
	}
	if unit, ok := (*localEventProvider)(nil).getBonusUnit(ctx, 1); ok || unit != "" {
		t.Fatalf("nil provider bonus unit = %q, %v", unit, ok)
	}
	provider.cards = &localCardProvider{}
	if unit, ok := provider.getBonusUnit(ctx, 1); ok || unit != "" {
		t.Fatalf("missing character provider bonus unit = %q, %v", unit, ok)
	}
}

func TestLocalEventWorldBloomRewardLoadingEdges(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "worldBlooms.json", `[
		{"id":2,"eventId":1,"gameCharacterId":2,"worldBloomChapterType":"character","chapterNo":2,"chapterStartAt":20},
		{"id":1,"eventId":1,"gameCharacterId":1,"worldBloomChapterType":"character","chapterNo":1,"chapterStartAt":10}
	]`)
	writeTestFile(t, root, "worldBloomChapterRankingRewardRanges.json", `[
		{"id":1,"eventId":0,"gameCharacterId":1,"fromRank":1,"toRank":10,"resourceBoxId":1},
		{"id":2,"eventId":1,"gameCharacterId":1,"fromRank":5,"toRank":10,"resourceBoxId":2},
		{"id":3,"eventId":1,"gameCharacterId":1,"fromRank":1,"toRank":10,"resourceBoxId":3},
		{"id":4,"eventId":1,"gameCharacterId":1,"fromRank":1,"toRank":5,"resourceBoxId":4}
	]`)
	provider := &localEventProvider{store: newLocalStore(root)}
	chapters := provider.GetWorldBloomChapters(context.Background(), 1)
	if len(chapters) != 2 || chapters[0].ID != 1 {
		t.Fatalf("sorted world bloom chapters = %+v", chapters)
	}
	ranges, err := provider.GetWorldBloomChapterRankingRewardRanges(context.Background(), 1, 1)
	if err != nil || len(ranges) != 3 {
		t.Fatalf("world bloom reward ranges = %+v, %v", ranges, err)
	}
	if ranges[0].ToRank != 5 || ranges[1].FromRank != 1 || ranges[2].FromRank != 5 {
		t.Fatalf("sorted world bloom reward ranges = %+v", ranges)
	}
	provider.worldBloomChapterRankingRewardRanges.val[worldBloomChapterRankingRewardKey{eventID: 2, gameCharacterID: 2}] = []masterdata.WorldBloomChapterRankingRewardRange{}
	if missing, err := provider.GetWorldBloomChapterRankingRewardRanges(context.Background(), 2, 2); err != nil || missing != nil {
		t.Fatalf("empty world bloom ranges = %+v, %v", missing, err)
	}
}
