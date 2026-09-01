package handler

import (
	"context"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/testutil"
	"reflect"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/parser"
)

func TestDeckAutoQueryParamsJSONRoundTripPreservesExtendedFields(t *testing.T) {
	original := deckAutoQueryParams{
		Boost:                      intPtr(5),
		AreaItemLevel:              intPtr(15),
		Selector:                   "u2",
		UnitFilter:                 "idol",
		AttrFilter:                 "cool",
		ExcludedCards:              []int{123, 456},
		UseCurrentDeck:             true,
		MaxProfile:                 true,
		SubMaxProfile:              true,
		SupportMasterMax:           true,
		SupportSkillMax:            true,
		MusicCompare:               true,
		MusicCompareQueries:        []string{"龙hard", "虾expert", "sage"},
		SpecificSkillOrder:         []int{0, 1, 2, 3, 4},
		WorldBloomFinaleTurn:       intPtr(3),
		ForcedLeaderCharacterID:    intPtr(21),
		ForcedLeaderCharacterQuery: "miku",
	}

	data, err := json.Marshal(original)
	testutil.Require(t, !(err != nil), "marshal params: %v", err)

	var decoded deckAutoQueryParams
	{
		err := json.Unmarshal(data, &decoded)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(decoded.Boost == nil), "unexpected boost: %+v", decoded.Boost)
		testutil.Require(t, !(*decoded.Boost != 5), "unexpected boost: %+v", decoded.Boost)
	}
	{
		testutil.Require(t, !(decoded.AreaItemLevel == nil), "unexpected area item level: %+v", decoded.AreaItemLevel)
		testutil.Require(t, !(*decoded.AreaItemLevel != 15), "unexpected area item level: %+v", decoded.AreaItemLevel)
	}
	testutil.Require(t, !(decoded.Selector != "u2"), "unexpected selector: %q", decoded.Selector)
	{
		testutil.Require(t, !(decoded.UnitFilter != "idol"), "unexpected filters: unit=%q attr=%q", decoded.UnitFilter, decoded.AttrFilter)
		testutil.Require(t, !(decoded.AttrFilter != "cool"), "unexpected filters: unit=%q attr=%q", decoded.UnitFilter, decoded.AttrFilter)
	}
	testutil.Require(t, reflect.DeepEqual(decoded.ExcludedCards, []int{123, 456}), "unexpected excluded cards: %+v", decoded.ExcludedCards)
	{
		testutil.Require(t, decoded.UseCurrentDeck, "unexpected flags: %+v", decoded)
		testutil.Require(t, decoded.MaxProfile, "unexpected flags: %+v", decoded)
		testutil.Require(t, decoded.SubMaxProfile, "unexpected flags: %+v", decoded)
		testutil.Require(t, decoded.SupportMasterMax, "unexpected flags: %+v", decoded)
		testutil.Require(t, decoded.SupportSkillMax, "unexpected flags: %+v", decoded)
		testutil.Require(t, decoded.MusicCompare, "unexpected flags: %+v", decoded)
	}
	testutil.Require(t, reflect.DeepEqual(decoded.MusicCompareQueries, []string{"龙hard", "虾expert", "sage"}), "unexpected music compare queries: %+v", decoded.MusicCompareQueries)
	testutil.Require(t, reflect.DeepEqual(decoded.SpecificSkillOrder, []int{0, 1, 2, 3, 4}), "unexpected specific skill order: %+v", decoded.SpecificSkillOrder)
	{
		testutil.Require(t, !(decoded.WorldBloomFinaleTurn == nil), "unexpected world bloom finale turn: %+v", decoded.WorldBloomFinaleTurn)
		testutil.Require(t, !(*decoded.WorldBloomFinaleTurn != 3), "unexpected world bloom finale turn: %+v", decoded.WorldBloomFinaleTurn)
	}
	{
		testutil.Require(t, !(decoded.ForcedLeaderCharacterID == nil), "unexpected forced leader id: %+v", decoded.ForcedLeaderCharacterID)
		testutil.Require(t, !(*decoded.ForcedLeaderCharacterID != 21), "unexpected forced leader id: %+v", decoded.ForcedLeaderCharacterID)
	}
	testutil.Require(t, !(decoded.ForcedLeaderCharacterQuery != "miku"), "unexpected forced leader query: %q", decoded.ForcedLeaderCharacterQuery)

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
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
	{

		testutil.Require(t, !(resolved.Module != parser.ModuleDeck), "unexpected command request: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "deck-event"), "unexpected command request: %+v", resolved)
	}

	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.EventID == nil), "unexpected event id: %+v", params.EventID)
		testutil.Require(t, !(*params.EventID != 123), "unexpected event id: %+v", params.EventID)
	}
	{
		testutil.

			// "miku" is now resolved to character ID 21 by the extractor
			Require(t, !(params.WorldBloomCharacterID == nil), "unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
		testutil.
			Require(t, !(*params.WorldBloomCharacterID != 21), "unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
	}
	testutil.Require(t, !(params.WorldBloomCharacterQuery != ""), "unexpected world bloom character query: %q", params.WorldBloomCharacterQuery)
	testutil.Require(t, !(params.LiveType != "auto"), "unexpected live type: %q", params.LiveType)
	testutil.Require(t, !(params.Target != "skill"), "unexpected target: %q", params.Target)
	{
		testutil.Require(t, !(len(params.FixedCards) != 2), "unexpected fixed cards: %+v", params.FixedCards)
		testutil.Require(t, !(params.FixedCards[0] != 123), "unexpected fixed cards: %+v", params.FixedCards)
		testutil.Require(t, !(params.FixedCards[1] != 456), "unexpected fixed cards: %+v", params.FixedCards)
	}
	{
		testutil.Require(t, !(params.Rarity1Config == nil), "unexpected rarity patch: %+v", params.Rarity1Config)
		testutil.Require(t, params.Rarity1Config.SkillMax, "unexpected rarity patch: %+v", params.Rarity1Config)
	}

}

func TestEventDeckHandleParsesMixedFixedCharactersAndCards(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/组卡",
		ArgText:    "180 #len #1237",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	var params deckAutoQueryParams
	{
		err := json.Unmarshal(result.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.EventID == nil), "unexpected event id: %+v", params.EventID)
		testutil.Require(t, !(*params.EventID != 180), "unexpected event id: %+v", params.EventID)
	}
	testutil.Require(t, reflect.DeepEqual(params.FixedCharacters, []int{23}), "unexpected fixed characters: %+v", params.FixedCharacters)
	testutil.Require(t, reflect.DeepEqual(params.FixedCards, []int{1237}), "unexpected fixed cards: %+v", params.FixedCards)
	testutil.Require(t, !(len(params.FixedCharacterQueries) != 0), "unexpected fixed character queries: %+v", params.FixedCharacterQueries)

}

