package mysekai

import (
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestIsMysekaiSnapshotExpiredUsesLatestRefreshBoundary(t *testing.T) {
	now := time.Date(2026, 4, 20, 18, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	lastRefresh, _ := mysekaiLastRefreshTimeAndReason(renderregion.JP, now)

	stale := map[string]any{
		"upload_time": float64(lastRefresh.Add(-time.Minute).UnixMilli()),
	}
	if !isMysekaiSnapshotExpired(renderregion.JP, stale, now) {
		t.Fatalf("expected snapshot uploaded before refresh to be expired")
	}

	fresh := map[string]any{
		"upload_time": float64(lastRefresh.UnixMilli()),
	}
	if isMysekaiSnapshotExpired(renderregion.JP, fresh, now) {
		t.Fatalf("expected snapshot uploaded at refresh time to stay fresh")
	}
}
