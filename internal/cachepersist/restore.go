package cachepersist

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"haruki-cloud/internal/storage"
	"haruki-cloud/utils/logger"
)

// ReadTimeout bounds one startup snapshot read.
const ReadTimeout = 10 * time.Second

// SnapshotReader applies one persisted snapshot object to the cache; replace
// resets the in-memory state before applying it.
type SnapshotReader func(ctx context.Context, key storage.Key, replace bool) error

// SnapshotRestorer restores a cache from its persisted snapshots at startup:
// the namespaced object first, and the pre-namespace shared object (Legacy)
// only as a migration fallback. The legacy object is read at most once per
// process. A zero restorer, or one without Read, does nothing.
type SnapshotRestorer struct {
	Read   SnapshotReader
	Legacy storage.Key
	// Cache labels the log line.
	Cache        string
	legacyLoaded bool
}

// Restore reads the namespaced snapshot and reports whether persistence may
// start. A missing namespaced object falls back to the legacy snapshot and
// still enables persistence. Any other read error leaves persistence
// disabled: the namespaced snapshot may exist unread, and a later write must
// not replace it with older data.
func (r *SnapshotRestorer) Restore(ctx context.Context, namespaced storage.Key) bool {
	if r == nil || r.Read == nil {
		return false
	}
	err := r.read(ctx, namespaced, true)
	switch {
	case err == nil:
		return true
	case errors.Is(err, storage.ErrNotExist):
		r.LoadLegacy(ctx)
		return true
	default:
		logger.WarnContext(ctx, "Cache snapshot namespace read failed; persistence stays disabled",
			slog.String("cache", r.Cache), slog.String("error_type", fmt.Sprintf("%T", err)))
		return false
	}
}

// LoadLegacy reads the pre-namespace shared object once, merging it into
// whatever the cache holds. Besides the Restore fallback it is the warm start
// of a cache that stays read-only.
func (r *SnapshotRestorer) LoadLegacy(ctx context.Context) {
	if r == nil || r.Read == nil || r.legacyLoaded || r.Legacy == "" {
		return
	}
	r.legacyLoaded = true
	_ = r.read(ctx, r.Legacy, false)
}

// MarkLegacyLoaded records that the caller already read the legacy object
// (an explicit local file is read eagerly at construction).
func (r *SnapshotRestorer) MarkLegacyLoaded() {
	if r != nil {
		r.legacyLoaded = true
	}
}

func (r *SnapshotRestorer) read(ctx context.Context, key storage.Key, replace bool) error {
	readCtx, cancel := context.WithTimeout(storage.WithBackgroundIO(ctx), ReadTimeout)
	defer cancel()
	return r.Read(readCtx, key, replace)
}
