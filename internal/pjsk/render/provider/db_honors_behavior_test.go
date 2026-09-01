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
	"haruki-cloud/internal/testutil"
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
	{

		_, err := client.Honor.Create().
			SetGameID(1).
			SetGroupID(10).
			SetHonorRarity("high").
			SetName("Top honor").
			SetAssetbundleName("honor_1").
			SetLevels(json.RawMessage(`[{"Level":1,"HonorRarity":"high","Description":"level one","AssetBundleName":"honor_1_1"}]`)).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create honor: %v", err)
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
		{
			_, err := builder.Save(ctx)
			testutil.Require(t, !(err != nil), "create honor group %d: %v", item.id, err)
		}

	}
	{
		_, err := client.Bondshonor.Create().
			SetGameID(20).
			SetBondsGroupID(200).
			SetGameCharacterUnitId1(501).
			SetGameCharacterUnitId2(502).
			SetHonorRarity("middle").
			SetName("Best partners").
			SetDescription("bonds description").
			SetConfigurableUnitVirtualSinger(true).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create bonds honor: %v", err)
	}
	{

		_, err := client.Gamecharacter.Create().
			SetGameID(5).
			SetFirstName("花里").
			SetGivenName("实乃理").
			SetFirstNameEnglish("Hanasato").
			SetGivenNameEnglish("Minori").
			SetServerRegion(renderregion.JP.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create game character: %v", err)
	}
	{

		_, err := client.Gamecharacterunit.Create().
			SetGameID(501).
			SetGameCharacterID(5).
			SetUnit("idol").
			SetColorCode("#abcdef").
			SetServerRegion(renderregion.JP.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create game character unit: %v", err)
	}
	{

		_, err := client.Event.Create().
			SetGameID(40).
			SetEventRankingRewardRanges(json.RawMessage(`[
			{"eventRankingRewardDetails":[
				{"resourceType":"honor","resourceId":1},
				{"resourceType":"jewel","resourceId":2},
				{"resourceType":"honor","resourceId":0}
			]}
		]`)).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create event honor mapping: %v", err)
	}
	{

		_, err := client.Event.Create().
			SetGameID(41).
			SetEventRankingRewardRanges(json.RawMessage(`[]`)).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create empty event honor mapping: %v", err)
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
	{
		_, err := honors.GetByID(ctx, 0)
		testutil.RequireArgs(t, !(err == nil), "GetByID(0) should reject an invalid honor ID")
	}

	honor, err := honors.GetByID(ctx, 1)
	{
		testutil.Require(t, !(err != nil), "GetByID(1) = %+v, %v", honor, err)
		testutil.Require(t, !(honor.Name != "Top honor"), "GetByID(1) = %+v, %v", honor, err)
		testutil.Require(t, !(len(honor.Levels) != 1), "GetByID(1) = %+v, %v", honor, err)
		testutil.Require(t, !(honor.Levels[0].Level != 1), "GetByID(1) = %+v, %v", honor, err)
	}

	honor.Name = "mutated"
	honor.Levels[0].Description = "mutated"
	cachedHonor, err := honors.GetByID(ctx, 1)
	{
		testutil.Require(t, !(err != nil), "cached honor = %+v, %v", cachedHonor, err)
		testutil.Require(t, !(cachedHonor.Name != "Top honor"), "cached honor = %+v, %v", cachedHonor, err)
		testutil.Require(t, !(cachedHonor.Levels[0].Description != "level one"), "cached honor = %+v, %v", cachedHonor, err)
	}
	{

		_, err := honors.GetByID(ctx, 404)
		testutil.RequireArgs(t, !(err == nil), "missing honor should return an error")
	}
	{

		_, err := honors.GetByID(ctx, 404)
		testutil.RequireArgs(t, !(err == nil), "tombstoned honor should still return an error")
	}

	testutil.Require(t, !(honorQueries.Load() != 2), "honor cache/tombstone should issue two queries total, got %d", honorQueries.Load())
	{

		_, err := honors.GetGroupByID(ctx, 0)
		testutil.RequireArgs(t, !(err == nil), "GetGroupByID(0) should reject an invalid group ID")
	}

	normalGroup, err := honors.GetGroupByID(ctx, 10)
	{
		testutil.Require(t, !(err != nil), "normal honor group = %+v, %v", normalGroup, err)
		testutil.Require(t, !(normalGroup.BackgroundAssetBundleName == nil), "normal honor group = %+v, %v", normalGroup, err)
		testutil.Require(t, !(*normalGroup.BackgroundAssetBundleName != "normal_bg"), "normal honor group = %+v, %v", normalGroup, err)
		testutil.Require(t, !(normalGroup.FrameName == nil), "normal honor group = %+v, %v", normalGroup, err)
		testutil.Require(t, !(*normalGroup.FrameName != "normal_frame"), "normal honor group = %+v, %v", normalGroup, err)
	}
	{

		cached, err := honors.GetGroupByID(ctx, 10)
		{
			testutil.Require(t, !(err != nil), "cached normal group = %+v, %v", cached, err)
			testutil.Require(t, !(*cached.BackgroundAssetBundleName != "normal_bg"), "cached normal group = %+v, %v", cached, err)
		}
	}

	birthdayGroup, err := honors.GetGroupByID(ctx, 11)
	{
		testutil.Require(t, !(err != nil), "derived birthday group = %+v, %v", birthdayGroup, err)
		testutil.Require(t, !(birthdayGroup.BackgroundAssetBundleName == nil), "derived birthday group = %+v, %v", birthdayGroup, err)
		testutil.Require(t, !(*birthdayGroup.BackgroundAssetBundleName != "honor_bg_birthday_01_05"), "derived birthday group = %+v, %v", birthdayGroup, err)
		testutil.Require(t, !(birthdayGroup.FrameName == nil), "derived birthday group = %+v, %v", birthdayGroup, err)
		testutil.Require(t, !(*birthdayGroup.FrameName != "honor_frame_birthday_01_05"), "derived birthday group = %+v, %v", birthdayGroup, err)
	}

	unmatchedGroup, err := honors.GetGroupByID(ctx, 12)
	{
		testutil.Require(t, !(err != nil), "unmatched birthday group = %+v, %v", unmatchedGroup, err)
		testutil.Require(t, !(unmatchedGroup.BackgroundAssetBundleName != nil), "unmatched birthday group = %+v, %v", unmatchedGroup, err)
		testutil.Require(t, !(unmatchedGroup.FrameName != nil), "unmatched birthday group = %+v, %v", unmatchedGroup, err)
	}
	{

		_, err := honors.GetGroupByID(ctx, 404)
		testutil.RequireArgs(t, !(err == nil), "missing honor group should return an error")
	}
	{

		_, err := honors.GetBondsHonorByID(ctx, 0)
		testutil.RequireArgs(t, !(err == nil), "GetBondsHonorByID(0) should reject an invalid ID")
	}

	bonds, err := honors.GetBondsHonorByID(ctx, 20)
	{
		testutil.Require(t, !(err != nil), "bonds honor = %+v, %v", bonds, err)
		testutil.Require(t, !(bonds.Name != "Best partners"), "bonds honor = %+v, %v", bonds, err)
		testutil.Require(t, !(bonds.GameCharacterUnitID1 != 501), "bonds honor = %+v, %v", bonds, err)
		testutil.Require(t, bonds.ConfigurableUnitVirtualSinger, "bonds honor = %+v, %v", bonds, err)
	}

	bonds.Name = "mutated"
	{
		cached, err := honors.GetBondsHonorByID(ctx, 20)
		{
			testutil.Require(t, !(err != nil), "cached bonds honor = %+v, %v", cached, err)
			testutil.Require(t, !(cached.Name != "Best partners"), "cached bonds honor = %+v, %v", cached, err)
		}
	}
	{

		_, err := honors.GetBondsHonorByID(ctx, 404)
		testutil.RequireArgs(t, !(err == nil), "missing bonds honor should return an error")
	}
	{

		_, err := honors.GetBondsHonorByID(ctx, 404)
		testutil.RequireArgs(t, !(err == nil), "tombstoned bonds honor should still return an error")
	}

	testutil.Require(t, !(bondsQueries.Load() != 2), "bonds cache/tombstone should issue two queries total, got %d", bondsQueries.Load())
	{

		_, err := honors.GetBondsHonorWordByID(ctx, 0)
		testutil.RequireArgs(t, !(err == nil), "GetBondsHonorWordByID(0) should reject an invalid ID")
	}

	word, err := honors.GetBondsHonorWordByID(ctx, 30)
	{
		testutil.Require(t, !(err != nil), "bonds honor word = %+v, %v", word, err)
		testutil.Require(t, !(word.Name != "Together"), "bonds honor word = %+v, %v", word, err)
	}

	word.Name = "mutated"
	{
		cached, err := honors.GetBondsHonorWordByID(ctx, 30)
		{
			testutil.Require(t, !(err != nil), "cached bonds honor word = %+v, %v", cached, err)
			testutil.Require(t, !(cached.Name != "Together"), "cached bonds honor word = %+v, %v", cached, err)
		}
	}
	{

		_, err := honors.GetBondsHonorWordByID(ctx, 404)
		testutil.RequireArgs(t, !(err == nil), "missing bonds honor word should return an error")
	}
	{

		unit, ok := honors.GetGameCharacterUnitByID(ctx, 0)
		{
			testutil.Require(t, !(ok), "invalid game character unit = %+v, %t", unit, ok)
			testutil.Require(t, !(unit != nil), "invalid game character unit = %+v, %t", unit, ok)
		}
	}

	unit, ok := honors.GetGameCharacterUnitByID(ctx, 501)
	{
		testutil.Require(t, ok, "game character unit = %+v, %t", unit, ok)
		testutil.Require(t, !(unit.Unit != "idol"), "game character unit = %+v, %t", unit, ok)
	}

	unit.Unit = "mutated"
	{
		cached, ok := honors.GetGameCharacterUnitByID(ctx, 501)
		{
			testutil.Require(t, ok, "cached game character unit = %+v, %t", cached, ok)
			testutil.Require(t, !(cached.Unit != "idol"), "cached game character unit = %+v, %t", cached, ok)
		}
	}
	{

		unit, ok := honors.GetGameCharacterUnitByID(ctx, 404)
		{
			testutil.Require(t, !(ok), "missing game character unit = %+v, %t", unit, ok)
			testutil.Require(t, !(unit != nil), "missing game character unit = %+v, %t", unit, ok)
		}
	}
	{

		got := honors.GetEventIDByHonorID(ctx, 0)
		testutil.Require(t, !(got != 0), "event ID for invalid honor = %d", got)
	}
	{

		got := honors.GetEventIDByHonorID(ctx, 1)
		testutil.Require(t, !(got != 40), "event ID for honor 1 = %d, want 40", got)
	}
	{

		got := honors.GetEventIDByHonorID(ctx, 999)
		testutil.Require(t, !(got != 0), "event ID for missing honor = %d", got)
	}
	{

		got := honors.GetEventIDByHonorID(ctx, 1)
		testutil.Require(t, !(got != 40), "cached event ID for honor 1 = %d", got)
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
		{
			_, err := provider.honors.GetByID(ctx, 1)
			{
				testutil.Require(t, !(err == nil), "transient honor error = %v", err)
				testutil.Require(t, strings.Contains(err.Error(), "synthetic honor failure"), "transient honor error = %v", err)
			}
		}
		{

			_, err := provider.honors.GetBondsHonorByID(ctx, 1)
			{
				testutil.Require(t, !(err == nil), "transient bonds error = %v", err)
				testutil.Require(t, strings.Contains(err.Error(), "synthetic bonds failure"), "transient bonds error = %v", err)
			}
		}

	}
	{
		testutil.Require(t, !(honorAttempts.Load() != 2), "transient errors should not be tombstoned: honor=%d bonds=%d", honorAttempts.Load(), bondsAttempts.Load())
		testutil.Require(t, !(bondsAttempts.Load() != 2), "transient errors should not be tombstoned: honor=%d bonds=%d", honorAttempts.Load(), bondsAttempts.Load())
	}
	{
		testutil.Require(t, !(len(provider.honors.honorMissing) != 0), "transient errors populated tombstones: honor=%v bonds=%v", provider.honors.honorMissing, provider.honors.bondsMissing)
		testutil.Require(t, !(len(provider.honors.bondsMissing) != 0), "transient errors populated tombstones: honor=%v bonds=%v", provider.honors.honorMissing, provider.honors.bondsMissing)
	}

	unconfigured := &dbHonorProvider{client: client, region: renderregion.JP}
	{
		_, err := unconfigured.GetBondsHonorWordByID(ctx, 1)
		{
			testutil.Require(t, !(err == nil), "unconfigured bonds words error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "not configured"), "unconfigured bonds words error = %v", err)
		}
	}

	badStoreRoot := t.TempDir()
	writeTestFile(t, badStoreRoot, "bondsHonorWords.json", `{not-json`)
	badStore := &dbHonorProvider{client: client, region: renderregion.JP, store: newLocalStore(badStoreRoot)}
	{
		_, err := badStore.GetBondsHonorWordByID(ctx, 1)
		{
			testutil.Require(t, !(err == nil), "invalid bonds words error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "not configured"), "invalid bonds words error = %v", err)
		}
	}
	{

		_, err := convertCloudHonor(&sekaiDB.Honor{Levels: json.RawMessage(`{not-json`)})
		testutil.RequireArgs(t, !(err == nil), "invalid honor levels should fail conversion")
	}

	closed := openProviderBehaviorDB(t, "honors_closed")
	{
		err := closed.client.Close()
		testutil.Require(t, !(err != nil), "close fixture database: %v", err)
	}
	{

		got := closed.honors.GetEventIDByHonorID(ctx, 1)
		testutil.Require(t, !(got != 0), "event lookup on closed database = %d, want 0", got)
	}
	{

		_, ok := closed.honors.deriveBirthdayAssetsForGroup(ctx, 1, "Minori birthday")
		testutil.RequireArgs(t, !(ok), "birthday assets should not resolve when character query fails")
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
			{
				got := honorBirthdayGroupMatchesCharacter(test.groupName, test.row)
				testutil.Require(t, !(got != test.want), "honorBirthdayGroupMatchesCharacter(%q) = %t, want %t", test.groupName, got, test.want)
			}

		})
	}
}
