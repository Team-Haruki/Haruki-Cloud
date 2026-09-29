package music

import (
	"context"
	"errors"
	"strings"
	"testing"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

func TestMusicRewardAchievementStageTrace(t *testing.T) {
	requireErr := errors.New("snapshot unavailable")
	for _, tc := range []struct {
		name       string
		snapshot   snapshot.Snapshot
		wantError  string
		wantLookup int
		wantDecode int
	}{
		{
			name: "direct", snapshot: &musicSnapshotStub{rawValues: map[string][]byte{"userMusicAchievements": []byte("[]")}},
			wantLookup: 1, wantDecode: 1,
		},
		{
			name: "compact", snapshot: &musicSnapshotStub{rawValues: map[string][]byte{"compactUserMusicAchievements": []byte("[]")}},
			wantLookup: 1, wantDecode: 1,
		},
		{
			name: "nested fallback", snapshot: &musicSnapshotStub{rawBytes: []byte(`{"nested":{"compactUserMusicAchievements":[{"musicId":1,"musicAchievementId":1}]}}`)},
			wantLookup: 1, wantDecode: 1,
		},
		{
			name: "lookup error", snapshot: &musicSnapshotStub{rawBytes: []byte("{}")},
			wantError: "unavailable", wantLookup: 1,
		},
		{
			name: "decode error", snapshot: &musicSnapshotStub{rawValues: map[string][]byte{"userMusicAchievements": []byte("not-json")}},
			wantError: "decode userMusicAchievements", wantLookup: 1, wantDecode: 1,
		},
		{
			name: "require error", snapshot: &failingMusicSnapshot{requireErr: requireErr}, wantError: requireErr.Error(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			controller, _ := buildRewardsCoverageController(t)
			ctx, trace := commandtrace.WithTrace(context.Background())
			controller = controller.WithContext(ctx)
			payload, err := controller.BuildMusicRewardsDetailRequestFromSnapshot(RewardsDetailQuery{Region: "jp"}, tc.snapshot)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("rewards error = %v, want %q", err, tc.wantError)
				}
			} else if err != nil || payload == nil {
				t.Fatalf("rewards result = %+v, %v", payload, err)
			}
			counts := map[string]int{}
			for _, operation := range trace.Snapshot().Operations {
				counts[operation.Name] = operation.Count
			}
			if counts["music.achievement_lookup"] != tc.wantLookup || counts["music.achievement_decode"] != tc.wantDecode {
				t.Fatalf("operation counts = %v, want lookup %d, decode %d", counts, tc.wantLookup, tc.wantDecode)
			}
		})
	}
}
