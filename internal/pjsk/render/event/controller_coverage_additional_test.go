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

	_ "github.com/mattn/go-sqlite3"
)

func TestEventProviderAdapterEmptyDatabase(t *testing.T) {
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:event_adapter_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = client.Close() })
	adapter := NewProviderAdapter(provider.NewDatabaseProvider(client, renderregion.JP))
	ctx := context.WithValue(context.Background(), eventContextKey("adapter"), "request")
	withContext := adapter.WithContext(ctx)
	if withContext == nil || withContext.(*ProviderAdapter).Context() != ctx {
		t.Fatal("adapter did not retain context")
	}
	if (*ProviderAdapter)(nil).WithContext(ctx) != nil {
		t.Fatal("nil adapter returned a source")
	}
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
	if color, ok := adapter.GetCharacterColorCode(1); ok || color != "" {
		t.Fatalf("empty character color = %q, %t", color, ok)
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
			if err != nil || !bytes.Equal(got, []byte("event-image")) {
				t.Fatalf("render = %q, %v", got, err)
			}
		})
	}

	withoutDrawing := NewController(source, nil, nil)
	if _, err := withoutDrawing.RenderEventDetail(DetailQuery{}); err == nil {
		t.Fatal("detail without drawing client succeeded")
	}
	if _, err := withoutDrawing.RenderEventList(ListQuery{}); err == nil {
		t.Fatal("list without drawing client succeeded")
	}
	if _, err := withoutDrawing.RenderEventRecord(validRecord); err == nil {
		t.Fatal("record without drawing client succeeded")
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
			if _, err := controller.BuildEventRecordRequest(req); err == nil {
				t.Fatal("invalid record unexpectedly succeeded")
			}
		})
	}
	if got, err := controller.BuildEventRecordRequest(validRecord); err != nil || got == nil {
		t.Fatalf("valid event record = %+v, %v", got, err)
	}
	if _, err := controller.RenderEventDetail(DetailQuery{Region: renderregion.JP, EventID: 99}); err == nil {
		t.Fatal("invalid detail render unexpectedly succeeded")
	}
	if _, err := controller.RenderEventList(ListQuery{Region: renderregion.JP, EventType: "missing"}); err == nil {
		t.Fatal("empty list render unexpectedly succeeded")
	}
	if _, err := controller.RenderEventRecord(drawing.EventRecordRequest{}); err == nil {
		t.Fatal("invalid record render unexpectedly succeeded")
	}
	if (*Controller)(nil).WithContext(context.Background()) != nil {
		t.Fatal("nil controller WithContext returned a controller")
	}
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

	if _, _, err := controller.resolveDetailQuery(DetailQuery{Region: renderregion.JP, EventID: 3}); err == nil {
		t.Fatal("explicit unreleased event unexpectedly succeeded")
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
			if _, _, err := controller.resolveDetailQuery(query); err == nil {
				t.Fatal("invalid detail query unexpectedly succeeded")
			}
		})
	}
	positive := 1
	if query, _, err := controller.resolveDetailQuery(DetailQuery{Region: renderregion.JP, Index: &positive}); err != nil || query.EventID != 3 {
		t.Fatalf("positive event index = %+v, %v", query, err)
	}
	zero := 0
	if query, _, err := controller.resolveDetailQuery(DetailQuery{Region: renderregion.JP, Index: &zero}); err != nil || query.EventID != 2 {
		t.Fatalf("current event index = %+v, %v", query, err)
	}

	empty := NewController(newTestEventSource(renderregion.JP), nil, nil)
	if _, _, err := empty.resolveDetailQuery(DetailQuery{Region: renderregion.JP, UseCurrent: true}); err == nil {
		t.Fatal("empty event source unexpectedly resolved")
	}
	if _, err := NewController(nil, nil, nil).BuildEventListRequest(ListQuery{Region: renderregion.JP}); err == nil {
		t.Fatal("missing event source unexpectedly built a list")
	}
}

