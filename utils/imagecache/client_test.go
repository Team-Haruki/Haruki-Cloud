package imagecache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
)

func TestStoreAndGetURLSkipsExistingContentFile(t *testing.T) {
	dir := t.TempDir()
	client := New("https://images.example.test/", dir)
	data := []byte("rendered image")

	ctx, trace := commandtrace.WithTrace(context.Background())
	url, err := client.StoreAndGetURL(ctx, data, "pjsk/profile")
	if err != nil {
		t.Fatalf("StoreAndGetURL() error = %v", err)
	}

	digest := sha256.Sum256(data)
	name := hex.EncodeToString(digest[:]) + ".png"
	target := filepath.Join(dir, "pjsk", "profile", name)
	wantURL := "https://images.example.test/pjsk/profile/" + name
	if url != wantURL {
		t.Fatalf("StoreAndGetURL() URL = %q, want %q", url, wantURL)
	}
	if got, readErr := os.ReadFile(target); readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	} else if string(got) != string(data) {
		t.Fatalf("stored data = %q, want %q", got, data)
	}
	assertTraceOperation(t, trace, "image.hash")
	assertTraceOperation(t, trace, "image.lookup")
	assertTraceOperation(t, trace, "image.write")

	oldTime := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(target, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}
	secondCtx, secondTrace := commandtrace.WithTrace(context.Background())
	if _, err := client.StoreAndGetURL(secondCtx, data, "pjsk/profile"); err != nil {
		t.Fatalf("second StoreAndGetURL() error = %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.ModTime().Equal(oldTime) {
		t.Fatalf("existing content file was rewritten: mtime = %v, want %v", info.ModTime(), oldTime)
	}
	assertTraceOperation(t, secondTrace, "image.hash")
	assertTraceOperation(t, secondTrace, "image.lookup")
	assertNoTraceOperation(t, secondTrace, "image.write")

	if matches, err := filepath.Glob(filepath.Join(filepath.Dir(target), ".*.tmp-*")); err != nil {
		t.Fatalf("Glob() error = %v", err)
	} else if len(matches) != 0 {
		t.Fatalf("temporary files were not cleaned up: %v", matches)
	}
}

func TestStoreAndGetURLSingleflightSurvivesLeaderCancellation(t *testing.T) {
	client := New("https://images.example.test", t.TempDir())
	data := []byte("shared rendered image")
	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	var writeCalls atomic.Int32
	client.write = func(ctx context.Context, target string, payload []byte) error {
		if writeCalls.Add(1) == 1 {
			close(writeStarted)
		}
		<-releaseWrite
		return writeFileAtomically(ctx, target, payload)
	}

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderDone := make(chan error, 1)
	go func() {
		_, err := client.StoreAndGetURL(leaderCtx, data, "pjsk/shared")
		leaderDone <- err
	}()
	<-writeStarted

	followerDone := make(chan error, 1)
	followerBaseCtx, followerTrace := commandtrace.WithTrace(context.Background())
	followerCtx := &doneObservedContext{
		Context:  followerBaseCtx,
		observed: make(chan struct{}),
	}
	go func() {
		_, err := client.StoreAndGetURL(followerCtx, data, "pjsk/shared")
		followerDone <- err
	}()
	<-followerCtx.observed
	cancelLeader()
	if err := <-leaderDone; err != context.Canceled {
		t.Fatalf("leader error = %v, want context.Canceled", err)
	}
	close(releaseWrite)
	if err := <-followerDone; err != nil {
		t.Fatalf("follower StoreAndGetURL() error = %v", err)
	}
	if got := writeCalls.Load(); got != 1 {
		t.Fatalf("write calls = %d, want 1", got)
	}
	assertTraceOperation(t, followerTrace, "image.write")
	assertTraceOperation(t, followerTrace, "image.shared")
}

type doneObservedContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *doneObservedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}

func TestStoreAndGetURLConcurrentColdWritePublishesOnce(t *testing.T) {
	client := New("https://images.example.test", t.TempDir())
	data := []byte("same image for every caller")
	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	var writeCalls atomic.Int32
	client.write = func(ctx context.Context, target string, payload []byte) error {
		if writeCalls.Add(1) == 1 {
			close(writeStarted)
		}
		<-releaseWrite
		return writeFileAtomically(ctx, target, payload)
	}

	const callers = 16
	start := make(chan struct{})
	errs := make(chan error, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	for range callers {
		go func() {
			ready.Done()
			<-start
			_, err := client.StoreAndGetURL(context.Background(), data, "pjsk/concurrent")
			errs <- err
		}()
	}
	ready.Wait()
	close(start)
	<-writeStarted
	close(releaseWrite)
	for range callers {
		if err := <-errs; err != nil {
			t.Fatalf("StoreAndGetURL() error = %v", err)
		}
	}
	if got := writeCalls.Load(); got != 1 {
		t.Fatalf("write calls = %d, want 1", got)
	}
}

func TestStoreAndGetURLOwnsDataAfterCallerCancellation(t *testing.T) {
	client := New("https://images.example.test", t.TempDir())
	data := bytes.Repeat([]byte("caller-owned-image"), 1<<12)
	want := bytes.Clone(data)
	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	writeDone := make(chan error, 1)
	client.write = func(ctx context.Context, target string, payload []byte) error {
		close(writeStarted)
		<-releaseWrite
		err := writeFileAtomically(ctx, target, payload)
		writeDone <- err
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	callerDone := make(chan error, 1)
	go func() {
		_, err := client.StoreAndGetURL(ctx, data, "pjsk/owned")
		callerDone <- err
	}()
	<-writeStarted
	cancel()
	if err := <-callerDone; err != context.Canceled {
		t.Fatalf("StoreAndGetURL() error = %v, want context.Canceled", err)
	}

	for i := range data {
		data[i] ^= 0xff
	}
	close(releaseWrite)
	if err := <-writeDone; err != nil {
		t.Fatalf("detached write error = %v", err)
	}
	if _, err := client.StoreAndGetURL(context.Background(), want, "pjsk/owned"); err != nil {
		t.Fatalf("wait for detached StoreAndGetURL() error = %v", err)
	}

	digest := sha256.Sum256(want)
	target := filepath.Join(client.dir, "pjsk", "owned", hex.EncodeToString(digest[:])+".png")
	stored, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(stored, want) {
		t.Fatal("stored content changed after caller mutated its source slice")
	}
}

func assertTraceOperation(t *testing.T, trace *commandtrace.Trace, name string) {
	t.Helper()
	for _, operation := range trace.Snapshot().Operations {
		if operation.Name == name && operation.Count > 0 {
			return
		}
	}
	t.Fatalf("trace operation %q was not recorded: %+v", name, trace.Snapshot().Operations)
}

func assertNoTraceOperation(t *testing.T, trace *commandtrace.Trace, name string) {
	t.Helper()
	for _, operation := range trace.Snapshot().Operations {
		if operation.Name == name && operation.Count > 0 {
			t.Fatalf("trace operation %q unexpectedly recorded: %+v", name, trace.Snapshot().Operations)
		}
	}
}