func TestEventDeckHandleParsesVirtualSingerSingleRuneAliases(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/组卡",
		ArgText:    "180 冰 #葱 #橘 #蕉 #鱼 #酒",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	var params deckAutoQueryParams
	{
		err := json.Unmarshal(result.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.ForcedLeaderCharacterID == nil), "unexpected forced leader character: %+v", params.ForcedLeaderCharacterID)
		testutil.Require(t, !(*params.ForcedLeaderCharacterID != 26), "unexpected forced leader character: %+v", params.ForcedLeaderCharacterID)
	}
	testutil.Require(t, reflect.DeepEqual(params.FixedCharacters, []int{21, 22, 23, 24, 25}), "unexpected fixed characters: %+v", params.FixedCharacters)
	testutil.Require(t, !(len(params.FixedCharacterQueries) != 0), "unexpected fixed character queries: %+v", params.FixedCharacterQueries)

}

func TestEventDeckHandleParsesFinalChapterForcedLeader(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/组卡",
		ArgText:    "180 miku #len #1237",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	var params deckAutoQueryParams
	{
		err := json.Unmarshal(result.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.EventID == nil), "unexpected event id: %+v", params.EventID)
		testutil.Require(t, !(*params.EventID != 180), "unexpected event id: %+v", params.EventID)
	}
	{
		testutil.Require(t, !(params.ForcedLeaderCharacterID == nil), "unexpected forced leader character: %+v", params.ForcedLeaderCharacterID)
		testutil.Require(t, !(*params.ForcedLeaderCharacterID != 21), "unexpected forced leader character: %+v", params.ForcedLeaderCharacterID)
	}
	{
		testutil.Require(t, !(params.WorldBloomCharacterID != nil), "unexpected world bloom selection: id=%+v query=%q", params.WorldBloomCharacterID, params.WorldBloomCharacterQuery)
		testutil.Require(t, !(params.WorldBloomCharacterQuery != ""), "unexpected world bloom selection: id=%+v query=%q", params.WorldBloomCharacterID, params.WorldBloomCharacterQuery)
	}
	testutil.Require(t, reflect.DeepEqual(params.FixedCharacters, []int{23}), "unexpected fixed characters: %+v", params.FixedCharacters)
	testutil.Require(t, reflect.DeepEqual(params.FixedCards, []int{1237}), "unexpected fixed cards: %+v", params.FixedCards)

}

func TestEventDeckHandleParsesWorldBloomFinaleTurnAndForcedLeader(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/组卡",
		ArgText:    "wl3 终章 akt",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	var params deckAutoQueryParams
	{
		err := json.Unmarshal(result.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.EventID != nil), "unexpected hard-coded event id: %+v", params.EventID)
	{
		testutil.Require(t, !(params.WorldBloomFinaleTurn == nil), "unexpected world bloom finale turn: %+v", params.WorldBloomFinaleTurn)
		testutil.Require(t, !(*params.WorldBloomFinaleTurn != 3), "unexpected world bloom finale turn: %+v", params.WorldBloomFinaleTurn)
	}
	{
		testutil.Require(t, !(params.ForcedLeaderCharacterID == nil), "unexpected forced leader character: %+v", params.ForcedLeaderCharacterID)
		testutil.Require(t, !(*params.ForcedLeaderCharacterID != 11), "unexpected forced leader character: %+v", params.ForcedLeaderCharacterID)
	}
	{
		testutil.Require(t, !(params.WorldBloomCharacterID != nil), "unexpected world bloom selection: id=%+v query=%q", params.WorldBloomCharacterID, params.WorldBloomCharacterQuery)
		testutil.Require(t, !(params.WorldBloomCharacterQuery != ""), "unexpected world bloom selection: id=%+v query=%q", params.WorldBloomCharacterID, params.WorldBloomCharacterQuery)
	}

}

func TestEventDeckHandleParsesSupportMaxOptionsWithoutAffectingMainDeck(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/活动组卡",
		ArgText:    "event123 支援满突破满技能",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)
	testutil.RequireArgs(t, !(result == nil), "expected command request, got nil")

	var params deckAutoQueryParams
	{
		err := json.Unmarshal(result.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, params.SupportMasterMax, "unexpected support flags: %+v", params)
		testutil.Require(t, params.SupportSkillMax, "unexpected support flags: %+v", params)
	}
	{
		testutil.Require(t, !(params.Rarity1Config != nil), "support options should not leak into main deck config: %+v", params)
		testutil.Require(t, !(params.Rarity2Config != nil), "support options should not leak into main deck config: %+v", params)
		testutil.Require(t, !(params.Rarity3Config != nil), "support options should not leak into main deck config: %+v", params)
		testutil.Require(t, !(params.Rarity4Config != nil), "support options should not leak into main deck config: %+v", params)
		testutil.Require(t, !(params.RarityBirthdayConfig != nil), "support options should not leak into main deck config: %+v", params)
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
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.Selector != "u2"), "unexpected selector: %q", params.Selector)
	{
		testutil.Require(t, !(params.EventID == nil), "unexpected event id: %+v", params.EventID)
		testutil.Require(t, !(*params.EventID != 123), "unexpected event id: %+v", params.EventID)
	}
	testutil.Require(t, params.UseCurrentDeck, "expected use_current_deck to be enabled")
	testutil.Require(t, !(params.MusicQuery != "sage neo"), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesCompactASCIIQueryDifficultySuffix(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/活动组卡",
		ArgText:    "segaex",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.MusicQuery != "sega"), "unexpected compact music query: query=%q diff=%q", params.MusicQuery, params.MusicDiff)
		testutil.Require(t, !(params.MusicDiff != "expert"), "unexpected compact music query: query=%q diff=%q", params.MusicQuery, params.MusicDiff)
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
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.Selector != "u1"), "unexpected selector: %q", params.Selector)
	{
		testutil.Require(t, !(params.EventID == nil), "unexpected event id: %+v", params.EventID)
		testutil.Require(t, !(*params.EventID != 123), "unexpected event id: %+v", params.EventID)
	}
	testutil.Require(t, !(params.MusicQuery != "sage neo"), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandlePrefersLastLiveTypeKeyword(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "solo auto",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.LiveType != "auto"), "unexpected live type: %q", params.LiveType)
	testutil.Require(t, !(params.MusicQuery != ""), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesSimulatedEvent(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "25h 可爱",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.EventUnit != "school_refusal"), "unexpected simulate event params: %+v", params)
		testutil.Require(t, !(params.EventAttr != "cute"), "unexpected simulate event params: %+v", params)
	}
	testutil.Require(t, !(params.EventID != nil), "simulated event should not set event id: %+v", params.EventID)

}

