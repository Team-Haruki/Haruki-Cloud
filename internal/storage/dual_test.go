package storage_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
	"haruki-cloud/utils/logger"
)

func TestDualConformance(t *testing.T) {
	for _, mode := range []storage.DualMode{storage.DualWrite, storage.DualWriteOnly} {
		t.Run(string(mode), func(t *testing.T) {
			storagetest.Conformance(t, func(t *testing.T) storage.Store {
				primary := storagetest.NewMemory()
				primary.MaxObjectBytes = storagetest.ConformanceMaxBytes
				return storage.NewDual(primary, storagetest.NewMemory(), mode, nil)
			})
		})
	}
}

func TestDualPutMirrorFailureIsLoggedAndSwallowed(t *testing.T) {
	primary := storagetest.NewMemory()
	mirror := storagetest.NewMemory()
	mirror.FailPut = func(storage.Key) error { return errors.New("garage down") }
	var logs bytes.Buffer
	store := storage.NewDual(primary, mirror, storage.DualWrite, logger.NewLogger("storage", "DEBUG", &logs))
	if err := store.Put(context.Background(), "u/1.png", []byte("x"), storage.PutOptions{}); err != nil {
		t.Fatalf("Put error = %v", err)
	}
	if _, err := primary.Get(context.Background(), "u/1.png"); err != nil {
		t.Fatalf("primary missing object: %v", err)
	}
	out := logs.String()
	for _, want := range []string{"WARN", "storage mirror put failed", "u/1.png", "mode=write", "garage down"} {
		if !strings.Contains(out, want) {
			t.Errorf("log %q missing %q", out, want)
		}
	}
}

func TestDualPutPrimaryErrorSkipsMirror(t *testing.T) {
	primary := storagetest.NewMemory()
	primaryErr := errors.New("disk full")
	primary.FailPut = func(storage.Key) error { return primaryErr }
	mirror := storagetest.NewMemory()
	store := storage.NewDual(primary, mirror, storage.DualWrite, nil)
	if err := store.Put(context.Background(), "a", []byte("x"), storage.PutOptions{}); !errors.Is(err, primaryErr) {
		t.Fatalf("Put error = %v", err)
	}
	if calls := mirror.Calls(); len(calls) != 0 {
		t.Fatalf("mirror calls = %v", calls)
	}
}

func TestDualReadThroughOnlyInWriteMode(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		mode        storage.DualMode
		readThrough bool
	}{
		{storage.DualWrite, true},
		{storage.DualWriteOnly, false},
		{"bogus", false},
	} {
		primary := storagetest.NewMemory()
		mirror := storagetest.NewMemory()
		mirror.Seed(map[string][]byte{"old/1.png": []byte("legacy")})
		store := storage.NewDual(primary, mirror, tc.mode, nil)
		data, getErr := store.Get(ctx, "old/1.png")
		object, statErr := store.Stat(ctx, "old/1.png")
		if tc.readThrough {
			if getErr != nil || string(data) != "legacy" || statErr != nil || object.Size != 6 {
				t.Errorf("mode %s: Get = %q, %v; Stat = %+v, %v", tc.mode, data, getErr, object, statErr)
			}
			continue
		}
		if !errors.Is(getErr, storage.ErrNotExist) || !errors.Is(statErr, storage.ErrNotExist) {
			t.Errorf("mode %s: Get error = %v, Stat error = %v", tc.mode, getErr, statErr)
		}
		if len(mirror.Calls()) != 0 {
			t.Errorf("mode %s read the mirror: %v", tc.mode, mirror.Calls())
		}
	}
}

func TestDualReadDoesNotFallThroughOnOtherErrors(t *testing.T) {
	primary := storagetest.NewMemory()
	boom := errors.New("io")
	primary.FailGet = func(storage.Key) error { return boom }
	primary.FailStat = func(storage.Key) error { return boom }
	mirror := storagetest.NewMemory()
	mirror.Seed(map[string][]byte{"k": []byte("v")})
	store := storage.NewDual(primary, mirror, storage.DualWrite, nil)
	if _, err := store.Get(context.Background(), "k"); !errors.Is(err, boom) {
		t.Fatalf("Get error = %v", err)
	}
	if _, err := store.Stat(context.Background(), "k"); !errors.Is(err, boom) {
		t.Fatalf("Stat error = %v", err)
	}
}

