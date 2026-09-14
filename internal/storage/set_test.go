package storage_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/storage"
	storages3 "haruki-cloud/internal/storage/s3"
	"haruki-cloud/internal/storage/storagetest"
	"haruki-cloud/utils/logger"
)

type fakeS3Backend struct {
	opened []storage.Resolved
	stores []*storagetest.Memory
	err    error
}

func (f *fakeS3Backend) open(resolved storage.Resolved) (storage.Store, error) {
	f.opened = append(f.opened, resolved)
	if f.err != nil {
		return nil, f.err
	}
	store := storagetest.NewMemory()
	f.stores = append(f.stores, store)
	return store, nil
}

func testLogger(buf *bytes.Buffer) *logger.Logger {
	return logger.NewLogger("storage", "DEBUG", buf)
}

func legacyRootsFor(slot storage.Slot, root string) storage.LegacyRoots {
	switch slot {
	case storage.SlotCache:
		return storage.LegacyRoots{CacheDir: root}
	case storage.SlotImageCache:
		return storage.LegacyRoots{ImageCacheDir: root}
	default:
		return storage.LegacyRoots{AssetPrimary: root}
	}
}

func writesToDir(t *testing.T, store storage.Store, dir string) bool {
	t.Helper()
	if err := store.Put(context.Background(), "probe/file.txt", []byte("x"), storage.PutOptions{}); err != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(dir, "probe", "file.txt"))
	return err == nil && string(data) == "x"
}

func TestBuildSetLegacyDerivationMatrix(t *testing.T) {
	for _, slot := range storage.Slots {
		for _, mode := range []string{"absent", "local", "s3"} {
			for _, legacySet := range []bool{true, false} {
				name := string(slot) + "/" + mode
				if legacySet {
					name += "/legacy"
				}
				t.Run(name, func(t *testing.T) {
					legacyDir := t.TempDir()
					slotDir := t.TempDir()
					var cfg storage.SetConfig
					switch mode {
					case "local":
						*cfg.Provider(slot) = storage.ProviderConfig{Scheme: "fs", Root: slotDir}
					case "s3":
						*cfg.Provider(slot) = storage.ProviderConfig{Scheme: "s3", Bucket: "b", Endpoint: "http://garage:3900"}
					}
					legacy := storage.LegacyRoots{}
					if legacySet {
						legacy = legacyRootsFor(slot, legacyDir)
					}
					backend := &fakeS3Backend{}
					var logs bytes.Buffer
					set, err := storage.BuildSet(cfg, legacy, storage.Backends{S3: backend.open}, testLogger(&logs))
					if err != nil {
						t.Fatalf("BuildSet error = %v", err)
					}
					for _, other := range storage.Slots {
						if *set.Store(other) == nil {
							t.Fatalf("slot %s is nil", other)
						}
					}
					store := *set.Store(slot)
					switch {
					case mode == "local":
						if !writesToDir(t, store, slotDir) {
							t.Fatal("slot block did not win")
						}
					case mode == "s3":
						if len(backend.stores) != 1 || store != storage.Store(backend.stores[0]) {
							t.Fatalf("s3 store not used: opened=%d", len(backend.opened))
						}
					case legacySet:
						if !writesToDir(t, store, legacyDir) {
							t.Fatal("legacy root not used")
						}
					default:
						if _, err := store.Get(context.Background(), "x"); !errors.Is(err, storage.ErrNotConfigured) {
							t.Fatalf("expected Disabled store, Get error = %v", err)
						}
					}
					warnings := strings.Count(logs.String(), "storage slot root disagrees with legacy path")
					wantWarnings := 0
					if legacySet && mode != "absent" {
						wantWarnings = 1
					}
					if warnings != wantWarnings {
						t.Fatalf("disagreement warnings = %d, want %d; logs:\n%s", warnings, wantWarnings, logs.String())
					}
					if got := strings.Count(logs.String(), "storage slot configured"); got != len(storage.Slots) {
						t.Fatalf("summary lines = %d, want %d", got, len(storage.Slots))
					}
				})
			}
		}
	}
}

func TestBuildSetAgreeingSlotRootDoesNotWarn(t *testing.T) {
	dir := t.TempDir()
	var logs bytes.Buffer
	cfg := storage.SetConfig{Cache: storage.ProviderConfig{Root: dir + "/"}}
	if _, err := storage.BuildSet(cfg, storage.LegacyRoots{CacheDir: dir}, storage.Backends{}, testLogger(&logs)); err != nil {
		t.Fatalf("BuildSet error = %v", err)
	}
	if strings.Contains(logs.String(), "disagrees") {
		t.Fatalf("unexpected warning:\n%s", logs.String())
	}
}

