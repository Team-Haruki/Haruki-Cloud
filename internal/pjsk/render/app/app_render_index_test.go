package app

import (
	"testing"
	"time"

	"haruki-cloud/internal/core/urlhost"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
	"haruki-cloud/utils/imagecache"
)

func TestConfigureRenderIndex(t *testing.T) {
	objects := storagetest.NewMemory()
	hosts := urlhost.Single("https://ic.example")
	store := &imagecache.PGStore{}
	cfg := Config{
		Stores:                             storage.Set{ImageCache: objects},
		ImageHosts:                         hosts,
		ImageCacheRenderIndexTouchInterval: 2 * time.Minute,
		DrawingArtifact:                    drawing.ArtifactConfig{FetchTimeout: 3 * time.Second},
	}

	var off drawing.RenderCacheConfig
	configureRenderIndex(t.Context(), &off, store, cfg)
	if off.Index != nil || off.Artifacts != nil || off.Hosts != nil {
		t.Fatalf("lookup disabled still configured the index: %+v", off)
	}

	cfg.ImageCacheRenderIndexLookup = true
	var noStore drawing.RenderCacheConfig
	configureRenderIndex(t.Context(), &noStore, nil, cfg)
	if noStore.Index != nil {
		t.Fatal("nil *PGStore reached the index field")
	}

	var on drawing.RenderCacheConfig
	configureRenderIndex(t.Context(), &on, store, cfg)
	if on.Index != store || on.Artifacts != objects || on.Hosts != hosts ||
		on.TouchInterval != 2*time.Minute || on.FetchTimeout != 3*time.Second {
		t.Fatalf("index config = %+v", on)
	}

	cfg.Stores.ImageCache = storage.Disabled()
	var disabled drawing.RenderCacheConfig
	configureRenderIndex(t.Context(), &disabled, store, cfg)
	if disabled.Index != store || disabled.Artifacts != nil {
		t.Fatalf("disabled slot reached Artifacts: %+v", disabled)
	}
}

func TestAppCloseStopsRenderIndexWriter(t *testing.T) {
	client := drawing.NewHarukiDrawingClient("http://drawing.invalid")
	store := &imagecache.PGStore{}
	client.SetRenderCache(drawing.NewRenderCacheClient(drawing.RenderCacheConfig{TTL: time.Hour, Index: store}))
	if err := (&App{Drawing: client}).Close(); err != nil {
		t.Fatal(err)
	}
}
