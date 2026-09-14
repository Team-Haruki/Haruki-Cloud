package meta

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func TestLoaderPersistsAndLoadsThroughStore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"music_id":1,"difficulty":"master","tap_count":321}]`))
	}))
	defer server.Close()
	oldURL := regionURLs["en"]
	regionURLs["en"] = server.URL
	defer func() { regionURLs["en"] = oldURL }()

	memory := storagetest.NewMemory()
	loader := NewLoader(nil, WithStore(memory))
	if err := loader.load(context.Background(), "en"); err != nil {
		t.Fatalf("load en: %v", err)
	}
	stored, err := memory.Get(context.Background(), "music_metas-en.json")
	if err != nil || !bytes.Equal(stored, loader.Get("en")) {
		t.Fatalf("stored payload = %q, %v", stored, err)
	}

	fresh := NewLoader(nil, WithStore(memory))
	if err := fresh.loadPersisted(context.Background(), "en"); err != nil {
		t.Fatalf("loadPersisted: %v", err)
	}
	if entry, ok := fresh.View("en").Find(1, "master"); !ok || entry.Int("tap_count") != 321 {
		t.Fatal("persisted object was not loaded from the store")
	}
}

func TestLoaderStoreDisabledAndFailing(t *testing.T) {
	disabled := NewLoader(nil, WithStore(storage.Disabled()))
	if err := disabled.persist(context.Background(), "jp", []byte("[]")); err != nil {
		t.Fatalf("disabled store persist = %v", err)
	}
	if err := disabled.loadPersisted(context.Background(), "jp"); !errors.Is(err, storage.ErrNotConfigured) {
		t.Fatalf("disabled store load = %v", err)
	}

	memory := storagetest.NewMemory()
	memory.FailPut = func(storage.Key) error { return errors.New("put down") }
	failing := NewLoader(nil, WithStore(memory))
	if err := failing.persist(context.Background(), "jp", []byte("[]")); err == nil {
		t.Fatal("failing store persist succeeded")
	}

	cleared := NewLoader(nil, WithStore(memory), WithOutputDir(" "))
	if cleared.store != nil {
		t.Fatal("empty output dir kept a store")
	}
}
