package logger

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAsyncWriterFlushesCopiedRecords(t *testing.T) {
	var destination bytes.Buffer
	writer := NewAsyncWriter(&destination, 4)
	payload := []byte("first\n")
	if _, err := writer.Write(payload); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	copy(payload, "other\n")
	if _, err := writer.Write([]byte("second\n")); err != nil {
		t.Fatalf("second Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := destination.String(); got != "first\nsecond\n" {
		t.Fatalf("destination = %q", got)
	}
}

func TestAsyncWriterCloseContextIsBounded(t *testing.T) {
	blocked := make(chan struct{})
	writer := NewAsyncWriter(blockingWriter{release: blocked}, 1)
	if _, err := writer.Write([]byte("record")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := writer.CloseContext(ctx); err != context.DeadlineExceeded {
		t.Fatalf("CloseContext() error = %v, want context.DeadlineExceeded", err)
	}
	close(blocked)
	if err := writer.Close(); err != nil {
		t.Fatalf("final Close() error = %v", err)
	}
}

func TestPriorityAsyncWriterReportsSaturationOutsideBlockedQueue(t *testing.T) {
	blocked := make(chan struct{})
	started := make(chan struct{})
	var reporter lockedBuffer
	writer := NewPriorityAsyncWriter(&signalingBlockingWriter{
		release: blocked,
		started: started,
	}, 1, 5*time.Millisecond, &reporter)

	if _, err := writer.Write([]byte("active\n")); err != nil {
		t.Fatalf("first Write() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("destination writer did not start")
	}
	if _, err := writer.Write([]byte("queued\n")); err != nil {
		t.Fatalf("second Write() error = %v", err)
	}
	startedAt := time.Now()
	if _, err := writer.Write([]byte("dropped\n")); err != nil {
		t.Fatalf("third Write() error = %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 100*time.Millisecond {
		t.Fatalf("bounded enqueue took %v", elapsed)
	}

	deadline := time.Now().Add(time.Second)
	for !strings.Contains(reporter.String(), "async_log_sink_health") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := reporter.String(); !strings.Contains(got, "dropped_records=1") {
		t.Fatalf("stderr health report missing drop count: %q", got)
	}

	close(blocked)
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestPriorityAsyncWriterReportsDestinationErrors(t *testing.T) {
	var reporter lockedBuffer
	writer := NewPriorityAsyncWriter(failingWriter{}, 1, 5*time.Millisecond, &reporter)
	if _, err := writer.Write([]byte("record\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != errWriterFailed {
		t.Fatalf("Close() error = %v, want %v", err, errWriterFailed)
	}
	if got := reporter.String(); !strings.Contains(got, "write_errors=1") {
		t.Fatalf("stderr health report missing write error count: %q", got)
	}
}

func TestReliableAsyncWriterFallsBackWithoutDroppingRecords(t *testing.T) {
	blocked := make(chan struct{})
	started := make(chan struct{})
	var overflow lockedBuffer
	writer := NewReliableAsyncWriter(&signalingBlockingWriter{
		release: blocked,
		started: started,
	}, 1, &overflow)

	if _, err := writer.Write([]byte("active\n")); err != nil {
		t.Fatalf("first Write() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("destination writer did not start")
	}
	if _, err := writer.Write([]byte("queued\n")); err != nil {
		t.Fatalf("second Write() error = %v", err)
	}
	if _, err := writer.Write([]byte("overflow\n")); err != nil {
		t.Fatalf("overflow Write() error = %v", err)
	}
	if got := overflow.String(); !strings.Contains(got, "overflow\n") {
		t.Fatalf("overflow record was not preserved: %q", got)
	}
	if got := writer.Dropped(); got != 0 {
		t.Fatalf("dropped records = %d, want 0", got)
	}
	if got := writer.Overflowed(); got != 1 {
		t.Fatalf("overflowed records = %d, want 1", got)
	}

	close(blocked)
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestReliableAsyncWriterConcurrentCloseAndOverflowIsSafe(t *testing.T) {
	destinationRelease := make(chan struct{})
	destinationStarted := make(chan struct{})
	overflowRelease := make(chan struct{})
	overflowStarted := make(chan struct{})
	writer := NewReliableAsyncWriter(
		&signalingBlockingWriter{release: destinationRelease, started: destinationStarted},
		1,
		&signalingBlockingWriter{release: overflowRelease, started: overflowStarted},
	)
	_, _ = writer.Write([]byte("active\n"))
	select {
	case <-destinationStarted:
	case <-time.After(time.Second):
		t.Fatal("destination writer did not start")
	}
	_, _ = writer.Write([]byte("queued\n"))
	writeDone := make(chan struct{})
	go func() {
		_, _ = writer.Write([]byte("overflow\n"))
		close(writeDone)
	}()
	select {
	case <-overflowStarted:
	case <-time.After(time.Second):
		t.Fatal("overflow writer did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	if err := writer.CloseContext(ctx); err != context.DeadlineExceeded {
		cancel()
		t.Fatalf("CloseContext() error = %v, want context.DeadlineExceeded", err)
	}
	cancel()
	close(overflowRelease)
	close(destinationRelease)
	select {
	case <-writeDone:
	case <-time.After(time.Second):
		t.Fatal("overflow write did not finish")
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("final Close() error = %v", err)
	}
}

func TestCommandWriterDoesNotBackpressureConcurrentSummaries(t *testing.T) {
	commandWriterMu.RLock()
	previousCommandWriter := commandFileWriter
	commandWriterMu.RUnlock()
	t.Cleanup(func() { SetCommandWriter(previousCommandWriter) })

	blocked := make(chan struct{})
	started := make(chan struct{})
	var reporter lockedBuffer
	writer := NewPriorityAsyncWriter(
		&signalingBlockingWriter{release: blocked, started: started},
		1,
		0,
		&reporter,
	)
	SetCommandWriter(writer)
	commandLogger := NewLoggerWithCommandWriter("Command", "INFO")
	commandLogger.Info("active")
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("destination writer did not start")
	}
	commandLogger.Info("queued")

	const concurrent = 20
	var group sync.WaitGroup
	group.Add(concurrent)
	startedAt := time.Now()
	for range concurrent {
		go func() {
			defer group.Done()
			commandLogger.Info("overflow")
		}()
	}
	group.Wait()
	if elapsed := time.Since(startedAt); elapsed > 100*time.Millisecond {
		t.Fatalf("concurrent command summaries blocked on the full sink: %v", elapsed)
	}
	if got := writer.Dropped(); got != concurrent {
		t.Fatalf("dropped summaries = %d, want %d", got, concurrent)
	}

	close(blocked)
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestReliableCommandWriterPreservesConcurrentSummaries(t *testing.T) {
	commandWriterMu.RLock()
	previousCommandWriter := commandFileWriter
	commandWriterMu.RUnlock()
	t.Cleanup(func() { SetCommandWriter(previousCommandWriter) })

	blocked := make(chan struct{})
	started := make(chan struct{})
	var overflow lockedBuffer
	writer := NewReliableAsyncWriter(
		&signalingBlockingWriter{release: blocked, started: started},
		1,
		&overflow,
	)
	SetCommandWriter(writer)
	commandLogger := NewLoggerWithCommandWriter("Command", "INFO")
	commandLogger.Info("active")
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("destination writer did not start")
	}
	commandLogger.Info("queued")

	const concurrent = 20
	var group sync.WaitGroup
	group.Add(concurrent)
	for index := range concurrent {
		go func() {
			defer group.Done()
			commandLogger.Info("overflow", "index", index)
		}()
	}
	group.Wait()
	if got := strings.Count(overflow.String(), "msg=overflow"); got != concurrent {
		t.Fatalf("preserved overflow summaries = %d, want %d; output=%q", got, concurrent, overflow.String())
	}
	if got := writer.Dropped(); got != 0 {
		t.Fatalf("dropped summaries = %d, want 0", got)
	}
	if got := writer.Overflowed(); got != concurrent {
		t.Fatalf("overflowed summaries = %d, want %d", got, concurrent)
	}

	close(blocked)
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestAsyncWritersFlushSharedSerializedDestinationWithoutDeadlock(t *testing.T) {
	var destination bytes.Buffer
	serialized := NewSerializedWriter(slowWriter{destination: &destination})
	mainWriter := NewAsyncWriter(serialized, 64)
	commandWriter := NewPriorityAsyncWriter(serialized, 64, 0, io.Discard)
	for i := range 50 {
		if _, err := mainWriter.Write([]byte(fmt.Sprintf("main-%d\n", i))); err != nil {
			t.Fatalf("main Write() error = %v", err)
		}
		if _, err := commandWriter.Write([]byte(fmt.Sprintf("command-%d\n", i))); err != nil {
			t.Fatalf("command Write() error = %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	results := make(chan error, 2)
	go func() { results <- mainWriter.CloseContext(ctx) }()
	go func() { results <- commandWriter.CloseContext(ctx) }()
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("parallel CloseContext() error = %v", err)
		}
	}
	if lines := strings.Count(destination.String(), "\n"); lines != 100 {
		t.Fatalf("flushed lines = %d, want 100", lines)
	}
}

func TestWriteEmergencyBypassesBlockedAsyncWriter(t *testing.T) {
	blocked := make(chan struct{})
	started := make(chan struct{})
	writer := NewAsyncWriter(&signalingBlockingWriter{release: blocked, started: started}, 1)
	if _, err := writer.Write([]byte("blocked\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("destination writer did not start")
	}

	var emergency bytes.Buffer
	startedAt := time.Now()
	if err := WriteEmergency(&emergency, "Main", "startup failed", "event", "startup_fatal"); err != nil {
		t.Fatalf("WriteEmergency() error = %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 100*time.Millisecond {
		t.Fatalf("emergency write waited behind async sink for %v", elapsed)
	}
	if got := emergency.String(); !strings.Contains(got, "level=CRITICAL") || !strings.Contains(got, "startup_fatal") {
		t.Fatalf("unexpected emergency record: %q", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if err := writer.CloseContext(ctx); err != context.DeadlineExceeded {
		t.Fatalf("CloseContext() error = %v", err)
	}
	close(blocked)
	if err := writer.Close(); err != nil {
		t.Fatalf("final Close() error = %v", err)
	}
}

type blockingWriter struct {
	release <-chan struct{}
}

type signalingBlockingWriter struct {
	release <-chan struct{}
	started chan<- struct{}
	once    sync.Once
}

func (w *signalingBlockingWriter) Write(payload []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.release
	return len(payload), nil
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

type slowWriter struct {
	destination *bytes.Buffer
}

func (w slowWriter) Write(payload []byte) (int, error) {
	time.Sleep(100 * time.Microsecond)
	return w.destination.Write(payload)
}

func (b *lockedBuffer) Write(payload []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(payload)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (w blockingWriter) Write(payload []byte) (int, error) {
	<-w.release
	return len(payload), nil
}
