package handler

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	sekaidb "haruki-cloud/database/sekai"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/testutil"
)

func TestDeckWorldBloomFinaleSimulationHelpers(t *testing.T) {
	turn := 3
	{
		err := prepareDeckWorldBloomFinaleSimulation(nil, 3)
		testutil.Require(t, !(err != nil), "nil finale query = %v", err)
	}

	query := &renderdeck.AutoQuery{}
	{
		err := prepareDeckWorldBloomFinaleSimulation(query, 3)
		testutil.RequireArgs(t, !(err == nil), "expected missing leader error")
	}

	leader := 21
	query = &renderdeck.AutoQuery{ForcedLeaderCharacterID: &leader, WorldBloomFinaleTurn: &turn}
	{
		err := prepareDeckWorldBloomFinaleSimulation(query, 3)
		testutil.Require(t, !(err != nil), "ID finale simulation = %v", err)
	}
	{

		testutil.Require(t, !(query.EventID != nil), "ID finale query = %+v", query)
		testutil.Require(t, !(query.WorldBloomFinaleTurn == nil), "ID finale query = %+v", query)
		testutil.Require(t, !(*query.WorldBloomFinaleTurn != 3), "ID finale query = %+v", query)
		testutil.Require(t, !(query.WorldBloomEventTurn != nil), "ID finale query = %+v", query)
		testutil.Require(t, !(query.WorldBloomCharacterID != nil), "ID finale query = %+v", query)
		testutil.Require(t, !(query.WorldBloomCharacterQuery != ""), "ID finale query = %+v", query)
		testutil.Require(t, !(query.EventUnit != ""), "ID finale query = %+v", query)
		testutil.Require(t, !(query.EventAttr != ""), "ID finale query = %+v", query)
		testutil.Require(t, query.MetadataWorldBloomFinale, "ID finale query = %+v", query)
	}

	query = &renderdeck.AutoQuery{ForcedLeaderCharacterQuery: "miku"}
	{
		err := prepareDeckWorldBloomFinaleSimulation(query, 4)
		testutil.Require(t, !(err != nil), "query finale simulation = %v", err)
	}
	{

		testutil.Require(t, !(query.WorldBloomCharacterID != nil), "query finale = %+v", query)
		testutil.Require(t, !(query.WorldBloomCharacterQuery != ""), "query finale = %+v", query)
		testutil.Require(t, !(query.WorldBloomEventTurn != nil), "query finale = %+v", query)
		testutil.Require(t, !(query.WorldBloomFinaleTurn == nil), "query finale = %+v", query)
		testutil.Require(t, !(*query.WorldBloomFinaleTurn != 4), "query finale = %+v", query)
		testutil.Require(t, !(query.ForcedLeaderCharacterQuery != "miku"), "query finale = %+v", query)
	}

}

func TestDeckWorldBloomSelectionNormalization(t *testing.T) {
	ctx := context.Background()
	{
		err := resolveDeckEventAndWorldBloomSelection(ctx, nil, nil, renderregion.JP)
		testutil.Require(t, !(err != nil), "nil query selection = %v", err)
	}

	query := &renderdeck.AutoQuery{RecommendType: "event"}
	{
		err := resolveDeckEventAndWorldBloomSelection(ctx, query, nil, renderregion.JP)
		testutil.Require(t, !(err != nil), "nil app selection = %v", err)
	}

	worldBloomCharacter := 21
	query = &renderdeck.AutoQuery{
		EventID:                  drawing.IntPtr(180),
		WorldBloomCharacterID:    &worldBloomCharacter,
		WorldBloomCharacterQuery: "miku",
		WorldBloomEventTurn:      drawing.IntPtr(2),
		WorldBloomFinaleTurn:     drawing.IntPtr(2),
		EventUnit:                "piapro",
	}
	testutil.RequireArgs(t, isResolvedDeckWorldBloomFinale(query), "event 180 should be finale")

	normalizeDeckWorldBloomFinaleSelection(query)
	{
		testutil.Require(t, query.MetadataWorldBloomFinale, "normalized finale = %+v", query)
		testutil.Require(t, !(query.ForcedLeaderCharacterID == nil), "normalized finale = %+v", query)
		testutil.Require(t, !(*query.ForcedLeaderCharacterID != 21), "normalized finale = %+v", query)
		testutil.Require(t, !(query.WorldBloomCharacterID != nil), "normalized finale = %+v", query)
		testutil.Require(t, !(query.WorldBloomCharacterQuery != ""), "normalized finale = %+v", query)
		testutil.Require(t, !(query.EventUnit != ""), "normalized finale = %+v", query)
	}

	normalizeDeckWorldBloomFinaleSelection(nil)
	testutil.RequireArgs(t, !(isResolvedDeckWorldBloomFinale(nil)), "nil query resolved as finale")

	query = &renderdeck.AutoQuery{MetadataWorldBloomFinale: true, WorldBloomCharacterQuery: "miku"}
	normalizeDeckWorldBloomFinaleSelection(query)
	testutil.Require(t, !(query.ForcedLeaderCharacterQuery != "miku"), "finale character query = %+v", query)

	query = &renderdeck.AutoQuery{MetadataWorldBloomFinale: true, WorldBloomCharacterQuery: "wl2"}
	normalizeDeckWorldBloomFinaleSelection(query)
	testutil.Require(t, !(query.ForcedLeaderCharacterQuery != ""), "selector query leaked into leader = %+v", query)

}

