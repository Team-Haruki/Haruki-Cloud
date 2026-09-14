package app

import (
	"context"
	"errors"
	"testing"

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
