package event

import (
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

func TestControllerBuildEventListRequestUsesRequestedRegionSource(t *testing.T) {
	cn := newTestEventSource(renderregion.CN)
	cnEvent := &masterdata.Event{ID: 1, EventType: "world_bloom", Name: "CN_NAME", AssetBundleName: "cn_ev", StartAt: 100, AggregateAt: 200}
	cn.events = []*masterdata.Event{cnEvent}
	cn.eventsByID[cnEvent.ID] = cnEvent

	jp := newTestEventSource(renderregion.JP)
	jpEvent := &masterdata.Event{ID: 1, EventType: "world_bloom", Name: "JP_NAME", AssetBundleName: "jp_ev", StartAt: 100, AggregateAt: 200}
	jp.events = []*masterdata.Event{jpEvent}
	jp.eventsByID[jpEvent.ID] = jpEvent

	controller := NewController(cn, nil, assets.NewAssetHelper("", nil))
	controller.RegisterSource(jp)

	req, err := controller.BuildEventListRequest(ListQuery{
		Region:        renderregion.JP,
		EventType:     "world_bloom",
		IncludePast:   true,
		IncludeFuture: true,
	})
	if err != nil {
		t.Fatalf("BuildEventListRequest failed: %v", err)
	}
	if len(req.EventInfo) != 1 {
		t.Fatalf("expected 1 event, got %d", len(req.EventInfo))
	}
	if req.EventInfo[0].EventName != "JP_NAME" {
		t.Fatalf("expected JP source event name, got %q", req.EventInfo[0].EventName)
	}
}

func TestControllerBuildEventListRequestDefaultsToAllEvents(t *testing.T) {
	now := time.Now().UnixMilli()

	source := newTestEventSource(renderregion.JP)
	past := &masterdata.Event{ID: 10, EventType: "marathon", Name: "Past", AssetBundleName: "past", StartAt: now - 20_000, AggregateAt: now - 10_000}
	future := &masterdata.Event{ID: 20, EventType: "marathon", Name: "Future", AssetBundleName: "future", StartAt: now + 10_000, AggregateAt: now + 20_000}
	source.events = []*masterdata.Event{past, future}
	source.eventsByID[past.ID] = past
	source.eventsByID[future.ID] = future

	controller := NewController(source, nil, assets.NewAssetHelper("", nil))
	req, err := controller.BuildEventListRequest(ListQuery{Region: renderregion.JP})
	if err != nil {
		t.Fatalf("BuildEventListRequest failed: %v", err)
	}
	if len(req.EventInfo) != 2 {
		t.Fatalf("expected 2 events, got %d", len(req.EventInfo))
	}
}

func TestControllerBuildEventDetailRequestUsesCurrentEventWhenRequested(t *testing.T) {
	now := time.Now().UnixMilli()

	source := newTestEventSource(renderregion.JP)
	past := &masterdata.Event{ID: 10, EventType: "marathon", Name: "Past", AssetBundleName: "past", StartAt: now - 10_000, AggregateAt: now - 5_000}
	current := &masterdata.Event{ID: 20, EventType: "marathon", Name: "Current", AssetBundleName: "current", StartAt: now - 1_000, AggregateAt: now + 5_000}
	source.events = []*masterdata.Event{past, current}
	source.eventsByID[past.ID] = past
	source.eventsByID[current.ID] = current

	controller := NewController(source, nil, assets.NewAssetHelper("", nil))
	req, err := controller.BuildEventDetailRequest(DetailQuery{
		Region:     renderregion.JP,
		UseCurrent: true,
	})
	if err != nil {
		t.Fatalf("BuildEventDetailRequest failed: %v", err)
	}
	if req.EventInfo.ID != 20 {
		t.Fatalf("expected current event id 20, got %v", req.EventInfo.ID)
	}
}

func TestControllerBuildEventDetailRequestFallsBackToNextEvent(t *testing.T) {
	now := time.Now().UnixMilli()

	source := newTestEventSource(renderregion.JP)
	past := &masterdata.Event{ID: 10, EventType: "marathon", Name: "Past", AssetBundleName: "past", StartAt: now - 20_000, AggregateAt: now - 10_000}
	next := &masterdata.Event{ID: 20, EventType: "marathon", Name: "Next", AssetBundleName: "next", StartAt: now + 10_000, AggregateAt: now + 20_000}
	source.events = []*masterdata.Event{past, next}
	source.eventsByID[past.ID] = past
	source.eventsByID[next.ID] = next

	controller := NewController(source, nil, assets.NewAssetHelper("", nil))
	req, err := controller.BuildEventDetailRequest(DetailQuery{
		Region:     renderregion.JP,
		UseCurrent: true,
	})
	if err != nil {
		t.Fatalf("BuildEventDetailRequest failed: %v", err)
	}
	if req.EventInfo.ID != 20 {
		t.Fatalf("expected next event id 20, got %v", req.EventInfo.ID)
	}
}

func TestControllerBuildEventDetailRequestResolvesBanSequence(t *testing.T) {
	now := time.Now().UnixMilli()

	source := newTestEventSource(renderregion.JP)
	first := &masterdata.Event{ID: 10, EventType: "marathon", Name: "First", AssetBundleName: "first", StartAt: now - 20_000, AggregateAt: now - 15_000}
	second := &masterdata.Event{ID: 20, EventType: "marathon", Name: "Second", AssetBundleName: "second", StartAt: now - 10_000, AggregateAt: now - 5_000}
	source.events = []*masterdata.Event{first, second}
	source.eventsByID[first.ID] = first
	source.eventsByID[second.ID] = second
	source.banEventsByChar[5] = []*masterdata.Event{first, second}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil))
	req, err := controller.BuildEventDetailRequest(DetailQuery{
		Region:    renderregion.JP,
		BanCharID: 5,
		BanSeq:    2,
	})
	if err != nil {
		t.Fatalf("BuildEventDetailRequest failed: %v", err)
	}
	if req.EventInfo.ID != 20 {
		t.Fatalf("expected second ban event id 20, got %v", req.EventInfo.ID)
	}
}

func TestControllerBuildEventDetailRequestResolvesNegativeSequenceLikeRefer(t *testing.T) {
	now := time.Now().UnixMilli()

	source := newTestEventSource(renderregion.JP)
	prev := &masterdata.Event{ID: 10, EventType: "marathon", Name: "Prev", AssetBundleName: "prev", StartAt: now - 20_000, AggregateAt: now - 10_000}
	current := &masterdata.Event{ID: 20, EventType: "marathon", Name: "Current", AssetBundleName: "current", StartAt: now - 1_000, AggregateAt: now + 5_000}
	next := &masterdata.Event{ID: 30, EventType: "marathon", Name: "Next", AssetBundleName: "next", StartAt: now + 10_000, AggregateAt: now + 20_000}
	source.events = []*masterdata.Event{prev, current, next}
	source.eventsByID[prev.ID] = prev
	source.eventsByID[current.ID] = current
	source.eventsByID[next.ID] = next

	index := -2
	controller := NewController(source, nil, assets.NewAssetHelper("", nil))
	req, err := controller.BuildEventDetailRequest(DetailQuery{
		Region: renderregion.JP,
		Index:  &index,
	})
	if err != nil {
		t.Fatalf("BuildEventDetailRequest failed: %v", err)
	}
	if req.EventInfo.ID != 10 {
		t.Fatalf("expected previous event id 10, got %v", req.EventInfo.ID)
	}
}