func TestDeckWorldBloomPendingAndCandidateHelpers(t *testing.T) {
	testutil.RequireArgs(t, !(hasPendingDeckWorldBloomSelection(nil)), "nil pending selection")

	turn := 1
	character := 21
	for _, query := range []*renderdeck.AutoQuery{
		{WorldBloomEventTurn: &turn},
		{WorldBloomCharacterID: &character},
		{WorldBloomCharacterQuery: "miku"},
	} {
		testutil.Check(t, hasPendingDeckWorldBloomSelection(query), "pending selection not detected: %+v", query)

	}
	testutil.RequireArgs(t, !(hasPendingDeckWorldBloomSelection(&renderdeck.AutoQuery{})), "empty selection reported pending")

	for _, query := range []*renderdeck.AutoQuery{nil, {}} {
		{
			err := resolveDeckWorldBloomTurnCharacterSelection(context.Background(), query, nil, renderregion.JP)
			testutil.Require(t, !(err != nil), "guarded character selection = %v", err)
		}

	}
	query := &renderdeck.AutoQuery{WorldBloomCharacterID: &character}
	{
		err := resolveDeckWorldBloomTurnCharacterSelection(context.Background(), query, &renderapp.App{}, renderregion.JP)
		testutil.Require(t, !(err != nil), "resolved character selection = %v", err)
	}

	query = &renderdeck.AutoQuery{WorldBloomCharacterQuery: "wl2"}
	{
		err := resolveDeckWorldBloomTurnCharacterSelection(context.Background(), query, &renderapp.App{}, renderregion.JP)
		testutil.Require(t, !(err != nil), "selector character selection = %v", err)
	}

	chapters := []*sekaidb.Worldbloom{{GameCharacterID: 21}}
	{
		err := tryResolveDeckMusicQueryAsWorldBloomCharacter(context.Background(), nil, nil, renderregion.JP, chapters)
		testutil.Require(t, !(err != nil), "nil music character query = %v", err)
	}

	query = &renderdeck.AutoQuery{}
	{
		err := tryResolveDeckMusicQueryAsWorldBloomCharacter(context.Background(), query, &renderapp.App{}, renderregion.JP, chapters)
		testutil.Require(t, !(err != nil), "empty music character query = %v", err)
	}

	query = &renderdeck.AutoQuery{MusicQuery: "miku"}
	{
		err := tryResolveDeckMusicQueryAsWorldBloomCharacter(context.Background(), query, &renderapp.App{}, renderregion.JP, chapters)
		testutil.RequireArgs(t, !(err == nil), "expected missing character resolver error")
	}

	query = &renderdeck.AutoQuery{MusicQuery: "onlyone"}
	{
		err := tryResolveDeckMusicQueryPrefixAsWorldBloomCharacter(context.Background(), query, &renderapp.App{}, renderregion.JP, chapters)
		{
			testutil.Require(t, !(err != nil), "single-token prefix query = %+v, %v", query, err)
			testutil.Require(t, !(query.WorldBloomCharacterID != nil), "single-token prefix query = %+v, %v", query, err)
		}
	}

	query = &renderdeck.AutoQuery{MusicQuery: "miku song"}
	{
		err := tryResolveDeckMusicQueryPrefixAsWorldBloomCharacter(context.Background(), query, &renderapp.App{}, renderregion.JP, chapters)
		testutil.RequireArgs(t, !(err == nil), "expected missing prefix character resolver error")
	}

	query = &renderdeck.AutoQuery{MusicCompare: true, MusicCompareQueries: []string{"miku", "song"}}
	{
		err := tryResolveDeckMusicCompareQueryAsWorldBloomCharacter(context.Background(), query, nil, renderregion.JP, chapters)
		testutil.Require(t, !(err != nil), "compare character query = %v", err)
	}
	{

		testutil.Require(t, !(query.WorldBloomCharacterID == nil), "compare character query = %+v", query)
		testutil.Require(t, !(*query.WorldBloomCharacterID != 21), "compare character query = %+v", query)
		testutil.Require(t, reflect.DeepEqual(query.MusicCompareQueries, []string{"song"}), "compare character query = %+v", query)
	}

	for _, query := range []*renderdeck.AutoQuery{nil, {}, {MusicCompare: true, MusicCompareQueries: []string{" ", ""}}} {
		{
			err := tryResolveDeckMusicCompareQueryAsWorldBloomCharacter(context.Background(), query, nil, renderregion.JP, chapters)
			testutil.Require(t, !(err != nil), "guarded compare query = %v", err)
		}

	}
	{

		id, err := resolveDeckWorldBloomCharacterCandidateID(context.Background(), nil, renderregion.JP, "miku")
		{
			testutil.Require(t, !(err != nil), "nickname candidate = %d, %v", id, err)
			testutil.Require(t, !(id != 21), "nickname candidate = %d, %v", id, err)
		}
	}
	{

		id, err := resolveDeckWorldBloomCharacterCandidateID(context.Background(), nil, renderregion.JP, "unknown")
		{
			testutil.Require(t, !(err != nil), "unknown candidate = %d, %v", id, err)
			testutil.Require(t, !(id != 0), "unknown candidate = %d, %v", id, err)
		}
	}

}

