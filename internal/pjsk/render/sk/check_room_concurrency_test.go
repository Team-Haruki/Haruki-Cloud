package sk

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

// overlapTrackerSource answers every rank with a fixed row and records how
// many trace and rank-query calls were in flight at the same time.
type overlapTrackerSource struct {
	additionalCloudTrackerSource
	inflight, peak atomic.Int32
	calls          atomic.Int32
}

func (s *overlapTrackerSource) enter() {
	s.calls.Add(1)
	current := s.inflight.Add(1)
	for {
		peak := s.peak.Load()
		if current <= peak || s.peak.CompareAndSwap(peak, current) {
			break
		}
	}
	time.Sleep(30 * time.Millisecond)
	s.inflight.Add(-1)
}

func (s *overlapTrackerSource) GetCloudSKQuery(_ string, _ int, _ *int, ranks []int, _ *int64, _, _ bool, _ int64) (*sekaiapi.CloudRankQueryResponse, error) {
	s.enter()
	uid := strconv.Itoa(ranks[0] * 7)
	return &sekaiapi.CloudRankQueryResponse{Ranks: []sekaiapi.CloudRankInfo{{Rank: ranks[0], UserID: &uid, Score: 1000 - ranks[0]}}}, nil
}

func (s *overlapTrackerSource) GetCloudSKTrace(string, int, *int, string, string, int) (*sekaiapi.CloudTraceResponse, error) {
	s.enter()
	now := time.Now().UnixMilli()
	return &sekaiapi.CloudTraceResponse{RankData: []sekaiapi.CloudRankInfo{{Rank: 50, Score: 100, Timestamp: now - 3600_000}, {Rank: 50, Score: 900, Timestamp: now}}}, nil
}

func TestCheckRoomOverlapsTraceAndAdjacentLookups(t *testing.T) {
	uid := "700"
	tracker := &overlapTrackerSource{}
	tracker.checkResp = &sekaiapi.CloudCheckRoomResponse{Rank: sekaiapi.CloudRankInfo{Rank: 50, UserID: &uid, Score: 950}}
	controller := NewController(nil)
	controller.SetTrackerIntegration(tracker, nil, nil)

	payload, err := controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{50}})
	if err != nil {
		t.Fatalf("BuildCheckRoomRequestFromTracker() error = %v", err)
	}
	if len(payload.Ranks) != 1 || payload.Ranks[0].Rank != 50 || payload.Ranks[0].RecordStartAt == nil {
		t.Fatalf("target rank was not enriched from its trace: %+v", payload.Ranks)
	}
	if payload.PrevRank == nil || payload.PrevRank.Rank != 49 || payload.NextRank == nil || payload.NextRank.Rank != 51 {
		t.Fatalf("adjacent ranks = %+v, %+v", payload.PrevRank, payload.NextRank)
	}
	if peak := tracker.peak.Load(); peak < 3 {
		t.Fatalf("peak concurrent tracker calls = %d, want the trace and both neighbours together", peak)
	}

	// Rank 1 has no previous neighbour: only the trace and the next rank run.
	tracker.checkResp = &sekaiapi.CloudCheckRoomResponse{Rank: sekaiapi.CloudRankInfo{Rank: 1, UserID: &uid, Score: 999}}
	tracker.calls.Store(0)
	payload, err = controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{1}})
	if err != nil || payload.PrevRank != nil || payload.NextRank == nil || payload.NextRank.Rank != 2 {
		t.Fatalf("rank 1 payload = %+v, %v", payload, err)
	}
	if calls := tracker.calls.Load(); calls != 3 {
		t.Fatalf("rank 1 tracker calls = %d, want trace + next rank (2 queries)", calls)
	}
}
