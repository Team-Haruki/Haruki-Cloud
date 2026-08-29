package provider

import (
	"context"
	"encoding/json"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func setProviderBehaviorLazy[T any](value *lazyValue[T], data T) {
	value.loaded = true
	value.val = data
	value.err = nil
}

func newEventBehaviorFallback(t *testing.T) (*localEventProvider, *localStore) {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, root, "events.json", `[
		{"id":11,"eventType":"marathon","name":"ranking fallback","startAt":1100,"eventRankingRewardRanges":[{"fromRank":1,"toRank":10,"eventRankingRewardDetails":[{"resourceType":"honor","resourceId":911}]}]},
		{"id":99,"eventType":"marathon","name":"local fallback","startAt":900,"eventRankingRewardRanges":[{"fromRank":1,"toRank":1,"eventRankingRewardDetails":[{"resourceType":"honor","resourceId":999}]}]}
	]`)
	writeTestFile(t, root, "worldBloomChapterRankingRewardRanges.json", `[
		{"id":1,"eventId":99,"gameCharacterId":5,"fromRank":1,"toRank":10,"resourceBoxId":500},
		{"id":2,"eventId":77,"gameCharacterId":7,"fromRank":11,"toRank":20,"resourceBoxId":501}
	]`)
	store := newLocalStore(root)

	localCards := &localCardProvider{store: store}
	localCard := &masterdata.Card{ID: 999, CharacterID: 9, CardSupplyID: 1}
	setProviderBehaviorLazy(&localCards.cards, cardIndex{
		all:  []*masterdata.Card{localCard},
		byID: map[int]*masterdata.Card{999: localCard},
	})
	setProviderBehaviorLazy(&localCards.supplies, map[int]string{1: "normal"})

	local := &localEventProvider{store: store, cards: localCards}
	duplicate := &masterdata.Event{ID: 10, EventType: "marathon", Name: "duplicate", StartAt: 1000}
	fallback := &masterdata.Event{ID: 99, EventType: "marathon", Name: "local fallback", StartAt: 900}
	setProviderBehaviorLazy(&local.events, eventIndex{
		all:  []*masterdata.Event{nil, duplicate, fallback},
		byID: map[int]*masterdata.Event{10: duplicate, 99: fallback},
	})
	setProviderBehaviorLazy(&local.eventCards, eventCardIndex{
		eventByCard:  map[int]int{999: 99},
		cardsByEvent: map[int][]int{99: {999}},
	})
	setProviderBehaviorLazy(&local.deckBonus, map[int][]*masterdata.EventDeckBonus{
		99: {{ID: 99, EventID: 99, GameCharacterUnitID: 900}},
	})
	localCharacterID := 9
	setProviderBehaviorLazy(&local.worldBloom, map[int][]*masterdata.WorldBloom{
		99: {{ID: 99, EventID: 99, GameCharacterID: &localCharacterID, ChapterNo: 1}},
	})
	setProviderBehaviorLazy(&local.worldBloomChapterRankingRewardRanges, map[worldBloomChapterRankingRewardKey][]masterdata.WorldBloomChapterRankingRewardRange{
		{eventID: 99, gameCharacterID: 5}: {{ID: 1, EventID: 99, GameCharacterID: 5, FromRank: 1, ToRank: 10, ResourceBoxID: 500}},
	})
	return local, store
}

