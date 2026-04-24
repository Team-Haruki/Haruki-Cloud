package event

import (
	"fmt"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type testEventSource struct {
	region          renderregion.Value
	events          []*masterdata.Event
	eventsByID      map[int]*masterdata.Event
	cardsByEvent    map[int][]*masterdata.Card
	bannerByEvent   map[int]int
	bonusesByEvent  map[int][]*masterdata.EventDeckBonus
	gcuByID         map[int]*masterdata.GameCharacterUnit
	worldByEvent    map[int][]*masterdata.WorldBloom
	characterByID   map[int]*masterdata.Character
	banEventsByChar map[int][]*masterdata.Event
}

func newTestEventSource(region renderregion.Value) *testEventSource {
	return &testEventSource{
		region:          region,
		eventsByID:      make(map[int]*masterdata.Event),
		cardsByEvent:    make(map[int][]*masterdata.Card),
		bannerByEvent:   make(map[int]int),
		bonusesByEvent:  make(map[int][]*masterdata.EventDeckBonus),
		gcuByID:         make(map[int]*masterdata.GameCharacterUnit),
		worldByEvent:    make(map[int][]*masterdata.WorldBloom),
		characterByID:   make(map[int]*masterdata.Character),
		banEventsByChar: make(map[int][]*masterdata.Event),
	}
}

func (s *testEventSource) DefaultRegion() renderregion.Value { return s.region }

func (s *testEventSource) GetEventByID(id int) (*masterdata.Event, error) {
	if eventInfo, ok := s.eventsByID[id]; ok {
		return new(*eventInfo), nil
	}
	return nil, fmt.Errorf("event not found: %d", id)
}

func (s *testEventSource) GetEventByCardID(cardID int) (*masterdata.Event, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *testEventSource) GetEvents() []*masterdata.Event {
	out := make([]*masterdata.Event, 0, len(s.events))
	for _, eventInfo := range s.events {
		out = append(out, new(*eventInfo))
	}
	return out
}

func (s *testEventSource) GetEventCards(eventID int) ([]*masterdata.Card, error) {
	items := s.cardsByEvent[eventID]
	out := make([]*masterdata.Card, 0, len(items))
	for _, item := range items {
		out = append(out, new(*item))
	}
	return out, nil
}

func (s *testEventSource) GetEventBannerCharacterID(eventID int) (int, error) {
	if value, ok := s.bannerByEvent[eventID]; ok {
		return value, nil
	}
	return 0, fmt.Errorf("banner not found: %d", eventID)
}

func (s *testEventSource) GetEventDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error) {
	items := s.bonusesByEvent[eventID]
	out := make([]*masterdata.EventDeckBonus, 0, len(items))
	for _, item := range items {
		out = append(out, new(*item))
	}
	return out, nil
}

func (s *testEventSource) GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error) {
	if item, ok := s.gcuByID[id]; ok {
		return new(*item), nil
	}
	return nil, fmt.Errorf("gcu not found: %d", id)
}

func (s *testEventSource) GetBanEvents(charID int) []*masterdata.Event {
	items := s.banEventsByChar[charID]
	out := make([]*masterdata.Event, 0, len(items))
	for _, item := range items {
		out = append(out, new(*item))
	}
	return out
}

func (s *testEventSource) GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom {
	items := s.worldByEvent[eventID]
	out := make([]*masterdata.WorldBloom, 0, len(items))
	for _, item := range items {
		out = append(out, new(*item))
	}
	return out
}

func (s *testEventSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	if item, ok := s.characterByID[id]; ok {
		return new(*item), nil
	}
	return nil, fmt.Errorf("character not found: %d", id)
}

func TestBuildEventListRequestWorldBloomNoCharacterAvatar(t *testing.T) {
	source := newTestEventSource(renderregion.JP)
	eventInfo := &masterdata.Event{ID: 101, EventType: "world_bloom", Name: "JP_WL", AssetBundleName: "wl_101", StartAt: 100, AggregateAt: 200}
	source.events = []*masterdata.Event{eventInfo}
	source.eventsByID[eventInfo.ID] = eventInfo
	source.cardsByEvent[eventInfo.ID] = []*masterdata.Card{{ID: 1001, CharacterID: 5, Attr: "cool", AssetBundleName: "card_1001"}}
	source.bannerByEvent[eventInfo.ID] = 5
	source.bonusesByEvent[eventInfo.ID] = []*masterdata.EventDeckBonus{{ID: 1, EventID: eventInfo.ID, GameCharacterUnitID: 501, CardAttr: "cool"}}
	source.gcuByID[501] = &masterdata.GameCharacterUnit{ID: 501, GameCharacterID: 5, Unit: "idol"}
	source.characterByID[5] = &masterdata.Character{ID: 5, Unit: "idol"}

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	req, err := builder.BuildEventListRequest(ListQuery{Region: renderregion.JP, EventType: "world_bloom", IncludePast: true, IncludeFuture: true})
	if err != nil {
		t.Fatalf("BuildEventListRequest failed: %v", err)
	}
	if len(req.EventInfo) != 1 {
		t.Fatalf("expected 1 event, got %d", len(req.EventInfo))
	}

	brief := req.EventInfo[0]
	if brief.EventType != "WorldLink" {
		t.Fatalf("expected WorldLink event type, got %q", brief.EventType)
	}
	if brief.EventCharaPath != nil {
		t.Fatalf("WL should not expose character avatar in list, got %q", *brief.EventCharaPath)
	}
	if brief.EventUnitPath == nil || *brief.EventUnitPath == "" {
		t.Fatal("WL should keep unit icon path")
	}
}

