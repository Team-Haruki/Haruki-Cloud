package pjsk

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeNonceStore records stored keys and can simulate replays / errors.
type fakeNonceStore struct {
	seen map[string]bool
	err  error
}

func newFakeNonceStore() *fakeNonceStore { return &fakeNonceStore{seen: map[string]bool{}} }

func (f *fakeNonceStore) storeNonce(_ context.Context, key string, _ time.Duration) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if f.seen[key] {
		return false, nil // already used -> replay
	}
	f.seen[key] = true
	return true, nil
}

func newReplayGuard(store nonceStore, requireNonce bool) *RequestGuard {
	return &RequestGuard{nonces: store, replayWindow: defaultReplayWindow, requireNonce: requireNonce}
}

func freshReq(nonce string) BotCommandRequest {
	return BotCommandRequest{Timestamp: time.Now().Unix(), Nonce: nonce}
}

func TestCheckReplayLenientAllowsMissingNonce(t *testing.T) {
	g := newReplayGuard(newFakeNonceStore(), false)
	if !g.checkReplay(context.Background(), BotCommandRequest{}) {
		t.Fatal("lenient mode must allow requests without a nonce")
	}
}

func TestCheckReplayStrictRejectsMissingNonce(t *testing.T) {
	g := newReplayGuard(newFakeNonceStore(), true)
	if g.checkReplay(context.Background(), BotCommandRequest{}) {
		t.Fatal("strict mode must reject requests without a nonce")
	}
}

func TestCheckReplayAcceptsFreshThenRejectsReplay(t *testing.T) {
	g := newReplayGuard(newFakeNonceStore(), true)
	req := freshReq("nonce-abc")
	if !g.checkReplay(context.Background(), req) {
		t.Fatal("first use of a fresh nonce must be accepted")
	}
	if g.checkReplay(context.Background(), req) {
		t.Fatal("second use of the same nonce must be rejected (replay)")
	}
}

func TestCheckReplayRejectsStaleAndFutureTimestamp(t *testing.T) {
	g := newReplayGuard(newFakeNonceStore(), false)
	stale := BotCommandRequest{Timestamp: time.Now().Add(-1 * time.Hour).Unix(), Nonce: "n1"}
	if g.checkReplay(context.Background(), stale) {
		t.Fatal("stale timestamp must be rejected")
	}
	future := BotCommandRequest{Timestamp: time.Now().Add(1 * time.Hour).Unix(), Nonce: "n2"}
	if g.checkReplay(context.Background(), future) {
		t.Fatal("implausibly future timestamp must be rejected")
	}
}

func TestCheckReplayFailsOpenOnStoreError(t *testing.T) {
	store := newFakeNonceStore()
	store.err = errors.New("redis down")
	g := newReplayGuard(store, true)
	if !g.checkReplay(context.Background(), freshReq("n3")) {
		t.Fatal("a store error must fail open (allow), not block traffic")
	}
}

func TestSetReplayProtectionNilSafe(t *testing.T) {
	var g *RequestGuard
	g.SetReplayProtection(time.Minute, true) // must not panic
	if !g.checkReplay(context.Background(), BotCommandRequest{}) {
		t.Fatal("nil guard must allow (no-op)")
	}
}
