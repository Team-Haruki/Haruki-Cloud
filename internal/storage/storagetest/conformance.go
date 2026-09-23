package storagetest

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"haruki-cloud/internal/storage"
)

// ConformanceMaxBytes is the object size limit a Conformance factory must
// configure on the stores it builds.
const ConformanceMaxBytes int64 = 1 << 16

// Conformance runs the shared backend suite. factory must return a fresh,
// empty store limited to ConformanceMaxBytes for every call.
func Conformance(t *testing.T, factory func(t *testing.T) storage.Store) {
	t.Helper()
	cases := []struct {
		name string
		run  func(*testing.T, storage.Store)
	}{
		{"RoundTrip", conformRoundTrip},
		{"Overwrite", conformOverwrite},
		{"NotExist", conformNotExist},
		{"Delete", conformDelete},
		{"List", conformList},
		{"ListAbort", conformListAbort},
		{"ListDir", conformListDir},
		{"ListDirAbort", conformListDirAbort},
		{"KeyRejection", conformKeyRejection},
		{"TooLarge", conformTooLarge},
		{"ContextCanceled", conformContextCanceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, factory(t))
		})
	}
}

func check(t *testing.T, ok bool, format string, args ...any) {
	t.Helper()
	if !ok {
		t.Fatalf(format, args...)
	}
}

func conformRoundTrip(t *testing.T, store storage.Store) {
	ctx := context.Background()
	payload := []byte("hello storage")
	check(t, store.Put(ctx, "dir/sub/a.txt", payload, storage.PutOptions{ContentType: "text/plain"}) == nil, "Put failed")
	payload[0] = 'X'
	got, err := store.Get(ctx, "./dir/sub/a.txt")
	check(t, err == nil && string(got) == "hello storage", "Get = %q, %v", got, err)
	got[0] = 'Y'
	again, err := store.Get(ctx, "dir/sub/a.txt")
	check(t, err == nil && string(again) == "hello storage", "Get after mutation = %q, %v", again, err)
	object, err := store.Stat(ctx, "dir/sub/a.txt")
	check(t, err == nil, "Stat error = %v", err)
	check(t, object.Key == "dir/sub/a.txt" && object.Size == int64(len(payload)) && !object.ModTime.IsZero(),
		"Stat = %+v", object)
}

func conformOverwrite(t *testing.T, store storage.Store) {
	ctx := context.Background()
	check(t, store.Put(ctx, "o.bin", bytes.Repeat([]byte("a"), 100), storage.PutOptions{}) == nil, "first Put failed")
	check(t, store.Put(ctx, "o.bin", []byte("bb"), storage.PutOptions{}) == nil, "second Put failed")
	got, err := store.Get(ctx, "o.bin")
	check(t, err == nil && string(got) == "bb", "Get = %q, %v", got, err)
	object, err := store.Stat(ctx, "o.bin")
	check(t, err == nil && object.Size == 2, "Stat = %+v, %v", object, err)
}

func conformNotExist(t *testing.T, store storage.Store) {
	ctx := context.Background()
	_, err := store.Get(ctx, "missing/file")
	check(t, errors.Is(err, storage.ErrNotExist), "Get missing error = %v", err)
	_, err = store.Stat(ctx, "missing/file")
	check(t, errors.Is(err, storage.ErrNotExist), "Stat missing error = %v", err)
}

func conformDelete(t *testing.T, store storage.Store) {
	ctx := context.Background()
	check(t, store.Delete(ctx, "never/written") == nil, "Delete of a missing key must be nil")
	check(t, store.Put(ctx, "gone.txt", []byte("x"), storage.PutOptions{}) == nil, "Put failed")
	check(t, store.Delete(ctx, "gone.txt") == nil, "Delete failed")
	_, err := store.Get(ctx, "gone.txt")
	check(t, errors.Is(err, storage.ErrNotExist), "Get after Delete error = %v", err)
}

func listKeys(t *testing.T, store storage.Store, prefix storage.Key) []string {
	t.Helper()
	var keys []string
	err := store.List(context.Background(), prefix, func(object storage.Object) error {
		keys = append(keys, string(object.Key))
		return nil
	})
	check(t, err == nil, "List(%q) error = %v", prefix, err)
	slices.Sort(keys)
	return keys
}

func conformList(t *testing.T, store storage.Store) {
	ctx := context.Background()
	for _, key := range []storage.Key{"p/a", "p/b/c", "pa", "q/x"} {
		check(t, store.Put(ctx, key, []byte(key), storage.PutOptions{}) == nil, "Put %q failed", key)
	}
	cases := []struct {
		prefix storage.Key
		want   []string
	}{
		{"", []string{"p/a", "p/b/c", "pa", "q/x"}},
		{"p/", []string{"p/a", "p/b/c"}},
		{"p", []string{"p/a", "p/b/c", "pa"}},
		{"p/b", []string{"p/b/c"}},
		{"q/x", []string{"q/x"}},
		{"zzz/", nil},
		{"p/b/c/", nil},
	}
	for _, tc := range cases {
		got := listKeys(t, store, tc.prefix)
		check(t, slices.Equal(got, tc.want), "List(%q) = %v, want %v", tc.prefix, got, tc.want)
	}
}

func conformListAbort(t *testing.T, store storage.Store) {
	ctx := context.Background()
	for _, key := range []storage.Key{"l/1", "l/2", "l/3"} {
		check(t, store.Put(ctx, key, []byte("x"), storage.PutOptions{}) == nil, "Put %q failed", key)
	}
	stop := errors.New("stop")
	visits := 0
	err := store.List(ctx, "l/", func(storage.Object) error {
		visits++
		return stop
	})
	check(t, errors.Is(err, stop) && visits == 1, "List abort = %v after %d visits", err, visits)
}