func TestEventDeckHandlePrefersSimulatedEventOverBareNumericEventFor25(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "25 蓝",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.EventID != nil), "simulated event should not set event id: %+v", params.EventID)
	{
		testutil.Require(t, !(params.EventUnit != "school_refusal"), "unexpected simulate event params: %+v", params)
		testutil.Require(t, !(params.EventAttr != "cool"), "unexpected simulate event params: %+v", params)
	}

}

func TestEventDeckHandleParsesMultiSkillLowerBound(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "多人 230实效 Song A",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.LiveType != "multi"), "unexpected live type: %q", params.LiveType)
	testutil.Require(t, !(params.Target != ""), "unexpected target: %q", params.Target)
	{
		testutil.Require(t, !(params.MultiLiveTeammateScoreUp == nil), "unexpected teammate score up: %+v", params.MultiLiveTeammateScoreUp)
		testutil.Require(t, !(*params.MultiLiveTeammateScoreUp != 230), "unexpected teammate score up: %+v", params.MultiLiveTeammateScoreUp)
	}
	{
		testutil.Require(t, !(params.MultiLiveScoreUpLowerBound == nil), "unexpected score up lower bound: %+v", params.MultiLiveScoreUpLowerBound)
		testutil.Require(t, !(*params.MultiLiveScoreUpLowerBound != 230), "unexpected score up lower bound: %+v", params.MultiLiveScoreUpLowerBound)
	}
	testutil.Require(t, !(params.MusicQuery != "song a"), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesSplitTeammateScoreUp(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "多人 队友实效 210 Song A",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.LiveType != "multi"), "unexpected live type: %q", params.LiveType)
	testutil.Require(t, !(params.Target != ""), "unexpected target: %q", params.Target)
	{
		testutil.Require(t, !(params.MultiLiveTeammateScoreUp == nil), "unexpected teammate score up: %+v", params.MultiLiveTeammateScoreUp)
		testutil.Require(t, !(*params.MultiLiveTeammateScoreUp != 210), "unexpected teammate score up: %+v", params.MultiLiveTeammateScoreUp)
	}
	testutil.Require(t, !(params.MultiLiveScoreUpLowerBound != nil), "teammate score up should not set score up lower bound: %+v", params.MultiLiveScoreUpLowerBound)
	testutil.Require(t, !(params.MusicQuery != "song a"), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesBareSkillTargetAfterMusicQuery(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "三星满破满技能 四星禁用 已读 画布 龙hd 实效",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.Target != "skill"), "unexpected target: %q", params.Target)
	testutil.Require(t, !(params.MusicDiff != "hard"), "unexpected music diff: %q", params.MusicDiff)
	testutil.Require(t, !(params.MusicQuery != "龙"), "unexpected music query: %q", params.MusicQuery)
	testutil.Require(t, !(params.MultiLiveScoreUpLowerBound != nil), "bare skill target should not set score up lower bound: %+v", params.MultiLiveScoreUpLowerBound)
	{
		testutil.Require(t, !(params.Rarity3Config == nil), "unexpected rarity 3 config: %+v", params.Rarity3Config)
		testutil.Require(t, params.Rarity3Config.MasterMax, "unexpected rarity 3 config: %+v", params.Rarity3Config)
		testutil.Require(t, params.Rarity3Config.SkillMax, "unexpected rarity 3 config: %+v", params.Rarity3Config)
	}
	{
		testutil.Require(t, !(params.Rarity4Config == nil), "unexpected rarity 4 config: %+v", params.Rarity4Config)
		testutil.Require(t, params.Rarity4Config.Disable, "unexpected rarity 4 config: %+v", params.Rarity4Config)
	}
	{
		testutil.Require(t, !(params.Rarity1Config == nil), "unexpected global config propagation: %+v", params.Rarity1Config)
		testutil.Require(t, params.Rarity1Config.EpisodeRead, "unexpected global config propagation: %+v", params.Rarity1Config)
		testutil.Require(t, params.Rarity1Config.Canvas, "unexpected global config propagation: %+v", params.Rarity1Config)
	}

}

func TestEventDeckHandleParsesSplitSkillLowerBound(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "多人 230 实效",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.Target != ""), "unexpected target: %q", params.Target)
	{
		testutil.Require(t, !(params.MultiLiveTeammateScoreUp == nil), "unexpected teammate score up: %+v", params.MultiLiveTeammateScoreUp)
		testutil.Require(t, !(*params.MultiLiveTeammateScoreUp != 230), "unexpected teammate score up: %+v", params.MultiLiveTeammateScoreUp)
	}
	{
		testutil.Require(t, !(params.MultiLiveScoreUpLowerBound == nil), "unexpected score up lower bound: %+v", params.MultiLiveScoreUpLowerBound)
		testutil.Require(t, !(*params.MultiLiveScoreUpLowerBound != 230), "unexpected score up lower bound: %+v", params.MultiLiveScoreUpLowerBound)
	}

}