func TestDeckEventSelectionGuardAndTimeHelpers(t *testing.T) {
	for _, value := range []string{"event", "BONUS", " mysekai "} {
		testutil.Check(t, shouldResolveDeckEventByRecommendType(value), "recommend type %q not resolved", value)

	}
	testutil.RequireArgs(t, !(shouldResolveDeckEventByRecommendType("no_event")), "no-event type should not resolve")
	{

		_, err := pickDeckAutoEvent(context.Background(), nil, renderregion.JP, "event")
		testutil.RequireArgs(t, !(err == nil), "expected nil app event error")
	}
	{

		_, err := pickCurrentOrNextDeckEvent(context.Background(), nil, renderregion.JP)
		testutil.RequireArgs(t, !(err == nil), "expected nil app current event error")
	}

	now := time.Now().UnixMilli()
	{
		err := ensureDeckEventUnlocked(context.Background(), nil, renderregion.JP, nil)
		testutil.Require(t, !(err != nil), "nil event unlock = %v", err)
	}
	{

		err := ensureDeckEventUnlocked(context.Background(), nil, renderregion.JP, &sekaidb.Event{StartAt: now - 1})
		testutil.Require(t, !(err != nil), "started event unlock = %v", err)
	}
	{

		err := ensureDeckEventUnlocked(context.Background(), nil, renderregion.JP, &sekaidb.Event{GameID: 1, StartAt: now + 60_000})
		testutil.RequireArgs(t, !(err == nil), "expected locked future JP event")
	}
	{

		err := ensureDeckEventUnlocked(context.Background(), nil, renderregion.CN, &sekaidb.Event{GameID: 1, StartAt: now + 60_000})
		testutil.Require(t, !(err != nil), "future non-JP event with no DB constraint = %v", err)
	}
	{
		testutil.RequireArgs(t, !(pickDeckDefaultWorldBloomChapter(nil, nil) != nil), "invalid chapter input resolved")
		testutil.RequireArgs(t, !(pickDeckDefaultWorldBloomChapter(nil, []*sekaidb.Worldbloom{{}}) != nil), "invalid chapter input resolved")
	}

	one := &sekaidb.Worldbloom{GameCharacterID: 1}
	{
		got := pickDeckDefaultWorldBloomChapter(&sekaidb.Event{}, []*sekaidb.Worldbloom{one})
		testutil.RequireArgs(t, !(got != one), "single chapter not selected")
	}

	first := &sekaidb.Worldbloom{GameCharacterID: 1, ChapterStartAt: now + 1_000, AggregateAt: now + 2_000}
	last := &sekaidb.Worldbloom{GameCharacterID: 2, ChapterStartAt: now + 3_000, AggregateAt: now + 4_000}
	futureEvent := &sekaidb.Event{StartAt: now + 500, AggregateAt: now + 10_000}
	{
		got := pickDeckDefaultWorldBloomChapter(futureEvent, []*sekaidb.Worldbloom{last, nil, first})
		testutil.Require(t, !(got != first), "future first chapter = %+v", got)
	}

	finishedEvent := &sekaidb.Event{StartAt: now - 10_000, AggregateAt: now - 500}
	{
		got := pickDeckDefaultWorldBloomChapter(finishedEvent, []*sekaidb.Worldbloom{first, nil, last})
		testutil.Require(t, !(got != last), "finished last chapter = %+v", got)
	}

	current := &sekaidb.Worldbloom{GameCharacterID: 3, ChapterStartAt: now - 500, AggregateAt: now + 500}
	currentEvent := &sekaidb.Event{StartAt: now - 1_000, AggregateAt: now + 10_000}
	{
		got := pickDeckDefaultWorldBloomChapter(currentEvent, []*sekaidb.Worldbloom{first, current})
		testutil.Require(t, !(got != current), "current chapter = %+v", got)
	}

}

