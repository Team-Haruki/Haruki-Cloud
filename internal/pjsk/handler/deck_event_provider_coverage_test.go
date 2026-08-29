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
	if err != nil || len(events) != 4 || events[0].GameID != 90 {
		t.Fatalf("queryDeckEvents = %#v, %v", events, err)
	}
	worldBlooms, err := queryDeckWorldBloomEvents(ctx, app, renderregion.JP)
	if err != nil || len(worldBlooms) != 2 {
		t.Fatalf("queryDeckWorldBloomEvents = %#v, %v", worldBlooms, err)
	}
	event, err := queryDeckEventByID(ctx, app, renderregion.JP, 100)
	if err != nil || event.GameID != 100 {
		t.Fatalf("queryDeckEventByID = %#v, %v", event, err)
	}
	if _, err := queryDeckEventByID(ctx, app, renderregion.JP, 999); err == nil {
		t.Fatal("missing local event unexpectedly resolved")
	}
	chapters, err := queryDeckWorldBloomChapters(ctx, app, renderregion.JP, 100)
	if err != nil || len(chapters) != 2 || chapters[0].GameCharacterID != 21 {
		t.Fatalf("queryDeckWorldBloomChapters = %#v, %v", chapters, err)
	}

	turn, err := resolveDeckWorldBloomCharacterTurnForEvent(ctx, app, renderregion.JP, 100, 21)
	if err != nil || turn != 1 {
		t.Fatalf("character turn = %d, %v", turn, err)
	}
	turn, err = resolveDeckWorldBloomUnitTurnForEvent(ctx, app, renderregion.JP, 100, "piapro")
	if err != nil || turn != 1 {
		t.Fatalf("unit turn = %d, %v", turn, err)
	}
	turn, err = resolveDeckWorldBloomUnitTurnForEvent(ctx, app, renderregion.JP, 101, "piapro")
	if err != nil || turn != 2 {
		t.Fatalf("second unit turn = %d, %v", turn, err)
	}

	query := &renderdeck.AutoQuery{WorldBloomCharacterID: drawing.IntPtr(21)}
	if got := resolveDeckWorldBloomEventTurn(ctx, app, renderregion.JP, event, chapters, query); got != 1 {
		t.Fatalf("resolved event turn = %d", got)
	}
	ensureDeckWorldBloomEventTurnMetadata(ctx, app, renderregion.JP, event, chapters, query)
	if query.MetadataWorldBloomEventTurn == nil || *query.MetadataWorldBloomEventTurn != 1 {
		t.Fatalf("event turn metadata = %+v", query)
	}

	selected, err := resolveDeckWorldBloomEventByCharacterTurn(ctx, app, renderregion.JP, 1, 21)
	if err != nil || selected.GameID != 100 {
		t.Fatalf("character WL selection = %#v, %v", selected, err)
	}
	if _, err := resolveDeckWorldBloomEventByCharacterTurn(ctx, app, renderregion.JP, 3, 21); err == nil {
		t.Fatal("future character WL selection unexpectedly succeeded")
	} else {
		var future *deckFutureWorldBloomTurnError
		if !errors.As(err, &future) || future.Available != 2 {
			t.Fatalf("future character error = %v", err)
		}
	}
	selected, err = resolveDeckWorldBloomEventByUnitTurn(ctx, app, renderregion.JP, 2, "piapro")
	if err != nil || selected.GameID != 101 {
		t.Fatalf("unit WL selection = %#v, %v", selected, err)
	}
	if _, err := resolveDeckWorldBloomEventByUnitTurn(ctx, app, renderregion.JP, 3, "piapro"); err == nil {
		t.Fatal("future unit WL selection unexpectedly succeeded")
	}
	finale, err := resolveDeckWorldBloomFinaleEventByTurn(ctx, app, renderregion.JP, 3)
	if err != nil || finale.GameID != 101 {
		t.Fatalf("future finale selection = %#v, %v", finale, err)
	}
	if _, err := resolveDeckWorldBloomFinaleEventByTurn(ctx, app, renderregion.JP, 4); err == nil {
		t.Fatal("unavailable finale unexpectedly succeeded")
	}
}

func TestDeckEventSelectionWithLocalProvider(t *testing.T) {
	ctx := context.Background()
	app := newDeckEventCoverageApp(t)

	current, err := pickDeckAutoEvent(ctx, app, renderregion.JP, "event")
	if err != nil || current == nil || current.GameID != 100 {
		t.Fatalf("current auto event = %#v, %v", current, err)
	}
	current, err = pickCurrentOrNextDeckEvent(ctx, app, renderregion.JP)
	if err != nil || current == nil || current.GameID != 100 {
		t.Fatalf("current-or-next event = %#v, %v", current, err)
	}

	query := &renderdeck.AutoQuery{EventID: drawing.IntPtr(100), RecommendType: "event", WorldBloomCharacterID: drawing.IntPtr(21)}
	if err := resolveDeckEventAndWorldBloomSelection(ctx, query, app, renderregion.JP); err != nil {
		t.Fatalf("resolve explicit WL event = %v", err)
	}
	if query.MetadataWorldBloomEventTurn == nil || *query.MetadataWorldBloomEventTurn != 1 {
		t.Fatalf("resolved WL query = %+v", query)
	}

	query = &renderdeck.AutoQuery{EventID: drawing.IntPtr(100), RecommendType: "event"}
	if err := resolveDeckEventAndWorldBloomSelection(ctx, query, app, renderregion.JP); err != nil {
		t.Fatalf("resolve default WL chapter = %v", err)
	}
	if query.WorldBloomCharacterID == nil || *query.WorldBloomCharacterID != 21 {
		t.Fatalf("default WL query = %+v", query)
	}

	query = &renderdeck.AutoQuery{EventID: drawing.IntPtr(90), RecommendType: "event", WorldBloomCharacterID: drawing.IntPtr(21)}
	if err := resolveDeckEventAndWorldBloomSelection(ctx, query, app, renderregion.JP); err != nil {
		t.Fatalf("resolve regular event = %v", err)
	}
	if query.WorldBloomCharacterID != nil {
		t.Fatalf("regular event retained WL character: %+v", query)
	}

	turn := 1
	query = &renderdeck.AutoQuery{RecommendType: "event", WorldBloomEventTurn: &turn, WorldBloomCharacterID: drawing.IntPtr(21)}
	if err := resolveDeckEventAndWorldBloomSelection(ctx, query, app, renderregion.JP); err != nil {
		t.Fatalf("resolve WL turn = %v", err)
	}
	if query.EventID == nil || *query.EventID != 100 || query.MetadataWorldBloomEventTurn == nil {
		t.Fatalf("resolved WL turn query = %+v", query)
	}

	turn = 3
	query = &renderdeck.AutoQuery{RecommendType: "event", WorldBloomEventTurn: &turn, WorldBloomCharacterID: drawing.IntPtr(21)}
	if err := resolveDeckEventAndWorldBloomSelection(ctx, query, app, renderregion.JP); err != nil || query.EventID != nil {
		t.Fatalf("future simulated WL selection = %+v, %v", query, err)
	}
}
