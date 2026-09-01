package handler

import (
	"context"
	"strings"
	"testing"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/testutil"
)

func TestDeckScoreUpHandlerAndExecutionEdges(t *testing.T) {
	handler := (sekaiHandlers{}).ScoreUpHandle()
	for _, args := range []string{"", "100 100", "100 bad 100 100 100", "100 -1 100 100 100"} {
		{
			_, err := handler.handleFunc(mysekaiEdgeContext(args))
			testutil.Require(t, !(err == nil), "score-up accepted %q", args)
		}

	}
	request, err := handler.handleFunc(mysekaiEdgeContext("160 150 140 130 120"))
	{
		testutil.Require(t, !(err != nil), "score-up request = %#v, %v", request, err)
		testutil.Require(t, !(request == nil), "score-up request = %#v, %v", request, err)
		testutil.Require(t, !(request.Mode != "deck-score-up"), "score-up request = %#v, %v", request, err)
		testutil.Require(t, strings.Contains(string(request.Params), "实效"), "score-up request = %#v, %v", request, err)
	}

	message, err := executeDeck(&RequestContext{Ctx: context.Background(), Cmd: request})
	{
		testutil.Require(t, !(err != nil), "score-up execution = %#v, %v", message, err)
		testutil.Require(t, !(len(message) != 1), "score-up execution = %#v, %v", message, err)
		testutil.Require(t, !(message[0].Type != onebot11.TypeText), "score-up execution = %#v, %v", message, err)
	}

	bad := &RequestContext{Ctx: context.Background(), Cmd: &CommandRequest{Mode: "deck-score-up", Params: []byte("{")}}
	{
		_, err := executeDeck(bad)
		testutil.RequireArgs(t, !(err == nil), "invalid score-up payload accepted")
	}

	disabled := &RequestContext{
		Cmd: &CommandRequest{Mode: "deck-event"},
		App: &renderapp.App{Config: renderapp.Config{DeckRecommend: renderapp.DeckRecommendConfig{Disable: true, DisableReason: " maintenance "}}},
	}
	message, err = executeDeck(disabled)
	{
		testutil.Require(t, !(err != nil), "disabled deck response = %#v, %v", message, err)
		testutil.Require(t, !(len(message) != 1), "disabled deck response = %#v, %v", message, err)
		testutil.Require(t, strings.Contains(message[0].Data.(onebot11.TextData).Text, "maintenance"), "disabled deck response = %#v, %v", message, err)
	}
	{

		msg, ok := deckRecommendDisabledMessage(&RequestContext{Cmd: &CommandRequest{Mode: "deck-score-up"}, App: disabled.App})
		{
			testutil.Require(t, !(ok), "non-recommend disabled response = %q, %v", msg, ok)
			testutil.Require(t, !(msg != ""), "non-recommend disabled response = %q, %v", msg, ok)
		}
	}
	testutil.RequireArgs(t, !(isDeckRecommendMode("unknown")), "unknown deck mode treated as recommendation")

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
	{
		testutil.Require(t, !(query.EventID != nil), "preserved MySekai metadata = %+v", query)
		testutil.Require(t, !(query.EventUnit != ""), "preserved MySekai metadata = %+v", query)
		testutil.Require(t, !(query.WorldBloomCharacterID != nil), "preserved MySekai metadata = %+v", query)
		testutil.Require(t, !(query.MetadataWorldBloomCharacterID == nil), "preserved MySekai metadata = %+v", query)
		testutil.Require(t, !(*query.MetadataWorldBloomCharacterID != character), "preserved MySekai metadata = %+v", query)
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
		{
			_, err := handler.handleFunc(target)
			testutil.Require(t, !(err == nil), "self-only handler %s accepted target", handler.Path)
		}

	}
}

func TestMusicBoardParameterErrorAndCompactEdges(t *testing.T) {
	{
		query, err := buildMusicBoardParams("")
		{
			testutil.Require(t, !(err != nil), "empty board params = %+v, %v", query, err)
			testutil.Require(t, !(query.Target != ""), "empty board params = %+v, %v", query, err)
		}
	}

	query, err := buildMusicBoardParams("多人 时间效率 升序 平均 综合20w 加成250% 间隔3s master")
	testutil.Require(t, !(err != nil), "full board params: %v", err)
	{
		testutil.Require(t, !(query.LiveType != "multi"), "full board params = %+v", query)
		testutil.Require(t, !(query.Target != "pt/time"), "full board params = %+v", query)
		testutil.Require(t, query.Ascend, "full board params = %+v", query)
		testutil.Require(t, !(query.Power != 200_000), "full board params = %+v", query)
		testutil.Require(t, !(query.DeckBonus != 250), "full board params = %+v", query)
		testutil.Require(t, !(query.PlayInterval != 3), "full board params = %+v", query)
	}

	for _, args := range []string{
		"多人 pt 综合bad",
		"多人 pt 加成bad",
		"多人 时间效率 间隔bad",
	} {
		{
			_, err := buildMusicBoardParams(args)
			testutil.Require(t, !(err == nil), "invalid board params %q accepted", args)
		}

	}

	for _, field := range []string{"歌曲", "short", "songmas", "song*master"} {
		testutil.Require(t, looksLikeCompactMusicBoardSpecQuery(field), "compact music query %q rejected", field)

	}
	for _, field := range []string{"", "TOOLONGUPPER", "bad!query"} {
		testutil.Require(t, !(looksLikeCompactMusicBoardSpecQuery(field)), "non-compact music query %q accepted", field)

	}
	testutil.RequireArgs(t, !(shouldSplitMusicBoardSpecQueriesByWhitespace([]string{"short", "bad!query"})), "mixed board queries unexpectedly split")
	{

		value, rest, err := extractMusicBoardPower("plain text")
		{
			testutil.Require(t, !(err != nil), "missing power = %d, %q, %v", value, rest, err)
			testutil.Require(t, !(value != 0), "missing power = %d, %q, %v", value, rest, err)
			testutil.Require(t, !(rest != "plain text"), "missing power = %d, %q, %v", value, rest, err)
		}
	}
	{

		value, rest, err := extractMusicBoardDeckBonus("plain text")
		{
			testutil.Require(t, !(err != nil), "missing bonus = %v, %q, %v", value, rest, err)
			testutil.Require(t, !(value != 0), "missing bonus = %v, %q, %v", value, rest, err)
			testutil.Require(t, !(rest != "plain text"), "missing bonus = %v, %q, %v", value, rest, err)
		}
	}
	{

		value, rest, err := extractMusicBoardInterval("plain text")
		{
			testutil.Require(t, !(err != nil), "missing interval = %v, %q, %v", value, rest, err)
			testutil.Require(t, !(value != 0), "missing interval = %v, %q, %v", value, rest, err)
			testutil.Require(t, !(rest != "plain text"), "missing interval = %v, %q, %v", value, rest, err)
		}
	}

}
