package event

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	"haruki-cloud/internal/testutil"

	_ "github.com/mattn/go-sqlite3"
)

func TestEventProviderAdapterEmptyDatabase(t *testing.T) {
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:event_adapter_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = client.Close() })
	adapter := NewProviderAdapter(provider.NewDatabaseProvider(client, renderregion.JP))
	ctx := context.WithValue(context.Background(), eventContextKey("adapter"), "request")
	withContext := adapter.WithContext(ctx)
	{
		testutil.RequireArgs(t, !(withContext == nil), "adapter did not retain context")
		testutil.RequireArgs(t, !(withContext.(*ProviderAdapter).Context() != ctx), "adapter did not retain context")
	}
	testutil.RequireArgs(t, !((*ProviderAdapter)(nil).WithContext(ctx) != nil), "nil adapter returned a source")

	_, _ = adapter.GetEventByID(1)
	_, _ = adapter.GetEventByCardID(1)
	_ = adapter.GetEvents()
	_, _ = adapter.GetEventCards(1)
	_, _ = adapter.GetEventRankingHonorRewards(1)
	_, _ = adapter.GetEventBannerCharacterID(1)
	_, _ = adapter.GetEventDeckBonuses(1)
	_, _ = adapter.GetGameCharacterUnit(1)
	_ = adapter.GetBanEvents(1)
	_ = adapter.GetWorldBloomChapters(1)
	_, _ = adapter.GetCharacterByID(1)
	{
		color, ok := adapter.GetCharacterColorCode(1)
		{
			testutil.Require(t, !(ok), "empty character color = %q, %t", color, ok)
			testutil.Require(t, !(color != ""), "empty character color = %q, %t", color, ok)
		}
	}

}

func TestEventRenderEntrypointsAndRecordValidation(t *testing.T) {
	now := time.Now().UnixMilli()
	source := newTestEventSource(renderregion.JP)
	eventInfo := &masterdata.Event{
		ID: 1, EventType: "marathon", Name: "Current", AssetBundleName: "event_1",
		StartAt: now - 1_000, AggregateAt: now + 10_000, ClosedAt: now + 20_000,
	}
	source.events = []*masterdata.Event{eventInfo}
	source.eventsByID[1] = eventInfo

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("event-image"))
	}))
	defer server.Close()
	controller := NewController(source, drawing.NewHarukiDrawingClient(server.URL), assets.NewAssetHelper("", nil))

	validRecord := drawing.EventRecordRequest{
		EventInfo: []drawing.EventHistory{{ID: 1}},
		UserInfo: drawing.DetailedProfileCardRequest{
			Region: "JP", Nickname: "Tester", LeaderImagePath: "leader.png",
		},
	}
	for name, render := range map[string]func() ([]byte, error){
		"detail": func() ([]byte, error) {
			return controller.RenderEventDetail(DetailQuery{Region: renderregion.JP, EventID: 1})
		},
		"list": func() ([]byte, error) {
			return controller.RenderEventList(ListQuery{Region: renderregion.JP, IncludePast: true, IncludeFuture: true})
		},
		"record": func() ([]byte, error) { return controller.RenderEventRecord(validRecord) },
	} {
		t.Run(name, func(t *testing.T) {
			got, err := render()
			{
				testutil.Require(t, !(err != nil), "render = %q, %v", got, err)
				testutil.Require(t, bytes.Equal(got, []byte("event-image")), "render = %q, %v", got, err)
			}

		})
	}

	withoutDrawing := NewController(source, nil, nil)
	{
		_, err := withoutDrawing.RenderEventDetail(DetailQuery{})
		testutil.RequireArgs(t, !(err == nil), "detail without drawing client succeeded")
	}
	{

		_, err := withoutDrawing.RenderEventList(ListQuery{})
		testutil.RequireArgs(t, !(err == nil), "list without drawing client succeeded")
	}
	{

		_, err := withoutDrawing.RenderEventRecord(validRecord)
		testutil.RequireArgs(t, !(err == nil), "record without drawing client succeeded")
	}

	for name, req := range map[string]drawing.EventRecordRequest{
		"history": {},
		"region": {
			EventInfo: []drawing.EventHistory{{ID: 1}},
		},
		"nickname": {
			EventInfo: []drawing.EventHistory{{ID: 1}}, UserInfo: drawing.DetailedProfileCardRequest{Region: "JP"},
		},
		"leader": {
			EventInfo: []drawing.EventHistory{{ID: 1}}, UserInfo: drawing.DetailedProfileCardRequest{Region: "JP", Nickname: "Tester"},
		},
	} {
		t.Run("invalid "+name, func(t *testing.T) {
			{
				_, err := controller.BuildEventRecordRequest(req)
				testutil.RequireArgs(t, !(err == nil), "invalid record unexpectedly succeeded")
			}

		})
	}
	{
		got, err := controller.BuildEventRecordRequest(validRecord)
		{
			testutil.Require(t, !(err != nil), "valid event record = %+v, %v", got, err)
			testutil.Require(t, !(got == nil), "valid event record = %+v, %v", got, err)
		}
	}
	{

		_, err := controller.RenderEventDetail(DetailQuery{Region: renderregion.JP, EventID: 99})
		testutil.RequireArgs(t, !(err == nil), "invalid detail render unexpectedly succeeded")
	}
	{

		_, err := controller.RenderEventList(ListQuery{Region: renderregion.JP, EventType: "missing"})
		testutil.RequireArgs(t, !(err == nil), "empty list render unexpectedly succeeded")
	}
	{

		_, err := controller.RenderEventRecord(drawing.EventRecordRequest{})
		testutil.RequireArgs(t, !(err == nil), "invalid record render unexpectedly succeeded")
	}
	testutil.RequireArgs(t, !((*Controller)(nil).WithContext(context.Background()) != nil), "nil controller WithContext returned a controller")

}

