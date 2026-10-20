package deck

import (
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestMemoryEventOptionsAndMetadata(t *testing.T) {
	c := newTestDeckController(t, RecommendConfig{})
	for _, unit := range []string{"light_sound", "idol", "street", "theme_park", "school_refusal", "piapro"} {
		for _, mode := range []string{"event", "bonus"} {
			q := AutoQuery{EventMEUnit: unit, RecommendType: mode}
			option, err := c.buildRecommendOption(renderregion.JP, mode, q)
			if err != nil {
				t.Fatal(err)
			}
			if option["event_me_unit"] != unit || option["event_type"] != "marathon" || option["event_id"] != nil {
				t.Fatalf("unexpected simulation options: %+v", option)
			}
			if !optionHasSimulatedEvent(option) || shouldUseFallbackEventMetadata(mode, option) {
				t.Fatal("memory event should not use current-event metadata")
			}
			request := &drawing.DeckRequest{}
			c.applyCommonRecommendMetadata(request, renderregion.JP, mode, option, q)
			if request.EventID != nil || request.IsWl || request.EventName == nil || !strings.Contains(*request.EventName, "总集篇模拟") {
				t.Fatalf("unexpected event metadata: %+v", request)
			}
		}
	}
}

func TestMemoryEventRejectsMixedAPISelectors(t *testing.T) {
	c := newTestDeckController(t, RecommendConfig{})
	for _, q := range []AutoQuery{
		{EventMEUnit: "invalid"},
		{EventMEUnit: "street", EventID: new(220)},
		{EventMEUnit: "street", EventAttr: "cute"},
		{EventMEUnit: "street", EventUnit: "idol"},
		{EventMEUnit: "street", WorldBloomEventTurn: new(1)},
		{EventMEUnit: "street", WorldBloomFinaleTurn: new(3)},
	} {
		if _, err := c.buildRecommendOption(renderregion.JP, "event", q); err == nil {
			t.Fatalf("accepted mixed selectors: %+v", q)
		}
	}
}