func TestDBEventProviderQueriesCachesAndCombinesLocalEvents(t *testing.T) {
	ctx := context.Background()
	provider := openProviderBehaviorDB(t, "events_success")
	client := provider.client
	local, store := newEventBehaviorFallback(t)
	provider.events.local = local
	provider.events.store = store

	events := []struct {
		id        int64
		eventType string
		name      string
		startAt   int64
		ranking   json.RawMessage
		region    renderregion.Value
	}{
		{id: 10, eventType: "marathon", name: "box event", startAt: 1000, ranking: json.RawMessage(`[{"fromRank":1,"toRank":100,"eventRankingRewardDetails":[{"resourceType":"honor","resourceId":700},{"resourceType":"jewel","resourceId":1}]}]`), region: renderregion.JP},
		{id: 11, eventType: "other", name: "ordinary event", startAt: 1100, region: renderregion.JP},
		{id: 12, eventType: "marathon", name: "festival-only event", startAt: 1200, region: renderregion.JP},
		{id: 13, eventType: "marathon", name: "missing-card event", startAt: 1300, region: renderregion.JP},
		{id: 14, eventType: "cheerful_carnival", name: "mixed-unit event", startAt: 1400, region: renderregion.JP},
		{id: 15, eventType: "marathon", name: "other region", startAt: 1500, region: renderregion.TW},
	}
	for _, item := range events {
		builder := client.Event.Create().
			SetGameID(item.id).
			SetEventType(item.eventType).
			SetName(item.name).
			SetStartAt(item.startAt).
			SetAggregateAt(item.startAt + 100).
			SetClosedAt(item.startAt + 200).
			SetServerRegion(item.region.String())
		if len(item.ranking) > 0 {
			builder.SetEventRankingRewardRanges(item.ranking)
		}
		if _, err := builder.Save(ctx); err != nil {
			t.Fatalf("create event %d: %v", item.id, err)
		}
	}

	for _, item := range []struct {
		id, characterID, supplyID int64
	}{
		{id: 100, characterID: 5, supplyID: 1},
		{id: 101, characterID: 6, supplyID: 2},
		{id: 102, characterID: 5, supplyID: 1},
	} {
		if _, err := client.Card.Create().
			SetGameID(item.id).
			SetCharacterID(item.characterID).
			SetCardSupplyID(item.supplyID).
			SetCardRarityType("rarity_4").
			SetAttr("cute").
			SetPrefix("card").
			SetAssetbundleName("card_asset").
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create card %d: %v", item.id, err)
		}
	}
	for _, item := range []struct {
		id, cardID, eventID int64
	}{
		{id: 1, cardID: 100, eventID: 10},
		{id: 2, cardID: 101, eventID: 10},
		{id: 3, cardID: 101, eventID: 12},
		{id: 4, cardID: 404, eventID: 13},
		{id: 5, cardID: 102, eventID: 14},
	} {
		if _, err := client.Eventcard.Create().
			SetGameID(item.id).
			SetCardID(item.cardID).
			SetEventID(item.eventID).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create event card %d: %v", item.id, err)
		}
	}
	for _, item := range []struct {
		id       int64
		typeName string
	}{
		{id: 1, typeName: "normal"},
		{id: 2, typeName: "colorful_festival_limited"},
	} {
		if _, err := client.Cardsupplie.Create().
			SetGameID(item.id).
			SetCardSupplyType(item.typeName).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create supply %d: %v", item.id, err)
		}
	}

	for _, item := range []struct {
		id, eventID, gcuID int64
		attr               string
	}{
		{id: 1, eventID: 10, gcuID: 501, attr: "cute"},
		{id: 2, eventID: 14, gcuID: 501, attr: "cute"},
		{id: 3, eventID: 14, gcuID: 502, attr: "cool"},
	} {
		if _, err := client.Eventdeckbonuse.Create().
			SetGameID(item.id).
			SetEventID(item.eventID).
			SetGameCharacterUnitID(item.gcuID).
			SetCardAttr(item.attr).
			SetBonusRate(0.25).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create event bonus %d: %v", item.id, err)
		}
	}
	for _, item := range []struct {
		id, characterID int64
		unit            string
	}{
		{id: 501, characterID: 5, unit: " idol "},
		{id: 502, characterID: 6, unit: "street"},
	} {
		if _, err := client.Gamecharacterunit.Create().
			SetGameID(item.id).
			SetGameCharacterID(item.characterID).
			SetUnit(item.unit).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create game character unit %d: %v", item.id, err)
		}
	}

	for _, item := range []struct {
		id, characterID, chapterNo int64
	}{
		{id: 1, characterID: 0, chapterNo: 1},
		{id: 2, characterID: 5, chapterNo: 2},
	} {
		if _, err := client.Worldbloom.Create().
			SetGameID(item.id).
			SetEventID(10).
			SetGameCharacterID(item.characterID).
			SetChapterNo(item.chapterNo).
			SetChapterStartAt(1000 + item.chapterNo).
			SetAggregateAt(1100 + item.chapterNo).
			SetChapterEndAt(1200 + item.chapterNo).
			SetWorldBloomChapterType("character").
			SetIsSupplemental(item.characterID == 0).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create world bloom %d: %v", item.id, err)
		}
	}

	eventProvider := provider.events
	if _, err := eventProvider.GetByID(ctx, 0); err == nil {
		t.Fatal("GetByID(0) should reject an invalid event ID")
	}
	eventInfo, err := eventProvider.GetByID(ctx, 10)
	if err != nil || eventInfo.Name != "box event" {
		t.Fatalf("GetByID(10) = %+v, %v", eventInfo, err)
	}
	eventInfo.Name = "mutated"
	if cached, err := eventProvider.GetByID(ctx, 10); err != nil || cached.Name != "box event" {
		t.Fatalf("cached event = %+v, %v", cached, err)
	}
	if _, err := eventProvider.GetByID(ctx, 404); err == nil {
		t.Fatal("missing event should return an error after local fallback misses")
	}
	byCard, err := eventProvider.GetByCardID(ctx, 100)
	if err != nil || byCard.ID != 10 {
		t.Fatalf("GetByCardID(100) = %+v, %v", byCard, err)
	}
	if _, err := eventProvider.GetByCardID(ctx, 4040); err == nil {
		t.Fatal("missing event-card relation should return an error")
	}

	all := eventProvider.GetAll(ctx)
	if len(all) != 6 || all[0] == nil || all[0].ID != 99 || all[len(all)-1].ID != 14 {
		t.Fatalf("combined/sorted event list = %+v", all)
	}
	if _, exists := eventProvider.eventCache[99]; !exists {
		t.Fatal("local-only event was not merged into the DB cache")
	}

	cards, err := eventProvider.GetCards(ctx, 10)
	if err != nil || len(cards) != 2 || cards[0].ID != 100 || cards[1].ID != 101 {
		t.Fatalf("GetCards(10) = %+v, %v", cards, err)
	}
	cards[0].CharacterID = -1
	cachedCards, err := eventProvider.GetCards(ctx, 10)
	if err != nil || cachedCards[0].CharacterID != 5 {
		t.Fatalf("cached cards = %+v, %v", cachedCards, err)
	}
	if _, err := eventProvider.GetCards(ctx, 11); err == nil {
		t.Fatal("event without cards should return an error")
	}
	if _, err := eventProvider.GetCards(ctx, 13); err == nil {
		t.Fatal("event relation to missing card should return an error")
	}

	rewards, err := eventProvider.GetRankingHonorRewards(ctx, 10)
	if err != nil || len(rewards) != 1 || rewards[0].HonorID != 700 {
		t.Fatalf("ranking rewards = %+v, %v", rewards, err)
	}
	fallbackRewards, err := eventProvider.GetRankingHonorRewards(ctx, 11)
	if err != nil || len(fallbackRewards) != 1 || fallbackRewards[0].HonorID != 911 {
		t.Fatalf("ranking fallback rewards = %+v, %v", fallbackRewards, err)
	}
	if _, err := eventProvider.GetRankingHonorRewards(ctx, 404); err == nil {
		t.Fatal("missing ranking-reward event should return an error")
	}

	bonuses, err := eventProvider.GetDeckBonuses(ctx, 10)
	if err != nil || len(bonuses) != 1 || bonuses[0].GameCharacterUnitID != 501 {
		t.Fatalf("deck bonuses = %+v, %v", bonuses, err)
	}
	if empty, err := eventProvider.GetDeckBonuses(ctx, 11); err != nil || len(empty) != 0 {
		t.Fatalf("empty deck bonuses = %+v, %v", empty, err)
	}
	banner, err := eventProvider.GetBannerCharacterID(ctx, 10)
	if err != nil || banner != 5 {
		t.Fatalf("banner character = %d, %v", banner, err)
	}
	if _, err := eventProvider.GetBannerCharacterID(ctx, 12); err == nil {
		t.Fatal("festival-only event should not have a banner character")
	}
	banEvents := eventProvider.GetBanEvents(ctx, 5)
	if len(banEvents) != 1 || banEvents[0].ID != 10 {
		t.Fatalf("box events for character 5 = %+v", banEvents)
	}
	if unit, ok := eventProvider.getBonusUnit(ctx, 0); ok || unit != "" {
		t.Fatalf("invalid bonus unit = %q, %t", unit, ok)
	}
	if unit, ok := eventProvider.getBonusUnit(ctx, 501); !ok || unit != "idol" {
		t.Fatalf("bonus unit = %q, %t", unit, ok)
	}
	if unit, ok := eventProvider.getBonusUnit(ctx, 501); !ok || unit != "idol" {
		t.Fatalf("cached bonus unit = %q, %t", unit, ok)
	}
	if _, ok := eventProvider.getBonusUnit(ctx, 999); ok {
		t.Fatal("missing bonus unit should not resolve")
	}

	chapters := eventProvider.GetWorldBloomChapters(ctx, 10)
	if len(chapters) != 2 || chapters[0].GameCharacterID != nil || chapters[1].GameCharacterID == nil || *chapters[1].GameCharacterID != 5 {
		t.Fatalf("world bloom chapters = %+v", chapters)
	}
	if got := eventProvider.GetWorldBloomChapters(ctx, 11); got != nil {
		t.Fatalf("missing world bloom chapters = %+v, want nil", got)
	}
	if ranges, err := eventProvider.GetWorldBloomChapterRankingRewardRanges(ctx, 0, 5); err != nil || ranges != nil {
		t.Fatalf("invalid world bloom ranges = %+v, %v", ranges, err)
	}
	ranges, err := eventProvider.GetWorldBloomChapterRankingRewardRanges(ctx, 99, 5)
	if err != nil || len(ranges) != 1 || ranges[0].ResourceBoxID != 500 {
		t.Fatalf("local world bloom ranges = %+v, %v", ranges, err)
	}
	storeRanges, err := eventProvider.GetWorldBloomChapterRankingRewardRanges(ctx, 77, 7)
	if err != nil || len(storeRanges) != 1 || storeRanges[0].ResourceBoxID != 501 {
		t.Fatalf("store world bloom ranges = %+v, %v", storeRanges, err)
	}
}

