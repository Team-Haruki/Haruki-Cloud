package handler

import (
	"context"
	"testing"

	json "haruki-cloud/internal/jsonutil"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/provider"
)

func TestMysekaiSimulationCommandsResolveWithoutCurrentEvent(t *testing.T) {
	root := t.TempDir()
	writeCustomProfileJSONFile(t, root+"/events.json", []map[string]any{{"id": 140, "eventType": "world_bloom", "unit": "piapro"}})
	writeCustomProfileJSONFile(t, root+"/worldBlooms.json", []map[string]any{{"id": 1, "eventId": 140, "gameCharacterId": 21, "worldBloomChapterType": "normal", "chapterNo": 1}})
	source := provider.NewLocalProvider(root, renderregion.JP)
	app := &renderapp.App{Provider: source, Providers: map[renderregion.Value]provider.MasterDataProvider{renderregion.JP: source}}
	for _, args := range []string{"me ln", "me mmj", "me vbs", "me ws", "me 25h", "me vs", "wl3 miku", "wl3 终章 miku"} {
		t.Run(args, func(t *testing.T) {
			h := sekaiHandlers{}.MysekaiDeckHandle()
			result, err := h.Handle(&PjskHandlerContext{
				Context: context.Background(), Platform: "qq", UserId: "42", TriggerCmd: "/烤森组卡", ArgText: args + " 纯蓝 满技能 #123",
			})
			if err != nil {
				t.Fatal(err)
			}
			var combined mysekaiDeckCombinedParams
			if err := json.Unmarshal(result.Params, &combined); err != nil {
				t.Fatal(err)
			}
			var query deck.AutoQuery
			body, err := json.Marshal(combined.Deck)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(body, &query); err != nil {
				t.Fatal(err)
			}
			query.RecommendType = "mysekai"
			if !hasExplicitMySekaiEventSelection(query) {
				t.Fatalf("simulation treated as implicit: %+v", query)
			}
			if query.AttrFilter != "cool" || len(query.FixedCards) != 1 || query.FixedCards[0] != 123 {
				t.Fatalf("ordinary options lost: %+v", query)
			}
			if err := resolveDeckEventAndWorldBloomSelection(context.Background(), &query, app, renderregion.JP); err != nil {
				t.Fatal(err)
			}
			if query.EventID != nil {
				t.Fatalf("simulation borrowed real event: %+v", query)
			}
			switch args {
			case "wl3 miku":
				if query.WorldBloomEventTurn == nil || *query.WorldBloomEventTurn != 3 || query.WorldBloomCharacterID == nil || *query.WorldBloomCharacterID != 21 {
					t.Fatalf("chapter lost: %+v", query)
				}
			case "wl3 终章 miku":
				if query.WorldBloomFinaleTurn == nil || *query.WorldBloomFinaleTurn != 3 || !query.MetadataWorldBloomFinale {
					t.Fatalf("finale lost: %+v", query)
				}
			default:
				if query.EventMEUnit == "" {
					t.Fatalf("ME selector lost: %+v", query)
				}
			}
		})
	}
}

func TestMysekaiMemoryEventRejectsMixedSelectors(t *testing.T) {
	for _, args := range []string{"me vbs event220", "me vbs wl3 miku", "me vbs wl3 终章 miku", "me vbs me ln"} {
		h := sekaiHandlers{}.MysekaiDeckHandle()
		_, err := h.Handle(&PjskHandlerContext{Context: context.Background(), TriggerCmd: "/烤森组卡", ArgText: args})
		if err == nil {
			t.Fatalf("accepted mixed selectors: %s", args)
		}
	}
}

func TestMysekaiExistingWorldBloomStillResolvesActualEvent(t *testing.T) {
	turn, character := 1, 21
	query := deck.AutoQuery{RecommendType: "mysekai", WorldBloomEventTurn: &turn, WorldBloomCharacterID: &character}
	if err := resolveDeckEventAndWorldBloomSelection(context.Background(), &query, newDeckEventCoverageApp(t), renderregion.JP); err != nil {
		t.Fatal(err)
	}
	if query.EventID == nil || *query.EventID != 100 || query.WorldBloomEventTurn != nil {
		t.Fatalf("existing event became simulation: %+v", query)
	}
}
