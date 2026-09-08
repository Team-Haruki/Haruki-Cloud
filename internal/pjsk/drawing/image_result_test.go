package drawing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/observability/commandtrace"
)

func TestCachedImageResultIsLazyAndRebuildsMissingFile(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "cached.png")
	original := []byte("original cached image")
	if err := os.WriteFile(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			raw, _ := json.Marshal(renderCacheRecord{FilePath: file})
			w.Write(raw)
		} else {
			w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()
	client := NewRenderCacheClient(RenderCacheConfig{BaseURL: server.URL, StorageDir: root, TTL: time.Hour})
	defer client.waitForPendingStores()
	ctx, trace := commandtrace.WithTrace(t.Context())
	var renders atomic.Int32
	render := func(context.Context) ([]byte, error) { renders.Add(1); return []byte("rebuilt"), nil }
	policy := renderCachePolicy{APIPath: "api/pjsk/card/list", UserID: "public", TTL: time.Hour}
	result, err := client.renderRemoteImageFlight(ctx, "/api/pjsk/card/list", strings.Repeat("a", 64), policy, render, false)
	if err != nil || result.FilePath() == "" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for _, op := range trace.Snapshot().Operations {
		if op.Name == "drawing.cache_read" {
			t.Fatal("artifact lookup read image bytes")
		}
	}
	bytes, err := result.Bytes(ctx)
	if err != nil || string(bytes) != string(original) {
		t.Fatalf("bytes=%q err=%v", bytes, err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := result.Bytes(ctx)
	if err != nil || string(rebuilt) != "rebuilt" || renders.Load() != 1 {
		t.Fatalf("rebuild=%q err=%v renders=%d", rebuilt, err, renders.Load())
	}
}

func TestCachedImageRejectsEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "private.png")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.png")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	client := &RenderCacheClient{storageDir: root}
	if _, err := client.cachedFile(link); err == nil {
		t.Fatal("escaping symlink accepted")
	}
	if _, err := client.cachedFile(root); err == nil {
		t.Fatal("directory accepted as image")
	}
}

type imageReadWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *imageReadWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestImageBytesWaiterCanCancelSharedRead(t *testing.T) {
	client := &RenderCacheClient{}
	file := "blocked-file"
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	shared := client.readFlight.DoChan(file, func() (any, error) {
		close(entered)
		<-release
		return renderFlightResult{data: []byte("read finished")}, nil
	})
	<-entered
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := &imageReadWaitContext{Context: parent, waiting: make(chan struct{})}
	returned := make(chan error, 1)
	go func() { _, err := (ImageResult{filePath: file, cache: client}).Bytes(ctx); returned <- err }()
	select {
	case <-ctx.waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("file waiter did not join")
	}
	cancel()
	select {
	case err := <-returned:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("file waiter could not cancel")
	}
	unblock()
	completed := <-shared
	if completed.Err != nil || string(completed.Val.(renderFlightResult).data) != "read finished" {
		t.Fatal("caller cancellation interrupted shared read")
	}
}

func TestCachedImagePinsSymlinkTarget(t *testing.T) {
	realRoot := t.TempDir()
	root := filepath.Join(t.TempDir(), "cache")
	if err := os.Symlink(realRoot, root); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first.png", "second.png"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(root, "current.png")
	if err := os.Symlink("first.png", link); err != nil {
		t.Fatal(err)
	}
	client := &RenderCacheClient{storageDir: root}
	result, err := client.cachedFile(link)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("second.png", link); err != nil {
		t.Fatal(err)
	}
	data, err := result.Bytes(t.Context())
	if err != nil || string(data) != "first.png" {
		t.Fatalf("data=%q err=%v", data, err)
	}
}