func TestDBEventProviderFallsBackWhenDatabaseUnavailable(t *testing.T) {
	ctx := context.Background()
	provider := openProviderBehaviorDB(t, "events_fallback")
	local, store := newEventBehaviorFallback(t)
	provider.events.local = local
	provider.events.store = store
	if err := provider.client.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}

	eventInfo, err := provider.events.GetByID(ctx, 99)
	if err != nil || eventInfo.ID != 99 {
		t.Fatalf("fallback GetByID() = %+v, %v", eventInfo, err)
	}
	byCard, err := provider.events.GetByCardID(ctx, 999)
	if err != nil || byCard.ID != 99 {
		t.Fatalf("fallback GetByCardID() = %+v, %v", byCard, err)
	}
	if all := provider.events.GetAll(ctx); len(all) != 3 {
		t.Fatalf("fallback GetAll() = %+v", all)
	}
	cards, err := provider.events.GetCards(ctx, 99)
	if err != nil || len(cards) != 1 || cards[0].ID != 999 {
		t.Fatalf("fallback GetCards() = %+v, %v", cards, err)
	}
	rewards, err := provider.events.GetRankingHonorRewards(ctx, 99)
	if err != nil || len(rewards) != 1 || rewards[0].HonorID != 999 {
		t.Fatalf("fallback ranking rewards = %+v, %v", rewards, err)
	}
	bonuses, err := provider.events.GetDeckBonuses(ctx, 99)
	if err != nil || len(bonuses) != 1 || bonuses[0].ID != 99 {
		t.Fatalf("fallback deck bonuses = %+v, %v", bonuses, err)
	}
	chapters := provider.events.GetWorldBloomChapters(ctx, 99)
	if len(chapters) != 1 || chapters[0].ID != 99 {
		t.Fatalf("fallback world bloom chapters = %+v", chapters)
	}

	if _, err := provider.events.GetByID(ctx, 404); err == nil {
		t.Fatal("database and local miss should return an error")
	}
	if _, err := provider.events.GetCards(ctx, 404); err == nil {
		t.Fatal("database and local card miss should return an error")
	}
}