func TestEventDeckHandleParsesSimulatedWorldBloomTurnAndCharacter(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "miku wl4",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	var params deckAutoQueryParams
	{
		err := json.Unmarshal(result.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.EventID != nil), "unexpected explicit event id: %+v", params.EventID)
	{
		testutil.Require(t, !(params.WorldBloomEventTurn == nil), "unexpected wl event turn: %+v", params.WorldBloomEventTurn)
		testutil.Require(t, !(*params.WorldBloomEventTurn != 4), "unexpected wl event turn: %+v", params.WorldBloomEventTurn)
	}
	{
		testutil.Require(t, !(params.WorldBloomCharacterID == nil), "unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
		testutil.Require(t, !(*params.WorldBloomCharacterID != 21), "unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
	}
	testutil.Require(t, !(params.MusicQuery != ""), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesSimulatedWorldBloomTurnAndAsciiAlias(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动组卡",
		ArgText:    "wl2 mzk",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	var params deckAutoQueryParams
	{
		err := json.Unmarshal(result.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.EventID != nil), "unexpected explicit event id: %+v", params.EventID)
	{
		testutil.Require(t, !(params.WorldBloomEventTurn == nil), "unexpected wl event turn: %+v", params.WorldBloomEventTurn)
		testutil.Require(t, !(*params.WorldBloomEventTurn != 2), "unexpected wl event turn: %+v", params.WorldBloomEventTurn)
	}
	{
		testutil.Require(t, !(params.WorldBloomCharacterID == nil), "unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
		testutil.Require(t, !(*params.WorldBloomCharacterID != 20), "unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
	}
	testutil.Require(t, !(params.MusicQuery != ""), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesSimulatedWorldBloomTurnWithTrailingMusicQuery(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "sage wl3 初音未来",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	var params deckAutoQueryParams
	{
		err := json.Unmarshal(result.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.WorldBloomEventTurn == nil), "unexpected wl event turn: %+v", params.WorldBloomEventTurn)
		testutil.Require(t, !(*params.WorldBloomEventTurn != 3), "unexpected wl event turn: %+v", params.WorldBloomEventTurn)
	}
	{
		testutil.Require(t, !(params.WorldBloomCharacterID == nil), "unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
		testutil.Require(t, !(*params.WorldBloomCharacterID != 21), "unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
	}
	testutil.Require(t, !(params.MusicQuery != "sage"), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesWorldBloomTurnCharacterAndMusicQuery(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "wl1 miku 虾 ex",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	var params deckAutoQueryParams
	{
		err := json.Unmarshal(result.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.WorldBloomEventTurn == nil), "unexpected wl event turn: %+v", params.WorldBloomEventTurn)
		testutil.Require(t, !(*params.WorldBloomEventTurn != 1), "unexpected wl event turn: %+v", params.WorldBloomEventTurn)
	}
	{
		testutil.Require(t, !(params.WorldBloomCharacterID == nil), "unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
		testutil.Require(t, !(*params.WorldBloomCharacterID != 21), "unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
	}
	testutil.Require(t, !(params.MusicQuery != "虾"), "unexpected music query: %q", params.MusicQuery)
	testutil.Require(t, !(params.MusicDiff != "expert"), "unexpected music diff: %q", params.MusicDiff)

}

func TestEventDeckHandlePreservesWorldBloomCharacterQueryAfterEventID(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 初音未来",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.EventID == nil), "unexpected event id: %+v", params.EventID)
		testutil.Require(t, !(*params.EventID != 123), "unexpected event id: %+v", params.EventID)
	}
	{
		testutil.

			// "初音未来" is resolved to character ID 21
			Require(t, !(params.WorldBloomCharacterID == nil), "unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
		testutil.
			Require(t, !(*params.WorldBloomCharacterID != 21), "unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
	}
	testutil.Require(t, !(params.WorldBloomCharacterQuery != ""), "unexpected world bloom character query: %q", params.WorldBloomCharacterQuery)

}

func TestEventDeckHandleRejectsDeprecatedWorldBloomChapterSelectorAfterEventID(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "140 wl3 sage",
	})
	testutil.Require(t, !(err == nil), "expected deprecated wl chapter selector to be rejected")
	testutil.Require(t, strings.Contains(err.Error(), "不再支持 wl2 这种 WL 章节写法"), "unexpected error: %v", err)

}

func TestEventDeckHandleRejectsDeprecatedStandaloneWorldBloomChapterSelector(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动组卡",
		ArgText:    "sage wl3",
	})
	testutil.Require(t, !(err == nil), "expected deprecated wl chapter selector to be rejected")
	testutil.Require(t, strings.Contains(err.Error(), "不再支持 wl2 这种 WL 章节写法"), "unexpected error: %v", err)

}

func TestEventDeckHandleParsesMaxProfile(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 顶配 sage neo",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, params.MaxProfile, "expected max_profile to be enabled")
	testutil.Require(t, !(params.MusicQuery != "sage neo"), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesSubMaxProfile(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 次顶配 sage neo",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, params.SubMaxProfile, "expected sub_max_profile to be enabled")
	testutil.Require(t, !(params.MusicQuery != "sage neo"), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesCurrentDeck(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 当前 sage neo",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, params.UseCurrentDeck, "expected use_current_deck to be enabled")
	testutil.Require(t, !(params.MusicQuery != "sage neo"), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesMusicCompareCurrent(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "歌曲比较 当前",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, params.MusicCompare, "unexpected compare current params: %+v", params)
		testutil.Require(t, params.UseCurrentDeck, "unexpected compare current params: %+v", params)
	}
	testutil.Require(t, !(len(params.MusicCompareQueries) != 0), "unexpected music compare queries: %+v", params.MusicCompareQueries)
	testutil.Require(t, !(params.MusicQuery != ""), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesMusicCompareQueriesAcrossKeyword(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "龙hard 歌曲比较 虾expert sage",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, params.MusicCompare, "expected music_compare to be enabled")
	testutil.Require(t, reflect.DeepEqual(params.MusicCompareQueries, []string{"龙hard", "虾expert", "sage"}), "unexpected music compare queries: %+v", params.MusicCompareQueries)
	testutil.Require(t, !(params.MusicQuery != ""), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleRejectsTooManyMusicCompareQueries(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "歌曲比较 a b c d e f",
	})
	testutil.Require(t, !(err == nil), "expected too many compare songs to fail")
	testutil.Require(t, strings.Contains(err.Error(), "最多只能指定 5 首歌曲"), "unexpected error: %v", err)

}

func TestEventDeckHandleParsesUnitFilter(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 仅vs sage neo",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.UnitFilter != "piapro"), "unexpected unit filter: %q", params.UnitFilter)
	testutil.Require(t, !(params.MusicQuery != "sage neo"), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesAttrFilter(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 仅紫 sage neo",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.AttrFilter != "mysterious"), "unexpected attr filter: %q", params.AttrFilter)
	testutil.Require(t, !(params.MusicQuery != "sage neo"), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesExcludedCards(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 sage neo -123 -456",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, reflect.DeepEqual(params.ExcludedCards, []int{123, 456}), "unexpected excluded cards: %+v", params.ExcludedCards)
	testutil.Require(t, !(params.MusicQuery != "sage neo"), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesAreaItemLevel(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 区域道具15级 sage neo",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.AreaItemLevel == nil), "unexpected area item level: %+v", params.AreaItemLevel)
		testutil.Require(t, !(*params.AreaItemLevel != 15), "unexpected area item level: %+v", params.AreaItemLevel)
	}
	testutil.Require(t, !(params.MusicQuery != "sage neo"), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesAreaItemLevelShorthand(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 15级 当前 sage neo",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.AreaItemLevel == nil), "unexpected area item level: %+v", params.AreaItemLevel)
		testutil.Require(t, !(*params.AreaItemLevel != 15), "unexpected area item level: %+v", params.AreaItemLevel)
	}
	testutil.Require(t, params.UseCurrentDeck, "expected use_current_deck to be enabled")
	testutil.Require(t, !(params.MusicQuery != "sage neo"), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesSkillOrderAverage(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 技能顺序平均 sage neo",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.SkillOrderChooseStrategy != "average"), "unexpected skill order choose strategy: %q", params.SkillOrderChooseStrategy)
	testutil.Require(t, !(len(params.SpecificSkillOrder) != 0), "unexpected specific skill order: %+v", params.SpecificSkillOrder)
	testutil.Require(t, !(params.MusicQuery != "sage neo"), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesSpecificSkillOrderWithCurrent(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 当前 技能顺序12345 sage neo",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, params.UseCurrentDeck, "expected use_current_deck to be enabled")
	testutil.Require(t, !(params.SkillOrderChooseStrategy != "specific"), "unexpected skill order choose strategy: %q", params.SkillOrderChooseStrategy)
	testutil.Require(t, reflect.DeepEqual(params.SpecificSkillOrder, []int{0, 1, 2, 3, 4}), "unexpected specific skill order: %+v", params.SpecificSkillOrder)
	testutil.Require(t, !(params.MusicQuery != "sage neo"), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesSpecificSkillOrderWithFixedCards(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 sage neo 技能顺序15234 #1 2 3 4 5",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, reflect.DeepEqual(params.FixedCards, []int{1, 2, 3, 4, 5}), "unexpected fixed cards: %+v", params.FixedCards)
	testutil.Require(t, !(params.SkillOrderChooseStrategy != "specific"), "unexpected skill order choose strategy: %q", params.SkillOrderChooseStrategy)
	testutil.Require(t, reflect.DeepEqual(params.SpecificSkillOrder, []int{0, 4, 1, 2, 3}), "unexpected specific skill order: %+v", params.SpecificSkillOrder)
	testutil.Require(t, !(params.MusicQuery != "sage neo"), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleRejectsSpecificSkillOrderWithoutCompleteFixedDeck(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 技能顺序12345 sage neo",
	})
	testutil.Require(t, !(err == nil), "expected specific skill order without fixed deck to fail")
	testutil.Require(t, strings.Contains(err.Error(), "仅在使用固定队伍"), "unexpected error: %v", err)

}

func TestEventDeckHandleRejectsSpecificSkillOrderWithFixedCharacters(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 sage neo 技能顺序12345 #miku rin",
	})
	testutil.Require(t, !(err == nil), "expected fixed characters with specific skill order to fail")
	testutil.Require(t, strings.Contains(err.Error(), "仅在使用固定队伍"), "unexpected error: %v", err)

}

func TestBonusDeckHandleParsesEventAndBonuses(t *testing.T) {
	h := sekaiHandlers{}.BonusDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/加成组卡",
		ArgText:    "event123 120 160",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.EventID == nil), "unexpected event id: %+v", params.EventID)
		testutil.Require(t, !(*params.EventID != 123), "unexpected event id: %+v", params.EventID)
	}
	{
		testutil.Require(t, !(len(params.TargetBonuses) != 2), "unexpected bonuses: %+v", params.TargetBonuses)
		testutil.Require(t, !(params.TargetBonuses[0] != 120), "unexpected bonuses: %+v", params.TargetBonuses)
		testutil.Require(t, !(params.TargetBonuses[1] != 160), "unexpected bonuses: %+v", params.TargetBonuses)
	}

}

func TestBonusDeckHandleParsesBonusKeywords(t *testing.T) {
	h := sekaiHandlers{}.BonusDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/加成组卡",
		ArgText:    "event123 120加成 160%",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.EventID == nil), "unexpected event id: %+v", params.EventID)
		testutil.Require(t, !(*params.EventID != 123), "unexpected event id: %+v", params.EventID)
	}
	{
		testutil.Require(t, !(len(params.TargetBonuses) != 2), "unexpected bonuses: %+v", params.TargetBonuses)
		testutil.Require(t, !(params.TargetBonuses[0] != 120), "unexpected bonuses: %+v", params.TargetBonuses)
		testutil.Require(t, !(params.TargetBonuses[1] != 160), "unexpected bonuses: %+v", params.TargetBonuses)
	}

}

func TestBonusDeckHandleTreatsBareNumericLeadingValueAsBonusTarget(t *testing.T) {
	h := sekaiHandlers{}.BonusDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/加成组卡",
		ArgText:    "123 120",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.EventID != nil), "unexpected event id: %+v", params.EventID)
	testutil.Require(t, reflect.DeepEqual(params.TargetBonuses, []int{123, 120}), "unexpected target bonuses: %+v", params.TargetBonuses)

}

func TestChallengeDeckHandleParsesCharacterAndAuto(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/挑战组卡",
		ArgText:    "miku auto",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.
			// "miku" is resolved to character ID 21
			Require(t, !(params.ChallengeLiveCharacterID == nil), "unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
		testutil.
			Require(t, !(*params.ChallengeLiveCharacterID != 21), "unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
	}
	testutil.Require(t, !(params.ChallengeLiveCharacterQuery != ""), "unexpected challenge character query: %q", params.ChallengeLiveCharacterQuery)
	testutil.Require(t, !(params.LiveType != "auto"), "unexpected live type: %q", params.LiveType)

}

func TestChallengeDeckHandleAllowsAllCharactersWhenCharacterOmitted(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/挑战组卡",
		ArgText:    "",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.ChallengeLiveCharacterID != nil), "unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
	testutil.Require(t, !(params.ChallengeLiveCharacterQuery != ""), "unexpected challenge character query: %q", params.ChallengeLiveCharacterQuery)
	{
		testutil.Require(t, !(params.MusicQuery != defaultChallengeDeckMusicQuery), "unexpected default music: query=%q diff=%q", params.MusicQuery, params.MusicDiff)
		testutil.Require(t, !(params.MusicDiff != defaultChallengeDeckMusicDiff), "unexpected default music: query=%q diff=%q", params.MusicQuery, params.MusicDiff)
	}

}

func TestChallengeDeckHandleTreatsInlineDifficultyTokenAsMusicQuery(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/挑战组卡",
		ArgText:    "群青ex",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.ChallengeLiveCharacterID != nil), "unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
	testutil.Require(t, !(params.ChallengeLiveCharacterQuery != ""), "unexpected challenge character query: %q", params.ChallengeLiveCharacterQuery)
	testutil.Require(t, !(params.MusicQuery != "群青"), "unexpected music query: %q", params.MusicQuery)
	testutil.Require(t, !(params.MusicDiff != "expert"), "unexpected music diff: %q", params.MusicDiff)

}

func TestChallengeDeckHandleParsesCurrentKeywordWithoutCharacter(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/挑战组卡",
		ArgText:    "当前",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, params.UseCurrentDeck, "expected use_current_deck to be enabled")
	testutil.Require(t, !(params.ChallengeLiveCharacterID != nil), "unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
	{
		testutil.Require(t, !(params.MusicQuery != defaultChallengeDeckMusicQuery), "unexpected default music: query=%q diff=%q", params.MusicQuery, params.MusicDiff)
		testutil.Require(t, !(params.MusicDiff != defaultChallengeDeckMusicDiff), "unexpected default music: query=%q diff=%q", params.MusicQuery, params.MusicDiff)
	}

}

func TestChallengeDeckHandleParsesCharacterAndCurrentKeyword(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/挑战组卡",
		ArgText:    "miku 当前",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, params.UseCurrentDeck, "expected use_current_deck to be enabled")
	{
		testutil.Require(t, !(params.ChallengeLiveCharacterID == nil), "unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
		testutil.Require(t, !(*params.ChallengeLiveCharacterID != 21), "unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
	}
	{
		testutil.Require(t, !(params.MusicQuery != defaultChallengeDeckMusicQuery), "unexpected default music: query=%q diff=%q", params.MusicQuery, params.MusicDiff)
		testutil.Require(t, !(params.MusicDiff != defaultChallengeDeckMusicDiff), "unexpected default music: query=%q diff=%q", params.MusicQuery, params.MusicDiff)
	}

}

func TestChallengeDeckHandleParsesMusicCompareQueries(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/挑战组卡",
		ArgText:    "miku 歌曲比较 10th 群青apd",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.ChallengeLiveCharacterID == nil), "unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
		testutil.Require(t, !(*params.ChallengeLiveCharacterID != 21), "unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
	}
	testutil.Require(t, params.MusicCompare, "expected music_compare to be enabled")
	testutil.Require(t, reflect.DeepEqual(params.MusicCompareQueries, []string{"10th", "群青apd"}), "unexpected music compare queries: %+v", params.MusicCompareQueries)
	testutil.Require(t, !(params.MusicQuery != ""), "unexpected music query: %q", params.MusicQuery)

}

func TestChallengeDeckHandleParsesMusicCompareAliasQueries(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/挑战组卡",
		ArgText:    "mzk 歌曲对比 群青apd 火花apd",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.ChallengeLiveCharacterID == nil), "unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
		testutil.Require(t, !(*params.ChallengeLiveCharacterID != 20), "unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
	}
	testutil.Require(t, params.MusicCompare, "expected music_compare to be enabled")
	testutil.Require(t, reflect.DeepEqual(params.MusicCompareQueries, []string{"群青apd", "火花apd"}), "unexpected music compare queries: %+v", params.MusicCompareQueries)
	testutil.Require(t, !(params.MusicQuery != ""), "unexpected music query: %q", params.MusicQuery)

}

func TestChallengeDeckHandlePreservesCharacterQuery(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/挑战组卡",
		ArgText:    "初音未来 auto",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.
			// "初音未来" is resolved to character ID 21
			Require(t, !(params.ChallengeLiveCharacterID == nil), "unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
		testutil.
			Require(t, !(*params.ChallengeLiveCharacterID != 21), "unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
	}
	testutil.Require(t, !(params.ChallengeLiveCharacterQuery != ""), "unexpected challenge character query: %q", params.ChallengeLiveCharacterQuery)
	testutil.Require(t, !(params.LiveType != "auto"), "unexpected live type: %q", params.LiveType)

}

func TestMysekaiDeckHandleParsesEventAndFixedCharacter(t *testing.T) {
	h := sekaiHandlers{}.MysekaiDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/ms组卡",
		ArgText:    "event123 #miku",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var combined mysekaiDeckCombinedParams
	{
		err := json.Unmarshal(resolved.Params, &combined)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	params := combined.Deck
	{
		testutil.Require(t, !(params.EventID == nil), "unexpected event id: %+v", params.EventID)
		testutil.Require(t, !(*params.EventID != 123), "unexpected event id: %+v", params.EventID)
	}
	{
		testutil.

			// "#miku" is now resolved to character ID 21
			Require(t, !(len(params.FixedCharacters) != 1), "unexpected fixed characters: %+v", params.FixedCharacters)
		testutil.
			Require(t, !(params.FixedCharacters[0] != 21), "unexpected fixed characters: %+v", params.FixedCharacters)
	}
	testutil.Require(t, !(len(params.FixedCharacterQueries) != 0), "unexpected fixed character queries: %+v", params.FixedCharacterQueries)

}

func TestMysekaiDeckHandleParsesMusicCompareQueries(t *testing.T) {
	h := sekaiHandlers{}.MysekaiDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/ms组卡",
		ArgText:    "歌曲比较 龙hard 虾expert",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var combined mysekaiDeckCombinedParams
	{
		err := json.Unmarshal(resolved.Params, &combined)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, combined.Deck.MusicCompare, "expected music_compare to be enabled")
	testutil.Require(t, reflect.DeepEqual(combined.Deck.MusicCompareQueries, []string{"龙hard", "虾expert"}), "unexpected music compare queries: %+v", combined.Deck.MusicCompareQueries)

}

func TestMysekaiDeckHandlePreservesFixedCharacterQueries(t *testing.T) {
	h := sekaiHandlers{}.MysekaiDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/ms组卡",
		ArgText:    "event123 #初音未来 巡音流歌",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var combined mysekaiDeckCombinedParams
	{
		err := json.Unmarshal(resolved.Params, &combined)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	params := combined.Deck
	{
		testutil.Require(t, !(params.EventID == nil), "unexpected event id: %+v", params.EventID)
		testutil.Require(t, !(*params.EventID != 123), "unexpected event id: %+v", params.EventID)
	}
	{
		testutil.

			// "初音未来" resolves to 21, "巡音流歌" resolves to 24
			Require(t, !(len(params.FixedCharacters) != 2), "unexpected fixed character ids: %+v", params.FixedCharacters)
		testutil.
			Require(t, !(params.FixedCharacters[0] != 21), "unexpected fixed character ids: %+v", params.FixedCharacters)
		testutil.
			Require(t, !(params.FixedCharacters[1] != 24), "unexpected fixed character ids: %+v", params.FixedCharacters)
	}
	testutil.Require(t, !(len(params.FixedCharacterQueries) != 0), "unexpected fixed character queries: %+v", params.FixedCharacterQueries)

}

func TestEventDeckHandleParsesMusicQueryAndDifficulty(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "Tell Your World ex",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.MusicQuery != "tell your world"), "unexpected music query: %q", params.MusicQuery)
	testutil.Require(t, !(params.MusicDiff != "expert"), "unexpected music diff: %q", params.MusicDiff)

}

func TestEventDeckHandleParsesMusicQueryAndDifficultyWithoutSpace(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "190 满画布 已读 虾ex 10火",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.EventID == nil), "unexpected event id: %+v", params.EventID)
		testutil.Require(t, !(*params.EventID != 190), "unexpected event id: %+v", params.EventID)
	}
	testutil.Require(t, !(params.MusicQuery != "虾"), "unexpected music query: %q", params.MusicQuery)
	testutil.Require(t, !(params.MusicDiff != "expert"), "unexpected music diff: %q", params.MusicDiff)

}

func TestEventDeckHandleParsesExplicitMusicID(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "music123 ex",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.MusicID == nil), "unexpected music id: %+v", params.MusicID)
		testutil.Require(t, !(*params.MusicID != 123), "unexpected music id: %+v", params.MusicID)
	}
	testutil.Require(t, !(params.MusicQuery != ""), "unexpected music query: %q", params.MusicQuery)
	testutil.Require(t, !(params.MusicDiff != "expert"), "unexpected music diff: %q", params.MusicDiff)

}

func TestEventDeckHandleKeepsBareNumericQuery(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "123 ex",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.MusicID != nil), "unexpected music id: %+v", params.MusicID)
	testutil.Require(t, !(params.MusicQuery != "123"), "unexpected music query: %q", params.MusicQuery)
	testutil.Require(t, !(params.MusicDiff != "expert"), "unexpected music diff: %q", params.MusicDiff)

}

func TestEventDeckHandleRecognizesNicknameAliasAfterEventID(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/缁勫崱",
		ArgText:    "event123 tks",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.EventID == nil), "unexpected event id: %+v", params.EventID)
		testutil.Require(t, !(*params.EventID != 123), "unexpected event id: %+v", params.EventID)
	}
	{
		testutil.Require(t, !(params.WorldBloomCharacterID == nil), "unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
		testutil.Require(t, !(*params.WorldBloomCharacterID != 13), "unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
	}
	testutil.Require(t, !(params.WorldBloomCharacterQuery != ""), "unexpected world bloom character query: %q", params.WorldBloomCharacterQuery)

}

func TestEventDeckHandleRecognizesLeadingNumericEventIDAndStripsBoostToken(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "163 tks sage 5火",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.EventID == nil), "unexpected event id: %+v", params.EventID)
		testutil.Require(t, !(*params.EventID != 163), "unexpected event id: %+v", params.EventID)
	}
	{
		testutil.Require(t, !(params.WorldBloomCharacterID == nil), "unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
		testutil.Require(t, !(*params.WorldBloomCharacterID != 13), "unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
	}
	{
		testutil.Require(t, !(params.Boost == nil), "unexpected boost: %+v", params.Boost)
		testutil.Require(t, !(*params.Boost != 5), "unexpected boost: %+v", params.Boost)
	}
	testutil.Require(t, !(params.MusicQuery != "sage"), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleStripsBoostTokenAfterExplicitEventID(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event163 sage neo 5火",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.EventID == nil), "unexpected event id: %+v", params.EventID)
		testutil.Require(t, !(*params.EventID != 163), "unexpected event id: %+v", params.EventID)
	}
	{
		testutil.Require(t, !(params.Boost == nil), "unexpected boost: %+v", params.Boost)
		testutil.Require(t, !(*params.Boost != 5), "unexpected boost: %+v", params.Boost)
	}
	testutil.Require(t, !(params.MusicQuery != "sage neo"), "unexpected music query: %q", params.MusicQuery)

}

func TestChallengeDeckHandleRecognizesNicknameAlias(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/鎸戞垬缁勫崱",
		ArgText:    "tks auto",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.ChallengeLiveCharacterID == nil), "unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
		testutil.Require(t, !(*params.ChallengeLiveCharacterID != 13), "unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
	}
	testutil.Require(t, !(params.ChallengeLiveCharacterQuery != ""), "unexpected challenge character query: %q", params.ChallengeLiveCharacterQuery)
	testutil.Require(t, !(params.LiveType != "auto"), "unexpected live type: %q", params.LiveType)

}

func TestMysekaiDeckHandleRecognizesFixedCharacterAlias(t *testing.T) {
	h := sekaiHandlers{}.MysekaiDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/ms缁勫崱",
		ArgText:    "event123 #tks",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var combined mysekaiDeckCombinedParams
	{
		err := json.Unmarshal(resolved.Params, &combined)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	params := combined.Deck
	{
		testutil.Require(t, !(len(params.FixedCharacters) != 1), "unexpected fixed character ids: %+v", params.FixedCharacters)
		testutil.Require(t, !(params.FixedCharacters[0] != 13), "unexpected fixed character ids: %+v", params.FixedCharacters)
	}
	testutil.Require(t, !(len(params.FixedCharacterQueries) != 0), "unexpected fixed character queries: %+v", params.FixedCharacterQueries)

}

func TestNoEventDeckHandleRecognizesBoostToken(t *testing.T) {
	h := sekaiHandlers{}.NoEventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/最强组卡",
		ArgText:    "sage 5火",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.Boost == nil), "unexpected boost: %+v", params.Boost)
		testutil.Require(t, !(*params.Boost != 5), "unexpected boost: %+v", params.Boost)
	}
	testutil.Require(t, !(params.MusicQuery != "sage"), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleParsesNewAlgorithmAliases(t *testing.T) {
	testCases := []struct {
		name      string
		argText   string
		algorithm string
		music     string
	}{
		{
			name:      "hybrid alias",
			argText:   "event123 dfs_ga sage neo",
			algorithm: "dfs_ga",
			music:     "sage neo",
		},
		{
			name:      "rl",
			argText:   "event123 rl sage neo",
			algorithm: "rl",
			music:     "sage neo",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := sekaiHandlers{}.EventDeckHandle()
			result, err := h.Handle(&PjskHandlerContext{
				Context:    context.Background(),
				TriggerCmd: "/组卡",
				ArgText:    tc.argText,
			})
			testutil.Require(t, !(err != nil), "Handle() error = %v", err)

			var params deckAutoQueryParams
			{
				err := json.Unmarshal(result.Params, &params)
				testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
			}

			testutil.Require(t, !(params.Algorithm != tc.algorithm), "unexpected algorithm: %q", params.Algorithm)
			testutil.Require(t, !(params.MusicQuery != tc.music), "unexpected music query: %q", params.MusicQuery)

		})
	}
}

func TestMysekaiDeckHandleRecognizesBoostToken(t *testing.T) {
	h := sekaiHandlers{}.MysekaiDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/ms组卡",
		ArgText:    "event123 5火",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var combined mysekaiDeckCombinedParams
	{
		err := json.Unmarshal(resolved.Params, &combined)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(combined.Deck.EventID == nil), "unexpected event id: %+v", combined.Deck.EventID)
		testutil.Require(t, !(*combined.Deck.EventID != 123), "unexpected event id: %+v", combined.Deck.EventID)
	}
	{
		testutil.Require(t, !(combined.Deck.Boost == nil), "unexpected boost: %+v", combined.Deck.Boost)
		testutil.Require(t, !(*combined.Deck.Boost != 5), "unexpected boost: %+v", combined.Deck.Boost)
	}

}

func TestEventDeckHandleRecognizesChinese25JiAlias(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "25时 紫",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.EventUnit != "school_refusal"), "unexpected simulated event params: %+v", params)
		testutil.Require(t, !(params.EventAttr != "mysterious"), "unexpected simulated event params: %+v", params)
	}
	testutil.Require(t, !(params.MusicQuery != ""), "unexpected music query: %q", params.MusicQuery)

}

