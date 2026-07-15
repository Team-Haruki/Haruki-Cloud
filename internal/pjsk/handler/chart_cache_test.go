package handler

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/onebot11"
	renderapp "haruki-cloud/internal/pjsk/render/app"
)

func TestStaticCachedImageMessageDoesNotOverwriteExistingFile(t *testing.T) {
	dir := t.TempDir()
	app := &renderapp.App{Config: renderapp.Config{ImageCacheDir: dir}}
	publicPath := "default/jp/42/master/no-skill.png"
	storagePath := "charts/" + publicPath
	ctx, trace := commandtrace.WithTrace(context.Background())

	message, err := staticCachedImageMessage(ctx, []byte("first image"), app, "https://charts.example.test", publicPath, storagePath)
	if err != nil {
		t.Fatalf("staticCachedImageMessage() error = %v", err)
	}
	assertImageMessageFile(t, message, "https://charts.example.test/"+publicPath)
	target := filepath.Join(dir, filepath.FromSlash(storagePath))
	oldTime := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(target, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	if _, err := staticCachedImageMessage(ctx, []byte("second image"), app, "https://charts.example.test", publicPath, storagePath); err != nil {
		t.Fatalf("second staticCachedImageMessage() error = %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got := string(data); got != "first image" {
		t.Fatalf("cached image = %q, want first image", got)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.ModTime().Equal(oldTime) {
		t.Fatalf("existing static image was rewritten: mtime = %v, want %v", info.ModTime(), oldTime)
	}
	assertHandlerTraceOperation(t, trace, "image.static_cache_store")
	for _, operation := range []string{
		"image.static_cache_copy",
		"image.static_cache_stat",
		"image.static_cache_mkdir",
		"image.static_cache_write",
		"image.static_cache_rename",
	} {
		assertHandlerTraceOperation(t, trace, operation)
	}
	if matches, err := filepath.Glob(filepath.Join(filepath.Dir(target), ".*.tmp-*")); err != nil {
		t.Fatalf("Glob() error = %v", err)
	} else if len(matches) != 0 {
		t.Fatalf("temporary files were not cleaned up: %v", matches)
	}
}

func TestStaticCachedImageMessageOwnsDataAfterCallerCancellation(t *testing.T) {
	dir := t.TempDir()
	app := &renderapp.App{Config: renderapp.Config{ImageCacheDir: dir}}
	publicPath := "default/jp/42/master/owned.png"
	storagePath := "charts/" + publicPath
	data := bytes.Repeat([]byte("caller-owned-chart"), 1<<12)
	want := bytes.Clone(data)
	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	writeDone := make(chan error, 1)
	writer := func(ctx context.Context, target string, payload []byte) error {
		close(writeStarted)
		<-releaseWrite
		err := writeStaticImageAtomically(ctx, target, payload)
		writeDone <- err
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	callerDone := make(chan error, 1)
	go func() {
		_, err := staticCachedImageMessageWithWriter(ctx, data, app, "https://charts.example.test", publicPath, storagePath, writer)
		callerDone <- err
	}()
	<-writeStarted
	cancel()
	if err := <-callerDone; err != context.Canceled {
		t.Fatalf("staticCachedImageMessage() error = %v, want context.Canceled", err)
	}

	for i := range data {
		data[i] ^= 0xff
	}
	close(releaseWrite)
	if err := <-writeDone; err != nil {
		t.Fatalf("detached write error = %v", err)
	}
	if _, err := staticCachedImageMessage(context.Background(), want, app, "https://charts.example.test", publicPath, storagePath); err != nil {
		t.Fatalf("wait for detached staticCachedImageMessage() error = %v", err)
	}
	stored, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(storagePath)))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(stored, want) {
		t.Fatal("stored chart changed after caller mutated its source slice")
	}
}

func TestStaticCachedImageMessageMergesSharedTraceToEveryWaiter(t *testing.T) {
	dir := t.TempDir()
	app := &renderapp.App{Config: renderapp.Config{ImageCacheDir: dir}}
	publicPath := "default/jp/42/master/shared.png"
	storagePath := "charts/" + publicPath
	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	var writeCalls atomic.Int32
	writer := func(ctx context.Context, target string, payload []byte) error {
		if writeCalls.Add(1) == 1 {
			close(writeStarted)
		}
		<-releaseWrite
		return writeStaticImageAtomically(ctx, target, payload)
	}

	contexts := make([]context.Context, 2)
	traces := make([]*commandtrace.Trace, 2)
	for i := range contexts {
		contexts[i], traces[i] = commandtrace.WithTrace(context.Background())
	}
	errs := make(chan error, 2)
	go func() {
		_, err := staticCachedImageMessageWithWriter(contexts[0], []byte("shared chart"), app, "https://charts.example.test", publicPath, storagePath, writer)
		errs <- err
	}()
	<-writeStarted
	go func() {
		_, err := staticCachedImageMessageWithWriter(contexts[1], []byte("shared chart"), app, "https://charts.example.test", publicPath, storagePath, writer)
		errs <- err
	}()
	time.Sleep(20 * time.Millisecond)
	close(releaseWrite)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("staticCachedImageMessage() error = %v", err)
		}
	}
	if got := writeCalls.Load(); got != 1 {
		t.Fatalf("write calls = %d, want 1", got)
	}
	for index, trace := range traces {
		for _, operation := range []string{
			"image.static_cache_stat",
			"image.static_cache_mkdir",
			"image.static_cache_write",
			"image.static_cache_rename",
		} {
			if count := handlerTraceOperationCount(trace, operation); count != 1 {
				t.Fatalf("trace[%d] %s count = %d, operations=%+v", index, operation, count, trace.Snapshot().Operations)
			}
		}
	}
	if got := handlerTraceOperationCount(traces[0], "image.static_cache_shared") + handlerTraceOperationCount(traces[1], "image.static_cache_shared"); got != 1 {
		t.Fatalf("shared marker count = %d, want 1", got)
	}
}

