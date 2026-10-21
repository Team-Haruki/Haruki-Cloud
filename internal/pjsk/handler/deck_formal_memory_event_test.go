package handler

import (
	"context"
	"testing"
	"time"

	json "haruki-cloud/internal/jsonutil"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderprovider "haruki-cloud/internal/pjsk/render/provider"
)

func TestDefaultEventCommandSelectsOfficialMemoryEvent(t *testing.T) {
	now := time.Now().UnixMilli()
	provider := bridgeDeckTestMasterProvider{
		region: renderregion.JP,
		events: &bridgeDeckTestEventProvider{events: []*masterdata.Event{
			{ID: 220, EventType: "marathon", Name: "Previous event", StartAt: now - 7200000, AggregateAt: now - 3600000},
			{ID: 221, EventType: "marathon", Name: "Vivid BAD SQUAD Memories", StartAt: now - 60000, AggregateAt: now + 3600000},
		}},
	}
	app := &renderapp.App{Provider: provider, Providers: map[renderregion.Value]renderprovider.MasterDataProvider{renderregion.JP: provider}}
	for _, args := range []string{"", "me ln", "me mmj", "me vbs", "me wxs", "me 25h", "me vs"} {
		t.Run(args, func(t *testing.T) {
			handler := sekaiHandlers{}.EventDeckHandle()
			result, err := handler.Handle(&PjskHandlerContext{
				Context: context.Background(), Platform: "qq", UserId: "42", TriggerCmd: "/活动组卡", ArgText: args,
			})
			if err != nil {
				t.Fatal(err)
			}
			recommendType, ok := deckRecommendType(result.Mode)
			if !ok {
				t.Fatalf("unexpected command mode: %s", result.Mode)
			}
			query := deck.AutoQuery{Region: "jp", RecommendType: recommendType}
			if err := json.Unmarshal(result.Params, &query); err != nil {
				t.Fatal(err)
			}
			if err := resolveDeckEventAndWorldBloomSelection(context.Background(), &query, app, renderregion.JP); err != nil {
				t.Fatal(err)
			}
			if args != "" {
				if query.EventID != nil || query.EventMEUnit == "" {
					t.Fatalf("simulation borrowed current event: %+v", query)
				}
				return
			}
			if query.EventID == nil || *query.EventID != 221 || query.RecommendType != "event" {
				t.Fatalf("default did not select official event: %+v", query)
			}
			if query.EventMEUnit != "" || query.EventAttr != "" || query.EventUnit != "" || query.WorldBloomEventTurn != nil || query.WorldBloomCharacterID != nil {
				t.Fatalf("official event acquired simulation selectors: %+v", query)
			}
		})
	}
}
