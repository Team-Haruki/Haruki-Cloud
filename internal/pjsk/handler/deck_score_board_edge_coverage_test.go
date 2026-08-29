package handler

import (
	"context"
	"strings"
	"testing"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
)

func TestDeckScoreUpHandlerAndExecutionEdges(t *testing.T) {
	handler := (sekaiHandlers{}).ScoreUpHandle()
	for _, args := range []string{"", "100 100", "100 bad 100 100 100", "100 -1 100 100 100"} {
		if _, err := handler.handleFunc(mysekaiEdgeContext(args)); err == nil {
			t.Fatalf("score-up accepted %q", args)
		}
	}
	request, err := handler.handleFunc(mysekaiEdgeContext("160 150 140 130 120"))
	if err != nil || request == nil || request.Mode != "deck-score-up" || !strings.Contains(string(request.Params), "实效") {
		t.Fatalf("score-up request = %#v, %v", request, err)
	}
	message, err := executeDeck(&RequestContext{Ctx: context.Background(), Cmd: request})
	if err != nil || len(message) != 1 || message[0].Type != onebot11.TypeText {
		t.Fatalf("score-up execution = %#v, %v", message, err)
	}
	bad := &RequestContext{Ctx: context.Background(), Cmd: &CommandRequest{Mode: "deck-score-up", Params: []byte("{")}}
	if _, err := executeDeck(bad); err == nil {
		t.Fatal("invalid score-up payload accepted")
	}

	disabled := &RequestContext{
		Cmd: &CommandRequest{Mode: "deck-event"},
		App: &renderapp.App{Config: renderapp.Config{DeckRecommend: renderapp.DeckRecommendConfig{Disable: true, DisableReason: " maintenance "}}},
	}
	message, err = executeDeck(disabled)
	if err != nil || len(message) != 1 || !strings.Contains(message[0].Data.(onebot11.TextData).Text, "maintenance") {
		t.Fatalf("disabled deck response = %#v, %v", message, err)
	}
	if msg, ok := deckRecommendDisabledMessage(&RequestContext{Cmd: &CommandRequest{Mode: "deck-score-up"}, App: disabled.App}); ok || msg != "" {
		t.Fatalf("non-recommend disabled response = %q, %v", msg, ok)
	}
	if isDeckRecommendMode("unknown") {
		t.Fatal("unknown deck mode treated as recommendation")
	}

	zero := 0
	character := 2
	query := renderdeck.AutoQuery{
		EventID:                  drawing.IntPtr(7),
		EventUnit:                "unit",
		EventAttr:                "cute",
		WorldBloomEventTurn:      drawing.IntPtr(1),
		WorldBloomFinaleTurn:     drawing.IntPtr(1),
		WorldBloomCharacterID:    &character,
		WorldBloomCharacterQuery: "miku",
	}
	preserveImplicitMysekaiWorldBloomMetadata(&query)
	if query.EventID != nil || query.EventUnit != "" || query.WorldBloomCharacterID != nil || query.MetadataWorldBloomCharacterID == nil || *query.MetadataWorldBloomCharacterID != character {
		t.Fatalf("preserved MySekai metadata = %+v", query)
	}
	query.WorldBloomCharacterID = &zero
	preserveImplicitMysekaiWorldBloomMetadata(&query)
	preserveImplicitMysekaiWorldBloomMetadata(nil)
}

func TestDeckSelfOnlyHandlerErrors(t *testing.T) {
	target := mysekaiEdgeContext("")
	target.uidArg = "@target"
	for _, handler := range []HarukiSekaiCommandHandler{
		(sekaiHandlers{}).ChallengeDeckHandle(),
		(sekaiHandlers{}).BonusDeckHandle(),
		(sekaiHandlers{}).MysekaiDeckHandle(),
	} {
		if _, err := handler.handleFunc(target); err == nil {
			t.Fatalf("self-only handler %s accepted target", handler.Path)
		}
	}
}

func TestMusicBoardParameterErrorAndCompactEdges(t *testing.T) {
	if query, err := buildMusicBoardParams(""); err != nil || query.Target != "" {
		t.Fatalf("empty board params = %+v, %v", query, err)
	}
	query, err := buildMusicBoardParams("多人 时间效率 升序 平均 综合20w 加成250% 间隔3s master")
	if err != nil {
		t.Fatalf("full board params: %v", err)
	}
	if query.LiveType != "multi" || query.Target != "pt/time" || !query.Ascend || query.Power != 200_000 || query.DeckBonus != 250 || query.PlayInterval != 3 {
		t.Fatalf("full board params = %+v", query)
	}
	for _, args := range []string{
		"多人 pt 综合bad",
		"多人 pt 加成bad",
		"多人 时间效率 间隔bad",
	} {
		if _, err := buildMusicBoardParams(args); err == nil {
			t.Fatalf("invalid board params %q accepted", args)
		}
	}

	for _, field := range []string{"歌曲", "short", "songmas", "song*master"} {
		if !looksLikeCompactMusicBoardSpecQuery(field) {
			t.Fatalf("compact music query %q rejected", field)
		}
	}
	for _, field := range []string{"", "TOOLONGUPPER", "bad!query"} {
		if looksLikeCompactMusicBoardSpecQuery(field) {
			t.Fatalf("non-compact music query %q accepted", field)
		}
	}
	if shouldSplitMusicBoardSpecQueriesByWhitespace([]string{"short", "bad!query"}) {
		t.Fatal("mixed board queries unexpectedly split")
	}

	if value, rest, err := extractMusicBoardPower("plain text"); err != nil || value != 0 || rest != "plain text" {
		t.Fatalf("missing power = %d, %q, %v", value, rest, err)
	}
	if value, rest, err := extractMusicBoardDeckBonus("plain text"); err != nil || value != 0 || rest != "plain text" {
		t.Fatalf("missing bonus = %v, %q, %v", value, rest, err)
	}
	if value, rest, err := extractMusicBoardInterval("plain text"); err != nil || value != 0 || rest != "plain text" {
		t.Fatalf("missing interval = %v, %q, %v", value, rest, err)
	}
}