func TestNoEventDeckHandleRejectsAttrOnlyAliasAsSongQuery(t *testing.T) {
	h := sekaiHandlers{}.NoEventDeckHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/最强组卡",
		ArgText:    "紫月",
	})
	testutil.Require(t, !(err == nil), "expected attr-only alias to trigger simulated-event hint")
	testutil.Require(t, strings.Contains(err.Error(), "/组卡 团名 属性"), "unexpected error: %v", err)

}

func TestNoEventDeckHandleRejectsFullAliasesWithNoEventHint(t *testing.T) {
	h := sekaiHandlers{}.NoEventDeckHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/最强组卡",
		ArgText:    "25时 蓝星",
	})
	testutil.Require(t, !(err == nil), "expected no-event deck to reject simulated event aliases")
	testutil.Require(t, strings.Contains(err.Error(), "/组卡 团名 属性"), "unexpected error: %v", err)

}

func TestEventDeckHandleRejectsDeprecatedWorldBloomSelectorAndCharacterAfterEventID(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "140 wl3 miku",
	})
	testutil.Require(t, !(err == nil), "expected deprecated WL chapter selector to be rejected")
	testutil.Require(t, strings.Contains(err.Error(), "不再支持 wl2 这种 WL 章节写法"), "unexpected error: %v", err)

}

