package sk

import (
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestEventCandidateRefactorBranches(t *testing.T) {
	now := time.Now().UnixMilli()
	candidates := eventCandidates{}
	candidates.consider(nil, now)
	candidates.consider(&masterdata.Event{ID: 1, StartAt: now - 2_000, AggregateAt: now + 2_000}, now)
	candidates.consider(&masterdata.Event{ID: 2, StartAt: now - 1_000, AggregateAt: now + 2_000}, now)
	candidates.consider(&masterdata.Event{ID: 4, StartAt: now + 4_000}, now)
	candidates.consider(&masterdata.Event{ID: 3, StartAt: now + 3_000}, now)
	if got := candidates.preferredID(); got != 2 {
		t.Fatalf("preferred current event = %d", got)
	}
	if got := (eventCandidates{next: candidates.next}).preferredID(); got != 3 {
		t.Fatalf("preferred next event = %d", got)
	}
	if got := (eventCandidates{latest: candidates.latest}).preferredID(); got != 4 {
		t.Fatalf("preferred latest event = %d", got)
	}
	if got := (eventCandidates{}).preferredID(); got != 0 {
		t.Fatalf("empty candidate event = %d", got)
	}
}

func TestTrackerMetaResidualBranches(t *testing.T) {
	name := "  Event Name  "
	startAt := int64(100)
	aggregateAt := int64(200)
	banner := " banner.png "
	meta := eventMeta{}
	meta.applyOverrides(TrackerRankQuery{
		EventName:        &name,
		EventStartAt:     &startAt,
		EventAggregateAt: &aggregateAt,
		BannerImgPath:    &banner,
	})
	if meta.name != "Event Name" || meta.startAt != startAt || meta.aggregateAt != aggregateAt || meta.bannerPath != "banner.png" {
		t.Fatalf("overridden tracker meta = %+v", meta)
	}
	if got := formatTrackerTimestamp(1_700_000_000_000); got != 1_700_000_000_000 {
		t.Fatalf("millisecond timestamp = %d", got)
	}
	if got := formatTrackerTimestamp(1_700_000_000); got != 1_700_000_000_000 {
		t.Fatalf("second timestamp = %d", got)
	}
	if got := formatTrackerTimestamp(0); got <= 0 {
		t.Fatalf("fallback timestamp = %d", got)
	}
}
