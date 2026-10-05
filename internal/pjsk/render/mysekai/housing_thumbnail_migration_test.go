package mysekai

import "testing"

func TestHousingThumbnailCacheMigratesAbsolutePathsWithoutLosingSamples(t *testing.T) {
	for _, host := range []string{"mk-prod-tos.tos-cn-shanghai.volces.com", "mkoversea-prod-bucket.s3.ap-northeast-1.amazonaws.com", "mkkorea-prod-bucket.s3.ap-northeast-1.amazonaws.com"} {
		t.Run(host, func(t *testing.T) {
			old := HousingCompetitionEntry{CompetitionID: 2, OwnerUserID: 42, EntryName: "work", SubmittedAt: 123, ReviewCount: 100, ThumbnailPath: "https://" + host + "/image/mysekai-housing-competition/thumbnail/hash/uuid", LastSeenAt: 1000}
			oldKey := housingCompetitionEntryCacheKey(old)
			old.CacheKey = oldKey
			old.OwnerUserID = 0 // persisted snapshots deliberately drop the owner ID
			next := old
			next.CacheKey = "new-key"
			next.ThumbnailPath = "hash/uuid"
			next.ReviewCount = 90
			next.LastSeenAt = 2000
			unrelated := old
			unrelated.CacheKey = "unrelated"
			unrelated.SubmittedAt++
			entries := map[string]HousingCompetitionEntry{oldKey: old, "unrelated": unrelated}
			mergeHousingCompetitionEntries(entries, []HousingCompetitionEntry{next})
			if len(entries) != 2 {
				t.Fatalf("duplicate or lost sample: %+v", entries)
			}
			got := entries["new-key"]
			if got.ReviewCount != 100 || got.LastSeenAt != 2000 || got.ThumbnailPath != "hash/uuid" || got.CacheKey != "new-key" {
				t.Fatalf("lost cached data: %+v", got)
			}
			if _, ok := entries[oldKey]; ok {
				t.Fatal("old duplicate remains")
			}
			next.ReviewCount = 110
			mergeHousingCompetitionEntries(entries, []HousingCompetitionEntry{next})
			if len(entries) != 2 || entries["new-key"].ReviewCount != 110 {
				t.Fatal("repeated update failed")
			}
		})
	}
}

func TestHousingThumbnailCacheKeepsDifferentSubmissionsAndExistingRelativeKey(t *testing.T) {
	old := HousingCompetitionEntry{CacheKey: "old", CompetitionID: 1, SubmittedAt: 1, EntryName: "work", ReviewCount: 100, ThumbnailPath: "https://example.com/image/mysekai-housing-competition/thumbnail/hash/uuid"}
	next := old
	next.CacheKey = "new"
	next.ThumbnailPath = "hash/uuid"
	for _, change := range []func(*HousingCompetitionEntry){
		func(e *HousingCompetitionEntry) { e.CompetitionID++ },
		func(e *HousingCompetitionEntry) { e.SubmittedAt++ },
		func(e *HousingCompetitionEntry) { e.EntryName = "other" },
		func(e *HousingCompetitionEntry) { e.ThumbnailPath = "different/uuid" },
	} {
		other := next
		change(&other)
		dst := map[string]HousingCompetitionEntry{"old": old}
		mergeHousingCompetitionEntries(dst, []HousingCompetitionEntry{other})
		if len(dst) != 2 {
			t.Fatal("merged distinct submissions")
		}
	}
	dst := map[string]HousingCompetitionEntry{"old": old, "new": next}
	mergeHousingCompetitionEntries(dst, []HousingCompetitionEntry{next})
	if len(dst) != 1 || dst["new"].ReviewCount != 100 {
		t.Fatal("existing duplicate was not consolidated")
	}
	for _, path := range []string{"hash/uuid", "https://%bad", "https://example.com/other", "https://example.com/image/mysekai-housing-competition/thumbnail/hash/uuid?x=1"} {
		old.ThumbnailPath = path
		if _, ok := legacyHousingThumbnailIdentity(old); ok {
			t.Fatalf("unexpected legacy path: %s", path)
		}
	}
}