func TestNoEventDeckHandleAllowsOnly25WithBareSkillTarget(t *testing.T) {
	h := sekaiHandlers{}.NoEventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/最强组卡",
		ArgText:    "仅25 实效",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.Target != "skill"), "unexpected target: %q", params.Target)
	testutil.Require(t, !(params.MultiLiveScoreUpLowerBound != nil), "bare skill target should not set score up lower bound: %+v", params.MultiLiveScoreUpLowerBound)
	testutil.Require(t, !(params.UnitFilter != "school_refusal"), "unexpected unit filter: %q", params.UnitFilter)
	testutil.Require(t, !(params.MusicQuery != ""), "unexpected music query: %q", params.MusicQuery)

}

func TestNoEventDeckHandleAllowsOnly25hWithBareSkillTarget(t *testing.T) {
	h := sekaiHandlers{}.NoEventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/最强组卡",
		ArgText:    "仅25h 实效",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.Target != "skill"), "unexpected target: %q", params.Target)
	testutil.Require(t, !(params.MultiLiveScoreUpLowerBound != nil), "bare skill target should not set score up lower bound: %+v", params.MultiLiveScoreUpLowerBound)
	testutil.Require(t, !(params.UnitFilter != "school_refusal"), "unexpected unit filter: %q", params.UnitFilter)
	testutil.Require(t, !(params.MusicQuery != ""), "unexpected music query: %q", params.MusicQuery)

}

func TestEventDeckHandleTreatsBareSingleNumberAsEventID(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动组卡",
		ArgText:    "118",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	var params deckAutoQueryParams
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.EventID == nil), "unexpected event id: %+v", params.EventID)
		testutil.Require(t, !(*params.EventID != 118), "unexpected event id: %+v", params.EventID)
	}
	testutil.Require(t, !(params.MusicQuery != ""), "unexpected music query: %q", params.MusicQuery)

}
