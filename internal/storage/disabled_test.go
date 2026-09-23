package storage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"testing"
)

func TestDisabledReturnsNotConfigured(t *testing.T) {
	store := Disabled()
	ctx := context.Background()
	_, getErr := store.Get(ctx, "a")
	_, statErr := store.Stat(ctx, "a")
	errs := []error{
		getErr,
		store.Put(ctx, "a", []byte("x"), PutOptions{}),
		statErr,
		store.Delete(ctx, "a"),
		store.List(ctx, "", func(Object) error { t.Fatal("List must not visit"); return nil }),
		store.ListDir(ctx, "", func(DirEntry) error { t.Fatal("ListDir must not visit"); return nil }),
	}
	for i, err := range errs {
		if !errors.Is(err, ErrNotConfigured) || !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("method %d error = %v", i, err)
		}
	}
}

func TestErrorHelpers(t *testing.T) {
	notExist := NotExistError("get", "a/b")
	if !errors.Is(notExist, ErrNotExist) || !os.IsNotExist(notExist) {
		t.Fatalf("NotExistError = %v", notExist)
	}
	tooLarge := TooLargeError("put", "a", 10, 5)
	if !errors.Is(tooLarge, ErrTooLarge) || errors.Is(tooLarge, ErrNotExist) {
		t.Fatalf("TooLargeError = %v", tooLarge)
	}
}
