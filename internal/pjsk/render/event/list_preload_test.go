package event

import (
	"errors"
	"reflect"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type preloadedEventListSource struct {
	*testEventSource
	preloaded []int
	calls     int
}

func (source *preloadedEventListSource) PreloadEventList(ids []int) error {
	source.preloaded = append([]int(nil), ids...)
	source.calls++
	return errors.New("prefetch failed")
}

func TestEventListPreloadsMetadataCandidatesWithoutChangingFallback(t *testing.T) {
	base := newTestEventSource(renderregion.JP)
	base.events = []*masterdata.Event{
		{ID: 3, EventType: "marathon", StartAt: 3000},
		{ID: 2, EventType: "world_bloom", StartAt: 2000},
		{ID: 1, EventType: "marathon", StartAt: 1000},
	}
	source := &preloadedEventListSource{testEventSource: base}
	builder := NewBuilder(source, assets.NewAssetHelper(t.TempDir(), nil))
	request, err := builder.BuildEventListRequest(ListQuery{EventType: "marathon", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if source.calls != 1 || !reflect.DeepEqual(source.preloaded, []int{1}) {
		t.Fatalf("preload calls=%d ids=%v", source.calls, source.preloaded)
	}
	if len(request.EventInfo) != 1 || request.EventInfo[0].ID != 1 {
		t.Fatalf("list order/limit/fallback changed: %+v", request.EventInfo)
	}
}

func TestEventListPreloadsAllCandidatesForRelationFilters(t *testing.T) {
	for _, query := range []ListQuery{
		{Unit: "idol"}, {OnlyUnit: true}, {Blend: true}, {Attr: "cute"},
		{CharacterID: 1}, {CharacterIDs: []int{1}}, {BannerCharID: new(1)},
	} {
		base := newTestEventSource(renderregion.JP)
		base.events = []*masterdata.Event{
			{ID: 3, EventType: "marathon", StartAt: 3000},
			{ID: 2, EventType: "world_bloom", StartAt: 2000},
			{ID: 1, EventType: "marathon", StartAt: 1000},
		}
		source := &preloadedEventListSource{testEventSource: base}
		builder := NewBuilder(source, assets.NewAssetHelper(t.TempDir(), nil))
		query.EventType, query.Limit = "marathon", 1
		_ = builder.filterEvents(query)
		if source.calls != 1 || !reflect.DeepEqual(source.preloaded, []int{3, 1}) {
			t.Fatalf("query=%+v preload calls=%d ids=%v", query, source.calls, source.preloaded)
		}
	}
}

func TestEventListPreloadMatchesDisplayedWindowWithEqualStartTimes(t *testing.T) {
	base := newTestEventSource(renderregion.JP)
	for _, id := range []int{3, 1, 2} {
		base.events = append(base.events, &masterdata.Event{ID: id, EventType: "marathon", StartAt: 1000})
	}
	source := &preloadedEventListSource{testEventSource: base}
	builder := NewBuilder(source, assets.NewAssetHelper(t.TempDir(), nil))
	result := builder.filterEvents(ListQuery{Limit: 2})
	ids := make([]int, len(result))
	for i, event := range result {
		ids[i] = event.ID
	}
	if !reflect.DeepEqual(source.preloaded, ids) || !reflect.DeepEqual(ids, []int{3, 1}) {
		t.Fatalf("preloaded=%v displayed=%v", source.preloaded, ids)
	}
}
