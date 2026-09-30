package server

import (
	"haruki-cloud/internal/storage"
	"testing"
	"time"
)

func TestStorageMetricDeltaDoesNotRepeatIdleCounters(t *testing.T) {
	previous := map[string]storage.IOMetric{}
	current := []storage.IOMetric{{Name: "storage.assets.background.get.endpoint_1.attempt", Count: 2, Duration: 10 * time.Millisecond, Max: 6 * time.Millisecond, Value: 200}}
	delta := storageMetricDelta(current, previous)
	if len(delta) != 1 || delta[0].Count != 2 || delta[0].Value != 200 {
		t.Fatalf("initial=%+v", delta)
	}
	if got := storageMetricDelta(current, previous); len(got) != 0 {
		t.Fatalf("idle=%+v", got)
	}
	current[0].Count++
	current[0].Duration += 3 * time.Millisecond
	current[0].Value += 40
	delta = storageMetricDelta(current, previous)
	if len(delta) != 1 || delta[0].Count != 1 || delta[0].Duration != 3*time.Millisecond || delta[0].Value != 40 || delta[0].Max != 6*time.Millisecond {
		t.Fatalf("next=%+v", delta)
	}
}
