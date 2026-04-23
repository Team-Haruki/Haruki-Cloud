package event

import (
	"context"
	"errors"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/releasecheck"
)

type eventContextKey string

type contextAwareEventSource struct {
	*testEventSource
	ctx       context.Context
	wantKey   eventContextKey
	wantValue string
}

func (s *contextAwareEventSource) WithContext(ctx context.Context) DataSource {
	clone := *s
	clone.ctx = ctx
	return &clone
}

func (s *contextAwareEventSource) GetEvents() []*masterdata.Event {
	if s.ctx == nil {
		return nil
	}
	value, _ := s.ctx.Value(s.wantKey).(string)
	if value != s.wantValue {
		return nil
	}
	return s.testEventSource.GetEvents()
}

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

func TestControllerBuildEventDetailRequestReturnsNoOngoingEventAfterAggregateBeforeNextStart(t *testing.T) {
	now := time.Now().UnixMilli()

	source := newTestEventSource(renderregion.JP)
	prev := &masterdata.Event{
		ID:              10,
		EventType:       "marathon",
		Name:            "Prev",
		AssetBundleName: "prev",
		StartAt:         now - 10_000,
		AggregateAt:     now - 5_000,
		ClosedAt:        now + 5_000,
	}
	next := &masterdata.Event{
		ID:              20,
		EventType:       "marathon",
		Name:            "Next",
		AssetBundleName: "next",
		StartAt:         now + 10_000,
		AggregateAt:     now + 20_000,
		ClosedAt:        now + 25_000,
	}
	source.events = []*masterdata.Event{prev, next}
	source.eventsByID[prev.ID] = prev
	source.eventsByID[next.ID] = next

	controller := NewController(source, nil, assets.NewAssetHelper("", nil))
	req, err := controller.BuildEventDetailRequest(DetailQuery{
		Region:     renderregion.JP,
		UseCurrent: true,
	})
	if err == nil {
		t.Fatalf("expected no ongoing event error, got %+v", req)
	}
	if !errors.Is(err, ErrNoOngoingEvent) {
		t.Fatalf("expected ErrNoOngoingEvent, got %v", err)
	}
}

func TestResolveCurrentEventIndexUsesClosedWindowBeforeNextStart(t *testing.T) {
	now := time.Now().UnixMilli()
	events := []*masterdata.Event{
		{
			ID:          10,
			EventType:   "marathon",
			Name:        "Prev",
			StartAt:     now - 10_000,
			AggregateAt: now - 5_000,
			ClosedAt:    now + 5_000,
		},
		{
			ID:          20,
			EventType:   "marathon",
			Name:        "Next",
			StartAt:     now + 10_000,
			AggregateAt: now + 20_000,
			ClosedAt:    now + 25_000,
		},
	}

	index, err := resolveCurrentEventIndex(events, "next_first")
	if err != nil {
		t.Fatalf("resolveCurrentEventIndex() error = %v", err)
	}
	if index != 0 {
		t.Fatalf("expected closed-window current index 0, got %d", index)
	}
}

func TestControllerBuildEventDetailRequestRejectsExplicitUnreleasedNextEvent(t *testing.T) {
	now := time.Now().UnixMilli()

	source := newTestEventSource(renderregion.JP)
	past := &masterdata.Event{ID: 10, EventType: "marathon", Name: "Past", AssetBundleName: "past", StartAt: now - 20_000, AggregateAt: now - 10_000}
	next := &masterdata.Event{ID: 20, EventType: "marathon", Name: "Next", AssetBundleName: "next", StartAt: now + 10_000, AggregateAt: now + 20_000}
	source.events = []*masterdata.Event{past, next}
	source.eventsByID[past.ID] = past
	source.eventsByID[next.ID] = next

	controller := NewController(source, nil, assets.NewAssetHelper("", nil))
	req, err := controller.BuildEventDetailRequest(DetailQuery{
		Region:  renderregion.JP,
		Keyword: "next",
	})
	if err == nil {
		t.Fatalf("expected unreleased next event to fail, got %+v", req)
	}
	var unreleased *releasecheck.UnreleasedError
	if !errors.As(err, &unreleased) {
		t.Fatalf("expected unreleased error, got %T (%v)", err, err)
	}
	if unreleased.Kind != releasecheck.KindEvent || unreleased.ID != 20 {
		t.Fatalf("unexpected unreleased error: %+v", unreleased)
	}
}

func TestControllerBuildEventDetailRequestAllowsExplicitUnreleasedEventForNonJPLookup(t *testing.T) {
	now := time.Now().UnixMilli()

	source := newTestEventSource(renderregion.CN)
	next := &masterdata.Event{ID: 20, EventType: "marathon", Name: "Next", AssetBundleName: "next", StartAt: now + 10_000, AggregateAt: now + 20_000}
	source.events = []*masterdata.Event{next}
	source.eventsByID[next.ID] = next

	controller := NewController(source, nil, assets.NewAssetHelper("", nil))
	req, err := controller.BuildEventDetailRequest(DetailQuery{
		Region:          renderregion.CN,
		EventID:         20,
		AllowUnreleased: true,
	})
	if err != nil {
		t.Fatalf("BuildEventDetailRequest failed: %v", err)
	}
	if req.EventInfo.ID != 20 {
		t.Fatalf("expected future event id 20, got %v", req.EventInfo.ID)
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

	controller := NewController(source, nil, assets.NewAssetHelper("", nil))
	req, err := controller.BuildEventDetailRequest(DetailQuery{
		Region: renderregion.JP,
		Index:  new(-2),
	})
	if err != nil {
		t.Fatalf("BuildEventDetailRequest failed: %v", err)
	}
	if req.EventInfo.ID != 10 {
		t.Fatalf("expected previous event id 10, got %v", req.EventInfo.ID)
	}
}

func TestControllerWithContextClonesEventSource(t *testing.T) {
	source := &contextAwareEventSource{
		testEventSource: newTestEventSource(renderregion.JP),
		wantKey:         eventContextKey("trace"),
		wantValue:       "event-list",
	}
	eventInfo := &masterdata.Event{ID: 1, EventType: "marathon", Name: "Ctx Event", AssetBundleName: "ctx_event", StartAt: 100, AggregateAt: 200}
	source.events = []*masterdata.Event{eventInfo}
	source.eventsByID[eventInfo.ID] = eventInfo

	controller := NewController(source, nil, assets.NewAssetHelper("", nil))
	ctx := context.WithValue(context.Background(), eventContextKey("trace"), "event-list")

	req, err := controller.WithContext(ctx).BuildEventListRequest(ListQuery{
		Region:        renderregion.JP,
		IncludePast:   true,
		IncludeFuture: true,
	})
	if err != nil {
		t.Fatalf("BuildEventListRequest failed: %v", err)
	}
	if len(req.EventInfo) != 1 || req.EventInfo[0].EventName != "Ctx Event" {
		t.Fatalf("unexpected event list payload: %+v", req.EventInfo)
	}
}
