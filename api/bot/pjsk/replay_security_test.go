package pjsk

import (
	"context"
	"sync"
	"testing"
	"time"

	"haruki-cloud/internal/core/secevent"
)

type replayEventSink struct {
	mu     sync.Mutex
	events []secevent.Event
}

func (s *replayEventSink) Report(_ context.Context, ev secevent.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
}

func TestReplayGuardReportsReusedNonce(t *testing.T) {
	sink := &replayEventSink{}
	guard := newTestReplayGuard(&fakeNonceStore{}, true, time.Unix(1_700_000_000, 0))
	guard.security = sink
	req := replayTestRequest("nonce-once", 1_700_000_000)
	if !guard.allow(context.Background(), "77", req) {
		t.Fatal("first use must pass")
	}
	if guard.allow(context.Background(), "77", req) {
		t.Fatal("second use must be rejected")
	}
	if len(sink.events) != 1 || sink.events[0].Kind != secevent.KindReplayDetected || sink.events[0].BotID != "77" || !sink.events[0].Enforced {
		t.Fatalf("events = %+v", sink.events)
	}
}
