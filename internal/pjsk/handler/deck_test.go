package handler

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/parser"
)

func TestDeckAutoQueryParamsJSONRoundTripPreservesExtendedFields(t *testing.T) {
	original := deckAutoQueryParams{
		Boost:               intPtr(5),
		AreaItemLevel:       intPtr(15),
		Selector:            "u2",
		UnitFilter:          "idol",
		AttrFilter:          "cool",
		ExcludedCards:       []int{123, 456},
		UseCurrentDeck:      true,
		MaxProfile:          true,
		SubMaxProfile:       true,
		MusicCompare:        true,
		MusicCompareQueries: []string{"龙hard", "虾expert", "sage"},
		SpecificSkillOrder:  []int{0, 1, 2, 3, 4},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	var decoded deckAutoQueryParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}

	if decoded.Boost == nil || *decoded.Boost != 5 {
		t.Fatalf("unexpected boost: %+v", decoded.Boost)
	}
	if decoded.AreaItemLevel == nil || *decoded.AreaItemLevel != 15 {
		t.Fatalf("unexpected area item level: %+v", decoded.AreaItemLevel)
	}
	if decoded.Selector != "u2" {
		t.Fatalf("unexpected selector: %q", decoded.Selector)
	}
	if decoded.UnitFilter != "idol" || decoded.AttrFilter != "cool" {
		t.Fatalf("unexpected filters: unit=%q attr=%q", decoded.UnitFilter, decoded.AttrFilter)
	}
	if !reflect.DeepEqual(decoded.ExcludedCards, []int{123, 456}) {
		t.Fatalf("unexpected excluded cards: %+v", decoded.ExcludedCards)
	}
	if !decoded.UseCurrentDeck || !decoded.MaxProfile || !decoded.SubMaxProfile || !decoded.MusicCompare {
		t.Fatalf("unexpected flags: %+v", decoded)
	}
	if !reflect.DeepEqual(decoded.MusicCompareQueries, []string{"龙hard", "虾expert", "sage"}) {
		t.Fatalf("unexpected music compare queries: %+v", decoded.MusicCompareQueries)
	}
	if !reflect.DeepEqual(decoded.SpecificSkillOrder, []int{0, 1, 2, 3, 4}) {
		t.Fatalf("unexpected specific skill order: %+v", decoded.SpecificSkillOrder)
	}
}

