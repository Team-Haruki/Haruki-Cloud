package event

import (
	"fmt"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type testEventSource struct {
	region          renderregion.Value
	events          []*masterdata.Event
	eventsByID      map[int]*masterdata.Event
	cardsByEvent    map[int][]*masterdata.Card
	cardErrByEvent  map[int]error
	bannerByEvent   map[int]int
	bonusesByEvent  map[int][]*masterdata.EventDeckBonus
	gcuByID         map[int]*masterdata.GameCharacterUnit
	worldByEvent    map[int][]*masterdata.WorldBloom
	characterByID   map[int]*masterdata.Character
	colorByCharID   map[int]string
	banEventsByChar map[int][]*masterdata.Event
}

func newTestEventSource(region renderregion.Value) *testEventSource {
	return &testEventSource{
		region:          region,
		eventsByID:      make(map[int]*masterdata.Event),
		cardsByEvent:    make(map[int][]*masterdata.Card),
		cardErrByEvent:  make(map[int]error),
		bannerByEvent:   make(map[int]int),
		bonusesByEvent:  make(map[int][]*masterdata.EventDeckBonus),
		gcuByID:         make(map[int]*masterdata.GameCharacterUnit),
		worldByEvent:    make(map[int][]*masterdata.WorldBloom),
		characterByID:   make(map[int]*masterdata.Character),
		colorByCharID:   make(map[int]string),
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
	if err, ok := s.cardErrByEvent[eventID]; ok {
		return nil, err
	}
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

func (s *testEventSource) GetCharacterColorCode(id int) (string, bool) {
	value, ok := s.colorByCharID[id]
	return value, ok && value != ""
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

func TestBuildEventListRequestIncludesCardIDsForDrawing(t *testing.T) {
	source := newTestEventSource(renderregion.JP)
	eventInfo := &masterdata.Event{ID: 203, EventType: "marathon", Name: "Cards", AssetBundleName: "cards", StartAt: 100, AggregateAt: 200}
	source.events = []*masterdata.Event{eventInfo}
	source.eventsByID[eventInfo.ID] = eventInfo
	source.cardsByEvent[eventInfo.ID] = []*masterdata.Card{
		{ID: 1201, CharacterID: 1, Attr: "cool", CardRarityType: "rarity_4", AssetBundleName: "card_1201"},
		{ID: 1202, CharacterID: 2, Attr: "pure", CardRarityType: "rarity_3", AssetBundleName: "card_1202"},
	}

	req, err := NewBuilder(source, assets.NewAssetHelper("", nil)).BuildEventListRequest(ListQuery{
		Region:        renderregion.JP,
		IncludePast:   true,
		IncludeFuture: true,
	})
	if err != nil {
		t.Fatalf("BuildEventListRequest failed: %v", err)
	}
	if len(req.EventInfo) != 1 || len(req.EventInfo[0].EventCards) != 2 {
		t.Fatalf("unexpected event card payload: %+v", req.EventInfo)
	}
	if got := []int{req.EventInfo[0].EventCards[0].CardID, req.EventInfo[0].EventCards[1].CardID}; got[0] != 1201 || got[1] != 1202 {
		t.Fatalf("event card ids = %v, want [1201 1202]", got)
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

func TestBuildEventListRequestWorldBloomUnitFilterPrefersEventUnit(t *testing.T) {
	source := newTestEventSource(renderregion.JP)
	eventInfo := &masterdata.Event{
		ID:              304,
		EventType:       "world_bloom",
		Unit:            "street",
		Name:            "street wl with vs chapter",
		AssetBundleName: "e304",
		StartAt:         700,
		AggregateAt:     800,
	}
	source.events = []*masterdata.Event{eventInfo}
	source.eventsByID[eventInfo.ID] = eventInfo
	source.bonusesByEvent[eventInfo.ID] = []*masterdata.EventDeckBonus{
		{ID: 1, EventID: eventInfo.ID, GameCharacterUnitID: 10, CardAttr: "cool"},
		{ID: 2, EventID: eventInfo.ID, GameCharacterUnitID: 21},
	}
	source.gcuByID[10] = &masterdata.GameCharacterUnit{ID: 10, GameCharacterID: 10, Unit: "street"}
	source.gcuByID[21] = &masterdata.GameCharacterUnit{ID: 21, GameCharacterID: 21, Unit: "piapro"}

	vsCharID := 21
	source.worldByEvent[eventInfo.ID] = []*masterdata.WorldBloom{
		{ID: 1, EventID: eventInfo.ID, ChapterNo: 1, GameCharacterID: &vsCharID},
	}
	source.characterByID[21] = &masterdata.Character{ID: 21, Unit: "piapro"}

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	matched := builder.filterEvents(ListQuery{
		Region:        renderregion.JP,
		EventType:     "world_bloom",
		Unit:          "piapro",
		IncludePast:   true,
		IncludeFuture: true,
	})
	if len(matched) != 0 {
		t.Fatalf("expected no piapro WL events, got %+v", matched)
	}
}

func TestBuildEventListRequestCharacterFilterUsesEventCards(t *testing.T) {
	source := newTestEventSource(renderregion.JP)
	bonusOnlyEvent := &masterdata.Event{ID: 501, EventType: "marathon", Name: "bonus only", AssetBundleName: "e501", StartAt: 100, AggregateAt: 200}
	cardEvent := &masterdata.Event{ID: 502, EventType: "marathon", Name: "card event", AssetBundleName: "e502", StartAt: 300, AggregateAt: 400}
	source.events = []*masterdata.Event{bonusOnlyEvent, cardEvent}
	source.eventsByID[bonusOnlyEvent.ID] = bonusOnlyEvent
	source.eventsByID[cardEvent.ID] = cardEvent
	source.cardsByEvent[bonusOnlyEvent.ID] = []*masterdata.Card{{ID: 5001, CharacterID: 5, Attr: "cool", AssetBundleName: "card_5001"}}
	source.cardsByEvent[cardEvent.ID] = []*masterdata.Card{{ID: 5002, CharacterID: 21, Attr: "cool", AssetBundleName: "card_5002"}}
	source.bonusesByEvent[bonusOnlyEvent.ID] = []*masterdata.EventDeckBonus{{ID: 1, EventID: bonusOnlyEvent.ID, GameCharacterUnitID: 210, CardAttr: "cool"}}
	source.gcuByID[210] = &masterdata.GameCharacterUnit{ID: 210, GameCharacterID: 21, Unit: "piapro"}

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	req, err := builder.BuildEventListRequest(ListQuery{
		Region:        renderregion.JP,
		CharacterID:   21,
		IncludePast:   true,
		IncludeFuture: true,
	})
	if err != nil {
		t.Fatalf("BuildEventListRequest failed: %v", err)
	}
	if len(req.EventInfo) != 1 || req.EventInfo[0].ID != cardEvent.ID {
		t.Fatalf("expected only event with Miku card, got %+v", req.EventInfo)
	}
}

func TestBuildEventListRequestOnlyUnitUsesAllEventCards(t *testing.T) {
	source := newTestEventSource(renderregion.JP)
	pureEvent := &masterdata.Event{ID: 601, EventType: "marathon", Name: "pure mmj", AssetBundleName: "e601", StartAt: 100, AggregateAt: 200}
	mixedEvent := &masterdata.Event{ID: 602, EventType: "marathon", Name: "mixed mmj", AssetBundleName: "e602", StartAt: 300, AggregateAt: 400}
	vsEvent := &masterdata.Event{ID: 603, EventType: "marathon", Name: "vs only", AssetBundleName: "e603", StartAt: 500, AggregateAt: 600}
	source.events = []*masterdata.Event{pureEvent, mixedEvent, vsEvent}
	for _, eventInfo := range source.events {
		source.eventsByID[eventInfo.ID] = eventInfo
	}
	source.characterByID[5] = &masterdata.Character{ID: 5, Unit: "idol"}
	source.characterByID[6] = &masterdata.Character{ID: 6, Unit: "idol"}
	source.characterByID[10] = &masterdata.Character{ID: 10, Unit: "street"}
	source.characterByID[21] = &masterdata.Character{ID: 21, Unit: "piapro"}
	source.cardsByEvent[pureEvent.ID] = []*masterdata.Card{
		{ID: 60101, CharacterID: 5, AssetBundleName: "card_60101"},
		{ID: 60102, CharacterID: 21, SupportUnit: "idol", AssetBundleName: "card_60102"},
		{ID: 60103, CharacterID: 6, AssetBundleName: "card_60103"},
	}
	source.cardsByEvent[mixedEvent.ID] = []*masterdata.Card{
		{ID: 60201, CharacterID: 5, AssetBundleName: "card_60201"},
		{ID: 60202, CharacterID: 21, SupportUnit: "street", AssetBundleName: "card_60202"},
	}
	source.cardsByEvent[vsEvent.ID] = []*masterdata.Card{
		{ID: 60301, CharacterID: 21, SupportUnit: "none", AssetBundleName: "card_60301"},
	}

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	req, err := builder.BuildEventListRequest(ListQuery{
		Region:        renderregion.JP,
		Unit:          "idol",
		OnlyUnit:      true,
		IncludePast:   true,
		IncludeFuture: true,
	})
	if err != nil {
		t.Fatalf("BuildEventListRequest failed: %v", err)
	}
	if len(req.EventInfo) != 1 || req.EventInfo[0].ID != pureEvent.ID {
		t.Fatalf("expected only pure MMJ event, got %+v", req.EventInfo)
	}
}

func TestBuildEventListRequestWorldBloomTurnUsesFilteredEvents(t *testing.T) {
	source := newTestEventSource(renderregion.JP)
	first := &masterdata.Event{ID: 140, EventType: "world_bloom", Unit: "none", Name: "miku wl1", AssetBundleName: "e140", StartAt: 100, AggregateAt: 200}
	other := &masterdata.Event{ID: 150, EventType: "world_bloom", Unit: "street", Name: "other wl", AssetBundleName: "e150", StartAt: 300, AggregateAt: 400}
	second := &masterdata.Event{ID: 179, EventType: "world_bloom", Unit: "none", Name: "miku wl2", AssetBundleName: "e179", StartAt: 500, AggregateAt: 600}
	third := &masterdata.Event{ID: 202, EventType: "world_bloom", Unit: "none", Name: "miku wl3", AssetBundleName: "e202", StartAt: 700, AggregateAt: 800}
	source.events = []*masterdata.Event{first, other, second, third}
	for _, eventInfo := range source.events {
		source.eventsByID[eventInfo.ID] = eventInfo
	}
	source.cardsByEvent[first.ID] = []*masterdata.Card{{ID: 14001, CharacterID: 21, Attr: "cool", AssetBundleName: "card_14001"}}
	source.cardsByEvent[other.ID] = []*masterdata.Card{{ID: 15001, CharacterID: 5, Attr: "cool", AssetBundleName: "card_15001"}}
	source.cardsByEvent[second.ID] = []*masterdata.Card{{ID: 17901, CharacterID: 21, Attr: "cool", AssetBundleName: "card_17901"}}
	source.cardsByEvent[third.ID] = []*masterdata.Card{
		{ID: 20201, CharacterID: 17, Attr: "cool", AssetBundleName: "card_20201"},
		{ID: 20202, CharacterID: 18, Attr: "cool", AssetBundleName: "card_20202"},
		{ID: 20203, CharacterID: 19, Attr: "cool", AssetBundleName: "card_20203"},
		{ID: 20204, CharacterID: 20, Attr: "cool", AssetBundleName: "card_20204"},
		{ID: 20205, CharacterID: 21, Attr: "cool", AssetBundleName: "card_20205"},
	}

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	req, err := builder.BuildEventListRequest(ListQuery{
		Region:         renderregion.JP,
		EventType:      "world_bloom",
		WorldBloomTurn: 3,
		CharacterID:    21,
		IncludePast:    true,
		IncludeFuture:  true,
	})
	if err != nil {
		t.Fatalf("BuildEventListRequest failed: %v", err)
	}
	if len(req.EventInfo) != 1 || req.EventInfo[0].ID != third.ID {
		t.Fatalf("expected Miku wl3 event 202, got %+v", req.EventInfo)
	}
}

func TestBuildEventListRequestWorldBloomTurnMatchesEachUnitRound(t *testing.T) {
	source := newTestEventSource(renderregion.JP)
	events := []*masterdata.Event{
		{ID: 112, EventType: "world_bloom", Unit: "school_refusal", Name: "n25 wl1", AssetBundleName: "e112", StartAt: 100, AggregateAt: 110},
		{ID: 118, EventType: "world_bloom", Unit: "street", Name: "vbs wl1", AssetBundleName: "e118", StartAt: 200, AggregateAt: 210},
		{ID: 140, EventType: "world_bloom", Unit: "none", Name: "vs wl1", AssetBundleName: "e140", StartAt: 300, AggregateAt: 310},
		{ID: 163, EventType: "world_bloom", Unit: "street", Name: "vbs wl2", AssetBundleName: "e163", StartAt: 400, AggregateAt: 410},
		{ID: 179, EventType: "world_bloom", Unit: "none", Name: "vs wl2", AssetBundleName: "e179", StartAt: 500, AggregateAt: 510},
		{ID: 180, EventType: "world_bloom", Unit: "none", Name: "vs finale", AssetBundleName: "e180", StartAt: 600, AggregateAt: 610},
		{ID: 202, EventType: "world_bloom", Unit: "none", Name: "vs wl3", AssetBundleName: "e202", StartAt: 700, AggregateAt: 710},
	}
	source.events = events
	for _, eventInfo := range source.events {
		source.eventsByID[eventInfo.ID] = eventInfo
	}
	source.worldByEvent[180] = []*masterdata.WorldBloom{{EventID: 180, ChapterType: "finale"}}

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	testCases := []struct {
		name string
		turn int
		want []int
	}{
		{name: "wl1", turn: 1, want: []int{112, 118, 140}},
		{name: "wl2", turn: 2, want: []int{163, 179}},
		{name: "wl3", turn: 3, want: []int{202}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := builder.BuildEventListRequest(ListQuery{
				Region:         renderregion.JP,
				EventType:      "world_bloom",
				WorldBloomTurn: tc.turn,
				IncludePast:    true,
				IncludeFuture:  true,
			})
			if err != nil {
				t.Fatalf("BuildEventListRequest failed: %v", err)
			}
			got := make([]int, 0, len(req.EventInfo))
			for _, eventInfo := range req.EventInfo {
				got = append(got, eventInfo.ID)
			}
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("unexpected %s events: got %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestBuildEventDetailRequestWorldBloomTimelineIncludesChapterColorsAndTimes(t *testing.T) {
	source := newTestEventSource(renderregion.JP)
	eventInfo := &masterdata.Event{
		ID:              701,
		EventType:       "world_bloom",
		Name:            "WL Detail",
		AssetBundleName: "wl_701",
		StartAt:         1000,
		AggregateAt:     9000,
	}
	source.eventsByID[eventInfo.ID] = eventInfo
	charA := 21
	charB := 22
	source.worldByEvent[eventInfo.ID] = []*masterdata.WorldBloom{
		{
			ID:              2,
			EventID:         eventInfo.ID,
			ChapterNo:       2,
			GameCharacterID: &charB,
			ChapterStartAt:  5000,
			AggregateAt:     7000,
			ChapterEndAt:    8000,
			ChapterType:     "chapter",
		},
		{
			ID:              1,
			EventID:         eventInfo.ID,
			ChapterNo:       1,
			GameCharacterID: &charA,
			ChapterStartAt:  1000,
			AggregateAt:     3000,
			ChapterEndAt:    3000 + 9*time.Minute.Milliseconds(),
			ChapterType:     "chapter",
		},
	}
	source.colorByCharID[charA] = "#33AAFF"
	source.colorByCharID[charB] = "#FFAA33"
	source.characterByID[charA] = &masterdata.Character{ID: charA, FirstName: "初音", GivenName: "未来"}
	source.characterByID[charB] = &masterdata.Character{ID: charB, FirstName: "镜音", GivenName: "铃"}

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	req, err := builder.BuildEventDetailRequest(DetailQuery{Region: renderregion.JP, EventID: eventInfo.ID})
	if err != nil {
		t.Fatalf("BuildEventDetailRequest failed: %v", err)
	}
	if len(req.EventInfo.WlTimeList) != 2 {
		t.Fatalf("expected 2 WL chapter timeline entries, got %+v", req.EventInfo.WlTimeList)
	}
	first := req.EventInfo.WlTimeList[0]
	if first["chapter_id"] != 1 || first["chapter_no"] != 1 {
		t.Fatalf("unexpected first chapter metadata: %+v", first)
	}
	if first["game_character_id"] != charA || first["color_code"] != "#33AAFF" || first["character_color_code"] != "#33AAFF" {
		t.Fatalf("unexpected first chapter character color: %+v", first)
	}
	if first["character_name"] != "初音未来" || first["character_icon_path"] != "static_images/chara_icon/miku.png" {
		t.Fatalf("unexpected first chapter character identity: %+v", first)
	}
	if first["start_at"] != int64(1000) || first["aggregate_at"] != int64(3000) || first["end_at"] != int64(4000) {
		t.Fatalf("unexpected legacy chapter times: %+v", first)
	}
	if first["chapter_start_at"] != int64(1000) || first["chapter_aggregate_at"] != int64(3000) || first["chapter_end_at"] != int64(4000) {
		t.Fatalf("unexpected explicit chapter times: %+v", first)
	}
	second := req.EventInfo.WlTimeList[1]
	if second["chapter_id"] != 2 || second["game_character_id"] != charB || second["color_code"] != "#FFAA33" {
		t.Fatalf("unexpected second chapter timeline entry: %+v", second)
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

func TestBuildEventDetailRequestAllowsEventWithoutCards(t *testing.T) {
	source := newTestEventSource(renderregion.JP)
	eventInfo := &masterdata.Event{
		ID:              166,
		EventType:       "marathon",
		Name:            "No Card Event",
		AssetBundleName: "event_166",
		StartAt:         1000,
		AggregateAt:     2000,
	}
	source.eventsByID[eventInfo.ID] = eventInfo
	source.cardErrByEvent[eventInfo.ID] = fmt.Errorf("no cards found for event %d", eventInfo.ID)

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	req, err := builder.BuildEventDetailRequest(DetailQuery{Region: renderregion.JP, EventID: eventInfo.ID})
	if err != nil {
		t.Fatalf("BuildEventDetailRequest failed: %v", err)
	}
	if req.EventInfo.ID != eventInfo.ID {
		t.Fatalf("unexpected event info: %+v", req.EventInfo)
	}
	if len(req.EventCards) != 0 {
		t.Fatalf("expected no event cards, got %+v", req.EventCards)
	}
}
