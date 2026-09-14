package storage_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func newLocal(t *testing.T, max int64) (storage.Store, string) {
	t.Helper()
	root := t.TempDir()
	store, err := storage.NewLocal(root, max)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	return store, root
}

func TestLocalConformance(t *testing.T) {
	storagetest.Conformance(t, func(t *testing.T) storage.Store {
		store, _ := newLocal(t, storagetest.ConformanceMaxBytes)
		return store
	})
}

func TestNewLocalValidatesRoot(t *testing.T) {
	for _, root := range []string{"", "   ", "relative/dir", "./x"} {
		if store, err := storage.NewLocal(root, 0); err == nil || store != nil {
			t.Errorf("NewLocal(%q) = %v, %v; want error", root, store, err)
		}
	}
	root := filepath.Join(t.TempDir(), "not", "yet", "created")
	store, err := storage.NewLocal(root+string(filepath.Separator), 0)
	if err != nil {
		t.Fatalf("NewLocal on a missing root: %v", err)
	}
	if err := store.Put(context.Background(), "k", []byte("v"), storage.PutOptions{}); err != nil {
		t.Fatalf("Put under a missing root: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "k")); err != nil || string(data) != "v" {
		t.Fatalf("file = %q, %v", data, err)
	}
}

func TestLocalPutWritesPathsAndModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}
	store, root := newLocal(t, 0)
	ctx := context.Background()
	payload := []byte{0x89, 'P', 'N', 'G', 0, 1, 2}
	if err := store.Put(ctx, "profile_bg/jp/42.png", payload, storage.PutOptions{ContentType: "image/png"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	full := filepath.Join(root, "profile_bg", "jp", "42.png")
	data, err := os.ReadFile(full)
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("on-disk bytes = %v, %v", data, err)
	}
	info, err := os.Stat(full)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("file mode = %v, %v", info.Mode(), err)
	}
	for _, dir := range []string{filepath.Join(root, "profile_bg"), filepath.Dir(full)} {
		dirInfo, err := os.Stat(dir)
		if err != nil || dirInfo.Mode().Perm()&^0o022 != 0o755&^0o022 || dirInfo.Mode().Perm()&0o700 != 0o700 {
			t.Fatalf("dir %s mode = %v, %v", dir, dirInfo.Mode(), err)
		}
	}
	assertNoTempResidue(t, root)
}

func assertNoTempResidue(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Errorf("temp residue %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

func TestLocalPutFailuresLeaveNoResidue(t *testing.T) {
	store, root := newLocal(t, 0)
	ctx := context.Background()

	// The target is a non-empty directory, so the final rename fails.
	if err := os.MkdirAll(filepath.Join(root, "busy", "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, "busy", []byte("x"), storage.PutOptions{}); err == nil {
		t.Fatal("Put over a non-empty directory succeeded")
	}
	// A parent component is a regular file, so MkdirAll fails.
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("f"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, "file/child", []byte("x"), storage.PutOptions{}); err == nil {
		t.Fatal("Put below a regular file succeeded")
	}
	assertNoTempResidue(t, root)
}

func TestLocalPutTempCreateFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs unix permissions as a non-root user")
	}
	store, root := newLocal(t, 0)
	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	if err := store.Put(context.Background(), "locked/x", []byte("x"), storage.PutOptions{}); err == nil {
		t.Fatal("Put into a read-only directory succeeded")
	}
	if err := store.Delete(context.Background(), "locked/x"); err != nil {
		t.Fatalf("Delete of a missing key in a read-only dir: %v", err)
	}
}

func TestLocalConcurrentOverwriteNeverPartial(t *testing.T) {
	store, _ := newLocal(t, 0)
	ctx := context.Background()
	first := bytes.Repeat([]byte("a"), 256<<10)
	second := bytes.Repeat([]byte("b"), 256<<10)
	if err := store.Put(ctx, "hot.bin", first, storage.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			payload := first
			if i%2 == 0 {
				payload = second
			}
			if err := store.Put(ctx, "hot.bin", payload, storage.PutOptions{}); err != nil {
				t.Errorf("Put: %v", err)
			}
		}
		close(stop)
	}()
	for {
		select {
		case <-stop:
			wg.Wait()
			return
		default:
		}
		data, err := store.Get(ctx, "hot.bin")
		if err != nil {
			t.Fatalf("Get during overwrite: %v", err)
		}
		if !bytes.Equal(data, first) && !bytes.Equal(data, second) {
			t.Fatalf("Get observed a partial file of %d bytes", len(data))
		}
	}
}

