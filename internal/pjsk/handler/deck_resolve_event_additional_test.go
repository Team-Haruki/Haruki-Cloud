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
)

func TestDeckWorldBloomFinaleSimulationHelpers(t *testing.T) {
	if isSimulatedDeckWorldBloomFinale(nil) {
		t.Fatal("nil query classified as finale simulation")
	}
	turn := 3
	query := &renderdeck.AutoQuery{WorldBloomEventTurn: &turn, MetadataWorldBloomFinale: true}
	if !isSimulatedDeckWorldBloomFinale(query) {
		t.Fatal("finale simulation was not recognized")
	}
	eventID := 1
	query.EventID = &eventID
	if isSimulatedDeckWorldBloomFinale(query) {
		t.Fatal("resolved event classified as simulation")
	}

	if err := prepareDeckWorldBloomFinaleSimulation(nil, 3); err != nil {
		t.Fatalf("nil finale query = %v", err)
	}
	query = &renderdeck.AutoQuery{}
	if err := prepareDeckWorldBloomFinaleSimulation(query, 3); err == nil {
		t.Fatal("expected missing leader error")
	}
	leader := 21
	query = &renderdeck.AutoQuery{ForcedLeaderCharacterID: &leader, WorldBloomFinaleTurn: &turn}
	if err := prepareDeckWorldBloomFinaleSimulation(query, 3); err != nil {
		t.Fatalf("ID finale simulation = %v", err)
	}
	if query.EventID != nil || query.WorldBloomFinaleTurn != nil || query.WorldBloomEventTurn == nil || *query.WorldBloomEventTurn != 3 || query.WorldBloomCharacterID == nil || *query.WorldBloomCharacterID != 21 || query.EventUnit != "piapro" {
		t.Fatalf("ID finale query = %+v", query)
	}
	query = &renderdeck.AutoQuery{ForcedLeaderCharacterQuery: "miku"}
	if err := prepareDeckWorldBloomFinaleSimulation(query, 4); err != nil {
		t.Fatalf("query finale simulation = %v", err)
	}
	if query.WorldBloomCharacterID != nil || query.WorldBloomCharacterQuery != "miku" || query.WorldBloomEventTurn == nil || *query.WorldBloomEventTurn != 4 {
		t.Fatalf("query finale = %+v", query)
	}

	if got := prependDeckUniqueCharacter([]int{1, 2, 1}, 1); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("prepended characters = %v", got)
	}
	if got := prependDeckUniqueCharacter(nil, 3); !reflect.DeepEqual(got, []int{3}) {
		t.Fatalf("prepended empty characters = %v", got)
	}
}

