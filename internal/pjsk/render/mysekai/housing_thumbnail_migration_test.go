package mysekai

import "testing"

func TestHousingCacheKeySurvivesOwnerRedaction(t *testing.T) {
	entry := HousingCompetitionEntry{CompetitionID: 2, OwnerUserID: 42, SubmittedAt: 123, ThumbnailPath: "hash/uuid", EntryName: "work", ReviewCount: 100}
	key := housingCompetitionEntryCacheKey(entry)
	entry.OwnerUserID = 0
	if housingCompetitionEntryCacheKey(entry) != key {
		t.Fatal("redacting owner changed upload identity")
	}
	cached := entry
	cached.CacheKey = key
	cached.ReviewCount = 120
	entries := map[string]HousingCompetitionEntry{key: cached}
	entry.OwnerUserID = 42
	mergeHousingCompetitionEntries(entries, []HousingCompetitionEntry{entry})
	if len(entries) != 1 || entries[key].ReviewCount != 120 {
		t.Fatal("persisted upload was duplicated or lost votes")
	}
	// This fixture is also checked by the one-time migration script.
	if key != "d5bbd469bace6d1c06c38dda56146a994a5213d442d6f887c2a3ea9801feb0e6" {
		t.Fatalf("migration key mismatch: %s", key)
	}
	for _, change := range []func(*HousingCompetitionEntry){
		func(e *HousingCompetitionEntry) { e.CompetitionID++ },
		func(e *HousingCompetitionEntry) { e.SubmittedAt++ },
		func(e *HousingCompetitionEntry) { e.EntryName = "other" },
		func(e *HousingCompetitionEntry) { e.ThumbnailPath = "different/uuid" },
	} {
		other := entry
		change(&other)
		if housingCompetitionEntryCacheKey(other) == key {
			t.Fatal("distinct submission has the same key")
		}
	}
}
