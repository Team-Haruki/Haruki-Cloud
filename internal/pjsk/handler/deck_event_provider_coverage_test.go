package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/provider"
	"haruki-cloud/internal/testutil"
)

func newDeckEventCoverageApp(t *testing.T) *renderapp.App {
	t.Helper()
	root := t.TempDir()
	now := time.Now().UnixMilli()
	writeCustomProfileJSONFile(t, root+"/events.json", []map[string]any{
		{"id": 90, "eventType": "marathon", "unit": "idol", "name": "old", "startAt": now - 30_000, "aggregateAt": now - 20_000, "closedAt": now - 10_000},
		{"id": 100, "eventType": "world_bloom", "unit": "", "name": "current-wl", "startAt": now - 5_000, "aggregateAt": now + 50_000, "closedAt": now + 60_000},
		{"id": 101, "eventType": "world_bloom", "unit": "piapro", "name": "future-wl", "startAt": now + 100_000, "aggregateAt": now + 150_000, "closedAt": now + 160_000},
		{"id": 102, "eventType": "marathon", "unit": "idol", "name": "future-regular", "startAt": now + 200_000, "aggregateAt": now + 250_000, "closedAt": now + 260_000},
	})
	writeCustomProfileJSONFile(t, root+"/worldBlooms.json", []map[string]any{
		{"id": 1, "eventId": 100, "gameCharacterId": 21, "worldBloomChapterType": "normal", "chapterNo": 1, "chapterStartAt": now - 5_000, "aggregateAt": now + 40_000},
		{"id": 2, "eventId": 100, "gameCharacterId": 5, "worldBloomChapterType": "normal", "chapterNo": 2, "chapterStartAt": now + 40_001, "aggregateAt": now + 49_000},
		{"id": 3, "eventId": 101, "gameCharacterId": 21, "worldBloomChapterType": "finale", "chapterNo": 1, "chapterStartAt": now + 100_000, "aggregateAt": now + 140_000},
	})
	src := provider.NewLocalProvider(root, renderregion.JP)
	return &renderapp.App{
		Provider: src,
		Providers: map[renderregion.Value]provider.MasterDataProvider{
			renderregion.JP: src,
		},
	}
}

func TestDeckEventLocalProviderResolution(t *testing.T) {
	ctx := context.Background()
	app := newDeckEventCoverageApp(t)

	events, err := queryDeckEvents(ctx, app, renderregion.JP)
	{
		testutil.Require(t, !(err != nil), "queryDeckEvents = %#v, %v", events, err)
		testutil.Require(t, !(len(events) != 4), "queryDeckEvents = %#v, %v", events, err)
		testutil.Require(t, !(events[0].GameID != 90), "queryDeckEvents = %#v, %v", events, err)
	}

	worldBlooms, err := queryDeckWorldBloomEvents(ctx, app, renderregion.JP)
	{
		testutil.Require(t, !(err != nil), "queryDeckWorldBloomEvents = %#v, %v", worldBlooms, err)
		testutil.Require(t, !(len(worldBlooms) != 2), "queryDeckWorldBloomEvents = %#v, %v", worldBlooms, err)
	}

	event, err := queryDeckEventByID(ctx, app, renderregion.JP, 100)
	{
		testutil.Require(t, !(err != nil), "queryDeckEventByID = %#v, %v", event, err)
		testutil.Require(t, !(event.GameID != 100), "queryDeckEventByID = %#v, %v", event, err)
	}
	{

		_, err := queryDeckEventByID(ctx, app, renderregion.JP, 999)
		testutil.RequireArgs(t, !(err == nil), "missing local event unexpectedly resolved")
	}

	chapters, err := queryDeckWorldBloomChapters(ctx, app, renderregion.JP, 100)
	{
		testutil.Require(t, !(err != nil), "queryDeckWorldBloomChapters = %#v, %v", chapters, err)
		testutil.Require(t, !(len(chapters) != 2), "queryDeckWorldBloomChapters = %#v, %v", chapters, err)
		testutil.Require(t, !(chapters[0].GameCharacterID != 21), "queryDeckWorldBloomChapters = %#v, %v", chapters, err)
	}

	turn, err := resolveDeckWorldBloomCharacterTurnForEvent(ctx, app, renderregion.JP, 100, 21)
	{
		testutil.Require(t, !(err != nil), "character turn = %d, %v", turn, err)
		testutil.Require(t, !(turn != 1), "character turn = %d, %v", turn, err)
	}

	turn, err = resolveDeckWorldBloomUnitTurnForEvent(ctx, app, renderregion.JP, 100, "piapro")
	{
		testutil.Require(t, !(err != nil), "unit turn = %d, %v", turn, err)
		testutil.Require(t, !(turn != 1), "unit turn = %d, %v", turn, err)
	}

	turn, err = resolveDeckWorldBloomUnitTurnForEvent(ctx, app, renderregion.JP, 101, "piapro")
	{
		testutil.Require(t, !(err != nil), "second unit turn = %d, %v", turn, err)
		testutil.Require(t, !(turn != 2), "second unit turn = %d, %v", turn, err)
	}

	query := &renderdeck.AutoQuery{WorldBloomCharacterID: drawing.IntPtr(21)}
	{
		got := resolveDeckWorldBloomEventTurn(ctx, app, renderregion.JP, event, chapters, query)
		testutil.Require(t, !(got != 1), "resolved event turn = %d", got)
	}

	ensureDeckWorldBloomEventTurnMetadata(ctx, app, renderregion.JP, event, chapters, query)
	{
		testutil.Require(t, !(query.MetadataWorldBloomEventTurn == nil), "event turn metadata = %+v", query)
		testutil.Require(t, !(*query.MetadataWorldBloomEventTurn != 1), "event turn metadata = %+v", query)
	}

	selected, err := resolveDeckWorldBloomEventByCharacterTurn(ctx, app, renderregion.JP, 1, 21)
	{
		testutil.Require(t, !(err != nil), "character WL selection = %#v, %v", selected, err)
		testutil.Require(t, !(selected.GameID != 100), "character WL selection = %#v, %v", selected, err)
	}

	if _, err := resolveDeckWorldBloomEventByCharacterTurn(ctx, app, renderregion.JP, 3, 21); err == nil {
		t.Fatal("future character WL selection unexpectedly succeeded")
	} else {
		var future *deckFutureWorldBloomTurnError
		{
			testutil.Require(t, errors.As(err, &future), "future character error = %v", err)
			testutil.Require(t, !(future.Available != 2), "future character error = %v", err)
		}

	}
	selected, err = resolveDeckWorldBloomEventByUnitTurn(ctx, app, renderregion.JP, 2, "piapro")
	{
		testutil.Require(t, !(err != nil), "unit WL selection = %#v, %v", selected, err)
		testutil.Require(t, !(selected.GameID != 101), "unit WL selection = %#v, %v", selected, err)
	}
	{

		_, err := resolveDeckWorldBloomEventByUnitTurn(ctx, app, renderregion.JP, 3, "piapro")
		testutil.RequireArgs(t, !(err == nil), "future unit WL selection unexpectedly succeeded")
	}

	finale, err := resolveDeckWorldBloomFinaleEventByTurn(ctx, app, renderregion.JP, 3)
	{
		testutil.Require(t, !(err != nil), "future finale selection = %#v, %v", finale, err)
		testutil.Require(t, !(finale.GameID != 101), "future finale selection = %#v, %v", finale, err)
	}
	{

		_, err := resolveDeckWorldBloomFinaleEventByTurn(ctx, app, renderregion.JP, 4)
		testutil.RequireArgs(t, !(err == nil), "unavailable finale unexpectedly succeeded")
	}

}

