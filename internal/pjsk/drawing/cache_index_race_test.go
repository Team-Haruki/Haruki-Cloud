package drawing

import (
	"context"
	"testing"
	"time"

	"haruki-cloud/utils/imagecache"
)

type expiryRaceIndex struct {
	expires time.Time
	cutoff  time.Time
}

func (*expiryRaceIndex) LookupRender(context.Context, string) (imagecache.RenderIndexEntry, bool, error) {
	return imagecache.RenderIndexEntry{}, false, nil
}
func (f *expiryRaceIndex) TouchRender(context.Context, []string) (int64, error) {
	f.expires = time.Now().Add(time.Hour)
	return 1, nil
}
func (f *expiryRaceIndex) DeleteExpiredRender(_ context.Context, _ []string, cutoff time.Time) (int64, error) {
	f.cutoff = cutoff
	if !f.expires.IsZero() && !f.expires.After(cutoff) {
		f.expires = time.Time{}
		return 1, nil
	}
	return 0, nil
}

func TestExpiredCleanupPreservesConcurrentTouch(t *testing.T) {
	now := time.Now()
	index := &expiryRaceIndex{expires: now.Add(-time.Hour)}
	writer := newRenderIndexWriter(index, time.Minute)
	writer.now = func() time.Time { return now }
	writer.flushEvery = time.Hour
	defer writer.close()
	writer.expire("k")
	writer.touch("k")
	writer.flush()
	if index.expires.IsZero() || !index.expires.After(now) {
		t.Fatal("stale expiry cleanup deleted the renewed row")
	}
	if !index.cutoff.Equal(now) {
		t.Fatalf("cutoff = %v, want observation time %v", index.cutoff, now)
	}
}

func TestExpiredCleanupPreservesReplacementAndInfiniteTTL(t *testing.T) {
	for _, replacement := range []time.Time{time.Now().Add(time.Hour), {}} {
		now := time.Now()
		index := &expiryRaceIndex{expires: now.Add(-time.Hour)}
		writer := newRenderIndexWriter(index, time.Minute)
		writer.now = func() time.Time { return now }
		writer.flushEvery = time.Hour
		writer.expire("k")
		index.expires = replacement
		writer.flush()
		if !index.expires.Equal(replacement) {
			t.Fatal("cleanup deleted replacement")
		}
		writer.close()
	}
}