func TestLocalNotExistShapes(t *testing.T) {
	store, root := newLocal(t, 0)
	ctx := context.Background()
	if err := os.MkdirAll(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plain"), []byte("p"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, key := range []storage.Key{"dir", "plain/below", "absent"} {
		_, err := store.Get(ctx, key)
		if !errors.Is(err, storage.ErrNotExist) || !os.IsNotExist(err) {
			t.Errorf("Get(%q) error = %v", key, err)
		}
		_, err = store.Stat(ctx, key)
		if !errors.Is(err, storage.ErrNotExist) || !os.IsNotExist(err) {
			t.Errorf("Stat(%q) error = %v", key, err)
		}
		if err := store.Delete(ctx, key); err != nil {
			t.Errorf("Delete(%q) error = %v", key, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "dir")); err != nil {
		t.Fatalf("Delete removed a directory: %v", err)
	}
}

func TestLocalGetTooLarge(t *testing.T) {
	store, root := newLocal(t, 8)
	if err := os.WriteFile(filepath.Join(root, "big"), make([]byte, 9), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), "big"); !errors.Is(err, storage.ErrTooLarge) {
		t.Fatalf("Get oversize error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fits"), make([]byte, 8), 0o644); err != nil {
		t.Fatal(err)
	}
	if data, err := store.Get(context.Background(), "fits"); err != nil || len(data) != 8 {
		t.Fatalf("Get at limit = %d bytes, %v", len(data), err)
	}
}

func TestLocalListSkipsTempFilesAndDirs(t *testing.T) {
	store, root := newLocal(t, 0)
	ctx := context.Background()
	for _, key := range []storage.Key{"bg/jp/1.png", "bg/jp/2.png", "bg/cn/3.png", "other.txt"} {
		if err := store.Put(ctx, key, []byte(key), storage.PutOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "bg", "jp", ".1.png.tmp-123"), []byte("t"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "bg", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(filepath.Join(root, "other.txt"), filepath.Join(root, "bg", "link")); err != nil {
			t.Fatal(err)
		}
	}
	var objects []storage.Object
	err := store.List(ctx, "bg/", func(object storage.Object) error {
		objects = append(objects, object)
		return nil
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var keys []string
	for _, object := range objects {
		keys = append(keys, string(object.Key))
		if object.Size != int64(len(object.Key)) || object.ModTime.IsZero() {
			t.Errorf("object %+v", object)
		}
	}
	slices.Sort(keys)
	if want := []string{"bg/cn/3.png", "bg/jp/1.png", "bg/jp/2.png"}; !slices.Equal(keys, want) {
		t.Fatalf("List keys = %v, want %v", keys, want)
	}
	if err := store.List(ctx, `bg\jp\`, func(storage.Object) error { return nil }); err != nil {
		t.Fatalf("List with backslash prefix: %v", err)
	}
}

func TestLocalListMissingRootAndFilePrefix(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	store, err := storage.NewLocal(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	visit := func(storage.Object) error { t.Fatal("unexpected visit"); return nil }
	if err := store.List(context.Background(), "", visit); err != nil {
		t.Fatalf("List on a missing root: %v", err)
	}
	present, presentRoot := newLocal(t, 0)
	if err := os.WriteFile(filepath.Join(presentRoot, "a"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := present.List(context.Background(), "a/", visit); err != nil {
		t.Fatalf("List below a regular file: %v", err)
	}
}

func TestLocalListUnreadableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs unix permissions as a non-root user")
	}
	store, root := newLocal(t, 0)
	sealed := filepath.Join(root, "sealed")
	if err := os.Mkdir(sealed, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sealed, 0o755) })
	err := store.List(context.Background(), "", func(storage.Object) error { return nil })
	if err == nil || errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("List over an unreadable directory error = %v", err)
	}
}

func TestLocalPermissionErrorsSurface(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs unix permissions as a non-root user")
	}
	store, root := newLocal(t, 0)
	ctx := context.Background()
	if err := store.Put(ctx, "sealed/f", []byte("x"), storage.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	sealed := filepath.Join(root, "sealed")
	if err := os.Chmod(sealed, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sealed, 0o755) })
	_, getErr := store.Get(ctx, "sealed/f")
	_, statErr := store.Stat(ctx, "sealed/f")
	for name, err := range map[string]error{"Get": getErr, "Stat": statErr, "Delete": store.Delete(ctx, "sealed/f")} {
		if err == nil || errors.Is(err, storage.ErrNotExist) {
			t.Errorf("%s error = %v, want a permission error", name, err)
		}
	}
}