func TestCachedStaticImageMessagePropagatesContextAndRecordsLookup(t *testing.T) {
	dir := t.TempDir()
	app := &renderapp.App{Config: renderapp.Config{ImageCacheDir: dir}}
	publicPath := "default/jp/42/master/no-skill.png"
	storagePath := "charts/" + publicPath
	if _, err := staticCachedImageMessage(context.Background(), []byte("image"), app, "https://charts.example.test", publicPath, storagePath); err != nil {
		t.Fatalf("staticCachedImageMessage() error = %v", err)
	}

	ctx, trace := commandtrace.WithTrace(context.Background())
	message, hit, err := cachedStaticImageMessage(ctx, app, "https://charts.example.test", publicPath, storagePath)
	if err != nil {
		t.Fatalf("cachedStaticImageMessage() error = %v", err)
	}
	if !hit {
		t.Fatal("cachedStaticImageMessage() did not report a hit")
	}
	assertImageMessageFile(t, message, "https://charts.example.test/"+publicPath)
	assertHandlerTraceOperation(t, trace, "image.static_cache_lookup")

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := cachedStaticImageMessage(canceled, app, "https://charts.example.test", publicPath, storagePath); err != context.Canceled {
		t.Fatalf("cachedStaticImageMessage() error = %v, want context.Canceled", err)
	}
}

func assertImageMessageFile(t *testing.T, message onebot11.Message, want string) {
	t.Helper()
	if len(message) != 1 || message[0].Type != onebot11.TypeImage {
		t.Fatalf("message = %#v, want one image segment", message)
	}
	data, ok := message[0].Data.(onebot11.ImageData)
	if !ok {
		t.Fatalf("image data type = %T, want onebot11.ImageData", message[0].Data)
	}
	if data.File != want {
		t.Fatalf("image file = %q, want %q", data.File, want)
	}
}

func assertHandlerTraceOperation(t *testing.T, trace *commandtrace.Trace, name string) {
	t.Helper()
	for _, operation := range trace.Snapshot().Operations {
		if operation.Name == name && operation.Count > 0 {
			return
		}
	}
	t.Fatalf("trace operation %q was not recorded: %+v", name, trace.Snapshot().Operations)
}
