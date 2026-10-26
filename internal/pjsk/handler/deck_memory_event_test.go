package handler

import (
	"context"
	"strings"
	"testing"

	json "haruki-cloud/internal/jsonutil"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/deck"
)

func TestMemoryEventCommand(t *testing.T) {
	for alias, unit := range map[string]string{
		"ln": "light_sound", "mmj": "idol", "VBS": "street",
		"ws": "theme_park", "wxs": "theme_park", "25h": "school_refusal",
		"25": "school_refusal", "vs": "piapro",
	} {
		t.Run(alias, func(t *testing.T) {
			h := sekaiHandlers{}.EventDeckHandle()
			result, err := h.Handle(&PjskHandlerContext{
				Context: context.Background(), Platform: "qq", UserId: "42",
				TriggerCmd: "/活动组卡", ArgText: "me " + alias + " auto 虾ex 纯蓝 满技能 #123",
			})
			if err != nil {
				t.Fatal(err)
			}
			var query deck.AutoQuery
			if err := json.Unmarshal(result.Params, &query); err != nil {
				t.Fatal(err)
			}
			if query.EventMEUnit != unit || query.EventID != nil || query.EventUnit != "" || query.EventAttr != "" {
				t.Fatalf("unexpected selection: %+v", query)
			}
			if query.LiveType != "auto" || query.AttrFilter != "cool" || len(query.FixedCards) != 1 || query.FixedCards[0] != 123 {
				t.Fatalf("lost ordinary deck options: %+v", query)
			}
			if !strings.Contains(formatDeckQuerySummary(query), "总集篇模拟") {
				t.Fatal("summary does not identify simulation")
			}
			// An empty app must not cause a live-event lookup for this simulation.
			if err := resolveDeckEventAndWorldBloomSelection(context.Background(), &query, &renderapp.App{}, renderregion.JP); err != nil {
				t.Fatal(err)
			}
			if query.EventID != nil {
				t.Fatal("resolved a real event for the simulation")
			}
		})
	}
}

func TestMemoryEventRejectsConflicts(t *testing.T) {
	for _, args := range []string{
		"me vbs me ln", "me vbs event220", "me vbs wl1", "me vbs wl3 终章 miku",
	} {
		t.Run(args, func(t *testing.T) {
			h := sekaiHandlers{}.EventDeckHandle()
			_, err := h.Handle(&PjskHandlerContext{
				Context: context.Background(), Platform: "qq", UserId: "42",
				TriggerCmd: "/活动组卡", ArgText: args,
			})
			if err == nil {
				t.Fatal("expected usage error")
			}
		})
	}

}

func TestMemoryEventBonusCommand(t *testing.T) {
	h := sekaiHandlers{}.BonusDeckHandle()
	for _, args := range []string{"me vbs 350", "me vbs event220 350"} {
		result, err := h.Handle(&PjskHandlerContext{
			Context: context.Background(), Platform: "qq", UserId: "42",
			TriggerCmd: "/加成组卡", ArgText: args,
		})
		if strings.Contains(args, "event220") {
			if err == nil {
				t.Fatal("expected conflicting event error")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var params deckAutoQueryParams
		if err := json.Unmarshal(result.Params, &params); err != nil {
			t.Fatal(err)
		}
		if params.EventMEUnit != "street" || len(params.TargetBonuses) != 1 || params.TargetBonuses[0] != 350 {
			t.Fatalf("unexpected memory bonus query: %+v", params)
		}
	}
}

func TestMemoryEventPreservesSongAliases(t *testing.T) {
	for _, query := range []string{"me", "me unknown", "Tell me", "Leave me alone", "me!", "melt", "event220 Tell me", "music123 me vbs", "歌曲比较 Tell me"} {
		for _, mode := range []string{deckEventCommand, deckBonusCommand, deckNoEventCommand, deckChallengeCommand, deckMySekaiCommand} {
			params := deckAutoQueryParams{}
			got := extractDeckMemoryEvent(query, &params, mode)
			if got != query || params.EventMEUnit != "" {
				t.Fatalf("%s changed song query %q to %q: %+v", mode, query, got, params)
			}
		}
	}
	for _, mode := range []string{deckNoEventCommand, deckChallengeCommand} {
		params := deckAutoQueryParams{}
		if got := extractDeckMemoryEvent("me vbs", &params, mode); got != "me vbs" || params.EventMEUnit != "" {
			t.Fatalf("%s should not consume ME syntax", mode)
		}
	}
	for _, query := range []string{"Tell me", "me", "me unknown", "me vbs Tell me", "me vbs me", "me vbs Happy Halloween", "me vbs street", "me vbs mmj", "me vbs cool me"} {
		t.Run(query, func(t *testing.T) {
			h := sekaiHandlers{}.EventDeckHandle()
			result, err := h.Handle(&PjskHandlerContext{Context: context.Background(), Platform: "qq", UserId: "42", TriggerCmd: "/活动组卡", ArgText: query})
			if err != nil {
				t.Fatal(err)
			}
			var params deckAutoQueryParams
			if err := json.Unmarshal(result.Params, &params); err != nil {
				t.Fatal(err)
			}
			want := strings.ToLower(strings.TrimPrefix(query, "me vbs "))
			if params.MusicQuery != want {
				t.Fatalf("song query = %q, want %q", params.MusicQuery, want)
			}
			if strings.HasPrefix(query, "me vbs ") != (params.EventMEUnit == "street") {
				t.Fatalf("wrong selection: %+v", params)
			}
		})
	}
}