func TestDeckWorldBloomErrorAndClearingHelpers(t *testing.T) {
	for _, query := range []string{"wl", "wl2", "WL future"} {
		testutil.Check(t, isDeckWorldBloomSelectorQuery(query), "selector %q not recognized", query)

	}
	testutil.RequireArgs(t, !(isDeckWorldBloomSelectorQuery("miku")), "character name recognized as selector")
	{
		testutil.RequireArgs(t, !(shouldFallbackDeckEventRecommendToNoEvent(nil)), "fallback recommendation classification mismatch")
		testutil.RequireArgs(t, shouldFallbackDeckEventRecommendToNoEvent(&renderdeck.AutoQuery{RecommendType: "EVENT"}), "fallback recommendation classification mismatch")
		testutil.RequireArgs(t, !(shouldFallbackDeckEventRecommendToNoEvent(&renderdeck.AutoQuery{RecommendType: "bonus"})), "fallback recommendation classification mismatch")
	}

	clearDeckAutoEventSelection(nil)
	query := &renderdeck.AutoQuery{
		EventID:                  drawing.IntPtr(1),
		EventUnit:                "piapro",
		EventAttr:                "cool",
		WorldBloomEventTurn:      drawing.IntPtr(1),
		WorldBloomFinaleTurn:     drawing.IntPtr(2),
		MetadataWorldBloomFinale: true,
		WorldBloomCharacterID:    drawing.IntPtr(21),
		WorldBloomCharacterQuery: "miku",
	}
	clearDeckAutoEventSelection(query)
	{
		testutil.Require(t, !(query.EventID != nil), "cleared selection = %+v", query)
		testutil.Require(t, !(query.EventUnit != ""), "cleared selection = %+v", query)
		testutil.Require(t, !(query.EventAttr != ""), "cleared selection = %+v", query)
		testutil.Require(t, !(query.WorldBloomEventTurn != nil), "cleared selection = %+v", query)
		testutil.Require(t, !(query.MetadataWorldBloomFinale), "cleared selection = %+v", query)
		testutil.Require(t, !(query.WorldBloomCharacterID != nil), "cleared selection = %+v", query)
		testutil.Require(t, !(query.WorldBloomCharacterQuery != ""), "cleared selection = %+v", query)
	}
	{

		_, err := resolveDeckWorldBloomFinaleEventByTurn(context.Background(), nil, renderregion.JP, 1)
		testutil.RequireArgs(t, !(err == nil), "expected invalid finale turn")
	}

	event, err := resolveDeckWorldBloomFinaleEventByTurn(context.Background(), nil, renderregion.JP, 2)
	{
		testutil.Require(t, !(err != nil), "wl2 finale = %+v, %v", event, err)
		testutil.Require(t, !(event.GameID != 180), "wl2 finale = %+v, %v", event, err)
	}
	{

		_, err := resolveDeckWorldBloomFinaleEventByTurn(context.Background(), nil, renderregion.JP, 3)
		testutil.RequireArgs(t, !(err == nil), "expected future finale turn error")
	}

	var nilFinale *deckFutureWorldBloomFinaleTurnError
	testutil.Require(t, !(nilFinale.Error() != "无法解析未来 WL 终章"), "nil finale error = %q", nilFinale.Error())
	{

		got := (&deckFutureWorldBloomFinaleTurnError{Turn: 4, Available: 2}).Error()
		testutil.Require(t, strings.Contains(got, "wl4"), "finale error = %q", got)
	}

	chapters := []*sekaidb.Worldbloom{nil, {WorldBloomChapterType: "normal"}, {WorldBloomChapterType: " Finale "}}
	{
		testutil.RequireArgs(t, deckWorldBloomHasFinaleChapter(chapters), "finale chapter classification mismatch")
		testutil.RequireArgs(t, !(deckWorldBloomHasFinaleChapter(nil)), "finale chapter classification mismatch")
	}
	{

		got := missingDeckWorldBloomChapterError(&renderdeck.AutoQuery{RecommendType: "mysekai"}, 10).Error()
		testutil.Require(t, strings.Contains(got, "烤森组卡"), "MySekai missing chapter = %q", got)
	}
	{

		got := missingDeckWorldBloomChapterError(&renderdeck.AutoQuery{}, 10).Error()
		testutil.Require(t, !(strings.Contains(got, "烤森组卡")), "normal missing chapter = %q", got)
	}
	{

		_, err := resolveDeckWorldBloomEventByTurnSelection(context.Background(), nil, renderregion.JP, nil)
		testutil.RequireArgs(t, !(err == nil), "expected invalid turn selection")
	}

	turn := 1
	{
		_, err := resolveDeckWorldBloomEventByTurnSelection(context.Background(), nil, renderregion.JP, &renderdeck.AutoQuery{WorldBloomEventTurn: &turn})
		testutil.RequireArgs(t, !(err == nil), "expected missing turn target")
	}
	{

		_, err := resolveDeckWorldBloomEventByCharacterTurn(context.Background(), nil, renderregion.JP, 1, 0)
		testutil.RequireArgs(t, !(err == nil), "expected invalid character ID")
	}
	{

		_, err := resolveDeckWorldBloomEventByUnitTurn(context.Background(), nil, renderregion.JP, 1, "bad")
		testutil.RequireArgs(t, !(err == nil), "expected invalid unit")
	}

	var nilTurn *deckFutureWorldBloomTurnError
	testutil.Require(t, !(nilTurn.Error() != "无法解析未来 WL 轮次"), "nil turn error = %q", nilTurn.Error())
	{

		got := (&deckFutureWorldBloomTurnError{Turn: 3, Available: 1, Character: 21}).Error()
		testutil.Require(t, strings.Contains(got, "角色 21"), "character turn error = %q", got)
	}
	{

		got := (&deckFutureWorldBloomTurnError{Turn: 3, Available: 1, Unit: "piapro"}).Error()
		testutil.Require(t, strings.Contains(got, "团 piapro"), "unit turn error = %q", got)
	}
	{
		testutil.RequireArgs(t, !(shouldKeepDeckWorldBloomSimulationSelection(nil)), "simulation retention mismatch")
		testutil.RequireArgs(t, shouldKeepDeckWorldBloomSimulationSelection(&renderdeck.AutoQuery{RecommendType: "event"}), "simulation retention mismatch")
		testutil.RequireArgs(t, !(shouldKeepDeckWorldBloomSimulationSelection(&renderdeck.AutoQuery{RecommendType: "bonus"})), "simulation retention mismatch")
	}

}

