package app

import (
	"context"
	"errors"
	"testing"

	"haruki-cloud/internal/core/urlhost"
	"haruki-cloud/internal/pjsk/meta"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func TestNewNormalizesZeroStoresToDisabled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cache := storagetest.NewMemory()
	runtime := New(nil, nil, Config{
		InitContext: ctx,
		MetaLoader:  meta.NewLoader(nil),
		Stores:      storage.Set{Cache: cache},
	})
	t.Cleanup(func() { _ = runtime.Close() })

	if runtime.Stores.Cache != storage.Store(cache) || runtime.Config.Stores.Cache != storage.Store(cache) {
		t.Fatal("configured cache store was replaced")
	}
	for _, slot := range []storage.Slot{storage.SlotAssets, storage.SlotUserUpload, storage.SlotStatic, storage.SlotImageCache} {
		store := *runtime.Stores.Store(slot)
		if store == nil {
			t.Fatalf("slot %s is nil", slot)
		}
		if _, err := store.Get(ctx, "probe"); !errors.Is(err, storage.ErrNotConfigured) {
			t.Fatalf("slot %s is not Disabled: %v", slot, err)
		}
	}
}

func TestNormalizeAppConfigZeroStores(t *testing.T) {
	cfg := Config{MetaLoader: meta.NewLoader(nil)}
	normalizeAppConfig(&cfg)
	for _, slot := range storage.Slots {
		if _, err := (*cfg.Stores.Store(slot)).Stat(context.Background(), "probe"); !errors.Is(err, storage.ErrNotConfigured) {
			t.Fatalf("slot %s = %v", slot, err)
		}
	}
}

func TestNormalizeAppConfigDerivesSingleHosts(t *testing.T) {
	cfg := Config{MetaLoader: meta.NewLoader(nil), ImageCacheURI: "https://ic.example/", AssetsBaseURL: "https://assets.example"}
	normalizeAppConfig(&cfg)
	if cfg.ImageHosts.Len() != 1 || cfg.ImageHosts.Base("") != "https://ic.example" {
		t.Fatalf("image hosts = %v", cfg.ImageHosts.Hosts())
	}
	if cfg.AssetHosts.Len() != 1 || cfg.AssetHosts.Base("") != "https://assets.example" {
		t.Fatalf("asset hosts = %v", cfg.AssetHosts.Hosts())
	}

	empty := Config{MetaLoader: meta.NewLoader(nil)}
	normalizeAppConfig(&empty)
	if empty.ImageHosts == nil || empty.ImageHosts.Len() != 0 || empty.AssetHosts == nil || empty.AssetHosts.Len() != 0 {
		t.Fatal("zero config must yield empty, non-nil host sets")
	}

	explicit, err := urlhost.FromList([]string{"https://a.example", "https://b.example"}, urlhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	kept := Config{MetaLoader: meta.NewLoader(nil), AssetsBaseURL: "https://legacy.example", AssetHosts: explicit}
	normalizeAppConfig(&kept)
	if kept.AssetHosts != explicit {
		t.Fatal("configured asset hosts were replaced")
	}
}

func TestNewBuildsAssetReaderOnAssetsSlot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assetsStore := storagetest.NewMemory()
	assetsStore.Seed(map[string][]byte{"jp-assets/startapp/x.png": []byte("x")})
	imageHosts, err := urlhost.New(map[string]string{"cn09": "https://ic-cn09.example"}, urlhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	runtime := New(nil, nil, Config{
		InitContext: ctx,
		MetaLoader:  meta.NewLoader(nil),
		Stores:      storage.Set{Assets: assetsStore},
		ImageHosts:  imageHosts,
	})
	t.Cleanup(func() { _ = runtime.Close() })

	if runtime.ImageHosts != imageHosts || runtime.AssetHosts == nil || runtime.AssetHosts.Len() != 0 {
		t.Fatal("host sets not threaded onto App")
	}
	data, resolved, err := runtime.AssetReader.ReadFirst(context.Background(), "asset/jp-assets/startapp/x.png")
	if err != nil || string(data) != "x" || resolved != "jp-assets/startapp/x.png" {
		t.Fatalf("AssetReader = %q %q %v", data, resolved, err)
	}
}