func TestDeckEventSelectionWithLocalProvider(t *testing.T) {
	ctx := context.Background()
	app := newDeckEventCoverageApp(t)

	current, err := pickDeckAutoEvent(ctx, app, renderregion.JP, "event")
	{
		testutil.Require(t, !(err != nil), "current auto event = %#v, %v", current, err)
		testutil.Require(t, !(current == nil), "current auto event = %#v, %v", current, err)
		testutil.Require(t, !(current.GameID != 100), "current auto event = %#v, %v", current, err)
	}

	current, err = pickCurrentOrNextDeckEvent(ctx, app, renderregion.JP)
	{
		testutil.Require(t, !(err != nil), "current-or-next event = %#v, %v", current, err)
		testutil.Require(t, !(current == nil), "current-or-next event = %#v, %v", current, err)
		testutil.Require(t, !(current.GameID != 100), "current-or-next event = %#v, %v", current, err)
	}

	query := &renderdeck.AutoQuery{EventID: drawing.IntPtr(100), RecommendType: "event", WorldBloomCharacterID: drawing.IntPtr(21)}
	{
		err := resolveDeckEventAndWorldBloomSelection(ctx, query, app, renderregion.JP)
		testutil.Require(t, !(err != nil), "resolve explicit WL event = %v", err)
	}
	{

		testutil.Require(t, !(query.MetadataWorldBloomEventTurn == nil), "resolved WL query = %+v", query)
		testutil.Require(t, !(*query.MetadataWorldBloomEventTurn != 1), "resolved WL query = %+v", query)
	}

	query = &renderdeck.AutoQuery{EventID: drawing.IntPtr(100), RecommendType: "event"}
	{
		err := resolveDeckEventAndWorldBloomSelection(ctx, query, app, renderregion.JP)
		testutil.Require(t, !(err != nil), "resolve default WL chapter = %v", err)
	}
	{

		testutil.Require(t, !(query.WorldBloomCharacterID == nil), "default WL query = %+v", query)
		testutil.Require(t, !(*query.WorldBloomCharacterID != 21), "default WL query = %+v", query)
	}

	query = &renderdeck.AutoQuery{EventID: drawing.IntPtr(90), RecommendType: "event", WorldBloomCharacterID: drawing.IntPtr(21)}
	{
		err := resolveDeckEventAndWorldBloomSelection(ctx, query, app, renderregion.JP)
		testutil.Require(t, !(err != nil), "resolve regular event = %v", err)
	}

	testutil.Require(t, !(query.WorldBloomCharacterID != nil), "regular event retained WL character: %+v", query)

	turn := 1
	query = &renderdeck.AutoQuery{RecommendType: "event", WorldBloomEventTurn: &turn, WorldBloomCharacterID: drawing.IntPtr(21)}
	{
		err := resolveDeckEventAndWorldBloomSelection(ctx, query, app, renderregion.JP)
		testutil.Require(t, !(err != nil), "resolve WL turn = %v", err)
	}
	{

		testutil.Require(t, !(query.EventID == nil), "resolved WL turn query = %+v", query)
		testutil.Require(t, !(*query.EventID != 100), "resolved WL turn query = %+v", query)
		testutil.Require(t, !(query.MetadataWorldBloomEventTurn == nil), "resolved WL turn query = %+v", query)
	}

	turn = 3
	query = &renderdeck.AutoQuery{RecommendType: "event", WorldBloomEventTurn: &turn, WorldBloomCharacterID: drawing.IntPtr(21)}
	{
		err := resolveDeckEventAndWorldBloomSelection(ctx, query, app, renderregion.JP)
		{
			testutil.Require(t, !(err != nil), "future simulated WL selection = %+v, %v", query, err)
			testutil.Require(t, !(query.EventID != nil), "future simulated WL selection = %+v, %v", query, err)
		}
	}

}