func TestResolveEventDetailValidationBranches(t *testing.T) {
	now := time.Now().UnixMilli()
	past := &masterdata.Event{ID: 1, StartAt: now - 30_000, AggregateAt: now - 20_000, ClosedAt: now - 10_000}
	current := &masterdata.Event{ID: 2, StartAt: now - 5_000, AggregateAt: now + 5_000, ClosedAt: now + 10_000}
	future := &masterdata.Event{ID: 3, StartAt: now + 20_000, AggregateAt: now + 30_000, ClosedAt: now + 40_000}
	source := newTestEventSource(renderregion.JP)
	source.events = []*masterdata.Event{future, past, current}
	for _, item := range source.events {
		source.eventsByID[item.ID] = item
	}
	source.banEventsByChar[7] = []*masterdata.Event{future, past}
	controller := NewController(source, nil, nil)
	{

		_, _, err := controller.resolveDetailQuery(DetailQuery{Region: renderregion.JP, EventID: 3})
		testutil.RequireArgs(t, !(err == nil), "explicit unreleased event unexpectedly succeeded")
	}

	for name, query := range map[string]DetailQuery{
		"ban sequence":     {Region: renderregion.JP, BanCharID: 7},
		"missing ban":      {Region: renderregion.JP, BanCharID: 8, BanSeq: 1},
		"ban out of range": {Region: renderregion.JP, BanCharID: 7, BanSeq: 3},
		"unsupported key":  {Region: renderregion.JP, Keyword: "later"},
		"index out":        {Region: renderregion.JP, Index: new(10)},
		"missing selector": {Region: renderregion.JP},
	} {
		t.Run(name, func(t *testing.T) {
			{
				_, _, err := controller.resolveDetailQuery(query)
				testutil.RequireArgs(t, !(err == nil), "invalid detail query unexpectedly succeeded")
			}

		})
	}
	positive := 1
	{
		query, _, err := controller.resolveDetailQuery(DetailQuery{Region: renderregion.JP, Index: &positive})
		{
			testutil.Require(t, !(err != nil), "positive event index = %+v, %v", query, err)
			testutil.Require(t, !(query.EventID != 3), "positive event index = %+v, %v", query, err)
		}
	}

	zero := 0
	{
		query, _, err := controller.resolveDetailQuery(DetailQuery{Region: renderregion.JP, Index: &zero})
		{
			testutil.Require(t, !(err != nil), "current event index = %+v, %v", query, err)
			testutil.Require(t, !(query.EventID != 2), "current event index = %+v, %v", query, err)
		}
	}

	empty := NewController(newTestEventSource(renderregion.JP), nil, nil)
	{
		_, _, err := empty.resolveDetailQuery(DetailQuery{Region: renderregion.JP, UseCurrent: true})
		testutil.RequireArgs(t, !(err == nil), "empty event source unexpectedly resolved")
	}
	{

		_, err := NewController(nil, nil, nil).BuildEventListRequest(ListQuery{Region: renderregion.JP})
		testutil.RequireArgs(t, !(err == nil), "missing event source unexpectedly built a list")
	}

}