func listDir(t *testing.T, store storage.Store, prefix storage.Key) (objects, dirs []string) {
	t.Helper()
	err := store.ListDir(context.Background(), prefix, func(entry storage.DirEntry) error {
		if entry.Dir {
			check(t, entry.Object == storage.Object{}, "ListDir(%q) dir %q carries an object", prefix, entry.Name)
			dirs = append(dirs, entry.Name)
			return nil
		}
		check(t, entry.Object.Key == storage.Key(strings.TrimSuffix(string(prefix), "/")+"/"+entry.Name) ||
			(prefix == "" && entry.Object.Key == storage.Key(entry.Name)),
			"ListDir(%q) object %q has key %q", prefix, entry.Name, entry.Object.Key)
		objects = append(objects, entry.Name)
		return nil
	})
	check(t, err == nil, "ListDir(%q) error = %v", prefix, err)
	slices.Sort(objects)
	slices.Sort(dirs)
	return objects, dirs
}

func conformListDir(t *testing.T, store storage.Store) {
	ctx := context.Background()
	for _, key := range []storage.Key{"d/a", "d/b/c", "d/b/e/f", "d/x/y", "da", "top"} {
		check(t, store.Put(ctx, key, []byte(key), storage.PutOptions{}) == nil, "Put %q failed", key)
	}
	cases := []struct {
		prefix        storage.Key
		objects, dirs []string
	}{
		{"", []string{"da", "top"}, []string{"d"}},
		{"d/", []string{"a"}, []string{"b", "x"}},
		{"d", []string{"a"}, []string{"b", "x"}},
		{"d/b/", []string{"c"}, []string{"e"}},
		{"d/b/e/", []string{"f"}, nil},
		{"zzz/", nil, nil},
		{"d/a/", nil, nil},
	}
	for _, tc := range cases {
		objects, dirs := listDir(t, store, tc.prefix)
		check(t, slices.Equal(objects, tc.objects), "ListDir(%q) objects = %v, want %v", tc.prefix, objects, tc.objects)
		check(t, slices.Equal(dirs, tc.dirs), "ListDir(%q) dirs = %v, want %v", tc.prefix, dirs, tc.dirs)
	}
}

func conformListDirAbort(t *testing.T, store storage.Store) {
	ctx := context.Background()
	for _, key := range []storage.Key{"m/1", "m/2", "m/3/4"} {
		check(t, store.Put(ctx, key, []byte("x"), storage.PutOptions{}) == nil, "Put %q failed", key)
	}
	stop := errors.New("stop")
	visits := 0
	err := store.ListDir(ctx, "m/", func(storage.DirEntry) error {
		visits++
		return stop
	})
	check(t, errors.Is(err, stop) && visits == 1, "ListDir abort = %v after %d visits", err, visits)
}

func conformKeyRejection(t *testing.T, store storage.Store) {
	ctx := context.Background()
	for _, key := range []storage.Key{"", "../x", "a/../../b", "a//b", "/"} {
		err := store.Put(ctx, key, []byte("x"), storage.PutOptions{})
		check(t, errors.Is(err, storage.ErrInvalidKey), "Put(%q) error = %v", key, err)
		_, err = store.Get(ctx, key)
		check(t, errors.Is(err, storage.ErrInvalidKey), "Get(%q) error = %v", key, err)
		_, err = store.Stat(ctx, key)
		check(t, errors.Is(err, storage.ErrInvalidKey), "Stat(%q) error = %v", key, err)
		check(t, errors.Is(store.Delete(ctx, key), storage.ErrInvalidKey), "Delete(%q) accepted", key)
	}
	err := store.List(ctx, "../", func(storage.Object) error { return nil })
	check(t, errors.Is(err, storage.ErrInvalidKey), "List(../) error = %v", err)
}

func conformTooLarge(t *testing.T, store storage.Store) {
	ctx := context.Background()
	err := store.Put(ctx, "big.bin", make([]byte, ConformanceMaxBytes+1), storage.PutOptions{})
	check(t, errors.Is(err, storage.ErrTooLarge), "oversize Put error = %v", err)
	_, err = store.Stat(ctx, "big.bin")
	check(t, errors.Is(err, storage.ErrNotExist), "oversize Put left an object: %v", err)
	check(t, store.Put(ctx, "fit.bin", make([]byte, ConformanceMaxBytes), storage.PutOptions{}) == nil,
		"Put at the limit failed")
}

func conformContextCanceled(t *testing.T, store storage.Store) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := store.Put(ctx, "c.txt", []byte("x"), storage.PutOptions{})
	check(t, errors.Is(err, context.Canceled), "Put error = %v", err)
	_, err = store.Get(ctx, "c.txt")
	check(t, errors.Is(err, context.Canceled), "Get error = %v", err)
	_, err = store.Stat(ctx, "c.txt")
	check(t, errors.Is(err, context.Canceled), "Stat error = %v", err)
	check(t, errors.Is(store.Delete(ctx, "c.txt"), context.Canceled), "Delete ignored cancellation")
	check(t, store.Put(context.Background(), "l/c", []byte("x"), storage.PutOptions{}) == nil, "Put failed")
	err = store.List(ctx, "", func(storage.Object) error { return nil })
	check(t, errors.Is(err, context.Canceled), "List error = %v", err)
	_, err = store.Stat(context.Background(), "c.txt")
	check(t, errors.Is(err, storage.ErrNotExist), "canceled Put left an object: %v", err)
}
