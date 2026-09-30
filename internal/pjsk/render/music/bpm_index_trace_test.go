package music

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

type tracedBPMIndexStore struct {
	storage.Store
	entered chan *commandtrace.Trace
	release <-chan struct{}
	failure bool
	gets    atomic.Int64
}

func (s *tracedBPMIndexStore) Get(ctx context.Context, key storage.Key) ([]byte, error) {
	s.gets.Add(1)
	commandtrace.RecordOperation(ctx, "storage.get.headers", 5*time.Millisecond)
	s.entered <- commandtrace.FromContext(ctx)
	select {
	case <-s.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	commandtrace.RecordOperation(ctx, "storage.get.body", 7*time.Millisecond)
	if s.failure {
		return nil, errors.New("synthetic index GET failure")
	}
	return s.Store.Get(ctx, key)
}

// load evaluates Done only after registering its singleflight waiter. Each
// wrapper makes that boundary observable without timing sleeps or fill hooks.
type bpmIndexWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *bpmIndexWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestBPMIndexSharedFillTraceIsolatedAndMergedOncePerWaitingRequest(t *testing.T) {
	for _, failure := range []bool{false, true} {
		name := "success"
		if failure {
			name = "failed_get"
		}
		t.Run(name, func(t *testing.T) {
			memory := storagetest.NewMemory()
			memory.Seed(map[string][]byte{storeChartKey: []byte("#BPM01:128\n#00008:01")})
			_, key := publishTestBPMIndex(t, memory, "r1")
			release := make(chan struct{})
			var releaseOnce sync.Once
			finishGet := func() { releaseOnce.Do(func() { close(release) }) }
			t.Cleanup(finishGet)
			objects := &tracedBPMIndexStore{Store: memory, entered: make(chan *commandtrace.Trace, 16), release: release, failure: failure}
			controller := newStoreChartController(objects)
			controller.SetBPMIndexSource(&mutableBPMIndexSource{revision: "r1", key: key}, objects)
			first, cancelFirst := context.WithCancel(t.Context())
			t.Cleanup(cancelFirst)
			first, firstTrace := commandtrace.WithTrace(first)
			firstDone := make(chan *BPMIndex, 1)
			go func() { firstDone <- controller.bpmIndex.load(first, "jp", "r1", key) }()
			var fillTrace *commandtrace.Trace
			select {
			case fillTrace = <-objects.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("index GET did not start")
			}
			if fillTrace == nil || fillTrace == firstTrace {
				t.Fatal("shared index GET retained the initiating request trace")
			}
			assertBPMTraceOperation(t, firstTrace, "storage.get.headers", 0, 0)

			requestA, traceA := commandtrace.WithTrace(t.Context())
			requestB, traceB := commandtrace.WithTrace(t.Context())
			const readers = 8
			contexts := make([]*bpmIndexWaitContext, readers)
			results := make([]*BPMIndex, readers)
			var waiters sync.WaitGroup
			for i := range readers {
				parent := requestA
				if i%2 != 0 {
					parent = requestB
				}
				contexts[i] = &bpmIndexWaitContext{Context: parent, waiting: make(chan struct{})}
				waiters.Go(func() { results[i] = controller.bpmIndex.load(contexts[i], "jp", "r1", key) })
			}
			for _, ctx := range contexts {
				select {
				case <-ctx.waiting:
				case <-time.After(5 * time.Second):
					t.Fatal("index waiter did not join the shared GET")
				}
			}
			cancelFirst()
			select {
			case result := <-firstDone:
				if result != nil {
					t.Fatal("canceled waiter received an index")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("canceled waiter did not return promptly")
			}
			canceledSnapshot := firstTrace.Snapshot()
			finishGet()
			waiters.Wait()
			for i, result := range results {
				if (result == nil) != failure {
					t.Fatalf("waiter %d result=%v, failure=%v", i, result, failure)
				}
			}
			if !reflect.DeepEqual(firstTrace.Snapshot(), canceledSnapshot) {
				t.Fatal("completed shared I/O mutated the canceled request trace")
			}
			for _, trace := range []*commandtrace.Trace{traceA, traceB} {
				assertBPMTraceOperation(t, trace, "storage.get.headers", 1, 5*time.Millisecond)
				assertBPMTraceOperation(t, trace, "storage.get.body", 1, 7*time.Millisecond)
			}
			warm, warmTrace := commandtrace.WithTrace(t.Context())
			controller.bpmIndex.load(warm, "jp", "r1", key)
			assertBPMTraceOperation(t, warmTrace, "storage.get.headers", 0, 0)
			assertBPMTraceOperation(t, warmTrace, "storage.get.body", 0, 0)
			if gets := objects.gets.Load(); gets != 1 {
				t.Fatalf("shared/cached index GETs=%d, want 1", gets)
			}
		})
	}
}

func assertBPMTraceOperation(t *testing.T, trace *commandtrace.Trace, name string, count int, duration time.Duration) {
	t.Helper()
	var got commandtrace.Stats
	for _, operation := range trace.Snapshot().Operations {
		if operation.Name == name {
			got = operation
			break
		}
	}
	if got.Count != count || got.Total != duration {
		t.Fatalf("operation %s=%+v, want count=%d duration=%s", name, got, count, duration)
	}
}