func TestBuildEventListRequestOrdersByStartAtAscending(t *testing.T) {
	source := newTestEventSource(renderregion.JP)
	later := &masterdata.Event{ID: 201, EventType: "marathon", Name: "Later", AssetBundleName: "later", StartAt: 200, AggregateAt: 300}
	earlier := &masterdata.Event{ID: 202, EventType: "marathon", Name: "Earlier", AssetBundleName: "earlier", StartAt: 100, AggregateAt: 150}
	source.events = []*masterdata.Event{later, earlier}
	source.eventsByID[later.ID] = later
	source.eventsByID[earlier.ID] = earlier

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	req, err := builder.BuildEventListRequest(ListQuery{Region: renderregion.JP, IncludePast: true, IncludeFuture: true})
	if err != nil {
		t.Fatalf("BuildEventListRequest failed: %v", err)
	}
	if len(req.EventInfo) != 2 {
		t.Fatalf("expected 2 events, got %d", len(req.EventInfo))
	}
	if req.EventInfo[0].ID != 202 || req.EventInfo[1].ID != 201 {
		t.Fatalf("unexpected order: got [%d, %d], want [202, 201]", req.EventInfo[0].ID, req.EventInfo[1].ID)
	}
}

func TestBuildEventListRequestBannerFilterOnlyKeepsBoxEvents(t *testing.T) {
	source := newTestEventSource(renderregion.JP)
	first := &masterdata.Event{ID: 101, EventType: "marathon", Name: "box-1", AssetBundleName: "e101", StartAt: 100, AggregateAt: 200}
	mixed := &masterdata.Event{ID: 102, EventType: "marathon", Name: "mixed", AssetBundleName: "e102", StartAt: 300, AggregateAt: 400}
	second := &masterdata.Event{ID: 103, EventType: "cheerful_carnival", Name: "box-2", AssetBundleName: "e103", StartAt: 500, AggregateAt: 600}
	source.events = []*masterdata.Event{first, mixed, second}
	source.eventsByID[first.ID] = first
	source.eventsByID[mixed.ID] = mixed
	source.eventsByID[second.ID] = second
	source.bannerByEvent[first.ID] = 10
	source.bannerByEvent[mixed.ID] = 10
	source.bannerByEvent[second.ID] = 10
	source.bonusesByEvent[first.ID] = []*masterdata.EventDeckBonus{
		{ID: 1, EventID: first.ID, GameCharacterUnitID: 10, CardAttr: "cool"},
		{ID: 2, EventID: first.ID, GameCharacterUnitID: 21},
	}
	source.bonusesByEvent[mixed.ID] = []*masterdata.EventDeckBonus{
		{ID: 3, EventID: mixed.ID, GameCharacterUnitID: 10, CardAttr: "cool"},
		{ID: 4, EventID: mixed.ID, GameCharacterUnitID: 105},
	}
	source.bonusesByEvent[second.ID] = []*masterdata.EventDeckBonus{
		{ID: 5, EventID: second.ID, GameCharacterUnitID: 10, CardAttr: "pure"},
		{ID: 6, EventID: second.ID, GameCharacterUnitID: 21},
	}
	source.gcuByID[10] = &masterdata.GameCharacterUnit{ID: 10, GameCharacterID: 10, Unit: "street"}
	source.gcuByID[21] = &masterdata.GameCharacterUnit{ID: 21, GameCharacterID: 21, Unit: "street"}
	source.gcuByID[105] = &masterdata.GameCharacterUnit{ID: 105, GameCharacterID: 5, Unit: "idol"}

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	req, err := builder.BuildEventListRequest(ListQuery{
		Region:        renderregion.JP,
		IncludePast:   true,
		IncludeFuture: true,
		BannerCharID:  new(10),
	})
	if err != nil {
		t.Fatalf("BuildEventListRequest failed: %v", err)
	}
	if len(req.EventInfo) != 2 {
		t.Fatalf("expected 2 box events, got %+v", req.EventInfo)
	}
	if req.EventInfo[0].ID != 101 || req.EventInfo[1].ID != 103 {
		t.Fatalf("unexpected banner-filter events: %+v", req.EventInfo)
	}
}

