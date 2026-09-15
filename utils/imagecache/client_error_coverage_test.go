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
	index, mock := newMockPGStore(t)
	mock.ExpectClose()
	if err := NewWithStore("https://images.example.test", t.TempDir(), index).Close(); err != nil {
		t.Fatalf("client with store close: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
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
	client.objects = &hookStore{Store: client.objects, putErr: errors.New("write failed")}
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

func TestImageCacheStoreUsesLocalObjectStore(t *testing.T) {
	client := New("https://images.example.test", t.TempDir())
	url, err := client.StoreAndGetURL(context.Background(), []byte("image"), "default-writer")
	if err != nil || !strings.HasPrefix(url, "https://images.example.test/default-writer/") {
		t.Fatalf("default writer result = %q, %v", url, err)
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
