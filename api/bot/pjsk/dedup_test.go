package pjsk

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

var errTransientCleanup = errors.New("transient cleanup failure")

func TestRequestGuardRejectsCanceledContextWithoutFailingOpen(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer client.Close()
	guard := NewRequestGuard(client)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if guard.Acquire(ctx, BotCommandRequest{}).proceed {
		t.Fatal("canceled request must not proceed when Redis returns context.Canceled")
	}
}

func TestRequestGuardFailOpenLeaseCannotCompleteAnotherOwner(t *testing.T) {
	lease := requestGuardLease{proceed: true}
	if lease.token != "" {
		t.Fatalf("fail-open lease unexpectedly owns a lock: %+v", lease)
	}
	if !lease.proceed {
		t.Fatal("fail-open lease must allow the command to proceed")
	}
}

func TestRequestGuardCleanupRetriesTransientFailure(t *testing.T) {
	var calls atomic.Int32
	dispatcher := newRequestGuardCleanupDispatcher(2, 1, func(_ context.Context, _ requestGuardCleanupJob) (bool, error) {
		if calls.Add(1) == 1 {
			return false, errTransientCleanup
		}
		return true, nil
	})
	if !dispatcher.Enqueue(testRequestGuardCleanupJob("owner-a")) {
		t.Fatal("cleanup enqueue failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := dispatcher.CloseContext(ctx); err != nil {
		t.Fatalf("CloseContext() error = %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("cleanup attempts = %d, want 2", got)
	}
}

func TestRequestGuardCleanupDoesNotReleaseDifferentOwner(t *testing.T) {
	currentOwner := "owner-b"
	var calls atomic.Int32
	dispatcher := newRequestGuardCleanupDispatcher(2, 1, func(_ context.Context, job requestGuardCleanupJob) (bool, error) {
		calls.Add(1)
		if job.owner != currentOwner {
			return false, nil
		}
		currentOwner = ""
		return true, nil
	})
	if !dispatcher.Enqueue(testRequestGuardCleanupJob("owner-a")) {
		t.Fatal("cleanup enqueue failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := dispatcher.CloseContext(ctx); err != nil {
		t.Fatalf("CloseContext() error = %v", err)
	}
	if currentOwner != "owner-b" {
		t.Fatalf("different owner lock was released: %q", currentOwner)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("owner mismatch attempts = %d, want 1", got)
	}
}

func TestRequestGuardMarkCompleteIsNonBlockingAndQueueIsBounded(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	dispatcher := newRequestGuardCleanupDispatcher(1, 1, func(ctx context.Context, _ requestGuardCleanupJob) (bool, error) {
		if calls.Add(1) == 1 {
			close(started)
			select {
			case <-release:
				return true, nil
			case <-ctx.Done():
				return false, ctx.Err()
			}
		}
		return true, nil
	})
	guard := &RequestGuard{cleanup: dispatcher}
	req := BotCommandRequest{Platform: "qq", PlatformUserID: "1", MatchedCommand: "/card"}
	lease := requestGuardLease{
		proceed: true,
		token:   "owner-a",
		lockKey: "haruki:bot:dedup:test",
		rateKey: "haruki:bot:ratelimit:test",
	}

	guard.MarkComplete(context.Background(), req, lease)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("cleanup worker did not start")
	}
	guard.MarkComplete(context.Background(), req, lease)
	startedAt := time.Now()
	guard.MarkComplete(context.Background(), req, lease)
	if elapsed := time.Since(startedAt); elapsed > 50*time.Millisecond {
		t.Fatalf("queue-full MarkComplete blocked response for %v", elapsed)
	}
	if got := len(dispatcher.jobs); got != 1 {
		t.Fatalf("queued cleanup jobs = %d, want bounded capacity 1", got)
	}

	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := dispatcher.CloseContext(ctx); err != nil {
		t.Fatalf("CloseContext() error = %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("cleanup calls = %d, want active + queued + fallback", got)
	}
}

func TestRequestGuardCleanupCloseContextIsBounded(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	dispatcher := newRequestGuardCleanupDispatcher(1, 1, func(_ context.Context, _ requestGuardCleanupJob) (bool, error) {
		close(started)
		<-release
		return true, nil
	})
	if !dispatcher.Enqueue(testRequestGuardCleanupJob("owner-a")) {
		t.Fatal("cleanup enqueue failed")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("cleanup worker did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if err := dispatcher.CloseContext(ctx); err != context.DeadlineExceeded {
		t.Fatalf("CloseContext() error = %v, want context.DeadlineExceeded", err)
	}
	close(release)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := dispatcher.CloseContext(ctx2); err != nil {
		t.Fatalf("final CloseContext() error = %v", err)
	}
}

func testRequestGuardCleanupJob(owner string) requestGuardCleanupJob {
	return requestGuardCleanupJob{
		lockKey: "haruki:bot:dedup:test",
		rateKey: "haruki:bot:ratelimit:test",
		owner:   owner,
	}
}