func TestBuildSetDisagreementWarnFields(t *testing.T) {
	var logs bytes.Buffer
	cfg := storage.SetConfig{Cache: storage.ProviderConfig{Root: "/slot/root"}}
	if _, err := storage.BuildSet(cfg, storage.LegacyRoots{CacheDir: "/legacy/cache"}, storage.Backends{}, testLogger(&logs)); err != nil {
		t.Fatalf("BuildSet error = %v", err)
	}
	out := logs.String()
	for _, want := range []string{"WARN", "slot=cache", "slot_root=/slot/root", "legacy_path=/legacy/cache"} {
		if !strings.Contains(out, want) {
			t.Errorf("logs missing %q:\n%s", want, out)
		}
	}
	invalid := storage.SetConfig{Cache: storage.ProviderConfig{Scheme: "ftp"}}
	logs.Reset()
	if _, err := storage.BuildSet(invalid, storage.LegacyRoots{CacheDir: "/legacy/cache"}, storage.Backends{}, testLogger(&logs)); err == nil {
		t.Fatal("expected scheme error")
	}
	if !strings.Contains(logs.String(), "disagrees") {
		t.Fatalf("invalid slot should still report the disagreement:\n%s", logs.String())
	}
}

func TestBuildSetRelativeLegacyRootIsMadeAbsolute(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	set, err := storage.BuildSet(storage.SetConfig{}, storage.LegacyRoots{ImageCacheDir: "imagecache"}, storage.Backends{}, nil)
	if err != nil {
		t.Fatalf("BuildSet error = %v", err)
	}
	if !writesToDir(t, set.ImageCache, filepath.Join(dir, "imagecache")) {
		t.Fatal("relative legacy root not resolved against the working directory")
	}
}

func TestBuildSetSummaryNeverLogsSecrets(t *testing.T) {
	var logs bytes.Buffer
	cfg := storage.SetConfig{ImageCache: storage.ProviderConfig{
		Scheme: "s3", Bucket: "image-cache", Endpoints: []string{"http://a:3900", "http://b:3900"},
		AccessKeyID: "GKsuperkeyid", SecretAccessKey: "topsecretvalue",
		Options: map[string]string{"secret_hint": "x"},
	}}
	backend := &fakeS3Backend{}
	if _, err := storage.BuildSet(cfg, storage.LegacyRoots{}, storage.Backends{S3: backend.open}, testLogger(&logs)); err != nil {
		t.Fatalf("BuildSet error = %v", err)
	}
	out := logs.String()
	for _, want := range []string{"slot=image_cache", "scheme=s3", "bucket=image-cache", "endpoints=2", "path_style=true", "creds=set", "unknown options kept: secret_hint", "slot=assets scheme=disabled"} {
		if !strings.Contains(out, want) {
			t.Errorf("logs missing %q:\n%s", want, out)
		}
	}
	for _, secret := range []string{"GKsuperkeyid", "topsecretvalue"} {
		if strings.Contains(out, secret) {
			t.Fatalf("logs leak %q:\n%s", secret, out)
		}
	}
	logs.Reset()
	anonymous := storage.SetConfig{ImageCache: storage.ProviderConfig{Scheme: "s3", Bucket: "b", Endpoint: "http://a"}}
	if _, err := storage.BuildSet(anonymous, storage.LegacyRoots{}, storage.Backends{S3: backend.open}, testLogger(&logs)); err != nil {
		t.Fatalf("BuildSet error = %v", err)
	}
	if !strings.Contains(logs.String(), "creds=anonymous") {
		t.Fatalf("anonymous summary missing:\n%s", logs.String())
	}
}

func TestBuildSetErrors(t *testing.T) {
	openErr := errors.New("bad option")
	cases := []struct {
		name     string
		cfg      storage.SetConfig
		backends storage.Backends
		want     string
		is       error
	}{
		{"validation", storage.SetConfig{Static: storage.ProviderConfig{Root: "relative"}}, storage.Backends{}, "storage.static.root", nil},
		{"s3 backend missing", storage.SetConfig{Assets: storage.ProviderConfig{Scheme: "s3", Bucket: "b", Endpoint: "http://e"}}, storage.Backends{}, "storage.assets.scheme", storage.ErrBackendUnavailable},
		{"s3 open error", storage.SetConfig{Cache: storage.ProviderConfig{Scheme: "s3", Bucket: "b", Endpoint: "http://e"}}, storage.Backends{S3: (&fakeS3Backend{err: openErr}).open}, "storage.cache.options", openErr},
		{"mirror s3 backend missing", storage.SetConfig{UserUpload: storage.ProviderConfig{Root: "/a", Mirror: &storage.ProviderConfig{Scheme: "s3", Bucket: "u", Endpoint: "http://e"}}}, storage.Backends{}, "storage.user_upload.mirror.scheme", storage.ErrBackendUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := storage.BuildSet(tc.cfg, storage.LegacyRoots{}, tc.backends, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
			if tc.is != nil && !errors.Is(err, tc.is) {
				t.Fatalf("error %v is not %v", err, tc.is)
			}
		})
	}
}

