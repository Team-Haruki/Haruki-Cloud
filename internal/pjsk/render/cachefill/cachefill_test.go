package cachefill

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/testutil"
)

func TestGroupSharesOneFillAcrossConcurrentCallers(t *testing.T) {
	var group Group
	var calls atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once

	const callers = 16
	var wg sync.WaitGroup
	errs := make([]error, callers)
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = group.Do(context.Background(), "table", func(context.Context) error {
				calls.Add(1)
				once.Do(func() { close(started) })
				<-release
				return nil
			})
		}()
	}
	<-started
	// Every caller has either joined the flight or is about to; the fill
	// holds until they are all queued so the count is meaningful.
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	testutil.Require(t, calls.Load() == 1, "concurrent callers ran %d fills, want 1", calls.Load())
	for i, err := range errs {
		testutil.Require(t, err == nil, "caller %d: %v", i, err)
	}
}

func TestGroupBacksOffAfterFailureAndRecovers(t *testing.T) {
	clock := testutil.NewFakeClock()
	group := Group{Backoff: 5 * time.Second, Now: clock.Now}
	var calls atomic.Int32
	boom := errors.New("database is down")
	failing := func(context.Context) error {
		calls.Add(1)
		return boom
	}

	err := group.Do(context.Background(), "k", failing)
	testutil.Require(t, errors.Is(err, boom) && !errors.Is(err, ErrBackoff), "first fill error = %v", err)
	testutil.Require(t, group.Failing("k"), "key must back off after a failure")

	for range 3 {
		err = group.Do(context.Background(), "k", failing)
		testutil.Require(t, errors.Is(err, ErrBackoff) && errors.Is(err, boom), "backoff error = %v", err)
	}
	testutil.Require(t, calls.Load() == 1, "fills during backoff = %d, want 1", calls.Load())
	testutil.Require(t, !group.Failing("other"), "unrelated keys must not back off")

	err = group.Do(context.Background(), "other", func(context.Context) error { return nil })
	testutil.Require(t, err == nil, "unrelated key fill = %v", err)

	clock.Advance(5 * time.Second)
	err = group.Do(context.Background(), "k", failing)
	testutil.Require(t, errors.Is(err, boom) && !errors.Is(err, ErrBackoff), "fill after backoff = %v", err)
	testutil.Require(t, calls.Load() == 2, "fills after backoff = %d, want 2", calls.Load())

	clock.Advance(5 * time.Second)
	err = group.Do(context.Background(), "k", func(context.Context) error { return nil })
	testutil.Require(t, err == nil && !group.Failing("k"), "successful fill must clear the failure: %v", err)

	err = group.Do(context.Background(), "k", failing)
	testutil.Require(t, errors.Is(err, boom), "fill after success = %v", err)
	group.Reset()
	testutil.Require(t, !group.Failing("k"), "reset must clear failures")
	err = group.Do(context.Background(), "k", failing)
	testutil.Require(t, errors.Is(err, boom) && !errors.Is(err, ErrBackoff), "fill after reset = %v", err)
}

func TestGroupResetIgnoresOutcomeOfFillsAlreadyInFlight(t *testing.T) {
	var group Group
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- group.Do(context.Background(), "k", func(context.Context) error {
			close(started)
			<-release
			return errors.New("database is down")
		})
	}()
	<-started
	group.Reset()
	close(release)
	err := <-done
	testutil.Require(t, err != nil && !errors.Is(err, ErrBackoff), "pre-reset fill error = %v", err)
	testutil.Require(t, !group.Failing("k"), "a failure from a fill started before the reset must not block the key")

	// A success from a pre-reset fill must not clear a failure recorded after it.
	started = make(chan struct{})
	release = make(chan struct{})
	go func() {
		done <- group.Do(context.Background(), "k", func(context.Context) error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started
	group.Reset()
	group.record("k", group.generation, errors.New("recorded after the reset"))
	close(release)
	testutil.Require(t, <-done == nil, "pre-reset fill must still report its own outcome")
	testutil.Require(t, group.Failing("k"), "a success from a fill started before the reset must not clear a later failure")
}

func TestGroupFillRunsDetachedFromRequestContext(t *testing.T) {
	group := Group{Timeout: time.Minute}
	type key string
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), key("trace"), "kept"))
	cancel()

	var fillErr error
	var value any
	var deadline bool
	err := group.Do(ctx, "k", func(fillCtx context.Context) error {
		fillErr = fillCtx.Err()
		value = fillCtx.Value(key("trace"))
		_, deadline = fillCtx.Deadline()
		return nil
	})
	testutil.Require(t, err == nil, "fill = %v", err)
	testutil.Require(t, fillErr == nil, "fill context inherited the cancellation: %v", fillErr)
	testutil.Require(t, value == "kept", "fill context lost the request values: %v", value)
	testutil.Require(t, deadline, "fill context has no timeout")

	var nilGroup *Group
	testutil.Require(t, nilGroup.Do(ctx, "k", nil) != nil, "nil group must refuse fills")
	testutil.Require(t, !nilGroup.Failing("k"), "nil group reports failures")
	nilGroup.Reset()
	fillCtx, cancelFill := nilGroup.Context(context.Background())
	cancelFill()
	_, hasDeadline := fillCtx.Deadline()
	testutil.Require(t, hasDeadline, "nil group context must still be bounded")
}

func TestUnavailableWrapsOnce(t *testing.T) {
	if Unavailable(nil) != nil {
		t.Fatal("Unavailable(nil) != nil")
	}
	cause := errors.New("sql: database is closed")
	err := Unavailable(cause)
	if !errors.Is(err, ErrUnavailable) || !errors.Is(err, cause) {
		t.Fatalf("Unavailable(cause) = %v; want ErrUnavailable wrapping cause", err)
	}
	if again := Unavailable(err); again != err {
		t.Fatalf("Unavailable wrapped twice: %v", again)
	}
}