func TestEventDeckHandleParsesCommonOptions(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/组卡",
		ArgText:    "event123 miku auto 倍率 满技能 #123 456",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleDeck || resolved.Mode != "deck-event" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID == nil || *params.EventID != 123 {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
	// "miku" is now resolved to character ID 21 by the extractor
	if params.WorldBloomCharacterID == nil || *params.WorldBloomCharacterID != 21 {
		t.Fatalf("unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
	}
	if params.WorldBloomCharacterQuery != "" {
		t.Fatalf("unexpected world bloom character query: %q", params.WorldBloomCharacterQuery)
	}
	if params.LiveType != "auto" {
		t.Fatalf("unexpected live type: %q", params.LiveType)
	}
	if params.Target != "skill" {
		t.Fatalf("unexpected target: %q", params.Target)
	}
	if len(params.FixedCards) != 2 || params.FixedCards[0] != 123 || params.FixedCards[1] != 456 {
		t.Fatalf("unexpected fixed cards: %+v", params.FixedCards)
	}
	if params.Rarity1Config == nil || !params.Rarity1Config.SkillMax {
		t.Fatalf("unexpected rarity patch: %+v", params.Rarity1Config)
	}
}

func TestEventDeckHandleParsesLeadingSelectorArg(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/组卡",
		ArgText:    "u2 event123 当前 sage neo",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Selector != "u2" {
		t.Fatalf("unexpected selector: %q", params.Selector)
	}
	if params.EventID == nil || *params.EventID != 123 {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
	if !params.UseCurrentDeck {
		t.Fatalf("expected use_current_deck to be enabled")
	}
	if params.MusicQuery != "sage neo" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleParsesTrailingSelectorArg(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/组卡",
		ArgText:    "event123 sage neo u1",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Selector != "u1" {
		t.Fatalf("unexpected selector: %q", params.Selector)
	}
	if params.EventID == nil || *params.EventID != 123 {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
	if params.MusicQuery != "sage neo" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandlePrefersLastLiveTypeKeyword(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "solo auto",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.LiveType != "auto" {
		t.Fatalf("unexpected live type: %q", params.LiveType)
	}
	if params.MusicQuery != "" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleParsesSimulatedEvent(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "25h 可爱",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventUnit != "school_refusal" || params.EventAttr != "cute" {
		t.Fatalf("unexpected simulate event params: %+v", params)
	}
	if params.EventID != nil {
		t.Fatalf("simulated event should not set event id: %+v", params.EventID)
	}
}

func TestEventDeckHandlePrefersSimulatedEventOverBareNumericEventFor25(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "25 蓝",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID != nil {
		t.Fatalf("simulated event should not set event id: %+v", params.EventID)
	}
	if params.EventUnit != "school_refusal" || params.EventAttr != "cool" {
		t.Fatalf("unexpected simulate event params: %+v", params)
	}
}

func TestEventDeckHandleParsesMultiSkillLowerBound(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "多人 230实效 Song A",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.LiveType != "multi" {
		t.Fatalf("unexpected live type: %q", params.LiveType)
	}
	if params.Target != "" {
		t.Fatalf("unexpected target: %q", params.Target)
	}
	if params.MultiLiveTeammateScoreUp == nil || *params.MultiLiveTeammateScoreUp != 230 {
		t.Fatalf("unexpected teammate score up: %+v", params.MultiLiveTeammateScoreUp)
	}
	if params.MultiLiveScoreUpLowerBound == nil || *params.MultiLiveScoreUpLowerBound != 230 {
		t.Fatalf("unexpected score up lower bound: %+v", params.MultiLiveScoreUpLowerBound)
	}
	if params.MusicQuery != "song a" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleParsesSplitTeammateScoreUp(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "多人 队友实效 210 Song A",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.LiveType != "multi" {
		t.Fatalf("unexpected live type: %q", params.LiveType)
	}
	if params.Target != "" {
		t.Fatalf("unexpected target: %q", params.Target)
	}
	if params.MultiLiveTeammateScoreUp == nil || *params.MultiLiveTeammateScoreUp != 210 {
		t.Fatalf("unexpected teammate score up: %+v", params.MultiLiveTeammateScoreUp)
	}
	if params.MultiLiveScoreUpLowerBound != nil {
		t.Fatalf("teammate score up should not set score up lower bound: %+v", params.MultiLiveScoreUpLowerBound)
	}
	if params.MusicQuery != "song a" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleParsesBareSkillTargetAfterMusicQuery(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "三星满破满技能 四星禁用 已读 画布 龙hd 实效",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Target != "skill" {
		t.Fatalf("unexpected target: %q", params.Target)
	}
	if params.MusicDiff != "hard" {
		t.Fatalf("unexpected music diff: %q", params.MusicDiff)
	}
	if params.MusicQuery != "龙" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
	if params.MultiLiveScoreUpLowerBound != nil {
		t.Fatalf("bare skill target should not set score up lower bound: %+v", params.MultiLiveScoreUpLowerBound)
	}
	if params.Rarity3Config == nil || !params.Rarity3Config.MasterMax || !params.Rarity3Config.SkillMax {
		t.Fatalf("unexpected rarity 3 config: %+v", params.Rarity3Config)
	}
	if params.Rarity4Config == nil || !params.Rarity4Config.Disable {
		t.Fatalf("unexpected rarity 4 config: %+v", params.Rarity4Config)
	}
	if params.Rarity1Config == nil || !params.Rarity1Config.EpisodeRead || !params.Rarity1Config.Canvas {
		t.Fatalf("unexpected global config propagation: %+v", params.Rarity1Config)
	}
}

func TestEventDeckHandleParsesSplitSkillLowerBound(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "多人 230 实效",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Target != "" {
		t.Fatalf("unexpected target: %q", params.Target)
	}
	if params.MultiLiveTeammateScoreUp == nil || *params.MultiLiveTeammateScoreUp != 230 {
		t.Fatalf("unexpected teammate score up: %+v", params.MultiLiveTeammateScoreUp)
	}
	if params.MultiLiveScoreUpLowerBound == nil || *params.MultiLiveScoreUpLowerBound != 230 {
		t.Fatalf("unexpected score up lower bound: %+v", params.MultiLiveScoreUpLowerBound)
	}
}

func TestEventDeckHandleParsesSimulatedWorldBloomTurnAndCharacter(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "miku wl4",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	var params deckAutoQueryParams
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID != nil {
		t.Fatalf("unexpected explicit event id: %+v", params.EventID)
	}
	if params.WorldBloomEventTurn == nil || *params.WorldBloomEventTurn != 4 {
		t.Fatalf("unexpected wl event turn: %+v", params.WorldBloomEventTurn)
	}
	if params.WorldBloomCharacterID == nil || *params.WorldBloomCharacterID != 21 {
		t.Fatalf("unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
	}
	if params.MusicQuery != "" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleParsesSimulatedWorldBloomTurnWithTrailingMusicQuery(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "sage wl3 初音未来",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	var params deckAutoQueryParams
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.WorldBloomEventTurn == nil || *params.WorldBloomEventTurn != 3 {
		t.Fatalf("unexpected wl event turn: %+v", params.WorldBloomEventTurn)
	}
	if params.WorldBloomCharacterID == nil || *params.WorldBloomCharacterID != 21 {
		t.Fatalf("unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
	}
	if params.MusicQuery != "sage" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandlePreservesWorldBloomCharacterQueryAfterEventID(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 初音未来",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID == nil || *params.EventID != 123 {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
	// "初音未来" is resolved to character ID 21
	if params.WorldBloomCharacterID == nil || *params.WorldBloomCharacterID != 21 {
		t.Fatalf("unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
	}
	if params.WorldBloomCharacterQuery != "" {
		t.Fatalf("unexpected world bloom character query: %q", params.WorldBloomCharacterQuery)
	}
}

func TestEventDeckHandleRejectsDeprecatedWorldBloomChapterSelectorAfterEventID(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "140 wl3 sage",
	})
	if err == nil {
		t.Fatalf("expected deprecated wl chapter selector to be rejected")
	}
	if !strings.Contains(err.Error(), "不再支持 wl2 这种 WL 章节写法") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEventDeckHandleRejectsDeprecatedStandaloneWorldBloomChapterSelector(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动组卡",
		ArgText:    "sage wl3",
	})
	if err == nil {
		t.Fatalf("expected deprecated wl chapter selector to be rejected")
	}
	if !strings.Contains(err.Error(), "不再支持 wl2 这种 WL 章节写法") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEventDeckHandleParsesMaxProfile(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 顶配 sage neo",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.MaxProfile {
		t.Fatalf("expected max_profile to be enabled")
	}
	if params.MusicQuery != "sage neo" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleParsesSubMaxProfile(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 次顶配 sage neo",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.SubMaxProfile {
		t.Fatalf("expected sub_max_profile to be enabled")
	}
	if params.MusicQuery != "sage neo" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleParsesCurrentDeck(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 当前 sage neo",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.UseCurrentDeck {
		t.Fatalf("expected use_current_deck to be enabled")
	}
	if params.MusicQuery != "sage neo" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleParsesMusicCompareCurrent(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "歌曲比较 当前",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.MusicCompare || !params.UseCurrentDeck {
		t.Fatalf("unexpected compare current params: %+v", params)
	}
	if len(params.MusicCompareQueries) != 0 {
		t.Fatalf("unexpected music compare queries: %+v", params.MusicCompareQueries)
	}
	if params.MusicQuery != "" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleParsesMusicCompareQueriesAcrossKeyword(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "龙hard 歌曲比较 虾expert sage",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.MusicCompare {
		t.Fatalf("expected music_compare to be enabled")
	}
	if !reflect.DeepEqual(params.MusicCompareQueries, []string{"龙hard", "虾expert", "sage"}) {
		t.Fatalf("unexpected music compare queries: %+v", params.MusicCompareQueries)
	}
	if params.MusicQuery != "" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleRejectsTooManyMusicCompareQueries(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "歌曲比较 a b c d e f",
	})
	if err == nil {
		t.Fatalf("expected too many compare songs to fail")
	}
	if !strings.Contains(err.Error(), "最多只能指定 5 首歌曲") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEventDeckHandleParsesUnitFilter(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 仅vs sage neo",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.UnitFilter != "piapro" {
		t.Fatalf("unexpected unit filter: %q", params.UnitFilter)
	}
	if params.MusicQuery != "sage neo" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleParsesAttrFilter(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 仅紫 sage neo",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.AttrFilter != "mysterious" {
		t.Fatalf("unexpected attr filter: %q", params.AttrFilter)
	}
	if params.MusicQuery != "sage neo" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleParsesExcludedCards(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 sage neo -123 -456",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !reflect.DeepEqual(params.ExcludedCards, []int{123, 456}) {
		t.Fatalf("unexpected excluded cards: %+v", params.ExcludedCards)
	}
	if params.MusicQuery != "sage neo" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleParsesAreaItemLevel(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 区域道具15级 sage neo",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.AreaItemLevel == nil || *params.AreaItemLevel != 15 {
		t.Fatalf("unexpected area item level: %+v", params.AreaItemLevel)
	}
	if params.MusicQuery != "sage neo" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleParsesAreaItemLevelShorthand(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 15级 当前 sage neo",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.AreaItemLevel == nil || *params.AreaItemLevel != 15 {
		t.Fatalf("unexpected area item level: %+v", params.AreaItemLevel)
	}
	if !params.UseCurrentDeck {
		t.Fatalf("expected use_current_deck to be enabled")
	}
	if params.MusicQuery != "sage neo" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleParsesSkillOrderAverage(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 技能顺序平均 sage neo",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.SkillOrderChooseStrategy != "average" {
		t.Fatalf("unexpected skill order choose strategy: %q", params.SkillOrderChooseStrategy)
	}
	if len(params.SpecificSkillOrder) != 0 {
		t.Fatalf("unexpected specific skill order: %+v", params.SpecificSkillOrder)
	}
	if params.MusicQuery != "sage neo" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleParsesSpecificSkillOrderWithCurrent(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 当前 技能顺序12345 sage neo",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.UseCurrentDeck {
		t.Fatalf("expected use_current_deck to be enabled")
	}
	if params.SkillOrderChooseStrategy != "specific" {
		t.Fatalf("unexpected skill order choose strategy: %q", params.SkillOrderChooseStrategy)
	}
	if !reflect.DeepEqual(params.SpecificSkillOrder, []int{0, 1, 2, 3, 4}) {
		t.Fatalf("unexpected specific skill order: %+v", params.SpecificSkillOrder)
	}
	if params.MusicQuery != "sage neo" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleParsesSpecificSkillOrderWithFixedCards(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 sage neo 技能顺序15234 #1 2 3 4 5",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !reflect.DeepEqual(params.FixedCards, []int{1, 2, 3, 4, 5}) {
		t.Fatalf("unexpected fixed cards: %+v", params.FixedCards)
	}
	if params.SkillOrderChooseStrategy != "specific" {
		t.Fatalf("unexpected skill order choose strategy: %q", params.SkillOrderChooseStrategy)
	}
	if !reflect.DeepEqual(params.SpecificSkillOrder, []int{0, 4, 1, 2, 3}) {
		t.Fatalf("unexpected specific skill order: %+v", params.SpecificSkillOrder)
	}
	if params.MusicQuery != "sage neo" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleRejectsSpecificSkillOrderWithoutCompleteFixedDeck(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 技能顺序12345 sage neo",
	})
	if err == nil {
		t.Fatalf("expected specific skill order without fixed deck to fail")
	}
	if !strings.Contains(err.Error(), "仅在使用固定队伍") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEventDeckHandleRejectsSpecificSkillOrderWithFixedCharacters(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 sage neo 技能顺序12345 #miku rin",
	})
	if err == nil {
		t.Fatalf("expected fixed characters with specific skill order to fail")
	}
	if !strings.Contains(err.Error(), "仅在使用固定队伍") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBonusDeckHandleParsesEventAndBonuses(t *testing.T) {
	h := sekaiHandlers{}.BonusDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/加成组卡",
		ArgText:    "event123 120 160",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID == nil || *params.EventID != 123 {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
	if len(params.TargetBonuses) != 2 || params.TargetBonuses[0] != 120 || params.TargetBonuses[1] != 160 {
		t.Fatalf("unexpected bonuses: %+v", params.TargetBonuses)
	}
}

func TestBonusDeckHandleParsesBonusKeywords(t *testing.T) {
	h := sekaiHandlers{}.BonusDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/加成组卡",
		ArgText:    "event123 120加成 160%",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID == nil || *params.EventID != 123 {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
	if len(params.TargetBonuses) != 2 || params.TargetBonuses[0] != 120 || params.TargetBonuses[1] != 160 {
		t.Fatalf("unexpected bonuses: %+v", params.TargetBonuses)
	}
}

func TestBonusDeckHandleTreatsBareNumericLeadingValueAsBonusTarget(t *testing.T) {
	h := sekaiHandlers{}.BonusDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/加成组卡",
		ArgText:    "123 120",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID != nil {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
	if !reflect.DeepEqual(params.TargetBonuses, []int{123, 120}) {
		t.Fatalf("unexpected target bonuses: %+v", params.TargetBonuses)
	}
}

func TestChallengeDeckHandleParsesCharacterAndAuto(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/挑战组卡",
		ArgText:    "miku auto",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	// "miku" is resolved to character ID 21
	if params.ChallengeLiveCharacterID == nil || *params.ChallengeLiveCharacterID != 21 {
		t.Fatalf("unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
	}
	if params.ChallengeLiveCharacterQuery != "" {
		t.Fatalf("unexpected challenge character query: %q", params.ChallengeLiveCharacterQuery)
	}
	if params.LiveType != "auto" {
		t.Fatalf("unexpected live type: %q", params.LiveType)
	}
}

func TestChallengeDeckHandleAllowsAllCharactersWhenCharacterOmitted(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/挑战组卡",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.ChallengeLiveCharacterID != nil {
		t.Fatalf("unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
	}
	if params.ChallengeLiveCharacterQuery != "" {
		t.Fatalf("unexpected challenge character query: %q", params.ChallengeLiveCharacterQuery)
	}
	if params.MusicQuery != "" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestChallengeDeckHandleTreatsInlineDifficultyTokenAsMusicQuery(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/挑战组卡",
		ArgText:    "群青ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.ChallengeLiveCharacterID != nil {
		t.Fatalf("unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
	}
	if params.ChallengeLiveCharacterQuery != "" {
		t.Fatalf("unexpected challenge character query: %q", params.ChallengeLiveCharacterQuery)
	}
	if params.MusicQuery != "群青" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
	if params.MusicDiff != "expert" {
		t.Fatalf("unexpected music diff: %q", params.MusicDiff)
	}
}

func TestChallengeDeckHandleParsesCurrentKeywordWithoutCharacter(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/挑战组卡",
		ArgText:    "当前",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.UseCurrentDeck {
		t.Fatalf("expected use_current_deck to be enabled")
	}
	if params.ChallengeLiveCharacterID != nil {
		t.Fatalf("unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
	}
	if params.MusicQuery != "" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestChallengeDeckHandleParsesCharacterAndCurrentKeyword(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/挑战组卡",
		ArgText:    "miku 当前",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.UseCurrentDeck {
		t.Fatalf("expected use_current_deck to be enabled")
	}
	if params.ChallengeLiveCharacterID == nil || *params.ChallengeLiveCharacterID != 21 {
		t.Fatalf("unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
	}
	if params.MusicQuery != "" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestChallengeDeckHandleParsesMusicCompareQueries(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/挑战组卡",
		ArgText:    "miku 歌曲比较 10th 群青apd",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.ChallengeLiveCharacterID == nil || *params.ChallengeLiveCharacterID != 21 {
		t.Fatalf("unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
	}
	if !params.MusicCompare {
		t.Fatalf("expected music_compare to be enabled")
	}
	if !reflect.DeepEqual(params.MusicCompareQueries, []string{"10th", "群青apd"}) {
		t.Fatalf("unexpected music compare queries: %+v", params.MusicCompareQueries)
	}
	if params.MusicQuery != "" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestChallengeDeckHandlePreservesCharacterQuery(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/挑战组卡",
		ArgText:    "初音未来 auto",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	// "初音未来" is resolved to character ID 21
	if params.ChallengeLiveCharacterID == nil || *params.ChallengeLiveCharacterID != 21 {
		t.Fatalf("unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
	}
	if params.ChallengeLiveCharacterQuery != "" {
		t.Fatalf("unexpected challenge character query: %q", params.ChallengeLiveCharacterQuery)
	}
	if params.LiveType != "auto" {
		t.Fatalf("unexpected live type: %q", params.LiveType)
	}
}

func TestMysekaiDeckHandleParsesEventAndFixedCharacter(t *testing.T) {
	h := sekaiHandlers{}.MysekaiDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/ms组卡",
		ArgText:    "event123 #miku",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var combined mysekaiDeckCombinedParams
	if err := json.Unmarshal(resolved.Params, &combined); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	params := combined.Deck
	if params.EventID == nil || *params.EventID != 123 {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
	// "#miku" is now resolved to character ID 21
	if len(params.FixedCharacters) != 1 || params.FixedCharacters[0] != 21 {
		t.Fatalf("unexpected fixed characters: %+v", params.FixedCharacters)
	}
	if len(params.FixedCharacterQueries) != 0 {
		t.Fatalf("unexpected fixed character queries: %+v", params.FixedCharacterQueries)
	}
}

func TestMysekaiDeckHandleParsesMusicCompareQueries(t *testing.T) {
	h := sekaiHandlers{}.MysekaiDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/ms组卡",
		ArgText:    "歌曲比较 龙hard 虾expert",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var combined mysekaiDeckCombinedParams
	if err := json.Unmarshal(resolved.Params, &combined); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !combined.Deck.MusicCompare {
		t.Fatalf("expected music_compare to be enabled")
	}
	if !reflect.DeepEqual(combined.Deck.MusicCompareQueries, []string{"龙hard", "虾expert"}) {
		t.Fatalf("unexpected music compare queries: %+v", combined.Deck.MusicCompareQueries)
	}
}

func TestMysekaiDeckHandlePreservesFixedCharacterQueries(t *testing.T) {
	h := sekaiHandlers{}.MysekaiDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/ms组卡",
		ArgText:    "event123 #初音未来 巡音流歌",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var combined mysekaiDeckCombinedParams
	if err := json.Unmarshal(resolved.Params, &combined); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	params := combined.Deck
	if params.EventID == nil || *params.EventID != 123 {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
	// "初音未来" resolves to 21, "巡音流歌" resolves to 24
	if len(params.FixedCharacters) != 2 || params.FixedCharacters[0] != 21 || params.FixedCharacters[1] != 24 {
		t.Fatalf("unexpected fixed character ids: %+v", params.FixedCharacters)
	}
	if len(params.FixedCharacterQueries) != 0 {
		t.Fatalf("unexpected fixed character queries: %+v", params.FixedCharacterQueries)
	}
}

func TestEventDeckHandleParsesMusicQueryAndDifficulty(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "Tell Your World ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.MusicQuery != "tell your world" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
	if params.MusicDiff != "expert" {
		t.Fatalf("unexpected music diff: %q", params.MusicDiff)
	}
}

func TestEventDeckHandleParsesMusicQueryAndDifficultyWithoutSpace(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "190 满画布 已读 虾ex 10火",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID == nil || *params.EventID != 190 {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
	if params.MusicQuery != "虾" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
	if params.MusicDiff != "expert" {
		t.Fatalf("unexpected music diff: %q", params.MusicDiff)
	}
}

func TestEventDeckHandleParsesExplicitMusicID(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "music123 ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.MusicID == nil || *params.MusicID != 123 {
		t.Fatalf("unexpected music id: %+v", params.MusicID)
	}
	if params.MusicQuery != "" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
	if params.MusicDiff != "expert" {
		t.Fatalf("unexpected music diff: %q", params.MusicDiff)
	}
}

func TestEventDeckHandleKeepsBareNumericQuery(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "123 ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.MusicID != nil {
		t.Fatalf("unexpected music id: %+v", params.MusicID)
	}
	if params.MusicQuery != "123" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
	if params.MusicDiff != "expert" {
		t.Fatalf("unexpected music diff: %q", params.MusicDiff)
	}
}

func TestEventDeckHandleRecognizesNicknameAliasAfterEventID(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/缁勫崱",
		ArgText:    "event123 tks",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID == nil || *params.EventID != 123 {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
	if params.WorldBloomCharacterID == nil || *params.WorldBloomCharacterID != 13 {
		t.Fatalf("unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
	}
	if params.WorldBloomCharacterQuery != "" {
		t.Fatalf("unexpected world bloom character query: %q", params.WorldBloomCharacterQuery)
	}
}

func TestEventDeckHandleRecognizesLeadingNumericEventIDAndStripsBoostToken(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "163 tks sage 5火",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID == nil || *params.EventID != 163 {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
	if params.WorldBloomCharacterID == nil || *params.WorldBloomCharacterID != 13 {
		t.Fatalf("unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
	}
	if params.Boost == nil || *params.Boost != 5 {
		t.Fatalf("unexpected boost: %+v", params.Boost)
	}
	if params.MusicQuery != "sage" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleStripsBoostTokenAfterExplicitEventID(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event163 sage neo 5火",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID == nil || *params.EventID != 163 {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
	if params.Boost == nil || *params.Boost != 5 {
		t.Fatalf("unexpected boost: %+v", params.Boost)
	}
	if params.MusicQuery != "sage neo" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestChallengeDeckHandleRecognizesNicknameAlias(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/鎸戞垬缁勫崱",
		ArgText:    "tks auto",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.ChallengeLiveCharacterID == nil || *params.ChallengeLiveCharacterID != 13 {
		t.Fatalf("unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
	}
	if params.ChallengeLiveCharacterQuery != "" {
		t.Fatalf("unexpected challenge character query: %q", params.ChallengeLiveCharacterQuery)
	}
	if params.LiveType != "auto" {
		t.Fatalf("unexpected live type: %q", params.LiveType)
	}
}

func TestMysekaiDeckHandleRecognizesFixedCharacterAlias(t *testing.T) {
	h := sekaiHandlers{}.MysekaiDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/ms缁勫崱",
		ArgText:    "event123 #tks",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var combined mysekaiDeckCombinedParams
	if err := json.Unmarshal(resolved.Params, &combined); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	params := combined.Deck
	if len(params.FixedCharacters) != 1 || params.FixedCharacters[0] != 13 {
		t.Fatalf("unexpected fixed character ids: %+v", params.FixedCharacters)
	}
	if len(params.FixedCharacterQueries) != 0 {
		t.Fatalf("unexpected fixed character queries: %+v", params.FixedCharacterQueries)
	}
}

func TestNoEventDeckHandleRecognizesBoostToken(t *testing.T) {
	h := sekaiHandlers{}.NoEventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/最强组卡",
		ArgText:    "sage 5火",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Boost == nil || *params.Boost != 5 {
		t.Fatalf("unexpected boost: %+v", params.Boost)
	}
	if params.MusicQuery != "sage" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestMysekaiDeckHandleRecognizesBoostToken(t *testing.T) {
	h := sekaiHandlers{}.MysekaiDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/ms组卡",
		ArgText:    "event123 5火",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var combined mysekaiDeckCombinedParams
	if err := json.Unmarshal(resolved.Params, &combined); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if combined.Deck.EventID == nil || *combined.Deck.EventID != 123 {
		t.Fatalf("unexpected event id: %+v", combined.Deck.EventID)
	}
	if combined.Deck.Boost == nil || *combined.Deck.Boost != 5 {
		t.Fatalf("unexpected boost: %+v", combined.Deck.Boost)
	}
}

func TestEventDeckHandleRecognizesChinese25JiAlias(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "25时 紫",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventUnit != "school_refusal" || params.EventAttr != "mysterious" {
		t.Fatalf("unexpected simulated event params: %+v", params)
	}
	if params.MusicQuery != "" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestNoEventDeckHandleRejectsAttrOnlyAliasAsSongQuery(t *testing.T) {
	h := sekaiHandlers{}.NoEventDeckHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/最强组卡",
		ArgText:    "紫月",
	})
	if err == nil {
		t.Fatalf("expected attr-only alias to trigger simulated-event hint")
	}
	if !strings.Contains(err.Error(), "/组卡 团名 属性") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNoEventDeckHandleRejectsFullAliasesWithNoEventHint(t *testing.T) {
	h := sekaiHandlers{}.NoEventDeckHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/最强组卡",
		ArgText:    "25时 蓝星",
	})
	if err == nil {
		t.Fatalf("expected no-event deck to reject simulated event aliases")
	}
	if !strings.Contains(err.Error(), "/组卡 团名 属性") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEventDeckHandleRejectsDeprecatedWorldBloomSelectorAndCharacterAfterEventID(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "140 wl3 miku",
	})
	if err == nil {
		t.Fatalf("expected deprecated WL chapter selector to be rejected")
	}
	if !strings.Contains(err.Error(), "不再支持 wl2 这种 WL 章节写法") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNoEventDeckHandleAllowsOnly25WithBareSkillTarget(t *testing.T) {
	h := sekaiHandlers{}.NoEventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/最强组卡",
		ArgText:    "仅25 实效",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Target != "skill" {
		t.Fatalf("unexpected target: %q", params.Target)
	}
	if params.MultiLiveScoreUpLowerBound != nil {
		t.Fatalf("bare skill target should not set score up lower bound: %+v", params.MultiLiveScoreUpLowerBound)
	}
	if params.UnitFilter != "school_refusal" {
		t.Fatalf("unexpected unit filter: %q", params.UnitFilter)
	}
	if params.MusicQuery != "" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestNoEventDeckHandleAllowsOnly25hWithBareSkillTarget(t *testing.T) {
	h := sekaiHandlers{}.NoEventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/最强组卡",
		ArgText:    "仅25h 实效",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Target != "skill" {
		t.Fatalf("unexpected target: %q", params.Target)
	}
	if params.MultiLiveScoreUpLowerBound != nil {
		t.Fatalf("bare skill target should not set score up lower bound: %+v", params.MultiLiveScoreUpLowerBound)
	}
	if params.UnitFilter != "school_refusal" {
		t.Fatalf("unexpected unit filter: %q", params.UnitFilter)
	}
	if params.MusicQuery != "" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}

func TestEventDeckHandleTreatsBareSingleNumberAsEventID(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动组卡",
		ArgText:    "118",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID == nil || *params.EventID != 118 {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
	if params.MusicQuery != "" {
		t.Fatalf("unexpected music query: %q", params.MusicQuery)
	}
}
