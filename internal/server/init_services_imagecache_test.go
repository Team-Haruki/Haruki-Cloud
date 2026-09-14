package server

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	harukiConfig "haruki-cloud/config"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

type fakeInitRuntime struct{ err error }

func (f fakeInitRuntime) InitError() error { return f.err }

func TestInitErrorSurfacesToStartup(t *testing.T) {
	indexErr := errors.New("image cache index: ping: refused")

	var output bytes.Buffer
	if err := renderInitFailure(startupTestLogger(&output), fakeInitRuntime{err: indexErr}, false); err != nil {
		t.Fatalf("require_pg=false returned %v", err)
	}
	if !strings.Contains(output.String(), "level=ERROR") || !strings.Contains(output.String(), "image cache index unavailable") {
		t.Fatalf("missing ERROR record:\n%s", output.String())
	}

	output.Reset()
	if err := renderInitFailure(startupTestLogger(&output), fakeInitRuntime{err: indexErr}, true); !errors.Is(err, indexErr) {
		t.Fatalf("require_pg=true returned %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("fatal path logged before fatalStartup:\n%s", output.String())
	}

	if err := renderInitFailure(nil, fakeInitRuntime{}, true); err != nil {
		t.Fatalf("healthy runtime returned %v", err)
	}
	if err := renderInitFailure(nil, nil, true); err != nil {
		t.Fatalf("nil runtime returned %v", err)
	}
	var zero *renderapp.App
	if err := renderInitFailure(nil, zero, true); err != nil {
		t.Fatalf("nil *App returned %v", err)
	}

	// The real runtime records an unreachable index instead of swallowing it.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runtime := renderapp.New(nil, nil, renderapp.Config{InitContext: ctx, ImageCachePGURL: "://invalid"})
	t.Cleanup(func() { _ = runtime.Close() })
	if err := renderInitFailure(nil, runtime, true); err == nil || !strings.Contains(err.Error(), "image cache index") {
		t.Fatalf("runtime init error = %v", err)
	}
}

func TestResolveImageCacheTarget(t *testing.T) {
	t.Run("nothing configured", func(t *testing.T) {
		stores := storage.Set{}.Normalized()
		root, err := resolveImageCacheTarget(harukiConfig.PJSKRenderConfig{}, &stores, nil)
		if err != nil || root != "" || stores.ImageCache != storage.Disabled() {
			t.Fatalf("root=%q err=%v store=%T", root, err, stores.ImageCache)
		}
	})

	t.Run("legacy dir is the local root", func(t *testing.T) {
		dir := t.TempDir()
		cfg := harukiConfig.PJSKRenderConfig{}
		cfg.ImageCache.Dir = " " + dir + " "
		stores, err := buildRenderStores(cfg, nil)
		if err != nil {
			t.Fatal(err)
		}
		root, err := resolveImageCacheTarget(cfg, &stores, nil)
		if err != nil || root != dir {
			t.Fatalf("root=%q err=%v", root, err)
		}
		assertCacheTargetWrites(t, stores.ImageCache, "pjsk/a.png", filepath.Join(dir, "pjsk", "a.png"))
	})

	t.Run("explicit dir shadows a configured slot with one warning", func(t *testing.T) {
		dir := t.TempDir()
		cfg := harukiConfig.PJSKRenderConfig{}
		cfg.ImageCache.Dir = dir
		cfg.Storage.ImageCache = storage.ProviderConfig{Scheme: "fs", Root: t.TempDir()}
		stores := storage.Set{ImageCache: storagetest.NewMemory()}
		var output bytes.Buffer
		root, err := resolveImageCacheTarget(cfg, &stores, startupTestLogger(&output))
		if err != nil || root != dir {
			t.Fatalf("root=%q err=%v", root, err)
		}
		assertCacheTargetWrites(t, stores.ImageCache, "pjsk/b.png", filepath.Join(dir, "pjsk", "b.png"))
		if got := strings.Count(output.String(), "image_cache.dir takes precedence"); got != 1 {
			t.Fatalf("precedence warnings = %d:\n%s", got, output.String())
		}
	})

	t.Run("fs slot reports its root", func(t *testing.T) {
		slotRoot := t.TempDir()
		cfg := harukiConfig.PJSKRenderConfig{}
		cfg.Storage.ImageCache = storage.ProviderConfig{Scheme: "fs", Root: slotRoot}
		stores, err := buildRenderStores(cfg, nil)
		if err != nil {
			t.Fatal(err)
		}
		root, err := resolveImageCacheTarget(cfg, &stores, nil)
		if err != nil || root != slotRoot {
			t.Fatalf("root=%q err=%v", root, err)
		}
	})

	t.Run("s3 slot has no local root", func(t *testing.T) {
		memory := storagetest.NewMemory()
		cfg := harukiConfig.PJSKRenderConfig{}
		cfg.Storage.ImageCache = storage.ProviderConfig{Scheme: "s3", Endpoint: "http://garage.local:3900", Bucket: "image-cache"}
		stores := storage.Set{ImageCache: memory}
		root, err := resolveImageCacheTarget(cfg, &stores, nil)
		if err != nil || root != "" || stores.ImageCache != storage.Store(memory) {
			t.Fatalf("root=%q err=%v", root, err)
		}
	})

	t.Run("invalid slot and relative dir", func(t *testing.T) {
		cfg := harukiConfig.PJSKRenderConfig{}
		cfg.Storage.ImageCache = storage.ProviderConfig{Scheme: "ftp"}
		stores := storage.Set{}.Normalized()
		if _, err := resolveImageCacheTarget(cfg, &stores, nil); err == nil || !strings.Contains(err.Error(), "storage.image_cache") {
			t.Fatalf("invalid scheme error = %v", err)
		}
		t.Chdir(t.TempDir())
		cfg = harukiConfig.PJSKRenderConfig{}
		cfg.ImageCache.Dir = "relative-images"
		root, err := resolveImageCacheTarget(cfg, &stores, nil)
		if err != nil || !filepath.IsAbs(root) || filepath.Base(root) != "relative-images" {
			t.Fatalf("relative dir root=%q err=%v", root, err)
		}
	})
}
