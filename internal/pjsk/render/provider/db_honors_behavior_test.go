package provider

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"entgo.io/ent"

	sekaiDB "haruki-cloud/database/sekai"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestDBHonorProviderQueriesCachesAndDerivesAssets(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "bondsHonorWords.json", `[
		{"ID":30,"Seq":1,"BondsGroupID":20,"AssetBundleName":"word_30","Name":"Together","Description":"word"}
	]`)

	provider := openProviderBehaviorDB(t, "honors_success")
	provider.honors.store = newLocalStore(root)
	client := provider.client

	if _, err := client.Honor.Create().
		SetGameID(1).
		SetGroupID(10).
		SetHonorRarity("high").
		SetName("Top honor").
		SetAssetbundleName("honor_1").
		SetLevels(json.RawMessage(`[{"Level":1,"HonorRarity":"high","Description":"level one","AssetBundleName":"honor_1_1"}]`)).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create honor: %v", err)
	}
	for _, item := range []struct {
		id         int64
		name       string
		honorType  string
		background string
		frame      string
	}{
		{id: 10, name: "Normal group", honorType: "normal", background: "normal_bg", frame: "normal_frame"},
		{id: 11, name: "花里实乃理 生日", honorType: "birthday"},
		{id: 12, name: "Unknown birthday", honorType: "birthday"},
	} {
		builder := client.Honorgroup.Create().
			SetGameID(item.id).
			SetName(item.name).
			SetHonorType(item.honorType).
			SetServerRegion(renderregion.JP.String())
		if item.background != "" {
			builder.SetBackgroundAssetbundleName(item.background)
		}
		if item.frame != "" {
			builder.SetFrameName(item.frame)
		}
		if _, err := builder.Save(ctx); err != nil {
			t.Fatalf("create honor group %d: %v", item.id, err)
		}
	}
	if _, err := client.Bondshonor.Create().
		SetGameID(20).
		SetBondsGroupID(200).
		SetGameCharacterUnitId1(501).
		SetGameCharacterUnitId2(502).
		SetHonorRarity("middle").
		SetName("Best partners").
		SetDescription("bonds description").
		SetConfigurableUnitVirtualSinger(true).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create bonds honor: %v", err)
	}
	if _, err := client.Gamecharacter.Create().
		SetGameID(5).
		SetFirstName("花里").
		SetGivenName("实乃理").
		SetFirstNameEnglish("Hanasato").
		SetGivenNameEnglish("Minori").
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create game character: %v", err)
	}
	if _, err := client.Gamecharacterunit.Create().
		SetGameID(501).
		SetGameCharacterID(5).
		SetUnit("idol").
		SetColorCode("#abcdef").
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create game character unit: %v", err)
	}
	if _, err := client.Event.Create().
		SetGameID(40).
		SetEventRankingRewardRanges(json.RawMessage(`[
			{"eventRankingRewardDetails":[
				{"resourceType":"honor","resourceId":1},
				{"resourceType":"jewel","resourceId":2},
				{"resourceType":"honor","resourceId":0}
			]}
		]`)).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create event honor mapping: %v", err)
	}
	if _, err := client.Event.Create().
		SetGameID(41).
		SetEventRankingRewardRanges(json.RawMessage(`[]`)).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create empty event honor mapping: %v", err)
	}

	var honorQueries atomic.Int32
	client.Honor.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			honorQueries.Add(1)
			return next.Query(ctx, query)
		})
	}))
	var bondsQueries atomic.Int32
	client.Bondshonor.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			bondsQueries.Add(1)
			return next.Query(ctx, query)
		})
	}))

	honors := provider.honors
	if _, err := honors.GetByID(ctx, 0); err == nil {
		t.Fatal("GetByID(0) should reject an invalid honor ID")
	}
	honor, err := honors.GetByID(ctx, 1)
	if err != nil || honor.Name != "Top honor" || len(honor.Levels) != 1 || honor.Levels[0].Level != 1 {
		t.Fatalf("GetByID(1) = %+v, %v", honor, err)
	}
	honor.Name = "mutated"
	honor.Levels[0].Description = "mutated"
	cachedHonor, err := honors.GetByID(ctx, 1)
	if err != nil || cachedHonor.Name != "Top honor" || cachedHonor.Levels[0].Description != "level one" {
		t.Fatalf("cached honor = %+v, %v", cachedHonor, err)
	}
	if _, err := honors.GetByID(ctx, 404); err == nil {
		t.Fatal("missing honor should return an error")
	}
	if _, err := honors.GetByID(ctx, 404); err == nil {
		t.Fatal("tombstoned honor should still return an error")
	}
	if honorQueries.Load() != 2 {
		t.Fatalf("honor cache/tombstone should issue two queries total, got %d", honorQueries.Load())
	}

	if _, err := honors.GetGroupByID(ctx, 0); err == nil {
		t.Fatal("GetGroupByID(0) should reject an invalid group ID")
	}
	normalGroup, err := honors.GetGroupByID(ctx, 10)
	if err != nil || normalGroup.BackgroundAssetBundleName == nil || *normalGroup.BackgroundAssetBundleName != "normal_bg" || normalGroup.FrameName == nil || *normalGroup.FrameName != "normal_frame" {
		t.Fatalf("normal honor group = %+v, %v", normalGroup, err)
	}
	if cached, err := honors.GetGroupByID(ctx, 10); err != nil || *cached.BackgroundAssetBundleName != "normal_bg" {
		t.Fatalf("cached normal group = %+v, %v", cached, err)
	}
	birthdayGroup, err := honors.GetGroupByID(ctx, 11)
	if err != nil || birthdayGroup.BackgroundAssetBundleName == nil || *birthdayGroup.BackgroundAssetBundleName != "honor_bg_birthday_01_05" || birthdayGroup.FrameName == nil || *birthdayGroup.FrameName != "honor_frame_birthday_01_05" {
		t.Fatalf("derived birthday group = %+v, %v", birthdayGroup, err)
	}
	unmatchedGroup, err := honors.GetGroupByID(ctx, 12)
	if err != nil || unmatchedGroup.BackgroundAssetBundleName != nil || unmatchedGroup.FrameName != nil {
		t.Fatalf("unmatched birthday group = %+v, %v", unmatchedGroup, err)
	}
	if _, err := honors.GetGroupByID(ctx, 404); err == nil {
		t.Fatal("missing honor group should return an error")
	}

	if _, err := honors.GetBondsHonorByID(ctx, 0); err == nil {
		t.Fatal("GetBondsHonorByID(0) should reject an invalid ID")
	}
	bonds, err := honors.GetBondsHonorByID(ctx, 20)
	if err != nil || bonds.Name != "Best partners" || bonds.GameCharacterUnitID1 != 501 || !bonds.ConfigurableUnitVirtualSinger {
		t.Fatalf("bonds honor = %+v, %v", bonds, err)
	}
	bonds.Name = "mutated"
	if cached, err := honors.GetBondsHonorByID(ctx, 20); err != nil || cached.Name != "Best partners" {
		t.Fatalf("cached bonds honor = %+v, %v", cached, err)
	}
	if _, err := honors.GetBondsHonorByID(ctx, 404); err == nil {
		t.Fatal("missing bonds honor should return an error")
	}
	if _, err := honors.GetBondsHonorByID(ctx, 404); err == nil {
		t.Fatal("tombstoned bonds honor should still return an error")
	}
	if bondsQueries.Load() != 2 {
		t.Fatalf("bonds cache/tombstone should issue two queries total, got %d", bondsQueries.Load())
	}

	if _, err := honors.GetBondsHonorWordByID(ctx, 0); err == nil {
		t.Fatal("GetBondsHonorWordByID(0) should reject an invalid ID")
	}
	word, err := honors.GetBondsHonorWordByID(ctx, 30)
	if err != nil || word.Name != "Together" {
		t.Fatalf("bonds honor word = %+v, %v", word, err)
	}
	word.Name = "mutated"
	if cached, err := honors.GetBondsHonorWordByID(ctx, 30); err != nil || cached.Name != "Together" {
		t.Fatalf("cached bonds honor word = %+v, %v", cached, err)
	}
	if _, err := honors.GetBondsHonorWordByID(ctx, 404); err == nil {
		t.Fatal("missing bonds honor word should return an error")
	}

	if unit, ok := honors.GetGameCharacterUnitByID(ctx, 0); ok || unit != nil {
		t.Fatalf("invalid game character unit = %+v, %t", unit, ok)
	}
	unit, ok := honors.GetGameCharacterUnitByID(ctx, 501)
	if !ok || unit.Unit != "idol" {
		t.Fatalf("game character unit = %+v, %t", unit, ok)
	}
	unit.Unit = "mutated"
	if cached, ok := honors.GetGameCharacterUnitByID(ctx, 501); !ok || cached.Unit != "idol" {
		t.Fatalf("cached game character unit = %+v, %t", cached, ok)
	}
	if unit, ok := honors.GetGameCharacterUnitByID(ctx, 404); ok || unit != nil {
		t.Fatalf("missing game character unit = %+v, %t", unit, ok)
	}

	if got := honors.GetEventIDByHonorID(ctx, 0); got != 0 {
		t.Fatalf("event ID for invalid honor = %d", got)
	}
	if got := honors.GetEventIDByHonorID(ctx, 1); got != 40 {
		t.Fatalf("event ID for honor 1 = %d, want 40", got)
	}
	if got := honors.GetEventIDByHonorID(ctx, 999); got != 0 {
		t.Fatalf("event ID for missing honor = %d", got)
	}
	if got := honors.GetEventIDByHonorID(ctx, 1); got != 40 {
		t.Fatalf("cached event ID for honor 1 = %d", got)
	}
}

