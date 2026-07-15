package drawingcache

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestCacheStatsTrackerBoundsHighCardinalityPaths(t *testing.T) {
	tracker := newCacheStatsTracker(nil)
	const pathCount = 10_000
	for i := 0; i < pathCount; i++ {
		tracker.recordMiss(fmt.Sprintf("api/pjsk/card/attacker-%d", i), cacheMissReasonLookup)
	}

	snapshot := tracker.snapshot("")
	if snapshot.Totals.Misses != pathCount {
		t.Fatalf("total misses = %d, want %d", snapshot.Totals.Misses, pathCount)
	}
	if len(snapshot.Paths) > cacheStatsMaxTrackedPaths+2 {
		t.Fatalf("path stats grew to %d entries, want at most %d", len(snapshot.Paths), cacheStatsMaxTrackedPaths+2)
	}

	var accounted int64
	var foundOther bool
	for _, path := range snapshot.Paths {
		accounted += path.Misses
		if path.APIPath == cacheStatsPathOther {
			foundOther = true
		}
	}
	if !foundOther {
		t.Fatal("expected evicted path counters to be aggregated into other")
	}
	if accounted != pathCount {
		t.Fatalf("path misses = %d, want %d", accounted, pathCount)
	}
}

func TestCacheStatsTrackerCollapsesUntrustedNamespacesAndOversizedPaths(t *testing.T) {
	tracker := newCacheStatsTracker(nil)
	for i := 0; i < 1_000; i++ {
		tracker.recordHit(fmt.Sprintf("attacker/%d", i))
	}
	tracker.recordStore("api/pjsk/" + strings.Repeat("x", maxCacheAPISegmentBytes+1))

	snapshot := tracker.snapshot(cacheStatsPathOther)
	if len(snapshot.Paths) != 1 || snapshot.Paths[0].APIPath != cacheStatsPathOther {
		t.Fatalf("unexpected other snapshot: %+v", snapshot.Paths)
	}
	if snapshot.Paths[0].Hits != 1_000 || snapshot.Paths[0].Stores != 1 {
		t.Fatalf("unexpected collapsed counters: %+v", snapshot.Paths[0])
	}
}

func TestCacheStatsTrackerConcurrentRecordAndSnapshot(t *testing.T) {
	tracker := newCacheStatsTracker(nil)
	const (
		workers    = 16
		iterations = 1_000
	)

	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				path := fmt.Sprintf("api/pjsk/music/path-%d", (worker*iterations+i)%256)
				tracker.recordHit(path)
				if i%10 == 0 {
					_ = tracker.snapshot("")
				}
			}
		}(worker)
	}
	wg.Wait()

	snapshot := tracker.snapshot("")
	if snapshot.Totals.Hits != workers*iterations {
		t.Fatalf("total hits = %d, want %d", snapshot.Totals.Hits, workers*iterations)
	}
	if len(snapshot.Paths) > cacheStatsMaxTrackedPaths+2 {
		t.Fatalf("path stats grew to %d entries", len(snapshot.Paths))
	}
}
