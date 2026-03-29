package sekai

import (
	"context"
	"encoding/json"
	"testing"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
)

func TestEventDeckHandleParsesCommonOptions(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 miku auto 倍率 满技能 #123 456",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleDeck || resolved.Mode != "deck-event" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID == nil || *params.EventID != 123 {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
	if params.WorldBloomCharacterID != nil {
		t.Fatalf("unexpected world bloom character: %+v", params.WorldBloomCharacterID)
	}
	if params.WorldBloomCharacterQuery != "miku" {
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

func TestEventDeckHandleParsesSimulatedEvent(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "25h 可爱",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result.(*parser.ResolvedCommand)
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

func TestEventDeckHandleParsesSimulatedWorldBloom(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "miku wl2",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result.(*parser.ResolvedCommand)
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.WorldBloomEventTurn == nil || *params.WorldBloomEventTurn != 2 {
		t.Fatalf("unexpected world bloom turn: %+v", params.WorldBloomEventTurn)
	}
	if params.WorldBloomCharacterID != nil {
		t.Fatalf("unexpected world bloom character: %+v", params.WorldBloomCharacterID)
	}
	if params.WorldBloomCharacterQuery != "miku" {
		t.Fatalf("unexpected world bloom character query: %q", params.WorldBloomCharacterQuery)
	}
}

func TestEventDeckHandlePreservesSimulatedWorldBloomCharacterQuery(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "初音未来 wl2",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result.(*parser.ResolvedCommand)
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.WorldBloomEventTurn == nil || *params.WorldBloomEventTurn != 2 {
		t.Fatalf("unexpected world bloom turn: %+v", params.WorldBloomEventTurn)
	}
	if params.WorldBloomCharacterID != nil {
		t.Fatalf("unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
	}
	if params.WorldBloomCharacterQuery != "初音未来" {
		t.Fatalf("unexpected world bloom character query: %q", params.WorldBloomCharacterQuery)
	}
}

func TestEventDeckHandlePreservesWorldBloomCharacterQueryAfterEventID(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "event123 初音未来",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result.(*parser.ResolvedCommand)
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID == nil || *params.EventID != 123 {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
	if params.WorldBloomCharacterID != nil {
		t.Fatalf("unexpected world bloom character id: %+v", params.WorldBloomCharacterID)
	}
	if params.WorldBloomCharacterQuery != "初音未来" {
		t.Fatalf("unexpected world bloom character query: %q", params.WorldBloomCharacterQuery)
	}
}

func TestBonusDeckHandleParsesEventAndBonuses(t *testing.T) {
	h := sekaiHandlers{}.BonusDeckHandle()
	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/加成组卡",
		ArgText:    "event123 120 160",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result.(*parser.ResolvedCommand)
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

func TestChallengeDeckHandleParsesCharacterAndAuto(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/挑战组卡",
		ArgText:    "miku auto",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result.(*parser.ResolvedCommand)
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.ChallengeLiveCharacterID != nil {
		t.Fatalf("unexpected challenge character: %+v", params.ChallengeLiveCharacterID)
	}
	if params.ChallengeLiveCharacterQuery != "miku" {
		t.Fatalf("unexpected challenge character query: %q", params.ChallengeLiveCharacterQuery)
	}
	if params.LiveType != "auto" {
		t.Fatalf("unexpected live type: %q", params.LiveType)
	}
}

func TestChallengeDeckHandlePreservesCharacterQuery(t *testing.T) {
	h := sekaiHandlers{}.ChallengeDeckHandle()
	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/挑战组卡",
		ArgText:    "初音未来 auto",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result.(*parser.ResolvedCommand)
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.ChallengeLiveCharacterID != nil {
		t.Fatalf("unexpected challenge character id: %+v", params.ChallengeLiveCharacterID)
	}
	if params.ChallengeLiveCharacterQuery != "初音未来" {
		t.Fatalf("unexpected challenge character query: %q", params.ChallengeLiveCharacterQuery)
	}
	if params.LiveType != "auto" {
		t.Fatalf("unexpected live type: %q", params.LiveType)
	}
}

func TestMysekaiDeckHandleParsesEventAndFixedCharacter(t *testing.T) {
	h := sekaiHandlers{}.MysekaiDeckHandle()
	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/ms组卡",
		ArgText:    "event123 #miku",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result.(*parser.ResolvedCommand)
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID == nil || *params.EventID != 123 {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
	if len(params.FixedCharacters) != 0 {
		t.Fatalf("unexpected fixed characters: %+v", params.FixedCharacters)
	}
	if len(params.FixedCharacterQueries) != 1 || params.FixedCharacterQueries[0] != "miku" {
		t.Fatalf("unexpected fixed character queries: %+v", params.FixedCharacterQueries)
	}
}

func TestMysekaiDeckHandlePreservesFixedCharacterQueries(t *testing.T) {
	h := sekaiHandlers{}.MysekaiDeckHandle()
	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/ms组卡",
		ArgText:    "event123 #初音未来 巡音流歌",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result.(*parser.ResolvedCommand)
	var params deckAutoQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID == nil || *params.EventID != 123 {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
	if len(params.FixedCharacters) != 0 {
		t.Fatalf("unexpected fixed character ids: %+v", params.FixedCharacters)
	}
	if len(params.FixedCharacterQueries) != 2 || params.FixedCharacterQueries[0] != "初音未来" || params.FixedCharacterQueries[1] != "巡音流歌" {
		t.Fatalf("unexpected fixed character queries: %+v", params.FixedCharacterQueries)
	}
}

func TestEventDeckHandleParsesMusicQueryAndDifficulty(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "Tell Your World ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result.(*parser.ResolvedCommand)
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

func TestEventDeckHandleParsesExplicitMusicID(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "music123 ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result.(*parser.ResolvedCommand)
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
	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "123 ex",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result.(*parser.ResolvedCommand)
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
