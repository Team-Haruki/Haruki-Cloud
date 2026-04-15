package sekai

import (
	"testing"

	"haruki-cloud/internal/pjsk/handler"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestBuildSKTrackerParamsDefaults(t *testing.T) {
	ctx := SekaiHandlerContext{
		HandlerContext: handler.HandlerContext{ArgText: ""},
		region:         renderregion.JP,
	}

	params, err := buildSKTrackerParams(ctx, false, true, false)
	if err != nil {
		t.Fatalf("build params: %v", err)
	}

	ranks, ok := params["ranks"].([]int)
	if !ok {
		t.Fatalf("ranks type mismatch: %#v", params["ranks"])
	}
	if len(ranks) != len(defaultSKRanks) {
		t.Fatalf("expected default ranks len=%d got=%d", len(defaultSKRanks), len(ranks))
	}
	if got, ok := params["default_ranks"].(bool); !ok || !got {
		t.Fatalf("expected default_ranks=true, got %#v", params["default_ranks"])
	}
}

func TestBuildSKTrackerParamsDefaultsWorldLink(t *testing.T) {
	ctx := SekaiHandlerContext{
		HandlerContext: handler.HandlerContext{ArgText: "初音未来"},
		region:         renderregion.JP,
		prefixArg:      "wl",
	}

	params, err := buildSKTrackerParams(ctx, false, true, false)
	if err != nil {
		t.Fatalf("build params: %v", err)
	}

	ranks, ok := params["ranks"].([]int)
	if !ok {
		t.Fatalf("ranks type mismatch: %#v", params["ranks"])
	}
	if len(ranks) != len(defaultSKRanksWorldLink) {
		t.Fatalf("expected world link default ranks len=%d got=%d", len(defaultSKRanksWorldLink), len(ranks))
	}
	if got, ok := params["default_ranks"].(bool); !ok || !got {
		t.Fatalf("expected default_ranks=true, got %#v", params["default_ranks"])
	}
}

func TestBuildSKTrackerParamsParsesEventAndRanks(t *testing.T) {
	ctx := SekaiHandlerContext{
		HandlerContext: handler.HandlerContext{ArgText: "event101 500 100"},
		region:         renderregion.JP,
	}

	params, err := buildSKTrackerParams(ctx, false, true, false)
	if err != nil {
		t.Fatalf("build params: %v", err)
	}

	eventID, ok := params["event_id"].(int)
	if !ok || eventID != 101 {
		t.Fatalf("unexpected event_id: %#v", params["event_id"])
	}
	ranks, ok := params["ranks"].([]int)
	if !ok {
		t.Fatalf("ranks type mismatch: %#v", params["ranks"])
	}
	if len(ranks) != 2 || ranks[0] != 100 || ranks[1] != 500 {
		t.Fatalf("unexpected ranks: %#v", ranks)
	}
	if _, ok := params["default_ranks"]; ok {
		t.Fatalf("expected default_ranks to be omitted for explicit ranks")
	}
}

func TestBuildSKTrackerParamsWlDefaultsToCurrentChapterSelector(t *testing.T) {
	ctx := SekaiHandlerContext{
		HandlerContext: handler.HandlerContext{ArgText: "100 500"},
		region:         renderregion.JP,
		prefixArg:      "wl",
	}

	params, err := buildSKTrackerParams(ctx, false, true, false)
	if err != nil {
		t.Fatalf("build params: %v", err)
	}

	if got, ok := params["wl_character_query"].(string); !ok || got != "wl" {
		t.Fatalf("unexpected wl_character_query: %#v", params["wl_character_query"])
	}
	ranks, ok := params["ranks"].([]int)
	if !ok {
		t.Fatalf("ranks type mismatch: %#v", params["ranks"])
	}
	if len(ranks) != 2 || ranks[0] != 100 || ranks[1] != 500 {
		t.Fatalf("unexpected ranks: %#v", ranks)
	}
}

func TestBuildSKTrackerParamsParsesUIDWhenAllowed(t *testing.T) {
	ctx := SekaiHandlerContext{
		HandlerContext: handler.HandlerContext{ArgText: "event101 1234567890"},
		region:         renderregion.JP,
	}

	params, err := buildSKTrackerParams(ctx, false, true, false)
	if err != nil {
		t.Fatalf("build params: %v", err)
	}

	if _, ok := params["ranks"]; ok {
		t.Fatalf("expected ranks key to be omitted for uid query")
	}
	if got, ok := params["user_id"].(int64); !ok || got != 1234567890 {
		t.Fatalf("unexpected user_id: %#v", params["user_id"])
	}
}

func TestBuildSKTrackerParamsRejectsUIDWhenDisallowed(t *testing.T) {
	ctx := SekaiHandlerContext{
		HandlerContext: handler.HandlerContext{ArgText: "1234567890"},
		region:         renderregion.JP,
	}

	_, err := buildSKTrackerParams(ctx, false, false, false)
	if err == nil {
		t.Fatalf("expected error when uid is disallowed")
	}
}

func TestBuildSKTrackerParamsUsesUIDArgWhenArgsEmpty(t *testing.T) {
	ctx := SekaiHandlerContext{
		HandlerContext: handler.HandlerContext{ArgText: ""},
		region:         renderregion.JP,
		uidArg:         "1234567890",
	}

	params, err := buildSKTrackerParams(ctx, false, true, false)
	if err != nil {
		t.Fatalf("build params: %v", err)
	}

	if got, ok := params["user_id"].(int64); !ok || got != 1234567890 {
		t.Fatalf("unexpected user_id: %#v", params["user_id"])
	}
}

func TestBuildSKTrackerParamsAddsAtTargetMetadata(t *testing.T) {
	ctx := SekaiHandlerContext{
		HandlerContext: handler.HandlerContext{
			ArgText:    "",
			Platform:   "qq",
			TriggerCmd: "/sk",
		},
		region: renderregion.JP,
		uidArg: "@987654321",
	}

	params, err := buildSKTrackerParams(ctx, false, true, false)
	if err != nil {
		t.Fatalf("build params: %v", err)
	}

	if got, ok := params["target_platform"].(string); !ok || got != "qq" {
		t.Fatalf("unexpected target_platform: %#v", params["target_platform"])
	}
	if got, ok := params["target_user_id"].(string); !ok || got != "987654321" {
		t.Fatalf("unexpected target_user_id: %#v", params["target_user_id"])
	}
	if got, ok := params["region_explicit"].(bool); !ok || got {
		t.Fatalf("unexpected region_explicit: %#v", params["region_explicit"])
	}
}

func TestBuildSKTrackerParamsDefaultsToSelfWhenEnabled(t *testing.T) {
	ctx := SekaiHandlerContext{
		HandlerContext: handler.HandlerContext{
			ArgText:    "",
			Platform:   "qq",
			UserId:     "24680",
			TriggerCmd: "/sk",
		},
		region: renderregion.JP,
	}

	params, err := buildSKTrackerParams(ctx, false, true, true)
	if err != nil {
		t.Fatalf("build params: %v", err)
	}

	if _, ok := params["ranks"]; ok {
		t.Fatalf("expected no default ranks when selfWhenEmpty=true: %#v", params["ranks"])
	}
	if got, ok := params["target_user_id"].(string); !ok || got != "24680" {
		t.Fatalf("unexpected target_user_id: %#v", params["target_user_id"])
	}
}

func TestBuildSKTrackerParamsAddsSelectorMetadata(t *testing.T) {
	ctx := SekaiHandlerContext{
		HandlerContext: handler.HandlerContext{
			ArgText:    "",
			Platform:   "qq",
			UserId:     "24680",
			TriggerCmd: "/sk",
		},
		region:         renderregion.TW,
		uidArg:         "u2",
		explicitRegion: true,
	}

	params, err := buildSKTrackerParams(ctx, false, true, false)
	if err != nil {
		t.Fatalf("build params: %v", err)
	}

	if got, ok := params["target_platform"].(string); !ok || got != "qq" {
		t.Fatalf("unexpected target_platform: %#v", params["target_platform"])
	}
	if got, ok := params["target_user_id"].(string); !ok || got != "24680" {
		t.Fatalf("unexpected target_user_id: %#v", params["target_user_id"])
	}
	if got, ok := params["target_selector"].(string); !ok || got != "u2" {
		t.Fatalf("unexpected target_selector: %#v", params["target_selector"])
	}
	if got, ok := params["region_explicit"].(bool); !ok || !got {
		t.Fatalf("unexpected region_explicit: %#v", params["region_explicit"])
	}
}

func TestBuildSKTrackerParamsPreservesWlCharacterQuery(t *testing.T) {
	ctx := SekaiHandlerContext{
		HandlerContext: handler.HandlerContext{ArgText: "初音未来 100 500"},
		region:         renderregion.JP,
		prefixArg:      "wl",
	}

	params, err := buildSKTrackerParams(ctx, false, true, false)
	if err != nil {
		t.Fatalf("build params: %v", err)
	}

	if _, ok := params["wl_character_id"]; ok {
		t.Fatalf("expected wl_character_id to be omitted: %#v", params["wl_character_id"])
	}
	if got, ok := params["wl_character_query"].(string); !ok || got != "初音未来" {
		t.Fatalf("unexpected wl_character_query: %#v", params["wl_character_query"])
	}
	ranks, ok := params["ranks"].([]int)
	if !ok {
		t.Fatalf("ranks type mismatch: %#v", params["ranks"])
	}
	if len(ranks) != 2 || ranks[0] != 100 || ranks[1] != 500 {
		t.Fatalf("unexpected ranks: %#v", ranks)
	}
}

func TestBuildSKTrackerParamsParsesPrefixedWlCharacterQuery(t *testing.T) {
	ctx := SekaiHandlerContext{
		HandlerContext: handler.HandlerContext{ArgText: "wl初音未来 100"},
		region:         renderregion.JP,
		prefixArg:      "wl",
	}

	params, err := buildSKTrackerParams(ctx, false, true, false)
	if err != nil {
		t.Fatalf("build params: %v", err)
	}

	if got, ok := params["wl_character_query"].(string); !ok || got != "初音未来" {
		t.Fatalf("unexpected wl_character_query: %#v", params["wl_character_query"])
	}
	ranks, ok := params["ranks"].([]int)
	if !ok {
		t.Fatalf("ranks type mismatch: %#v", params["ranks"])
	}
	if len(ranks) != 1 || ranks[0] != 100 {
		t.Fatalf("unexpected ranks: %#v", ranks)
	}
}

func TestBuildSKTrackerParamsParsesLeadingWlChapterSelector(t *testing.T) {
	ctx := SekaiHandlerContext{
		HandlerContext: handler.HandlerContext{ArgText: "wl2 100"},
		region:         renderregion.JP,
	}

	params, err := buildSKTrackerParams(ctx, false, true, false)
	if err != nil {
		t.Fatalf("build params: %v", err)
	}

	if got, ok := params["wl_character_query"].(string); !ok || got != "wl2" {
		t.Fatalf("unexpected wl_character_query: %#v", params["wl_character_query"])
	}
	ranks, ok := params["ranks"].([]int)
	if !ok {
		t.Fatalf("ranks type mismatch: %#v", params["ranks"])
	}
	if len(ranks) != 1 || ranks[0] != 100 {
		t.Fatalf("unexpected ranks: %#v", ranks)
	}
}
