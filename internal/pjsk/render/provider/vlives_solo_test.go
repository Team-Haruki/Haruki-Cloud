package provider

import (
	"context"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/testutil"
)

const soloVLiveJSON = `[
 {"id":480,"name":"normal","virtualLiveType":"normal","startAt":1,"endAt":2},
 {"id":491,"name":"solo","virtualLiveType":"solo_virtual_live","virtualLiveGroupId":2,"startAt":1,"endAt":2,
  "virtualLiveTotalCheerPointRewards":[{"id":1,"virtualLiveId":491,"threshold":300,"resourceBoxId":101001},{"id":2,"threshold":0,"resourceBoxId":5}],
  "virtualLiveTotalCheerPointSurplusReward":{"id":1,"virtualLiveId":491,"basePoint":10,"resourceBoxId":1},
  "virtualLiveVirtualItemOverrideCost":{"id":1,"virtualLiveId":491,"costResourceType":"material","costResourceId":282,"assetbundleName":"virtual_cheer_coin"}}
]`

func TestLocalVLivesDecodeSoloFieldsAndGroups(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "virtualLives.json", soloVLiveJSON)
	writeTestFile(t, root, "virtualLiveGroups.json", `[{"id":2,"name":"Solo Group","virtualLiveGroupType":"solo_virtual_live","assetbundleName":"g2","startAt":10,"endAt":20}]`)
	p := &localVLiveProvider{store: newLocalStore(root)}

	lives, err := p.GetLives(context.Background(), renderregion.JP)
	testutil.Require(t, err == nil && len(lives) == 2, "lives = %+v err=%v", lives, err)
	normal, solo := lives[0], lives[1]
	testutil.Require(t, normal.VirtualLiveType == "normal" && normal.TotalCheerPointRewards == nil && normal.VirtualItemOverrideCost == nil, "normal live = %+v", normal)
	testutil.Require(t, solo.VirtualLiveGroupID == 2 && len(solo.TotalCheerPointRewards) == 1 && solo.TotalCheerPointRewards[0] == VLiveTotalCheerPointReward{Threshold: 300, ResourceBoxID: 101001}, "solo rewards = %+v", solo.TotalCheerPointRewards)
	testutil.Require(t, solo.TotalCheerPointSurplusReward != nil && *solo.TotalCheerPointSurplusReward == VLiveSurplusReward{BasePoint: 10, ResourceBoxID: 1}, "surplus = %+v", solo.TotalCheerPointSurplusReward)
	testutil.Require(t, solo.VirtualItemOverrideCost != nil && solo.VirtualItemOverrideCost.CostResourceID == 282 && solo.VirtualItemOverrideCost.CostResourceType == "material", "override = %+v", solo.VirtualItemOverrideCost)

	groups, ok := p.GetGroups(context.Background(), renderregion.JP)
	testutil.Require(t, ok && groups[2] != nil && groups[2].Name == "Solo Group" && groups[2].VirtualLiveGroupType == "solo_virtual_live" && groups[2].EndAt == 20, "groups = %+v ok=%v", groups, ok)
}

func TestLocalVLiveGroupsMissingFile(t *testing.T) {
	p := &localVLiveProvider{store: newLocalStore(t.TempDir())}
	_, ok := p.GetGroups(context.Background(), renderregion.EN)
	testutil.Require(t, !ok, "missing virtualLiveGroups.json must report unserved")
}

type fakeMasterRows map[string]map[int]map[string]any

func (f fakeMasterRows) LoadMasterRows(_ context.Context, filename string) (map[int]map[string]any, bool) {
	rows, ok := f[filename]
	return rows, ok
}

func TestDBVLiveSoloFieldsComeFromRawRows(t *testing.T) {
	lives := []*VLive{{ID: 480, VirtualLiveType: "normal"}, {ID: 491, VirtualLiveType: VLiveTypeSolo, VirtualLiveGroupID: 2}}
	p := &dbVLiveProvider{rows: fakeMasterRows{soloVirtualLivesFile: {491: {
		"virtualLiveTotalCheerPointRewards":       []any{map[string]any{"threshold": int64(300), "resourceBoxId": int64(101001)}},
		"virtualLiveTotalCheerPointSurplusReward": map[string]any{"basePoint": int64(10), "resourceBoxId": int64(1)},
	}}}}
	p.applySoloFields(context.Background(), lives)
	testutil.Require(t, lives[0].TotalCheerPointRewards == nil, "normal live got solo fields: %+v", lives[0])
	testutil.Require(t, len(lives[1].TotalCheerPointRewards) == 1 && lives[1].TotalCheerPointSurplusReward != nil && lives[1].VirtualItemOverrideCost == nil, "solo live = %+v", lives[1])

	// Rows without the new columns (not ingested yet) leave the live bare.
	bare := []*VLive{{ID: 491, VirtualLiveType: VLiveTypeSolo}}
	(&dbVLiveProvider{rows: fakeMasterRows{soloVirtualLivesFile: {491: {"name": "solo"}}}}).applySoloFields(context.Background(), bare)
	testutil.Require(t, bare[0].TotalCheerPointRewards == nil && bare[0].TotalCheerPointSurplusReward == nil, "bare solo live = %+v", bare[0])

	// A region without solo lives never reads the raw rows.
	old := []*VLive{{ID: 1, VirtualLiveType: "normal"}}
	(&dbVLiveProvider{rows: fakeMasterRows{}}).applySoloFields(context.Background(), old)
	_, ok := (&dbVLiveProvider{rows: fakeMasterRows{}}).GetGroups(context.Background(), renderregion.EN)
	testutil.Require(t, !ok, "missing groups table must report unserved")
}

func TestMasterRowsServeSoloVirtualLivesSlice(t *testing.T) {
	ctx := context.Background()
	p := openMasterRowsProvider(t, "solo_vlives")
	client := p.client
	_, err := client.Virtuallive.Create().SetGameID(480).SetVirtualLiveType("normal").SetName("normal").SetServerRegion("jp").Save(ctx)
	testutil.Require(t, err == nil, "create normal live: %v", err)
	_, err = client.Virtuallive.Create().SetGameID(491).SetVirtualLiveType(VLiveTypeSolo).SetVirtualLiveGroupID(2).SetName("solo").SetServerRegion("jp").Save(ctx)
	testutil.Require(t, err == nil, "create solo live: %v", err)

	// The cheer-point columns do not exist in this schema yet: SELECT * still
	// serves the solo rows, without those keys.
	rows, ok := p.mysekai.LoadMasterRows(ctx, soloVirtualLivesFile)
	testutil.Require(t, ok && len(rows) == 1 && rows[491]["name"] == "solo", "solo rows = %#v ok=%v", rows, ok)
	lives, err := p.vlives.GetLives(ctx, renderregion.JP)
	testutil.Require(t, err == nil && len(lives) == 2, "lives = %+v err=%v", lives, err)
	for _, live := range lives {
		testutil.Require(t, live.TotalCheerPointRewards == nil, "live %d got cheer rewards", live.ID)
		if live.ID == 491 {
			testutil.Require(t, live.VirtualLiveType == VLiveTypeSolo && live.VirtualLiveGroupID == 2, "solo live = %+v", live)
		}
	}
	_, ok = p.vlives.GetGroups(ctx, renderregion.JP)
	testutil.Require(t, !ok, "virtuallivegroups table is absent from this schema: unserved")
}
