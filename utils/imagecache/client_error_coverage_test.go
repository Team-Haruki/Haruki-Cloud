package imagecache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageCacheNilClientAndConfiguration(t *testing.T) {
	var client *Client
	if err := client.Close(); err != nil {
		t.Fatalf("nil close: %v", err)
	}
	if _, err := client.StoreAndGetURL(context.Background(), []byte("image"), "group"); err == nil {
		t.Fatal("nil client store succeeded")
	}
	if New("", t.TempDir()) != nil {
		t.Fatal("empty URI client was configured")
	}
	if New("https://images.example.test", " ") != nil {
		t.Fatal("empty directory client was configured")
	}
	if err := New("https://images.example.test", t.TempDir()).Close(); err != nil {
		t.Fatalf("client without store close: %v", err)
	}
}

func TestImageCacheStoreContextAndGroupErrors(t *testing.T) {
	client := New("https://images.example.test", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.StoreAndGetURL(ctx, []byte("image"), "group"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled store error = %v", err)
	}
	if _, err := client.StoreAndGetURL(context.Background(), []byte("image"), "../outside"); err == nil {
		t.Fatal("escaping group store succeeded")
	}

	var nilContext context.Context
	if _, err := client.StoreAndGetURL(nilContext, []byte("image"), "nil-context"); err != nil {
		t.Fatalf("nil context store: %v", err)
	}
}

func TestImageCacheStoreReportsWriteAndDirectoryErrors(t *testing.T) {
	client := New("https://images.example.test", t.TempDir())
	client.write = func(context.Context, string, []byte) error {
		return errors.New("write failed")
	}
	if _, err := client.StoreAndGetURL(context.Background(), []byte("image"), "write-error"); err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("write error = %v", err)
	}

	rootFile := filepath.Join(t.TempDir(), "root-file")
	if err := os.WriteFile(rootFile, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	client = New("https://images.example.test", rootFile)
	if _, err := client.StoreAndGetURL(context.Background(), []byte("image"), "mkdir-error"); err == nil {
		t.Fatal("store below file root succeeded")
	}
}

func TestImageCacheStoreUsesDefaultAtomicWriter(t *testing.T) {
	client := New("https://images.example.test", t.TempDir())
	client.write = nil
	url, err := client.StoreAndGetURL(context.Background(), []byte("image"), "default-writer")
	if err != nil || !strings.HasPrefix(url, "https://images.example.test/default-writer/") {
		t.Fatalf("default writer result = %q, %v", url, err)
	}
}

type cancelOnSecondErrContext struct {
	context.Context
	calls int
}

func (ctx *cancelOnSecondErrContext) Err() error {
	ctx.calls++
	if ctx.calls > 1 {
		return context.Canceled
	}
	return nil
}

func TestWriteFileAtomicallyErrorBranches(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := writeFileAtomically(ctx, filepath.Join(t.TempDir(), "image.png"), []byte("image")); !errors.Is(err, context.Canceled) {
		t.Fatalf("initial cancellation error = %v", err)
	}
	if err := writeFileAtomically(context.Background(), filepath.Join(t.TempDir(), "missing", "image.png"), []byte("image")); err == nil {
		t.Fatal("write in missing directory succeeded")
	}

	dir := t.TempDir()
	stagedCtx := &cancelOnSecondErrContext{Context: context.Background()}
	target := filepath.Join(dir, "image.png")
	if err := writeFileAtomically(stagedCtx, target, []byte("image")); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-rename cancellation error = %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled target exists: %v", err)
	}
}

func TestImageExtensionDetection(t *testing.T) {
	for _, test := range []struct {
		data []byte
		want string
	}{
		{data: []byte{0xff, 0xd8, 0xff, 0xdb}, want: ".jpg"},
		{data: []byte("GIF89a"), want: ".gif"},
		{data: []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), want: ".webp"},
		{data: []byte("plain data"), want: ".png"},
	} {
		if got := extFromData(test.data); got != test.want {
			t.Fatalf("extension for %q = %q, want %q", test.data, got, test.want)
		}
	}
	longData := make([]byte, 513)
	if got := extFromData(longData); got != ".png" {
		t.Fatalf("long data extension = %q", got)
	}
}
