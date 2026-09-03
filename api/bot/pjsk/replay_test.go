package pjsk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func noncesKeyHashForTest(nonce string) string {
	sum := sha256.Sum256([]byte(nonce))
	return hex.EncodeToString(sum[:])
}

type fakeNonceStore struct {
	seen map[string]bool
	err  error
}

func (s *fakeNonceStore) storeNonce(_ context.Context, key string, _ time.Duration) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	if s.seen == nil {
		s.seen = make(map[string]bool)
	}
	if s.seen[key] {
		return false, nil
	}
	s.seen[key] = true
	return true, nil
}

func replayTestRequest(nonce string, timestamp int64) BotCommandRequest {
	request := responseElectionTestRequest("replay-guard")
	request.Nonce = nonce
	request.Timestamp = timestamp
	return request
}

func newTestReplayGuard(store nonceStore, requireNonce bool, now time.Time) *replayGuard {
	return &replayGuard{
		nonces:       store,
		window:       defaultReplayWindow,
		requireNonce: requireNonce,
		now:          func() time.Time { return now },
	}
}

func TestReplayGuardNilAndMissingFieldsAreLenient(t *testing.T) {
	var nilGuard *replayGuard
	if !nilGuard.allow(context.Background(), "", replayTestRequest("", 0)) {
		t.Fatal("nil guard must allow everything")
	}

	guard := newTestReplayGuard(&fakeNonceStore{}, false, time.Now())
	if !guard.allow(context.Background(), "", replayTestRequest("", 0)) {
		t.Fatal("lenient guard must allow a request without nonce fields")
	}
	if !guard.allow(context.Background(), "", replayTestRequest("nonce-without-timestamp", 0)) {
		t.Fatal("lenient guard must allow a nonce without timestamp (incomplete fields)")
	}
}

func TestReplayGuardStrictRejectsMissingNonce(t *testing.T) {
	guard := newTestReplayGuard(&fakeNonceStore{}, true, time.Now())
	if guard.allow(context.Background(), "", replayTestRequest("", 0)) {
		t.Fatal("strict guard must reject a request without nonce fields")
	}
	now := time.Now()
	if !guard.allow(context.Background(), "", replayTestRequest("fresh-nonce-0123456789abcdef", now.Unix())) {
		t.Fatal("strict guard must allow a valid nonce")
	}
}

func TestReplayGuardRejectsTimestampOutsideWindow(t *testing.T) {
	now := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	guard := newTestReplayGuard(&fakeNonceStore{}, false, now)

	stale := now.Add(-defaultReplayWindow - time.Minute).Unix()
	if guard.allow(context.Background(), "", replayTestRequest("stale-nonce-0123456789abcdef", stale)) {
		t.Fatal("timestamp older than the window must be rejected")
	}
	future := now.Add(defaultReplayWindow + time.Minute).Unix()
	if guard.allow(context.Background(), "", replayTestRequest("future-nonce-0123456789abcdef", future)) {
		t.Fatal("timestamp beyond the window in the future must be rejected")
	}
	edge := now.Add(-defaultReplayWindow + time.Second).Unix()
	if !guard.allow(context.Background(), "", replayTestRequest("edge-nonce-0123456789abcdef", edge)) {
		t.Fatal("timestamp inside the window must be accepted")
	}
}

func TestReplayGuardNonceIsSingleUse(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	guard := newReplayGuard(client, 0, false, nil)
	request := replayTestRequest("single-use-nonce-0123456789abcdef", time.Now().Unix())
	if !guard.allow(context.Background(), "", request) {
		t.Fatal("first use of a nonce must be accepted")
	}
	if guard.allow(context.Background(), "", request) {
		t.Fatal("second use of the same nonce must be rejected as a replay")
	}
	// The stored nonce carries the window as its TTL, so by the time it
	// expires the timestamp check rejects the request instead.
	if ttl := server.TTL("haruki:bot:nonce:" + noncesKeyHashForTest(request.Nonce)); ttl <= 0 || ttl > defaultReplayWindow {
		t.Fatalf("nonce TTL = %v, want (0, %v]", ttl, defaultReplayWindow)
	}
}

func TestReplayGuardFailsOpenOnStoreError(t *testing.T) {
	guard := newTestReplayGuard(&fakeNonceStore{err: errors.New("redis down")}, true, time.Now())
	if !guard.allow(context.Background(), "", replayTestRequest("any-nonce-0123456789abcdef", time.Now().Unix())) {
		t.Fatal("store errors must fail open")
	}
}