func TestEventIndexAndTimelineHelperBranches(t *testing.T) {
	now := time.Now().UnixMilli()
	past := &masterdata.Event{ID: 1, StartAt: now - 30_000, AggregateAt: now - 20_000, ClosedAt: now - 10_000}
	future := &masterdata.Event{ID: 2, StartAt: now + 10_000, AggregateAt: now + 20_000, ClosedAt: now + 30_000}
	current := &masterdata.Event{ID: 3, StartAt: now - 1_000, AggregateAt: now + 1_000, ClosedAt: now + 2_000}
	if index, err := resolveCurrentEventIndex([]*masterdata.Event{current}, "prev"); err != nil || index != 0 {
		t.Fatalf("current index = %d, %v", index, err)
	}
	for fallback, want := range map[string]int{"prev": 0, "next": 1, "prev_first": 0, "next_first": 1} {
		if index, err := resolveCurrentEventIndex([]*masterdata.Event{past, future}, fallback); err != nil || index != want {
			t.Errorf("fallback %s = %d, %v", fallback, index, err)
		}
	}
	if _, err := resolveCurrentEventIndex(nil, "unknown"); err == nil {
		t.Fatal("empty current-event lookup unexpectedly succeeded")
	}
	if index, err := resolveEventKeywordIndex([]*masterdata.Event{past, future}, "prev"); err != nil || index != 0 {
		t.Fatalf("previous keyword = %d, %v", index, err)
	}
	if index, err := resolveEventKeywordIndex([]*masterdata.Event{past, future}, "next"); err != nil || index != 1 {
		t.Fatalf("next keyword = %d, %v", index, err)
	}
	if _, err := resolveEventKeywordIndex([]*masterdata.Event{future}, "prev"); err == nil {
		t.Fatal("missing previous event unexpectedly resolved")
	}
	if _, err := resolveEventKeywordIndex([]*masterdata.Event{past}, "next"); err == nil {
		t.Fatal("missing next event unexpectedly resolved")
	}

	source := newTestEventSource(renderregion.JP)
	source.banEventsByChar[5] = []*masterdata.Event{future, past}
	builder := NewBuilder(source, nil)
	if index := builder.getBannerIndex(5, future.ID); index == nil || *index != 2 {
		t.Fatalf("banner index = %v", index)
	}
	if builder.getBannerIndex(5, 99) != nil {
		t.Fatal("missing banner event returned an index")
	}
	if resolveWorldBloomChapterEndAt(nil) != 0 ||
		resolveWorldBloomChapterEndAt(&masterdata.WorldBloom{AggregateAt: 10}) != 1010 ||
		resolveWorldBloomChapterEndAt(&masterdata.WorldBloom{ChapterEndAt: 20}) != 20 ||
		resolveWorldBloomChapterEndAt(&masterdata.WorldBloom{}) != 0 {
		t.Fatal("world-bloom chapter end resolution failed")
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

	if !builder.eventHasCardCharacters(1, 5, []int{0, 6}) {
		t.Fatal("existing card characters were not matched")
	}
	if builder.eventHasCardCharacters(1, 9, nil) || builder.eventHasCardCharacters(1, 0, []int{9}) || builder.eventHasCardCharacters(99, 0, nil) {
		t.Fatal("missing card characters were matched")
	}
	if !builder.eventCardsAllInUnit(2, "idol") || builder.eventCardsAllInUnit(2, "") {
		t.Fatal("all-in-unit classification failed")
	}
	if builder.eventCardsAllInUnit(3, "idol") || builder.eventCardsAllInUnit(99, "idol") {
		t.Fatal("mixed or empty event was classified as one unit")
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
			if unit != tc.unit || ok != tc.ok {
				t.Fatalf("eventCardUnit() = %q, %t", unit, ok)
			}
		})
	}
	if got := builder.characterIconPath(999, renderregion.JP); got == "" {
		t.Fatal("unknown character icon path is empty")
	}
	if builder.characterDisplayName(999) != "" {
		t.Fatal("missing character has a display name")
	}
	for code, want := range map[string]string{
		"marathon": "马拉松", "cheerful_carnival": "5v5", "world_bloom": "WorldLink", "custom": "custom",
	} {
		if got := builder.displayEventType(code); got != want {
			t.Errorf("displayEventType(%q) = %q", code, got)
		}
	}

	if units, ok := builder.extractWorldBloomChapterUnits(99); ok || units != nil {
		t.Fatal("missing chapters returned units")
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
	if !ok {
		t.Fatal("chapter data was not recognized")
	}
	if _, exists := units["idol"]; !exists {
		t.Fatalf("chapter units = %#v", units)
	}
}
