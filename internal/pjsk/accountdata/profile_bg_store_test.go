package accountdata

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"haruki-cloud/internal/observability/commandtrace"
)

type profileBGRoundTripFunc func(*http.Request) (*http.Response, error)

func (f profileBGRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func profileBGOperationCount(snapshot commandtrace.Snapshot, name string) int {
	for _, operation := range snapshot.Operations {
		if operation.Name == name {
			return operation.Count
		}
	}
	return 0
}

func TestSaveProfileBackgroundContextAndTrace(t *testing.T) {
	type contextKey string
	const key contextKey = "request"
	const value = "profile-background"

	raw := pngBytes(t, 20, 10)
	client := &http.Client{Transport: profileBGRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Context().Value(key); got != value {
			return nil, fmt.Errorf("request context value = %v, want %q", got, value)
		}
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        make(http.Header),
			Body:          io.NopCloser(bytes.NewReader(raw)),
			ContentLength: int64(len(raw)),
			Request:       req,
		}, nil
	})}
	root := t.TempDir()
	store := NewLocalProfileBGStoreWithClient(root, client)
	base := context.WithValue(context.Background(), key, value)
	ctx, trace := commandtrace.WithTrace(base)

	settings, err := store.SaveProfileBackground(ctx, "JP", "123456", "https://example.test/background.png")
	if err != nil {
		t.Fatalf("SaveProfileBackground() error = %v", err)
	}
	if settings == nil || settings.ImgPath == nil {
		t.Fatal("SaveProfileBackground() returned no image path")
	}
	storedPath := filepath.Join(root, filepath.FromSlash(*settings.ImgPath))
	if _, err := os.Stat(storedPath); err != nil {
		t.Fatalf("stored background: %v", err)
	}

	snapshot := trace.Snapshot()
	for _, operation := range []string{
		"profile_bg.download",
		"profile_bg.read",
		"profile_bg.decode",
		"profile_bg.encode",
		"profile_bg.store",
	} {
		if got := profileBGOperationCount(snapshot, operation); got != 1 {
			t.Fatalf("%s count = %d, want 1", operation, got)
		}
	}
}

func TestProfileBackgroundProcessingHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := decodeBoundedImageContext(ctx, pngBytes(t, 2, 2), 100); !errors.Is(err, context.Canceled) {
		t.Fatalf("decodeBoundedImageContext() error = %v, want context.Canceled", err)
	}
	if _, err := encodeJPEGCompressedContext(ctx, image.NewRGBA(image.Rect(0, 0, 2, 2))); !errors.Is(err, context.Canceled) {
		t.Fatalf("encodeJPEGCompressedContext() error = %v, want context.Canceled", err)
	}
}
