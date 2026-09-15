package storagetest

import (
	"context"
	"errors"
	"slices"
	"testing"

	"haruki-cloud/internal/storage"
)

func TestMemoryConformance(t *testing.T) {
	Conformance(t, func(t *testing.T) storage.Store {
		store := NewMemory()
		store.MaxObjectBytes = ConformanceMaxBytes
		return store
	})
}

func TestMemoryHooksAndRecorder(t *testing.T) {
	ctx := context.Background()
	store := NewMemory()
	boom := errors.New("boom")
	var hookKeys []storage.Key
	hook := func(key storage.Key) error {
		hookKeys = append(hookKeys, key)
		return boom
	}
	store.FailGet, store.FailPut, store.FailStat, store.FailDelete, store.FailList = hook, hook, hook, hook, hook

	_, getErr := store.Get(ctx, "./a")
	putErr := store.Put(ctx, "a", []byte("x"), storage.PutOptions{})
	_, statErr := store.Stat(ctx, "a")
	deleteErr := store.Delete(ctx, "a")
	listErr := store.List(ctx, "p/", func(storage.Object) error { return nil })
	for i, err := range []error{getErr, putErr, statErr, deleteErr, listErr} {
		if !errors.Is(err, boom) {
			t.Errorf("call %d error = %v", i, err)
		}
	}
	if want := []storage.Key{"a", "a", "a", "a", "p/"}; !slices.Equal(hookKeys, want) {
		t.Fatalf("hook keys = %v, want %v", hookKeys, want)
	}
	want := []Op{{"Get", "./a"}, {"Put", "a"}, {"Stat", "a"}, {"Delete", "a"}, {"List", "p/"}}
	if got := store.Calls(); !slices.Equal(got, want) {
		t.Fatalf("Calls = %v, want %v", got, want)
	}
}

func TestMemorySeedAndDefaults(t *testing.T) {
	store := NewMemory()
	store.MaxObjectBytes = 0
	store.Seed(map[string][]byte{"/seeded/a": []byte("v")})
	data, err := store.Get(context.Background(), "seeded/a")
	if err != nil || string(data) != "v" {
		t.Fatalf("Get = %q, %v", data, err)
	}
	if err := store.Put(context.Background(), "big", make([]byte, 1024), storage.PutOptions{}); err != nil {
		t.Fatalf("Put with default limit = %v", err)
	}
	if calls := store.Calls(); len(calls) != 2 {
		t.Fatalf("Seed must not be recorded: %v", calls)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("Seed with an invalid key did not panic")
		}
	}()
	store.Seed(map[string][]byte{"../x": nil})
}
