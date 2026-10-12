package cachepersist

import (
	"context"
	"errors"
	"testing"

	"haruki-cloud/internal/storage"
)

type restoreCall struct {
	key     storage.Key
	replace bool
}

func TestSnapshotRestorer(t *testing.T) {
	ctx := context.Background()
	newRestorer := func(errs map[storage.Key]error) (*SnapshotRestorer, *[]restoreCall) {
		calls := &[]restoreCall{}
		return &SnapshotRestorer{Cache: "test", Legacy: "legacy.json", Read: func(ctx context.Context, key storage.Key, replace bool) error {
			if !storage.IsBackgroundIO(ctx) {
				t.Fatal("snapshot read is not background I/O")
			}
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("snapshot read has no deadline")
			}
			*calls = append(*calls, restoreCall{key, replace})
			return errs[key]
		}}, calls
	}

	// Namespaced object present: one replacing read, persistence enabled.
	r, calls := newRestorer(nil)
	if !r.Restore(ctx, "instances/a/x.json") || len(*calls) != 1 || (*calls)[0] != (restoreCall{"instances/a/x.json", true}) {
		t.Fatalf("present namespaced snapshot: calls=%+v", *calls)
	}

	// Namespaced object missing: legacy fallback (merging), persistence enabled,
	// and the legacy object is never read twice.
	r, calls = newRestorer(map[storage.Key]error{"instances/a/x.json": storage.NotExistError("get", "instances/a/x.json")})
	if !r.Restore(ctx, "instances/a/x.json") {
		t.Fatal("missing namespaced snapshot disabled persistence")
	}
	r.LoadLegacy(ctx)
	if len(*calls) != 2 || (*calls)[1] != (restoreCall{"legacy.json", false}) {
		t.Fatalf("missing namespaced snapshot: calls=%+v", *calls)
	}

	// Any other read error: no legacy read, persistence stays disabled.
	r, calls = newRestorer(map[storage.Key]error{"instances/a/x.json": errors.New("transient")})
	if r.Restore(ctx, "instances/a/x.json") || len(*calls) != 1 {
		t.Fatalf("failed namespaced read: calls=%+v", *calls)
	}

	// A legacy object already read by the caller is not read again.
	r, calls = newRestorer(nil)
	r.MarkLegacyLoaded()
	r.LoadLegacy(ctx)
	if len(*calls) != 0 {
		t.Fatalf("marked legacy was read: %+v", *calls)
	}

	// Zero and nil restorers are inert; no legacy key means no legacy read.
	var nilRestorer *SnapshotRestorer
	nilRestorer.LoadLegacy(ctx)
	nilRestorer.MarkLegacyLoaded()
	if nilRestorer.Restore(ctx, "k") || (&SnapshotRestorer{}).Restore(ctx, "k") {
		t.Fatal("restorer without a reader enabled persistence")
	}
	r, calls = newRestorer(nil)
	r.Legacy = ""
	r.LoadLegacy(ctx)
	if len(*calls) != 0 {
		t.Fatalf("empty legacy key was read: %+v", *calls)
	}
}
