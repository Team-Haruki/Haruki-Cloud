package app

import (
	"context"
	"testing"
	"time"

	"haruki-cloud/internal/core/urlhost"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
	"haruki-cloud/utils/imagecache"
)

func TestAppArtifactConfigFillsImageCacheDependencies(t *testing.T) {
	objects := storagetest.NewMemory()
	hosts := urlhost.Single("https://ic.example")
	cfg := Config{
		DrawingArtifact: drawing.ArtifactConfig{Endpoints: []string{"api/pjsk/card/box"}, FetchTimeout: time.Second},
		Stores:          storage.Set{ImageCache: objects},
		ImageHosts:      hosts,
	}
	got := appArtifactConfig(context.Background(), cfg, nil)
	if got.Objects != objects || got.Hosts != hosts || got.FetchTimeout != time.Second || len(got.Endpoints) != 1 {
		t.Fatalf("artifact config = %+v", got)
	}
	explicitObjects := storagetest.NewMemory()
	explicitHosts := urlhost.Single("https://other.example")
	cfg.DrawingArtifact.Objects, cfg.DrawingArtifact.Hosts = explicitObjects, explicitHosts
	if got := appArtifactConfig(context.Background(), cfg, nil); got.Objects != explicitObjects || got.Hosts != explicitHosts {
		t.Fatalf("explicit dependencies overridden: %+v", got)
	}
}

func TestAppDrawingOptionsAddsArtifactOptionOnlyWhenAllowListed(t *testing.T) {
	if got := len(appDrawingOptions(context.Background(), Config{}, nil)); got != 0 {
		t.Fatalf("zero config options = %d", got)
	}
	cfg := Config{DrawingArtifact: drawing.ArtifactConfig{Endpoints: []string{"*"}}}
	options := appDrawingOptions(context.Background(), cfg, nil)
	if len(options) != 1 {
		t.Fatalf("artifact options = %d", len(options))
	}
	if client := drawing.NewHarukiDrawingClient("http://drawing.invalid", options...); client == nil {
		t.Fatal("client with artifact option is nil")
	}
}

func TestAppArtifactConfigWiresStoreRef(t *testing.T) {
	index := &imagecache.PGStore{}
	base := Config{
		DrawingArtifact: drawing.ArtifactConfig{Endpoints: []string{"*"}, StoreRefPaths: []string{"api/pjsk/sk"}},
		Stores:          storage.Set{ImageCache: storagetest.NewMemory()},
		ImageHosts:      urlhost.Single("https://ic.example"),
	}
	got := appArtifactConfig(context.Background(), base, index)
	if len(got.StoreRefPaths) != 1 || got.StoreRefIndexer != drawing.StoreRefIndexer(index) {
		t.Fatalf("remote image cache: %+v", got)
	}
	if got := appArtifactConfig(context.Background(), base, nil); len(got.StoreRefPaths) != 1 || got.StoreRefIndexer != nil {
		t.Fatalf("no index keeps store-ref without rows: %+v", got)
	}
	local := base
	local.ImageCacheLocalRoot = "/srv/ic"
	if got := appArtifactConfig(context.Background(), local, index); got.StoreRefPaths != nil {
		t.Fatalf("local image cache must disable store-ref: %+v", got.StoreRefPaths)
	}
	disabled := base
	disabled.Stores = storage.Set{ImageCache: storage.Disabled()}
	if got := appArtifactConfig(context.Background(), disabled, index); got.StoreRefPaths != nil {
		t.Fatalf("disabled image cache must disable store-ref: %+v", got.StoreRefPaths)
	}
}
