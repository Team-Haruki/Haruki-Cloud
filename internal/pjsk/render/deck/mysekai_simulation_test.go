package deck

import (
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"testing"
)

func TestMysekaiSimulationOptionsAndTitles(t *testing.T) {
	c := newTestDeckController(t, RecommendConfig{})
	for _, tc := range []struct {
		name, title string
		query       AutoQuery
	}{
		{"ME", "烤森总集篇活动模拟{noun}", AutoQuery{EventMEUnit: "street"}},
		{"WL3", "烤森 WL3 活动模拟{noun}", AutoQuery{WorldBloomEventTurn: new(3), WorldBloomCharacterID: new(21), EventUnit: "piapro"}},
		{"WL3 finale", "烤森 WL3 终章活动模拟{noun}", AutoQuery{WorldBloomFinaleTurn: new(3), ForcedLeaderCharacterID: new(21), MetadataWorldBloomFinale: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			option, err := c.buildRecommendOption(renderregion.JP, "mysekai", tc.query)
			if err != nil {
				t.Fatal(err)
			}
			if option["live_type"] != "mysekai" || option["event_id"] != nil {
				t.Fatalf("wrong engine options: %+v", option)
			}
			if tc.query.WorldBloomEventTurn != nil && (option["world_bloom_event_turn"] != 3 || option["world_bloom_character_id"] != 21) {
				t.Fatalf("chapter missing: %+v", option)
			}
			if tc.query.WorldBloomFinaleTurn != nil && (option["world_bloom_finale_turn"] != 3 || option["forced_leader_character_id"] != 21) {
				t.Fatalf("finale leader missing: %+v", option)
			}
			req := &drawing.DeckRequest{RecommendType: "mysekai"}
			c.applyCommonRecommendMetadata(req, renderregion.JP, "mysekai", metadataOption(option, "mysekai", tc.query), tc.query)
			if req.RecommendType != "mysekai" || req.EventID != nil || req.Labels["deck.title.mysekai_simulated"] != tc.title {
				t.Fatalf("wrong simulation metadata: %+v", req)
			}
		})
	}
	for _, option := range []map[string]any{{"event_id": 220}, {"event_attr": "cute", "event_unit": "street"}} {
		req := &drawing.DeckRequest{RecommendType: "mysekai"}
		applyMysekaiSimulationTitle(req, "mysekai", option)
		if len(req.Labels) != 0 {
			t.Fatalf("changed ordinary title: %+v", req.Labels)
		}
	}
}