func TestBuildEventListRequestWorldBloomUnitFilterUsesChapterUnits(t *testing.T) {
	source := newTestEventSource(renderregion.JP)
	streetEvent := &masterdata.Event{ID: 301, EventType: "world_bloom", Name: "street wl", AssetBundleName: "e301", StartAt: 100, AggregateAt: 200}
	vsEvent := &masterdata.Event{ID: 302, EventType: "world_bloom", Name: "vs wl", AssetBundleName: "e302", StartAt: 300, AggregateAt: 400}
	finaleEvent := &masterdata.Event{ID: 303, EventType: "world_bloom", Name: "finale", AssetBundleName: "e303", StartAt: 500, AggregateAt: 600}
	source.events = []*masterdata.Event{streetEvent, vsEvent, finaleEvent}
	source.eventsByID[streetEvent.ID] = streetEvent
	source.eventsByID[vsEvent.ID] = vsEvent
	source.eventsByID[finaleEvent.ID] = finaleEvent

	source.bonusesByEvent[streetEvent.ID] = []*masterdata.EventDeckBonus{
		{ID: 1, EventID: streetEvent.ID, GameCharacterUnitID: 10, CardAttr: "cool"},
		{ID: 2, EventID: streetEvent.ID, GameCharacterUnitID: 21},
	}
	source.bonusesByEvent[vsEvent.ID] = []*masterdata.EventDeckBonus{
		{ID: 3, EventID: vsEvent.ID, GameCharacterUnitID: 10, CardAttr: "cool"},
		{ID: 4, EventID: vsEvent.ID, GameCharacterUnitID: 21},
	}
	source.bonusesByEvent[finaleEvent.ID] = []*masterdata.EventDeckBonus{
		{ID: 5, EventID: finaleEvent.ID, GameCharacterUnitID: 10, CardAttr: "cool"},
		{ID: 6, EventID: finaleEvent.ID, GameCharacterUnitID: 21},
	}
	source.gcuByID[10] = &masterdata.GameCharacterUnit{ID: 10, GameCharacterID: 10, Unit: "street"}
	source.gcuByID[21] = &masterdata.GameCharacterUnit{ID: 21, GameCharacterID: 21, Unit: "piapro"}

	streetCharID := 10
	vsCharID := 21
	source.worldByEvent[streetEvent.ID] = []*masterdata.WorldBloom{
		{ID: 1, EventID: streetEvent.ID, ChapterNo: 1, GameCharacterID: &streetCharID},
	}
	source.worldByEvent[vsEvent.ID] = []*masterdata.WorldBloom{
		{ID: 2, EventID: vsEvent.ID, ChapterNo: 1, GameCharacterID: &vsCharID},
	}
	source.worldByEvent[finaleEvent.ID] = []*masterdata.WorldBloom{
		{ID: 3, EventID: finaleEvent.ID, ChapterNo: 1, ChapterType: "finale"},
	}
	source.characterByID[10] = &masterdata.Character{ID: 10, Unit: "street"}
	source.characterByID[21] = &masterdata.Character{ID: 21, Unit: "piapro"}

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	req, err := builder.BuildEventListRequest(ListQuery{
		Region:        renderregion.JP,
		EventType:     "world_bloom",
		Unit:          "street",
		IncludePast:   true,
		IncludeFuture: true,
	})
	if err != nil {
		t.Fatalf("BuildEventListRequest failed: %v", err)
	}
	if len(req.EventInfo) != 1 {
		t.Fatalf("expected only street WL event, got %+v", req.EventInfo)
	}
	if req.EventInfo[0].ID != streetEvent.ID {
		t.Fatalf("unexpected world bloom unit filter result: %+v", req.EventInfo)
	}
}

func TestBuildEventDetailRequestMixedEventDoesNotExposeBoxMetadata(t *testing.T) {
	source := newTestEventSource(renderregion.JP)
	eventInfo := &masterdata.Event{ID: 401, EventType: "marathon", Name: "mixed", AssetBundleName: "e401", StartAt: 100, AggregateAt: 200}
	source.events = []*masterdata.Event{eventInfo}
	source.eventsByID[eventInfo.ID] = eventInfo
	source.bannerByEvent[eventInfo.ID] = 10
	source.bonusesByEvent[eventInfo.ID] = []*masterdata.EventDeckBonus{
		{ID: 1, EventID: eventInfo.ID, GameCharacterUnitID: 10, CardAttr: "cool"},
		{ID: 2, EventID: eventInfo.ID, GameCharacterUnitID: 105},
	}
	source.gcuByID[10] = &masterdata.GameCharacterUnit{ID: 10, GameCharacterID: 10, Unit: "street"}
	source.gcuByID[105] = &masterdata.GameCharacterUnit{ID: 105, GameCharacterID: 5, Unit: "idol"}

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	req, err := builder.BuildEventDetailRequest(DetailQuery{Region: renderregion.JP, EventID: eventInfo.ID})
	if err != nil {
		t.Fatalf("BuildEventDetailRequest failed: %v", err)
	}
	if req.EventInfo.BannerCid != 0 {
		t.Fatalf("expected mixed event banner cid to stay empty, got %+v", req.EventInfo)
	}
	if req.EventInfo.BannerIndex != 0 {
		t.Fatalf("expected mixed event banner index to stay empty, got %+v", req.EventInfo)
	}
}
