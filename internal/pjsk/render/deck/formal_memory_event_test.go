package deck

import "testing"

func TestOfficialMemoryEventOptionsKeepMasterdataSelector(t *testing.T) {
	id := 221
	for _, mode := range []string{"event", "bonus", "mysekai"} {
		t.Run(mode, func(t *testing.T) {
			option := map[string]any{}
			applyRecommendOptionOverrides(option, mode, AutoQuery{RecommendType: mode, EventID: &id})
			if optionInt(option, "event_id") != id {
				t.Fatalf("official event lost: %+v", option)
			}
			for _, key := range []string{"event_me_unit", "event_type", "event_attr", "event_unit", "world_bloom_event_turn", "world_bloom_character_id", "custom_bonus_character_ids"} {
				if _, present := option[key]; present {
					t.Fatalf("official event has %s: %+v", key, option)
				}
			}
		})
	}
}