func TestEventIndexAndTimelineHelperBranches(t *testing.T) {
	now := time.Now().UnixMilli()
	past := &masterdata.Event{ID: 1, StartAt: now - 30_000, AggregateAt: now - 20_000, ClosedAt: now - 10_000}
	future := &masterdata.Event{ID: 2, StartAt: now + 10_000, AggregateAt: now + 20_000, ClosedAt: now + 30_000}
	current := &masterdata.Event{ID: 3, StartAt: now - 1_000, AggregateAt: now + 1_000, ClosedAt: now + 2_000}
	{
		index, err := resolveCurrentEventIndex([]*masterdata.Event{current}, "prev")
		{
			testutil.Require(t, !(err != nil), "current index = %d, %v", index, err)
			testutil.Require(t, !(index != 0), "current index = %d, %v", index, err)
		}
	}

	for fallback, want := range map[string]int{"prev": 0, "next": 1, "prev_first": 0, "next_first": 1} {
		{
			index, err := resolveCurrentEventIndex([]*masterdata.Event{past, future}, fallback)
			testutil.Check(t, !(err != nil || index != want), "fallback %s = %d, %v", fallback, index, err)
		}

	}
	{
		_, err := resolveCurrentEventIndex(nil, "unknown")
		testutil.RequireArgs(t, !(err == nil), "empty current-event lookup unexpectedly succeeded")
	}
	{

		index, err := resolveEventKeywordIndex([]*masterdata.Event{past, future}, "prev")
		{
			testutil.Require(t, !(err != nil), "previous keyword = %d, %v", index, err)
			testutil.Require(t, !(index != 0), "previous keyword = %d, %v", index, err)
		}
	}
	{

		index, err := resolveEventKeywordIndex([]*masterdata.Event{past, future}, "next")
		{
			testutil.Require(t, !(err != nil), "next keyword = %d, %v", index, err)
			testutil.Require(t, !(index != 1), "next keyword = %d, %v", index, err)
		}
	}
	{

		_, err := resolveEventKeywordIndex([]*masterdata.Event{future}, "prev")
		testutil.RequireArgs(t, !(err == nil), "missing previous event unexpectedly resolved")
	}
	{

		_, err := resolveEventKeywordIndex([]*masterdata.Event{past}, "next")
		testutil.RequireArgs(t, !(err == nil), "missing next event unexpectedly resolved")
	}

	source := newTestEventSource(renderregion.JP)
	source.banEventsByChar[5] = []*masterdata.Event{future, past}
	builder := NewBuilder(source, nil)
	{
		index := builder.getBannerIndex(5, future.ID)
		{
			testutil.Require(t, !(index == nil), "banner index = %v", index)
			testutil.Require(t, !(*index != 2), "banner index = %v", index)
		}
	}
	testutil.RequireArgs(t, !(builder.getBannerIndex(5, 99) != nil), "missing banner event returned an index")
	{
		testutil.RequireArgs(t, !(resolveWorldBloomChapterEndAt(nil) != 0), "world-bloom chapter end resolution failed")
		testutil.RequireArgs(t, !(resolveWorldBloomChapterEndAt(&masterdata.WorldBloom{AggregateAt: 10}) != 1010), "world-bloom chapter end resolution failed")
		testutil.RequireArgs(t, !(resolveWorldBloomChapterEndAt(&masterdata.WorldBloom{ChapterEndAt: 20}) != 20), "world-bloom chapter end resolution failed")
		testutil.RequireArgs(t, !(resolveWorldBloomChapterEndAt(&masterdata.WorldBloom{}) != 0), "world-bloom chapter end resolution failed")
	}

}