func TestDeckWorldBloomDataConversionAndAvailability(t *testing.T) {
	ctx := context.Background()
	{
		_, err := queryDeckWorldBloomEvents(ctx, nil, renderregion.JP)
		{
			testutil.Require(t, !(err == nil), "nil world-bloom query error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "world bloom"), "nil world-bloom query error = %v", err)
		}
	}
	{

		_, err := queryDeckEvents(ctx, nil, renderregion.JP)
		testutil.RequireArgs(t, !(err == nil), "expected nil app event query error")
	}
	{

		_, err := queryDeckEventByID(ctx, nil, renderregion.JP, 0)
		testutil.RequireArgs(t, !(err == nil), "expected missing event ID error")
	}
	{

		_, err := queryDeckEventByID(ctx, nil, renderregion.JP, 1)
		testutil.RequireArgs(t, !(err == nil), "expected missing event provider error")
	}
	{

		ids, err := queryDeckDBEventIDs(ctx, nil, renderregion.JP)
		{
			testutil.Require(t, !(err != nil), "nil DB event IDs = %v, %v", ids, err)
			testutil.Require(t, !(ids != nil), "nil DB event IDs = %v, %v", ids, err)
		}
	}
	{
		testutil.RequireArgs(t, !(deckWorldBloomHasUnit(nil, "piapro")), "empty unit unexpectedly matched")
		testutil.RequireArgs(t, !(deckWorldBloomHasUnit([]*sekaidb.Worldbloom{{GameCharacterID: 21}}, "")), "empty unit unexpectedly matched")
	}
	testutil.RequireArgs(t, deckWorldBloomHasUnit([]*sekaidb.Worldbloom{nil, {GameCharacterID: 0}, {GameCharacterID: 21}}, "piapro"), "piapro chapter unit not matched")
	testutil.RequireArgs(t, !(deckWorldBloomHasUnit([]*sekaidb.Worldbloom{{GameCharacterID: 1}}, "piapro")), "wrong chapter unit matched")

	now := time.Now().UnixMilli()
	testutil.RequireArgs(t, !(isDeckFutureEventAvailable(ctx, nil, renderregion.JP, nil, nil, now)), "nil event available")

	started := &sekaidb.Event{GameID: 1, StartAt: now - 1}
	testutil.RequireArgs(t, isDeckFutureEventAvailable(ctx, nil, renderregion.JP, started, nil, now), "started event unavailable")

	future := &sekaidb.Event{GameID: 2, StartAt: now + 1000}
	testutil.RequireArgs(t, !(isDeckFutureEventAvailable(ctx, nil, renderregion.JP, future, nil, now)), "unreleased JP event available")
	testutil.RequireArgs(t, isDeckFutureEventAvailable(ctx, nil, renderregion.CN, future, nil, now), "future non-JP event without DB set unavailable")
	testutil.RequireArgs(t, isDeckFutureEventAvailable(ctx, nil, renderregion.CN, future, map[int]struct{}{2: {}}, now), "known non-JP event unavailable")
	testutil.RequireArgs(t, !(isDeckFutureEventAvailable(ctx, nil, renderregion.CN, future, map[int]struct{}{}, now)), "unknown non-JP event available")
	testutil.RequireArgs(t, !(deckEventLeakReleased(ctx, nil, 1, now)), "nil provider leak released")
	testutil.RequireArgs(t, !(deckEventProviderForRegion(nil, renderregion.JP) != nil), "nil app returned provider")
	{
		testutil.RequireArgs(t, !(deckEventFromMasterdata(nil) != nil), "nil masterdata converted")
		testutil.RequireArgs(t, !(deckWorldBloomFromMasterdata(nil) != nil), "nil masterdata converted")
	}

	event := deckEventFromMasterdata(&masterdata.Event{ID: 7, EventType: "world_bloom", Unit: "piapro", Name: "WL", AssetBundleName: "bundle", StartAt: 1, AggregateAt: 2, ClosedAt: 3})
	{
		testutil.Require(t, !(event.GameID != 7), "converted event = %+v", event)
		testutil.Require(t, !(event.AssetbundleName != "bundle"), "converted event = %+v", event)
		testutil.Require(t, !(event.EventType != "world_bloom"), "converted event = %+v", event)
	}

	character := 21
	chapter := deckWorldBloomFromMasterdata(&masterdata.WorldBloom{EventID: 7, GameCharacterID: &character, ChapterType: "normal", ChapterNo: 2, ChapterStartAt: 1, AggregateAt: 2, ChapterEndAt: 3, IsSupplemental: true})
	{
		testutil.Require(t, !(chapter.EventID != 7), "converted chapter = %+v", chapter)
		testutil.Require(t, !(chapter.GameCharacterID != 21), "converted chapter = %+v", chapter)
		testutil.Require(t, !(chapter.ChapterNo != 2), "converted chapter = %+v", chapter)
		testutil.Require(t, chapter.IsSupplemental, "converted chapter = %+v", chapter)
	}

	chapter = deckWorldBloomFromMasterdata(&masterdata.WorldBloom{EventID: 8})
	testutil.Require(t, !(chapter.GameCharacterID != 0), "nil character conversion = %+v", chapter)

}

func TestDeckWorldBloomTurnGuardFunctions(t *testing.T) {
	ctx := context.Background()
	{
		got := resolveDeckWorldBloomEventTurn(ctx, nil, renderregion.JP, nil, nil, &renderdeck.AutoQuery{})
		testutil.Require(t, !(got != 0), "nil event turn = %d", got)
	}
	{

		got := resolveDeckWorldBloomEventTurn(ctx, nil, renderregion.JP, &sekaidb.Event{}, nil, nil)
		testutil.Require(t, !(got != 0), "nil query turn = %d", got)
	}

	query := &renderdeck.AutoQuery{}
	ensureDeckWorldBloomEventTurnMetadata(ctx, nil, renderregion.JP, nil, nil, nil)
	ensureDeckWorldBloomEventTurnMetadata(ctx, nil, renderregion.JP, &sekaidb.Event{}, nil, query)
	testutil.Require(t, !(query.MetadataWorldBloomEventTurn != nil), "unexpected turn metadata = %+v", query)

	already := 3
	query.MetadataWorldBloomEventTurn = &already
	ensureDeckWorldBloomEventTurnMetadata(ctx, nil, renderregion.JP, &sekaidb.Event{}, nil, query)
	testutil.RequireArgs(t, !(*query.MetadataWorldBloomEventTurn != 3), "existing metadata changed")
	{

		turn, err := resolveDeckWorldBloomCharacterTurnForEvent(ctx, nil, renderregion.JP, 0, 21)
		{
			testutil.Require(t, !(err != nil), "invalid character turn = %d, %v", turn, err)
			testutil.Require(t, !(turn != 0), "invalid character turn = %d, %v", turn, err)
		}
	}
	{

		turn, err := resolveDeckWorldBloomUnitTurnForEvent(ctx, nil, renderregion.JP, 0, "piapro")
		{
			testutil.Require(t, !(err != nil), "invalid unit event turn = %d, %v", turn, err)
			testutil.Require(t, !(turn != 0), "invalid unit event turn = %d, %v", turn, err)
		}
	}
	{

		turn, err := resolveDeckWorldBloomUnitTurnForEvent(ctx, nil, renderregion.JP, 1, "bad")
		{
			testutil.Require(t, !(err != nil), "invalid unit turn = %d, %v", turn, err)
			testutil.Require(t, !(turn != 0), "invalid unit turn = %d, %v", turn, err)
		}
	}
	{

		_, err := resolveDeckWorldBloomCharacterTurnForEvent(ctx, nil, renderregion.JP, 1, 21)
		testutil.RequireArgs(t, !(err == nil), "expected missing event data error")
	}
	{

		_, err := resolveDeckWorldBloomUnitTurnForEvent(ctx, nil, renderregion.JP, 1, "piapro")
		testutil.RequireArgs(t, !(err == nil), "expected missing unit event data error")
	}

	query = &renderdeck.AutoQuery{WorldBloomEventTurn: drawing.IntPtr(1), WorldBloomCharacterID: drawing.IntPtr(21)}
	{
		_, err := resolveDeckWorldBloomEventByTurnSelection(ctx, nil, renderregion.JP, query)
		testutil.RequireArgs(t, !(err == nil), "expected missing character event data error")
	}

}
