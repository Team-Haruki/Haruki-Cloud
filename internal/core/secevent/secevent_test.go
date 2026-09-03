package secevent

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type memCounter struct {
	mu     sync.Mutex
	counts map[string]int64
	ttls   map[string]time.Duration
}

func (m *memCounter) Incr(_ context.Context, key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.counts == nil {
		m.counts = map[string]int64{}
		m.ttls = map[string]time.Duration{}
	}
	m.counts[key]++
	return m.counts[key], nil
}

func (m *memCounter) Expire(_ context.Context, key string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ttls[key] = ttl
	return nil
}

func TestMonitorAlertsOncePerWindow(t *testing.T) {
	counter := &memCounter{}
	m := New(Config{WebhookURL: "http://example.invalid/hook", Threshold: 3, Window: time.Minute, Node: "test"}, counter)
	var delivered [][]byte
	m.post = func(_ context.Context, payload []byte) error {
		delivered = append(delivered, payload)
		return nil
	}
	m.spawn = func(f func()) { f() }

	ev := Event{Kind: KindAuthFailed, BotID: "42", SourceIP: "192.0.2.9", Reason: "bad credential"}
	for range 5 {
		m.Report(context.Background(), ev)
	}
	if len(delivered) != 1 {
		t.Fatalf("alerts delivered = %d, want exactly 1", len(delivered))
	}
	var alert alertPayload
	if err := json.Unmarshal(delivered[0], &alert); err != nil {
		t.Fatal(err)
	}
	if alert.Kind != "auth_failed" || alert.BotID != "42" || alert.Count != 3 || alert.Threshold != 3 || alert.WindowSeconds != 60 || alert.Node != "test" {
		t.Fatalf("alert = %+v", alert)
	}
	if ttl := counter.ttls["haruki:sec:auth_failed:42"]; ttl != time.Minute {
		t.Fatalf("window ttl = %v", ttl)
	}

	// A different subject counts separately; an event without a bot id is
	// keyed by its source address.
	m.Report(context.Background(), Event{Kind: KindAuthFailed, SourceIP: "198.51.100.1"})
	if counter.counts["haruki:sec:auth_failed:198.51.100.1"] != 1 {
		t.Fatalf("source-keyed count = %v", counter.counts)
	}
}

func TestMonitorDefaultsAndNilSafety(t *testing.T) {
	m := New(Config{}, nil)
	if m.cfg.Threshold != DefaultThreshold || m.cfg.Window != DefaultWindow {
		t.Fatalf("defaults = %+v", m.cfg)
	}
	m.Report(context.Background(), Event{Kind: KindReplayDetected, BotID: "1"}) // no counter: log only
	var nilMonitor *Monitor
	nilMonitor.Report(context.Background(), Event{Kind: KindReplayDetected})
	Report(context.Background(), nil, Event{Kind: KindReplayDetected})
}