func TestDualDeleteHitsBoth(t *testing.T) {
	ctx := context.Background()
	primary := storagetest.NewMemory()
	mirror := storagetest.NewMemory()
	primary.Seed(map[string][]byte{"k": []byte("p")})
	mirror.Seed(map[string][]byte{"k": []byte("m")})
	store := storage.NewDual(primary, mirror, storage.DualWriteOnly, nil)
	if err := store.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	for name, backend := range map[string]*storagetest.Memory{"primary": primary, "mirror": mirror} {
		if _, err := backend.Get(ctx, "k"); !errors.Is(err, storage.ErrNotExist) {
			t.Errorf("%s still has the object: %v", name, err)
		}
	}

	primaryErr := errors.New("primary delete")
	mirrorErr := errors.New("mirror delete")
	primary.FailDelete = func(storage.Key) error { return primaryErr }
	mirror.FailDelete = func(storage.Key) error { return mirrorErr }
	if err := store.Delete(ctx, "k"); !errors.Is(err, primaryErr) {
		t.Fatalf("Delete error = %v, want the primary error first", err)
	}
	if got := len(mirror.Calls()); got != 3 {
		t.Fatalf("mirror calls = %d, want the mirror attempted even after a primary failure", got)
	}
	primary.FailDelete = nil
	if err := store.Delete(ctx, "k"); !errors.Is(err, mirrorErr) {
		t.Fatalf("Delete error = %v, want the mirror error", err)
	}
	primary.FailDelete = func(storage.Key) error { return storage.NotExistError("delete", "k") }
	mirror.FailDelete = func(storage.Key) error { return storage.NotExistError("delete", "k") }
	if err := store.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete with ENOENT on both = %v", err)
	}
}

func TestDualListUsesPrimaryOnly(t *testing.T) {
	primary := storagetest.NewMemory()
	mirror := storagetest.NewMemory()
	primary.Seed(map[string][]byte{"p/1": []byte("x")})
	mirror.Seed(map[string][]byte{"p/2": []byte("y")})
	store := storage.NewDual(primary, mirror, storage.DualWrite, nil)
	var keys []storage.Key
	if err := store.List(context.Background(), "p/", func(object storage.Object) error {
		keys = append(keys, object.Key)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != "p/1" || len(mirror.Calls()) != 0 {
		t.Fatalf("keys = %v, mirror calls = %v", keys, mirror.Calls())
	}
	var names []string
	if err := store.ListDir(context.Background(), "p/", func(entry storage.DirEntry) error {
		names = append(names, entry.Name)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "1" || len(mirror.Calls()) != 0 {
		t.Fatalf("ListDir names = %v, mirror calls = %v", names, mirror.Calls())
	}
}

func TestDualNilStoresAreDisabled(t *testing.T) {
	store := storage.NewDual(nil, nil, storage.DualWrite, nil)
	if err := store.Put(context.Background(), "a", []byte("x"), storage.PutOptions{}); !errors.Is(err, storage.ErrNotConfigured) {
		t.Fatalf("Put error = %v", err)
	}
	if _, err := store.Get(context.Background(), "a"); !errors.Is(err, storage.ErrNotConfigured) {
		t.Fatalf("Get error = %v", err)
	}
	if err := store.Delete(context.Background(), "a"); err != nil {
		t.Fatalf("Delete on disabled stores = %v", err)
	}
	primary := storagetest.NewMemory()
	withNilMirror := storage.NewDual(primary, nil, storage.DualWrite, nil)
	if err := withNilMirror.Put(context.Background(), "a", []byte("x"), storage.PutOptions{}); err != nil {
		t.Fatalf("Put with a nil mirror = %v", err)
	}
}