func TestEventCardAndCharacterFilterBranches(t *testing.T) {
	source := newTestEventSource(renderregion.JP)
	source.cardsByEvent[1] = []*masterdata.Card{
		{},
		{CharacterID: 5},
		{CharacterID: 6},
	}
	source.cardsByEvent[2] = []*masterdata.Card{{CharacterID: 5}, {CharacterID: 6}}
	source.cardsByEvent[3] = []*masterdata.Card{{CharacterID: 5}, {CharacterID: 7}}
	source.characterByID[5] = &masterdata.Character{ID: 5, Unit: "idol"}
	source.characterByID[6] = &masterdata.Character{ID: 6, Unit: "idol"}
	source.characterByID[7] = &masterdata.Character{ID: 7, Unit: "piapro"}
	source.characterByID[8] = &masterdata.Character{ID: 8, Unit: ""}
	builder := NewBuilder(source, nil)
	testutil.RequireArgs(t, builder.eventHasCardCharacters(1, 5, []int{0, 6}), "existing card characters were not matched")
	{
		testutil.RequireArgs(t, !(builder.eventHasCardCharacters(1, 9, nil)), "missing card characters were matched")
		testutil.RequireArgs(t, !(builder.eventHasCardCharacters(1, 0, []int{9})), "missing card characters were matched")
		testutil.RequireArgs(t, !(builder.eventHasCardCharacters(99, 0, nil)), "missing card characters were matched")
	}
	{
		testutil.RequireArgs(t, builder.eventCardsAllInUnit(2, "idol"), "all-in-unit classification failed")
		testutil.RequireArgs(t, !(builder.eventCardsAllInUnit(2, "")), "all-in-unit classification failed")
	}
	{
		testutil.RequireArgs(t, !(builder.eventCardsAllInUnit(3, "idol")), "mixed or empty event was classified as one unit")
		testutil.RequireArgs(t, !(builder.eventCardsAllInUnit(99, "idol")), "mixed or empty event was classified as one unit")
	}

	for name, tc := range map[string]struct {
		card *masterdata.Card
		unit string
		ok   bool
	}{
		"nil":            {card: nil},
		"zero character": {card: &masterdata.Card{}},
		"missing char":   {card: &masterdata.Card{CharacterID: 99}},
		"normal":         {card: &masterdata.Card{CharacterID: 5}, unit: "idol", ok: true},
		"piapro support": {card: &masterdata.Card{CharacterID: 7, SupportUnit: "idol"}, unit: "idol", ok: true},
		"piapro default": {card: &masterdata.Card{CharacterID: 7}, unit: "piapro", ok: true},
		"support only":   {card: &masterdata.Card{CharacterID: 8, SupportUnit: "idol"}, unit: "idol", ok: true},
		"no unit":        {card: &masterdata.Card{CharacterID: 8}},
	} {
		t.Run(name, func(t *testing.T) {
			unit, ok := builder.eventCardUnit(tc.card)
			{
				testutil.Require(t, !(unit != tc.unit), "eventCardUnit() = %q, %t", unit, ok)
				testutil.Require(t, !(ok != tc.ok), "eventCardUnit() = %q, %t", unit, ok)
			}

		})
	}
	{
		got := builder.characterIconPath(999, renderregion.JP)
		testutil.RequireArgs(t, !(got == ""), "unknown character icon path is empty")
	}
	testutil.RequireArgs(t, !(builder.characterDisplayName(999) != ""), "missing character has a display name")

	for code, want := range map[string]string{
		"marathon": "马拉松", "cheerful_carnival": "5v5", "world_bloom": "WorldLink", "custom": "custom",
	} {
		{
			got := builder.displayEventType(code)
			testutil.Check(t, !(got != want), "displayEventType(%q) = %q", code, got)
		}

	}
	{

		units, ok := builder.extractWorldBloomChapterUnits(99)
		{
			testutil.RequireArgs(t, !(ok), "missing chapters returned units")
			testutil.RequireArgs(t, !(units != nil), "missing chapters returned units")
		}
	}

	zero := 0
	charID := 5
	source.worldByEvent[3] = []*masterdata.WorldBloom{
		{GameCharacterID: nil},
		{GameCharacterID: &zero},
		{GameCharacterID: new(99)},
		{GameCharacterID: &charID},
	}
	units, ok := builder.extractWorldBloomChapterUnits(3)
	testutil.RequireArgs(t, ok, "chapter data was not recognized")
	{

		_, exists := units["idol"]
		testutil.Require(t, exists, "chapter units = %#v", units)
	}

}
