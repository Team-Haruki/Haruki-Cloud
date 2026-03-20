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