func TestDeckWorldBloomSelectionNormalization(t *testing.T) {
	ctx := context.Background()
	if err := resolveDeckEventAndWorldBloomSelection(ctx, nil, nil, renderregion.JP); err != nil {
		t.Fatalf("nil query selection = %v", err)
	}
	query := &renderdeck.AutoQuery{RecommendType: "event"}
	if err := resolveDeckEventAndWorldBloomSelection(ctx, query, nil, renderregion.JP); err != nil {
		t.Fatalf("nil app selection = %v", err)
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
	if !isResolvedDeckWorldBloomFinale(query) {
		t.Fatal("event 180 should be finale")
	}
	normalizeDeckWorldBloomFinaleSelection(query)
	if !query.MetadataWorldBloomFinale || query.ForcedLeaderCharacterID == nil || *query.ForcedLeaderCharacterID != 21 || query.WorldBloomCharacterID != nil || query.WorldBloomCharacterQuery != "" || query.EventUnit != "" {
		t.Fatalf("normalized finale = %+v", query)
	}
	normalizeDeckWorldBloomFinaleSelection(nil)
	if isResolvedDeckWorldBloomFinale(nil) {
		t.Fatal("nil query resolved as finale")
	}

	query = &renderdeck.AutoQuery{MetadataWorldBloomFinale: true, WorldBloomCharacterQuery: "miku"}
	normalizeDeckWorldBloomFinaleSelection(query)
	if query.ForcedLeaderCharacterQuery != "miku" {
		t.Fatalf("finale character query = %+v", query)
	}
	query = &renderdeck.AutoQuery{MetadataWorldBloomFinale: true, WorldBloomCharacterQuery: "wl2"}
	normalizeDeckWorldBloomFinaleSelection(query)
	if query.ForcedLeaderCharacterQuery != "" {
		t.Fatalf("selector query leaked into leader = %+v", query)
	}
}

func TestDeckWorldBloomPendingAndCandidateHelpers(t *testing.T) {
	if hasPendingDeckWorldBloomSelection(nil) {
		t.Fatal("nil pending selection")
	}
	turn := 1
	character := 21
	for _, query := range []*renderdeck.AutoQuery{
		{WorldBloomEventTurn: &turn},
		{WorldBloomCharacterID: &character},
		{WorldBloomCharacterQuery: "miku"},
	} {
		if !hasPendingDeckWorldBloomSelection(query) {
			t.Errorf("pending selection not detected: %+v", query)
		}
	}
	if hasPendingDeckWorldBloomSelection(&renderdeck.AutoQuery{}) {
		t.Fatal("empty selection reported pending")
	}

	for _, query := range []*renderdeck.AutoQuery{nil, {}} {
		if err := resolveDeckWorldBloomTurnCharacterSelection(context.Background(), query, nil, renderregion.JP); err != nil {
			t.Fatalf("guarded character selection = %v", err)
		}
	}
	query := &renderdeck.AutoQuery{WorldBloomCharacterID: &character}
	if err := resolveDeckWorldBloomTurnCharacterSelection(context.Background(), query, &renderapp.App{}, renderregion.JP); err != nil {
		t.Fatalf("resolved character selection = %v", err)
	}
	query = &renderdeck.AutoQuery{WorldBloomCharacterQuery: "wl2"}
	if err := resolveDeckWorldBloomTurnCharacterSelection(context.Background(), query, &renderapp.App{}, renderregion.JP); err != nil {
		t.Fatalf("selector character selection = %v", err)
	}

	chapters := []*sekaidb.Worldbloom{{GameCharacterID: 21}}
	if err := tryResolveDeckMusicQueryAsWorldBloomCharacter(context.Background(), nil, nil, renderregion.JP, chapters); err != nil {
		t.Fatalf("nil music character query = %v", err)
	}
	query = &renderdeck.AutoQuery{}
	if err := tryResolveDeckMusicQueryAsWorldBloomCharacter(context.Background(), query, &renderapp.App{}, renderregion.JP, chapters); err != nil {
		t.Fatalf("empty music character query = %v", err)
	}
	query = &renderdeck.AutoQuery{MusicQuery: "miku"}
	if err := tryResolveDeckMusicQueryAsWorldBloomCharacter(context.Background(), query, &renderapp.App{}, renderregion.JP, chapters); err == nil {
		t.Fatal("expected missing character resolver error")
	}

	query = &renderdeck.AutoQuery{MusicQuery: "onlyone"}
	if err := tryResolveDeckMusicQueryPrefixAsWorldBloomCharacter(context.Background(), query, &renderapp.App{}, renderregion.JP, chapters); err != nil || query.WorldBloomCharacterID != nil {
		t.Fatalf("single-token prefix query = %+v, %v", query, err)
	}
	query = &renderdeck.AutoQuery{MusicQuery: "miku song"}
	if err := tryResolveDeckMusicQueryPrefixAsWorldBloomCharacter(context.Background(), query, &renderapp.App{}, renderregion.JP, chapters); err == nil {
		t.Fatal("expected missing prefix character resolver error")
	}

	query = &renderdeck.AutoQuery{MusicCompare: true, MusicCompareQueries: []string{"miku", "song"}}
	if err := tryResolveDeckMusicCompareQueryAsWorldBloomCharacter(context.Background(), query, nil, renderregion.JP, chapters); err != nil {
		t.Fatalf("compare character query = %v", err)
	}
	if query.WorldBloomCharacterID == nil || *query.WorldBloomCharacterID != 21 || !reflect.DeepEqual(query.MusicCompareQueries, []string{"song"}) {
		t.Fatalf("compare character query = %+v", query)
	}
	for _, query := range []*renderdeck.AutoQuery{nil, {}, {MusicCompare: true, MusicCompareQueries: []string{" ", ""}}} {
		if err := tryResolveDeckMusicCompareQueryAsWorldBloomCharacter(context.Background(), query, nil, renderregion.JP, chapters); err != nil {
			t.Fatalf("guarded compare query = %v", err)
		}
	}

	if id, err := resolveDeckWorldBloomCharacterCandidateID(context.Background(), nil, renderregion.JP, "miku"); err != nil || id != 21 {
		t.Fatalf("nickname candidate = %d, %v", id, err)
	}
	if id, err := resolveDeckWorldBloomCharacterCandidateID(context.Background(), nil, renderregion.JP, "unknown"); err != nil || id != 0 {
		t.Fatalf("unknown candidate = %d, %v", id, err)
	}
}

func TestDeckEventSelectionGuardAndTimeHelpers(t *testing.T) {
	for _, value := range []string{"event", "BONUS", " mysekai "} {
		if !shouldResolveDeckEventByRecommendType(value) {
			t.Errorf("recommend type %q not resolved", value)
		}
	}
	if shouldResolveDeckEventByRecommendType("no_event") {
		t.Fatal("no-event type should not resolve")
	}
	if _, err := pickDeckAutoEvent(context.Background(), nil, renderregion.JP, "event"); err == nil {
		t.Fatal("expected nil app event error")
	}
	if _, err := pickCurrentOrNextDeckEvent(context.Background(), nil, renderregion.JP); err == nil {
		t.Fatal("expected nil app current event error")
	}

	now := time.Now().UnixMilli()
	if err := ensureDeckEventUnlocked(context.Background(), nil, renderregion.JP, nil); err != nil {
		t.Fatalf("nil event unlock = %v", err)
	}
	if err := ensureDeckEventUnlocked(context.Background(), nil, renderregion.JP, &sekaidb.Event{StartAt: now - 1}); err != nil {
		t.Fatalf("started event unlock = %v", err)
	}
	if err := ensureDeckEventUnlocked(context.Background(), nil, renderregion.JP, &sekaidb.Event{GameID: 1, StartAt: now + 60_000}); err == nil {
		t.Fatal("expected locked future JP event")
	}
	if err := ensureDeckEventUnlocked(context.Background(), nil, renderregion.CN, &sekaidb.Event{GameID: 1, StartAt: now + 60_000}); err != nil {
		t.Fatalf("future non-JP event with no DB constraint = %v", err)
	}

	if pickDeckDefaultWorldBloomChapter(nil, nil) != nil || pickDeckDefaultWorldBloomChapter(nil, []*sekaidb.Worldbloom{{}}) != nil {
		t.Fatal("invalid chapter input resolved")
	}
	one := &sekaidb.Worldbloom{GameCharacterID: 1}
	if got := pickDeckDefaultWorldBloomChapter(&sekaidb.Event{}, []*sekaidb.Worldbloom{one}); got != one {
		t.Fatal("single chapter not selected")
	}
	first := &sekaidb.Worldbloom{GameCharacterID: 1, ChapterStartAt: now + 1_000, AggregateAt: now + 2_000}
	last := &sekaidb.Worldbloom{GameCharacterID: 2, ChapterStartAt: now + 3_000, AggregateAt: now + 4_000}
	futureEvent := &sekaidb.Event{StartAt: now + 500, AggregateAt: now + 10_000}
	if got := pickDeckDefaultWorldBloomChapter(futureEvent, []*sekaidb.Worldbloom{last, nil, first}); got != first {
		t.Fatalf("future first chapter = %+v", got)
	}
	finishedEvent := &sekaidb.Event{StartAt: now - 10_000, AggregateAt: now - 500}
	if got := pickDeckDefaultWorldBloomChapter(finishedEvent, []*sekaidb.Worldbloom{first, nil, last}); got != last {
		t.Fatalf("finished last chapter = %+v", got)
	}
	current := &sekaidb.Worldbloom{GameCharacterID: 3, ChapterStartAt: now - 500, AggregateAt: now + 500}
	currentEvent := &sekaidb.Event{StartAt: now - 1_000, AggregateAt: now + 10_000}
	if got := pickDeckDefaultWorldBloomChapter(currentEvent, []*sekaidb.Worldbloom{first, current}); got != current {
		t.Fatalf("current chapter = %+v", got)
	}
}

func TestDeckWorldBloomErrorAndClearingHelpers(t *testing.T) {
	for _, query := range []string{"wl", "wl2", "WL future"} {
		if !isDeckWorldBloomSelectorQuery(query) {
			t.Errorf("selector %q not recognized", query)
		}
	}
	if isDeckWorldBloomSelectorQuery("miku") {
		t.Fatal("character name recognized as selector")
	}
	if shouldFallbackDeckEventRecommendToNoEvent(nil) || !shouldFallbackDeckEventRecommendToNoEvent(&renderdeck.AutoQuery{RecommendType: "EVENT"}) || shouldFallbackDeckEventRecommendToNoEvent(&renderdeck.AutoQuery{RecommendType: "bonus"}) {
		t.Fatal("fallback recommendation classification mismatch")
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
	if query.EventID != nil || query.EventUnit != "" || query.EventAttr != "" || query.WorldBloomEventTurn != nil || query.MetadataWorldBloomFinale || query.WorldBloomCharacterID != nil || query.WorldBloomCharacterQuery != "" {
		t.Fatalf("cleared selection = %+v", query)
	}

	if _, err := resolveDeckWorldBloomFinaleEventByTurn(context.Background(), nil, renderregion.JP, 1); err == nil {
		t.Fatal("expected invalid finale turn")
	}
	event, err := resolveDeckWorldBloomFinaleEventByTurn(context.Background(), nil, renderregion.JP, 2)
	if err != nil || event.GameID != 180 {
		t.Fatalf("wl2 finale = %+v, %v", event, err)
	}
	if _, err := resolveDeckWorldBloomFinaleEventByTurn(context.Background(), nil, renderregion.JP, 3); err == nil {
		t.Fatal("expected future finale turn error")
	}
	var nilFinale *deckFutureWorldBloomFinaleTurnError
	if nilFinale.Error() != "无法解析未来 WL 终章" {
		t.Fatalf("nil finale error = %q", nilFinale.Error())
	}
	if got := (&deckFutureWorldBloomFinaleTurnError{Turn: 4, Available: 2}).Error(); !strings.Contains(got, "wl4") {
		t.Fatalf("finale error = %q", got)
	}

	chapters := []*sekaidb.Worldbloom{nil, {WorldBloomChapterType: "normal"}, {WorldBloomChapterType: " Finale "}}
	if !deckWorldBloomHasFinaleChapter(chapters) || deckWorldBloomHasFinaleChapter(nil) {
		t.Fatal("finale chapter classification mismatch")
	}
	if got := missingDeckWorldBloomChapterError(&renderdeck.AutoQuery{RecommendType: "mysekai"}, 10).Error(); !strings.Contains(got, "烤森组卡") {
		t.Fatalf("MySekai missing chapter = %q", got)
	}
	if got := missingDeckWorldBloomChapterError(&renderdeck.AutoQuery{}, 10).Error(); strings.Contains(got, "烤森组卡") {
		t.Fatalf("normal missing chapter = %q", got)
	}

	if _, err := resolveDeckWorldBloomEventByTurnSelection(context.Background(), nil, renderregion.JP, nil); err == nil {
		t.Fatal("expected invalid turn selection")
	}
	turn := 1
	if _, err := resolveDeckWorldBloomEventByTurnSelection(context.Background(), nil, renderregion.JP, &renderdeck.AutoQuery{WorldBloomEventTurn: &turn}); err == nil {
		t.Fatal("expected missing turn target")
	}
	if _, err := resolveDeckWorldBloomEventByCharacterTurn(context.Background(), nil, renderregion.JP, 1, 0); err == nil {
		t.Fatal("expected invalid character ID")
	}
	if _, err := resolveDeckWorldBloomEventByUnitTurn(context.Background(), nil, renderregion.JP, 1, "bad"); err == nil {
		t.Fatal("expected invalid unit")
	}
	var nilTurn *deckFutureWorldBloomTurnError
	if nilTurn.Error() != "无法解析未来 WL 轮次" {
		t.Fatalf("nil turn error = %q", nilTurn.Error())
	}
	if got := (&deckFutureWorldBloomTurnError{Turn: 3, Available: 1, Character: 21}).Error(); !strings.Contains(got, "角色 21") {
		t.Fatalf("character turn error = %q", got)
	}
	if got := (&deckFutureWorldBloomTurnError{Turn: 3, Available: 1, Unit: "piapro"}).Error(); !strings.Contains(got, "团 piapro") {
		t.Fatalf("unit turn error = %q", got)
	}
	if shouldKeepDeckWorldBloomSimulationSelection(nil) || !shouldKeepDeckWorldBloomSimulationSelection(&renderdeck.AutoQuery{RecommendType: "event"}) || shouldKeepDeckWorldBloomSimulationSelection(&renderdeck.AutoQuery{RecommendType: "bonus"}) {
		t.Fatal("simulation retention mismatch")
	}
}

func TestDeckWorldBloomDataConversionAndAvailability(t *testing.T) {
	ctx := context.Background()
	if _, err := queryDeckWorldBloomEvents(ctx, nil, renderregion.JP); err == nil || !strings.Contains(err.Error(), "world bloom") {
		t.Fatalf("nil world-bloom query error = %v", err)
	}
	if _, err := queryDeckEvents(ctx, nil, renderregion.JP); err == nil {
		t.Fatal("expected nil app event query error")
	}
	if _, err := queryDeckEventByID(ctx, nil, renderregion.JP, 0); err == nil {
		t.Fatal("expected missing event ID error")
	}
	if _, err := queryDeckEventByID(ctx, nil, renderregion.JP, 1); err == nil {
		t.Fatal("expected missing event provider error")
	}
	if ids, err := queryDeckDBEventIDs(ctx, nil, renderregion.JP); err != nil || ids != nil {
		t.Fatalf("nil DB event IDs = %v, %v", ids, err)
	}

	if deckWorldBloomHasUnit(nil, "piapro") || deckWorldBloomHasUnit([]*sekaidb.Worldbloom{{GameCharacterID: 21}}, "") {
		t.Fatal("empty unit unexpectedly matched")
	}
	if !deckWorldBloomHasUnit([]*sekaidb.Worldbloom{nil, {GameCharacterID: 0}, {GameCharacterID: 21}}, "piapro") {
		t.Fatal("piapro chapter unit not matched")
	}
	if deckWorldBloomHasUnit([]*sekaidb.Worldbloom{{GameCharacterID: 1}}, "piapro") {
		t.Fatal("wrong chapter unit matched")
	}

	now := time.Now().UnixMilli()
	if isDeckFutureEventAvailable(ctx, nil, renderregion.JP, nil, nil, now) {
		t.Fatal("nil event available")
	}
	started := &sekaidb.Event{GameID: 1, StartAt: now - 1}
	if !isDeckFutureEventAvailable(ctx, nil, renderregion.JP, started, nil, now) {
		t.Fatal("started event unavailable")
	}
	future := &sekaidb.Event{GameID: 2, StartAt: now + 1000}
	if isDeckFutureEventAvailable(ctx, nil, renderregion.JP, future, nil, now) {
		t.Fatal("unreleased JP event available")
	}
	if !isDeckFutureEventAvailable(ctx, nil, renderregion.CN, future, nil, now) {
		t.Fatal("future non-JP event without DB set unavailable")
	}
	if !isDeckFutureEventAvailable(ctx, nil, renderregion.CN, future, map[int]struct{}{2: {}}, now) {
		t.Fatal("known non-JP event unavailable")
	}
	if isDeckFutureEventAvailable(ctx, nil, renderregion.CN, future, map[int]struct{}{}, now) {
		t.Fatal("unknown non-JP event available")
	}
	if deckEventLeakReleased(ctx, nil, 1, now) {
		t.Fatal("nil provider leak released")
	}
	if deckEventProviderForRegion(nil, renderregion.JP) != nil {
		t.Fatal("nil app returned provider")
	}

	if deckEventFromMasterdata(nil) != nil || deckWorldBloomFromMasterdata(nil) != nil {
		t.Fatal("nil masterdata converted")
	}
	event := deckEventFromMasterdata(&masterdata.Event{ID: 7, EventType: "world_bloom", Unit: "piapro", Name: "WL", AssetBundleName: "bundle", StartAt: 1, AggregateAt: 2, ClosedAt: 3})
	if event.GameID != 7 || event.AssetbundleName != "bundle" || event.EventType != "world_bloom" {
		t.Fatalf("converted event = %+v", event)
	}
	character := 21
	chapter := deckWorldBloomFromMasterdata(&masterdata.WorldBloom{EventID: 7, GameCharacterID: &character, ChapterType: "normal", ChapterNo: 2, ChapterStartAt: 1, AggregateAt: 2, ChapterEndAt: 3, IsSupplemental: true})
	if chapter.EventID != 7 || chapter.GameCharacterID != 21 || chapter.ChapterNo != 2 || !chapter.IsSupplemental {
		t.Fatalf("converted chapter = %+v", chapter)
	}
	chapter = deckWorldBloomFromMasterdata(&masterdata.WorldBloom{EventID: 8})
	if chapter.GameCharacterID != 0 {
		t.Fatalf("nil character conversion = %+v", chapter)
	}
}

func TestDeckWorldBloomTurnGuardFunctions(t *testing.T) {
	ctx := context.Background()
	if got := resolveDeckWorldBloomEventTurn(ctx, nil, renderregion.JP, nil, nil, &renderdeck.AutoQuery{}); got != 0 {
		t.Fatalf("nil event turn = %d", got)
	}
	if got := resolveDeckWorldBloomEventTurn(ctx, nil, renderregion.JP, &sekaidb.Event{}, nil, nil); got != 0 {
		t.Fatalf("nil query turn = %d", got)
	}
	query := &renderdeck.AutoQuery{}
	ensureDeckWorldBloomEventTurnMetadata(ctx, nil, renderregion.JP, nil, nil, nil)
	ensureDeckWorldBloomEventTurnMetadata(ctx, nil, renderregion.JP, &sekaidb.Event{}, nil, query)
	if query.MetadataWorldBloomEventTurn != nil {
		t.Fatalf("unexpected turn metadata = %+v", query)
	}
	already := 3
	query.MetadataWorldBloomEventTurn = &already
	ensureDeckWorldBloomEventTurnMetadata(ctx, nil, renderregion.JP, &sekaidb.Event{}, nil, query)
	if *query.MetadataWorldBloomEventTurn != 3 {
		t.Fatal("existing metadata changed")
	}

	if turn, err := resolveDeckWorldBloomCharacterTurnForEvent(ctx, nil, renderregion.JP, 0, 21); err != nil || turn != 0 {
		t.Fatalf("invalid character turn = %d, %v", turn, err)
	}
	if turn, err := resolveDeckWorldBloomUnitTurnForEvent(ctx, nil, renderregion.JP, 0, "piapro"); err != nil || turn != 0 {
		t.Fatalf("invalid unit event turn = %d, %v", turn, err)
	}
	if turn, err := resolveDeckWorldBloomUnitTurnForEvent(ctx, nil, renderregion.JP, 1, "bad"); err != nil || turn != 0 {
		t.Fatalf("invalid unit turn = %d, %v", turn, err)
	}
	if _, err := resolveDeckWorldBloomCharacterTurnForEvent(ctx, nil, renderregion.JP, 1, 21); err == nil {
		t.Fatal("expected missing event data error")
	}
	if _, err := resolveDeckWorldBloomUnitTurnForEvent(ctx, nil, renderregion.JP, 1, "piapro"); err == nil {
		t.Fatal("expected missing unit event data error")
	}

	query = &renderdeck.AutoQuery{WorldBloomEventTurn: drawing.IntPtr(1), WorldBloomCharacterID: drawing.IntPtr(21)}
	if _, err := resolveDeckWorldBloomEventByTurnSelection(ctx, nil, renderregion.JP, query); err == nil {
		t.Fatal("expected missing character event data error")
	}
}
