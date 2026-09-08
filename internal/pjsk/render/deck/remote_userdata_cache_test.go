package deck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"
)

func TestRemoteUserdataReusesUploadAndRecoversEviction(t *testing.T) {
	var uploads, recommends atomic.Int32
	var available atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body)
		defer req.Body.Close()
		switch req.URL.Path {
		case "/cache_userdata":
			uploads.Add(1)
			available.Store(true)
			_, _ = w.Write([]byte(`{"userdata_hash":"same-hash"}`))
		case "/recommend":
			recommends.Add(1)
			if !available.Load() {
				http.Error(w, "userdata_hash user data not found", http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer server.Close()
	remote := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	remote.logger = logger.NewLogger("UserdataCacheTest", "ERROR", nil)
	exec := &remoteExecution{state: testRemoteTargetState(t, remote)}
	req := testRemoteRecommendRequest()
	for range 20 {
		if _, err := remote.doRecommendBatch(t.Context(), exec, req); err != nil {
			t.Fatal(err)
		}
	}
	if uploads.Load() != 1 || recommends.Load() != 20 {
		t.Fatalf("uploads=%d recommends=%d", uploads.Load(), recommends.Load())
	}
	traceCtx, trace := commandtrace.WithTrace(t.Context())
	if _, err := remote.doRecommendBatch(traceCtx, exec, req); err != nil {
		t.Fatal(err)
	}
	compress, ok := traceOperation(trace.Snapshot(), "deck.compress")
	if !ok || compress.Count != 1 {
		t.Fatalf("warm path should only compress recommendation options: %+v", compress)
	}
	available.Store(false)
	if _, err := remote.doRecommendBatch(t.Context(), exec, req); err != nil {
		t.Fatal(err)
	}
	if uploads.Load() != 2 || recommends.Load() != 23 {
		t.Fatalf("recovery uploads=%d recommends=%d", uploads.Load(), recommends.Load())
	}
	req.UserData = []byte(`{"user":"different-preset"}`)
	if _, err := remote.doRecommendBatch(t.Context(), exec, req); err != nil {
		t.Fatal(err)
	}
	if uploads.Load() != 3 {
		t.Fatal("changed payload reused upload")
	}
	// Even the same digest on another target state needs its own remote handle.
	other := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	other.logger = remote.logger
	if _, err := other.doRecommendBatch(t.Context(), &remoteExecution{state: testRemoteTargetState(t, other)}, req); err != nil {
		t.Fatal(err)
	}
	if uploads.Load() != 4 {
		t.Fatal("target switch reused another target's handle")
	}
}

type userdataWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *userdataWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}
func waitUserdataSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("userdata operation did not progress")
	}
}

func TestRemoteUserdataConcurrentUpload(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var uploads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body)
		defer req.Body.Close()
		if uploads.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
		case <-req.Context().Done():
			return
		}
		_, _ = w.Write([]byte(`{"userdata_hash":"shared"}`))
	}))
	defer server.Close()
	defer unblock()
	remote := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	remote.logger = logger.NewLogger("UserdataConcurrentTest", "ERROR", nil)
	state := testRemoteTargetState(t, remote)
	data := []byte(`{"user":"same"}`)
	key := remoteUserdataDigest(t.Context(), data)
	done := make(chan error, 16)
	for range 16 {
		ctx := &userdataWaitContext{Context: t.Context(), waiting: make(chan struct{})}
		go func() {
			entry, err := remote.cachedUserdata(ctx, state, key, data)
			if err == nil && entry.hash != "shared" {
				err = errors.New("incorrect shared handle")
			}
			done <- err
		}()
		waitUserdataSignal(t, ctx.waiting)
	}
	waitUserdataSignal(t, started)
	unblock()
	for range 16 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if uploads.Load() != 1 {
		t.Fatalf("uploads=%d", uploads.Load())
	}
}

func TestRemoteUserdataCanceledLeaderRetriesForSurvivor(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var uploads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body)
		defer req.Body.Close()
		if uploads.Add(1) == 1 {
			close(started)
			select {
			case <-req.Context().Done():
				return
			case <-release:
			}
		}
		_, _ = w.Write([]byte(`{"userdata_hash":"recovered"}`))
	}))
	defer server.Close()
	defer unblock()
	remote := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	remote.logger = logger.NewLogger("UserdataCancelTest", "ERROR", nil)
	state := testRemoteTargetState(t, remote)
	data := []byte(`{"user":"same"}`)
	key := remoteUserdataDigest(t.Context(), data)
	leader, cancel := context.WithCancel(t.Context())
	defer cancel()
	leaderDone := make(chan error, 1)
	go func() { _, err := remote.cachedUserdata(leader, state, key, data); leaderDone <- err }()
	waitUserdataSignal(t, started)
	waiter := &userdataWaitContext{Context: t.Context(), waiting: make(chan struct{})}
	waiterDone := make(chan error, 1)
	go func() { _, err := remote.cachedUserdata(waiter, state, key, data); waiterDone <- err }()
	waitUserdataSignal(t, waiter.waiting)
	cancel()
	select {
	case err := <-leaderDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled upload did not stop")
	}
	select {
	case err := <-waiterDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("survivor did not retry")
	}
	if uploads.Load() != 2 {
		t.Fatalf("uploads=%d", uploads.Load())
	}
}

func TestRemoteUserdataCacheBoundsAndStaleInvalidation(t *testing.T) {
	var cache remoteUserdataCache
	key := remoteUserdataDigest(t.Context(), []byte("same"))
	old := cache.put(key, "same-hash")
	current := cache.put(key, "same-hash")
	cache.invalidate(key, old)
	if cache.get(key) != current {
		t.Fatal("old failure invalidated newer upload")
	}
	current.storedAt = time.Now().Add(-remoteUserdataCacheTTL)
	if cache.get(key) != nil {
		t.Fatal("expired handle reused")
	}
	for i := 0; i < remoteUserdataCacheEntries+1; i++ {
		key := remoteUserdataDigest(t.Context(), []byte(fmt.Sprint(i)))
		cache.put(key, "handle")
	}
	if len(cache.items) != remoteUserdataCacheEntries {
		t.Fatal("entry bound exceeded")
	}
	if cache.get(remoteUserdataDigest(t.Context(), []byte("0"))) != nil {
		t.Fatal("oldest entry not evicted")
	}
}

func TestRemoteUserdataUploadFailureIsNotCached(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body)
		defer req.Body.Close()
		if calls.Add(1) == 1 {
			http.Error(w, "upload failed", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"userdata_hash":"retry"}`))
	}))
	defer server.Close()
	remote := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	remote.logger = logger.NewLogger("UserdataFailureTest", "ERROR", nil)
	state := testRemoteTargetState(t, remote)
	data := []byte("payload")
	key := remoteUserdataDigest(t.Context(), data)
	if _, err := remote.cachedUserdata(t.Context(), state, key, data); err == nil {
		t.Fatal("expected upload error")
	}
	entry, err := remote.cachedUserdata(t.Context(), state, key, data)
	if err != nil || entry.hash != "retry" || calls.Load() != 2 {
		t.Fatalf("retry=%v calls=%d", err, calls.Load())
	}
}
