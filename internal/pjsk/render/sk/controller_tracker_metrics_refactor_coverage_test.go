package sk

import (
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
)

func TestTrackerMetricRefactorResidualBranches(t *testing.T) {
	now := time.Unix(10_000, 0)
	applyRankInfoMetricsAt(nil, nil, now)

	info := drawing.RankInfo{}
	applyRankInfoMetricsAt(&info, []trackerScoreSample{{score: 10, timestamp: 1}}, now)
	applyTrackerDeltaMetrics(&info, []trackerScoreSample{{score: 20}, {score: 10}})
	if info.LatestPt != nil {
		t.Fatalf("non-positive deltas produced latest points: %+v", info.LatestPt)
	}

	applyTrackerHourMetrics(&info, nil, trackerScoreSample{}, 0)
	applyTrackerTwentyMinuteMetrics(&info, nil, trackerScoreSample{}, 0)
	if got := recoveryRecordStartAt(nil); got != nil {
		t.Fatalf("empty recovery start = %v", *got)
	}
	if got := countPositiveDeltas(nil); got != 0 {
		t.Fatalf("empty positive delta count = %d", got)
	}
	if got := normalizedTrackerScoreSamples([]trackerScoreSample{{timestamp: 0}, {timestamp: 1}}); len(got) != 1 {
		t.Fatalf("normalized tracker samples = %+v", got)
	}
}
