package app

import (
	"context"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/render/assetindex"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func TestDependenciesUsePublishedAssetIndexForReaderAndResolver(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	objects := storagetest.NewMemory()
	objects.Seed(map[string][]byte{"jp-assets/startapp/home/banner/Exact.png": []byte("image")})
	manifest, err := assetindex.Prepare(ctx, objects, "jp")
	if err != nil {
		t.Fatal(err)
	}
	if err = assetindex.Commit(ctx, objects, manifest); err != nil {
		t.Fatal(err)
	}
	cfg := Config{InitContext: ctx, Stores: storage.Set{Assets: objects}, AssetIndex: assetindex.Config{Enabled: true, PollInterval: time.Hour}}
	normalizeAppConfig(&cfg)
	deps := newAppDependencies(ctx, nil, cfg)
	t.Cleanup(func() { cancel(); deps.assetIndex.Close(); deps.assets.Close(); _ = deps.drawing.Close() })
	if err = deps.assetIndex.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	before := len(objects.Calls())
	key, found, err := deps.assetReader.StatResult(ctx, "jp-assets/startapp/home/banner/exact.png")
	if err != nil || !found || key != "jp-assets/startapp/home/banner/Exact.png" {
		t.Fatalf("stat=(%q,%v,%v)", key, found, err)
	}
	_, found, err = deps.assetReader.StatResult(ctx, "jp-assets/startapp/home/banner/missing.png")
	if err != nil || found {
		t.Fatalf("missing=(%v,%v)", found, err)
	}
	for _, call := range objects.Calls()[before:] {
		if call.Method == "Stat" || call.Method == "ListDir" {
			t.Fatalf("manifest should avoid probes: %+v", call)
		}
	}
}