func TestDBHonorProviderErrorAndUnconfiguredBranches(t *testing.T) {
	ctx := context.Background()
	provider := openProviderBehaviorDB(t, "honors_errors")
	client := provider.client

	var honorAttempts atomic.Int32
	client.Honor.Intercept(ent.InterceptFunc(func(ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(context.Context, ent.Query) (ent.Value, error) {
			honorAttempts.Add(1)
			return nil, errors.New("synthetic honor failure")
		})
	}))
	var bondsAttempts atomic.Int32
	client.Bondshonor.Intercept(ent.InterceptFunc(func(ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(context.Context, ent.Query) (ent.Value, error) {
			bondsAttempts.Add(1)
			return nil, errors.New("synthetic bonds failure")
		})
	}))

	for attempt := 0; attempt < 2; attempt++ {
		if _, err := provider.honors.GetByID(ctx, 1); err == nil || !strings.Contains(err.Error(), "synthetic honor failure") {
			t.Fatalf("transient honor error = %v", err)
		}
		if _, err := provider.honors.GetBondsHonorByID(ctx, 1); err == nil || !strings.Contains(err.Error(), "synthetic bonds failure") {
			t.Fatalf("transient bonds error = %v", err)
		}
	}
	if honorAttempts.Load() != 2 || bondsAttempts.Load() != 2 {
		t.Fatalf("transient errors should not be tombstoned: honor=%d bonds=%d", honorAttempts.Load(), bondsAttempts.Load())
	}
	if len(provider.honors.honorMissing) != 0 || len(provider.honors.bondsMissing) != 0 {
		t.Fatalf("transient errors populated tombstones: honor=%v bonds=%v", provider.honors.honorMissing, provider.honors.bondsMissing)
	}

	unconfigured := &dbHonorProvider{client: client, region: renderregion.JP}
	if _, err := unconfigured.GetBondsHonorWordByID(ctx, 1); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("unconfigured bonds words error = %v", err)
	}
	badStoreRoot := t.TempDir()
	writeTestFile(t, badStoreRoot, "bondsHonorWords.json", `{not-json`)
	badStore := &dbHonorProvider{client: client, region: renderregion.JP, store: newLocalStore(badStoreRoot)}
	if _, err := badStore.GetBondsHonorWordByID(ctx, 1); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("invalid bonds words error = %v", err)
	}

	if _, err := convertCloudHonor(&sekaiDB.Honor{Levels: json.RawMessage(`{not-json`)}); err == nil {
		t.Fatal("invalid honor levels should fail conversion")
	}

	closed := openProviderBehaviorDB(t, "honors_closed")
	if err := closed.client.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}
	if got := closed.honors.GetEventIDByHonorID(ctx, 1); got != 0 {
		t.Fatalf("event lookup on closed database = %d, want 0", got)
	}
	if _, ok := closed.honors.deriveBirthdayAssetsForGroup(ctx, 1, "Minori birthday"); ok {
		t.Fatal("birthday assets should not resolve when character query fails")
	}
}

func TestHonorBirthdayGroupMatchesCharacter(t *testing.T) {
	row := &sekaiDB.Gamecharacter{
		GameID:           5,
		FirstName:        "花里",
		GivenName:        "实乃理",
		FirstNameEnglish: "Hanasato",
		GivenNameEnglish: "Minori",
	}
	tests := []struct {
		name      string
		groupName string
		row       *sekaiDB.Gamecharacter
		want      bool
	}{
		{name: "nil row", groupName: "Minori", row: nil, want: false},
		{name: "blank group", groupName: " ", row: row, want: false},
		{name: "given name", groupName: "实乃理生日", row: row, want: true},
		{name: "full English name", groupName: "HanasatoMinori Birthday", row: row, want: true},
		{name: "default nickname", groupName: "mnr birthday", row: row, want: true},
		{name: "unmatched", groupName: "Someone else", row: row, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := honorBirthdayGroupMatchesCharacter(test.groupName, test.row); got != test.want {
				t.Fatalf("honorBirthdayGroupMatchesCharacter(%q) = %t, want %t", test.groupName, got, test.want)
			}
		})
	}
}
