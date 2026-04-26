package sk

import (
	"testing"
	"time"
)

func TestPredictRenderBucketStartUsesFiveMinuteBuckets(t *testing.T) {
	now := time.Date(2026, 4, 26, 18, 17, 25, 0, time.UTC)
	got := predictRenderBucketStart(now, 0)
	want := time.Date(2026, 4, 26, 18, 15, 0, 0, time.UTC).UnixMilli()
	if got != want {
		t.Fatalf("predict bucket start = %d, want %d", got, want)
	}
}

func TestPredictRenderBucketStartFreezesDuringFinalHour(t *testing.T) {
	aggregateAt := time.Date(2026, 4, 26, 20, 0, 0, 0, time.UTC).UnixMilli()

	gotEarly := predictRenderBucketStart(time.Date(2026, 4, 26, 18, 59, 59, 0, time.UTC), aggregateAt)
	wantEarly := time.Date(2026, 4, 26, 18, 55, 0, 0, time.UTC).UnixMilli()
	if gotEarly != wantEarly {
		t.Fatalf("predict bucket before final hour = %d, want %d", gotEarly, wantEarly)
	}

	gotFinalHour := predictRenderBucketStart(time.Date(2026, 4, 26, 19, 5, 0, 0, time.UTC), aggregateAt)
	if gotFinalHour != wantEarly {
		t.Fatalf("predict bucket in final hour should freeze at last pre-final bucket: got %d want %d", gotFinalHour, wantEarly)
	}

	gotNearEnd := predictRenderBucketStart(time.Date(2026, 4, 26, 19, 55, 0, 0, time.UTC), aggregateAt)
	if gotNearEnd != wantEarly {
		t.Fatalf("predict bucket near event end should stay frozen: got %d want %d", gotNearEnd, wantEarly)
	}
}

func TestPredictRenderCacheTTLUsesLongRetention(t *testing.T) {
	if got := predictRenderCacheTTL(); got != 2*time.Hour {
		t.Fatalf("predict cache ttl = %v, want %v", got, 2*time.Hour)
	}
}
