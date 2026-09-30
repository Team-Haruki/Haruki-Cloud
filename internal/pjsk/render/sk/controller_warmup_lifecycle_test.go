package sk

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/storage"
)

type warmupLifecycleState struct {
	entered, canceled, release chan struct{}
	enterOnce, cancelOnce      sync.Once
	listCalls, detailCalls     atomic.Int32
	background                 atomic.Bool
}

type warmupLifecycleSource struct {
	renderevent.DataSource
	ctx   context.Context
	state *warmupLifecycleState
}

func (s *warmupLifecycleSource) DefaultRegion() renderregion.Value { return renderregion.JP }
func (s *warmupLifecycleSource) WithContext(ctx context.Context) renderevent.DataSource {
	clone := *s
	clone.ctx = ctx
	return &clone
}
func (s *warmupLifecycleSource) GetEvents() []*masterdata.Event {
	s.state.listCalls.Add(1)
	s.state.enterOnce.Do(func() { close(s.state.entered) })
	if s.ctx != nil {
		s.state.background.Store(storage.IsBackgroundIO(s.ctx))
		<-s.ctx.Done()
		s.state.cancelOnce.Do(func() { close(s.state.canceled) })
	}
	<-s.state.release
	now := time.Now().UnixMilli()
	return []*masterdata.Event{{ID: 100, StartAt: now - 1000, AggregateAt: now + 60000}}
}
func (s *warmupLifecycleSource) GetEventByID(int) (*masterdata.Event, error) {
	s.state.detailCalls.Add(1)
	return nil, nil
}

func waitWarmupSignal(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal(message)
	}
}

func TestDefaultPredictWarmupCancelsProviderAndDrains(t *testing.T) {
	controller := NewController(nil)
	state := &warmupLifecycleState{entered: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{})}
	source := &warmupLifecycleSource{state: state}
	controller.RegisterEventSource(source)
	lifeCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err := controller.ConfigureCachePersistence(lifeCtx, "warmup-test"); err != nil {
		t.Fatal(err)
	}
	controller.StartDefaultPredictWarmupContext(lifeCtx)
	waitWarmupSignal(t, state.entered, "warmup did not query events")
	cancel()
	waitWarmupSignal(t, state.canceled, "event query did not receive lifecycle cancellation")
	closed := make(chan error, 1)
	go func() {
		ctx, done := context.WithTimeout(context.Background(), time.Second)
		defer done()
		closed <- controller.CloseCachePersistence(ctx)
	}()
	select {
	case err := <-closed:
		t.Fatalf("shutdown returned before warmup provider drained: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(state.release)
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not drain warmup")
	}
	if !state.background.Load() {
		t.Fatal("warmup did not use background I/O budget")
	}
	if source.ctx != nil {
		t.Fatal("warmup changed shared provider context")
	}
	if state.listCalls.Load() != 1 || state.detailCalls.Load() != 0 {
		t.Fatalf("warmup continued after cancellation: list=%d detail=%d", state.listCalls.Load(), state.detailCalls.Load())
	}
	controller.StartDefaultPredictWarmup()
	if state.listCalls.Load() != 1 {
		t.Fatal("warmup restarted after shutdown")
	}
}

type warmupTickSource struct {
	EventSource
	calls chan struct{}
}

func (s *warmupTickSource) DefaultRegion() renderregion.Value { return renderregion.JP }
func (s *warmupTickSource) GetEvents() []*masterdata.Event {
	s.calls <- struct{}{}
	return nil
}

func TestDefaultPredictWarmupStopsTickLoop(t *testing.T) {
	controller := NewController(nil)
	source := &warmupTickSource{calls: make(chan struct{}, 3)}
	controller.RegisterEventSource(source)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ticks := make(chan time.Time, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		controller.WithContext(ctx).runDefaultPredictWarmup([]string{"jp"}, ticks)
	}()
	waitWarmupSignal(t, source.calls, "initial warmup did not run")
	ticks <- time.Now()
	waitWarmupSignal(t, source.calls, "periodic warmup did not run")
	cancel()
	ticks <- time.Now()
	waitWarmupSignal(t, done, "warmup ticker loop did not stop")
	if len(source.calls) != 0 {
		t.Fatal("canceled warmup tick queried provider")
	}
}