func TestOpenZeroIsDisabled(t *testing.T) {
	store, err := storage.Open(storage.SlotCache, storage.ProviderConfig{}, storage.Backends{}, nil)
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}
	if _, err := store.Stat(context.Background(), "a"); !errors.Is(err, storage.ErrNotConfigured) {
		t.Fatalf("Stat error = %v", err)
	}
}

func TestOpenS3RootWarningOnAssetsAndImageCache(t *testing.T) {
	for _, slot := range []storage.Slot{storage.SlotAssets, storage.SlotImageCache, storage.SlotStatic} {
		var logs bytes.Buffer
		backend := &fakeS3Backend{}
		cfg := storage.ProviderConfig{Scheme: "s3", Bucket: "b", Endpoint: "http://e", Root: "jp"}
		if _, err := storage.Open(slot, cfg, storage.Backends{S3: backend.open}, testLogger(&logs)); err != nil {
			t.Fatalf("Open error = %v", err)
		}
		warned := strings.Contains(logs.String(), "root must stay empty")
		if want := slot != storage.SlotStatic; warned != want {
			t.Fatalf("slot %s warned=%v want %v:\n%s", slot, warned, want, logs.String())
		}
	}
}

func TestOpenMirrorBuildsDualStore(t *testing.T) {
	for _, tc := range []struct {
		mode        string
		readThrough bool
	}{{"", true}, {"write", true}, {"write_only", false}} {
		t.Run("mode="+tc.mode, func(t *testing.T) {
			primaryDir := t.TempDir()
			mirrorDir := t.TempDir()
			var logs bytes.Buffer
			cfg := storage.ProviderConfig{
				Root:       primaryDir,
				Mirror:     &storage.ProviderConfig{Scheme: "local", Root: mirrorDir},
				MirrorMode: tc.mode,
			}
			store, err := storage.Open(storage.SlotUserUpload, cfg, storage.Backends{}, testLogger(&logs))
			if err != nil {
				t.Fatalf("Open error = %v", err)
			}
			if !writesToDir(t, store, primaryDir) {
				t.Fatal("primary not written")
			}
			if data, err := os.ReadFile(filepath.Join(mirrorDir, "probe", "file.txt")); err != nil || string(data) != "x" {
				t.Fatalf("mirror not written: %v", err)
			}
			mirrorOnly := filepath.Join(mirrorDir, "only.txt")
			if err := os.WriteFile(mirrorOnly, []byte("m"), 0o644); err != nil {
				t.Fatal(err)
			}
			_, getErr := store.Get(context.Background(), "only.txt")
			if (getErr == nil) != tc.readThrough {
				t.Fatalf("read-through = %v, want %v", getErr == nil, tc.readThrough)
			}
			if !strings.Contains(logs.String(), "slot=user_upload.mirror") {
				t.Fatalf("mirror summary missing:\n%s", logs.String())
			}
		})
	}
}

func TestBuildSetWithRealS3Opener(t *testing.T) {
	cfg := storage.SetConfig{ImageCache: storage.ProviderConfig{
		Scheme: "s3", Bucket: "image-cache", Endpoint: "http://127.0.0.1:1",
		Options: map[string]string{"request_timeout": "nonsense"},
	}}
	if _, err := storage.BuildSet(cfg, storage.LegacyRoots{}, storage.Backends{S3: storages3.Open}, nil); err == nil ||
		!strings.Contains(err.Error(), "request_timeout") {
		t.Fatalf("error = %v", err)
	}
	cfg.ImageCache.Options = nil
	set, err := storage.BuildSet(cfg, storage.LegacyRoots{}, storage.Backends{S3: storages3.Open}, nil)
	if err != nil || set.ImageCache == nil {
		t.Fatalf("BuildSet error = %v", err)
	}
}

func TestSetNormalizedAndAccessors(t *testing.T) {
	memory := storagetest.NewMemory()
	set := storage.Set{Cache: memory}.Normalized()
	if set.Cache != storage.Store(memory) {
		t.Fatal("Normalized replaced a configured store")
	}
	for _, slot := range []storage.Slot{storage.SlotAssets, storage.SlotUserUpload, storage.SlotStatic, storage.SlotImageCache} {
		if _, err := (*set.Store(slot)).Get(context.Background(), "a"); !errors.Is(err, storage.ErrNotConfigured) {
			t.Fatalf("slot %s not Disabled: %v", slot, err)
		}
	}
	if set.Store("unknown") != nil {
		t.Fatal("unknown slot store accessor should be nil")
	}
	var cfg storage.SetConfig
	if cfg.Provider("unknown") != nil {
		t.Fatal("unknown slot provider accessor should be nil")
	}
}
