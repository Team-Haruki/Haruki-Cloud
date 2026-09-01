package inventory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/testutil"
)

func TestBuildListRequestFromSnapshotGroupsInventoryItems(t *testing.T) {
	dir := t.TempDir()
	masterDir := filepath.Join(dir, "haruki-sekai-sc-master", "master")
	{
		err := os.MkdirAll(masterDir, 0o755)
		testutil.Require(t, !(err != nil), "MkdirAll() error = %v", err)
	}
	{

		err := os.WriteFile(filepath.Join(masterDir, "materials.json"), []byte(`[
		{"id":5,"seq":50,"materialType":"music","name":"音乐卡","flavorText":"用于解锁歌曲。"},
		{"id":11,"seq":11,"materialType":"special_training","name":"中级魔法布","flavorText":"用于特训。"}
	]`), 0o644)
		testutil.Require(t, !(err != nil), "WriteFile(materials) error = %v", err)
	}
	{

		err := os.WriteFile(filepath.Join(masterDir, "boostItems.json"), []byte(`[
		{"id":2,"seq":2,"name":"演出能量饮料（大）","recoveryValue":10,"flavorText":"恢复10演出能量。"}
	]`), 0o644)
		testutil.Require(t, !(err != nil), "WriteFile(boostItems) error = %v", err)
	}
	{

		err := os.WriteFile(filepath.Join(masterDir, "eventItems.json"), []byte(`[
		{"id":7,"eventId":70,"name":"活动徽章","assetbundleName":"badge_cute"}
	]`), 0o644)
		testutil.Require(t, !(err != nil), "WriteFile(eventItems) error = %v", err)
	}
	{

		err := os.WriteFile(filepath.Join(masterDir, "gachaTickets.json"), []byte(`[
		{"id":8,"seq":8,"name":"招募券","assetbundleName":"gacha_ticket"}
	]`), 0o644)
		testutil.Require(t, !(err != nil), "WriteFile(gachaTickets) error = %v", err)
	}
	{

		err := os.WriteFile(filepath.Join(masterDir, "practiceTickets.json"), []byte(`[
		{"id":10101,"characterId":1,"exp":1000,"name":"练习乐谱（星乃一歌）","flavorText":"成员获得1000经验值。"}
	]`), 0o644)
		testutil.Require(t, !(err != nil), "WriteFile(practiceTickets) error = %v", err)
	}
	{

		err := os.WriteFile(filepath.Join(masterDir, "skillPracticeTickets.json"), []byte(`[
		{"id":10201,"characterId":2,"exp":1000,"name":"技能升级乐谱（天马咲希）"}
	]`), 0o644)
		testutil.Require(t, !(err != nil), "WriteFile(skillPracticeTickets) error = %v", err)
	}
	{

		err := os.WriteFile(filepath.Join(masterDir, "gachaCeilItems.json"), []byte(`[
		{"id":9,"seq":9,"name":"招募贴纸","assetbundleName":"ceil_item"}
	]`), 0o644)
		testutil.Require(t, !(err != nil), "WriteFile(gachaCeilItems) error = %v", err)
	}
	{

		err := os.WriteFile(filepath.Join(masterDir, "mysekaiMaterials.json"), []byte(`[
		{"id":6,"seq":6,"mysekaiMaterialType":"mineral","name":"心愿石块","description":"这是心愿形成的石块。","iconAssetbundleName":"item_mineral_1"},
		{"id":35,"seq":601,"mysekaiMaterialType":"game_character","name":"蓝色记忆","description":"角色的记忆碎片。","iconAssetbundleName":"item_memoria_1"}
	]`), 0o644)
		testutil.Require(t, !(err != nil), "WriteFile(mysekaiMaterials) error = %v", err)
	}

	profile := &drawing.DetailedProfileCardRequest{
		ID:              "123456789",
		Region:          "cn",
		Nickname:        "tester",
		Source:          "suite",
		UpdateTime:      1700000000000,
		LeaderImagePath: "asset/jp-assets/startapp/thumbnail/chara/chr_ts_01.png",
	}
	raw := &snapshot.RawUserData{
		UserGamedata:        snapshot.RawUserGamedata{Coin: 123456, VirtualCoin: 77},
		UserChargedCurrency: snapshot.RawUserChargedCurrency{Free: 300, Paid: 20},
		UserMaterials: []snapshot.RawUserMaterial{
			{MaterialID: 11, Quantity: 8},
			{MaterialID: 5, Quantity: 3},
			{MaterialID: 99, Quantity: 0},
		},
		UserBoostItems: []snapshot.RawUserBoostItem{
			{BoostItemID: 2, Quantity: 4},
		},
		UserEventItems:           []snapshot.RawUserEventItem{{EventItemID: 7, Quantity: 9}},
		UserGachaTickets:         []snapshot.RawUserGachaTicket{{GachaTicketID: 8, Quantity: 1}},
		UserPracticeTickets:      []snapshot.RawUserPracticeTicket{{PracticeTicketID: 10101, Quantity: 2}},
		UserSkillPracticeTickets: []snapshot.RawUserSkillPracticeTicket{{SkillPracticeTicketID: 10201, Quantity: 3}},
		UserGachaCeilItems:       []snapshot.RawUserGachaCeilItem{{GachaCeilItemID: 9, Quantity: 4}},
		UserMysekaiMaterials: []snapshot.RawUserMysekaiMaterial{
			{MysekaiMaterialID: 6, Quantity: 5},
			{MysekaiMaterialID: 35, Quantity: 6},
		},
	}
	ctrl := NewController(nil, nil, nil, renderregion.CN, MasterdataOptions{LocalDir: dir})

	req, err := ctrl.BuildListRequestFromSnapshot(Query{
		Region:   renderregion.CN,
		Profile:  profile,
		Snapshot: &inventorySnapshotStub{raw: raw, profile: profile},
	})
	testutil.Require(t, !(err != nil), "BuildListRequestFromSnapshot() error = %v", err)
	testutil.Require(t, !(req.Profile.Nickname != "tester"), "Profile.Nickname = %q, want tester", req.Profile.Nickname)
	testutil.Require(t, !(req.TotalItems != 8), "TotalItems = %d, want 8", req.TotalItems)
	{

		got := findInventoryItem(t, req.Sections, "currency", 0)
		{
			testutil.Require(t, !(got.Quantity != 123456), "coin item = %+v", got)
			testutil.Require(t, !(got.Name != "金币"), "coin item = %+v", got)
		}
	}

	assertInventoryItemMissing(t, req.Sections, "currency", -1)
	assertInventoryItemMissing(t, req.Sections, "currency", -2)
	{
		got := findInventoryItem(t, req.Sections, "currency", -3)
		{
			testutil.Require(t, !(got.Quantity != 77), "virtual coin item = %+v", got)
			testutil.Require(t, !(got.Name != "虚拟币"), "virtual coin item = %+v", got)
		}
	}
	{

		got := findInventoryItem(t, req.Sections, "training", 11)
		{
			testutil.Require(t, !(got.Name != "中级魔法布"), "training material = %+v", got)
			testutil.Require(t, !(got.Quantity != 8), "training material = %+v", got)
			testutil.Require(t, !(got.Description != "用于特训。"), "training material = %+v", got)
		}
	}
	{

		got := findInventoryItem(t, req.Sections, "music", 5)
		{
			testutil.Require(t, !(got.Name != "音乐卡"), "music material = %+v", got)
			testutil.Require(t, !(got.Quantity != 3), "music material = %+v", got)
			testutil.Require(t, !(got.Description != "用于解锁歌曲。"), "music material = %+v", got)
		}
	}

	assertInventoryItemMissing(t, req.Sections, "boost", 2)
	assertInventoryItemMissing(t, req.Sections, "event", 7)
	{
		got := findInventoryItem(t, req.Sections, "tickets", 8)
		{
			testutil.Require(t, !(got.Name != "招募券"), "gacha ticket = %+v", got)
			testutil.Require(t, !(got.Quantity != 1), "gacha ticket = %+v", got)
		}
	}
	{

		got := findInventoryItem(t, req.Sections, "tickets", 9)
		{
			testutil.Require(t, !(got.Name != "招募贴纸"), "gacha ceil item = %+v", got)
			testutil.Require(t, !(got.Quantity != 4), "gacha ceil item = %+v", got)
		}
	}

	if got := findInventoryItem(t, req.Sections, "training", 10101); got.Name != "练习乐谱（星乃一歌）" || got.Quantity != 2 || got.Description != "成员获得1000经验值。" {
		t.Fatalf("practice ticket = %+v", got)
	} else {
		testutil.Require(t, strings.Contains(got.IconPath, "thumbnail/practice_ticket/ticket10101.png"), "practice ticket icon path = %q", got.IconPath)
	}

	if got := findInventoryItem(t, req.Sections, "training", 10201); got.Name != "技能升级乐谱（天马咲希）" || got.Quantity != 3 {
		t.Fatalf("skill practice ticket = %+v", got)
	} else {
		testutil.Require(t, strings.Contains(got.IconPath, "thumbnail/skill_practice_ticket/ticket10201.png"), "skill practice ticket icon path = %q", got.IconPath)
	}

	assertInventoryItemMissing(t, req.Sections, "mysekai", 6)
	assertInventoryItemMissing(t, req.Sections, "memory", 35)
}

func TestBuildListRequestFromSnapshotUsesDefaultRegion(t *testing.T) {
	dir := t.TempDir()
	cnMasterDir := filepath.Join(dir, "haruki-sekai-sc-master", "master")
	jpMasterDir := filepath.Join(dir, "haruki-sekai-master", "master")
	for _, masterDir := range []string{cnMasterDir, jpMasterDir} {
		{
			err := os.MkdirAll(masterDir, 0o755)
			testutil.Require(t, !(err != nil), "MkdirAll(%s) error = %v", masterDir, err)
		}

	}
	{
		err := os.WriteFile(filepath.Join(cnMasterDir, "materials.json"), []byte(`[
		{"id":5,"seq":5,"materialType":"music","name":"国服音乐卡"}
	]`), 0o644)
		testutil.Require(t, !(err != nil), "WriteFile(cn materials) error = %v", err)
	}
	{

		err := os.WriteFile(filepath.Join(jpMasterDir, "materials.json"), []byte(`[
		{"id":5,"seq":5,"materialType":"music","name":"日服音乐卡"}
	]`), 0o644)
		testutil.Require(t, !(err != nil), "WriteFile(jp materials) error = %v", err)
	}

	profile := &drawing.DetailedProfileCardRequest{
		ID:              "123456789",
		Region:          "cn",
		Nickname:        "tester",
		Source:          "suite",
		UpdateTime:      1700000000000,
		LeaderImagePath: "asset/jp-assets/startapp/thumbnail/chara/chr_ts_01.png",
	}
	raw := &snapshot.RawUserData{
		UserMaterials: []snapshot.RawUserMaterial{{MaterialID: 5, Quantity: 1}},
	}
	ctrl := NewController(nil, nil, nil, renderregion.CN, MasterdataOptions{LocalDir: dir})

	req, err := ctrl.BuildListRequestFromSnapshot(Query{
		Profile:  profile,
		Snapshot: &inventorySnapshotStub{raw: raw, profile: profile},
	})
	testutil.Require(t, !(err != nil), "BuildListRequestFromSnapshot() error = %v", err)
	{

		got := findInventoryItem(t, req.Sections, "music", 5)
		testutil.Require(t, !(got.Name != "国服音乐卡"), "material name = %q, want 国服音乐卡", got.Name)
	}

}

func TestBuildListRequestFromSnapshotSkipsEventItemsForAllRegions(t *testing.T) {
	dir := t.TempDir()
	masterDir := filepath.Join(dir, "haruki-sekai-master", "master")
	{
		err := os.MkdirAll(masterDir, 0o755)
		testutil.Require(t, !(err != nil), "MkdirAll() error = %v", err)
	}
	{

		err := os.WriteFile(filepath.Join(masterDir, "eventItems.json"), []byte(`[
		{"id":7,"eventId":70,"name":"Event Token","assetbundleName":"event_token","flavorText":"Event exchange item."}
	]`), 0o644)
		testutil.Require(t, !(err != nil), "WriteFile(eventItems) error = %v", err)
	}

	profile := &drawing.DetailedProfileCardRequest{
		ID:              "123456789",
		Region:          "jp",
		Nickname:        "tester",
		Source:          "suite",
		UpdateTime:      1700000000000,
		LeaderImagePath: "asset/jp-assets/startapp/thumbnail/chara/chr_ts_01.png",
	}
	raw := &snapshot.RawUserData{
		UserGamedata:   snapshot.RawUserGamedata{Coin: 10},
		UserEventItems: []snapshot.RawUserEventItem{{EventItemID: 7, Quantity: 9}},
	}
	ctrl := NewController(nil, nil, nil, renderregion.JP, MasterdataOptions{LocalDir: dir})

	req, err := ctrl.BuildListRequestFromSnapshot(Query{
		Region:   renderregion.JP,
		Profile:  profile,
		Snapshot: &inventorySnapshotStub{raw: raw, profile: profile},
	})
	testutil.Require(t, !(err != nil), "BuildListRequestFromSnapshot() error = %v", err)
	testutil.Require(t, !(req.TotalItems != 1), "TotalItems = %d, want only coin", req.TotalItems)

	assertInventoryItemMissing(t, req.Sections, "event", 7)
}

func TestBuildListRequestFromSnapshotUsesDedicatedTicketAssetDirectories(t *testing.T) {
	dir := t.TempDir()
	masterDir := filepath.Join(dir, "haruki-sekai-master", "master")
	{
		err := os.MkdirAll(masterDir, 0o755)
		testutil.Require(t, !(err != nil), "MkdirAll(master) error = %v", err)
	}
	{

		err := os.WriteFile(filepath.Join(masterDir, "gachaTickets.json"), []byte(`[
		{"id":8,"seq":8,"name":"Mission Gacha Ticket","assetbundleName":"mission_ticket"}
	]`), 0o644)
		testutil.Require(t, !(err != nil), "WriteFile(gachaTickets) error = %v", err)
	}
	{

		err := os.WriteFile(filepath.Join(masterDir, "gachaCeilItems.json"), []byte(`[
		{"id":9,"seq":9,"name":"Limited Gacha Sticker","assetbundleName":"ceil_item_limited"}
	]`), 0o644)
		testutil.Require(t, !(err != nil), "WriteFile(gachaCeilItems) error = %v", err)
	}

	for _, rel := range []string{
		filepath.Join("asset", "jp-assets", "startapp", "thumbnail", "gacha_ticket", "mission_ticket.png"),
		filepath.Join("asset", "jp-assets", "startapp", "thumbnail", "gacha_item", "ceil_item_limited.png"),
	} {
		path := filepath.Join(dir, rel)
		{
			err := os.MkdirAll(filepath.Dir(path), 0o755)
			testutil.Require(t, !(err != nil), "MkdirAll(asset) error = %v", err)
		}
		{

			err := os.WriteFile(path, []byte("png"), 0o644)
			testutil.Require(t, !(err != nil), "WriteFile(asset) error = %v", err)
		}

	}

	profile := &drawing.DetailedProfileCardRequest{
		ID:              "123456789",
		Region:          "jp",
		Nickname:        "tester",
		Source:          "suite",
		UpdateTime:      1700000000000,
		LeaderImagePath: "asset/jp-assets/startapp/thumbnail/chara/chr_ts_01.png",
	}
	raw := &snapshot.RawUserData{
		UserGachaTickets:   []snapshot.RawUserGachaTicket{{GachaTicketID: 8, Quantity: 1}},
		UserGachaCeilItems: []snapshot.RawUserGachaCeilItem{{GachaCeilItemID: 9, Quantity: 2}},
	}
	ctrl := NewController(nil, assets.NewAssetHelper(dir, nil), nil, renderregion.JP, MasterdataOptions{LocalDir: dir})

	req, err := ctrl.BuildListRequestFromSnapshot(Query{
		Region:   renderregion.JP,
		Profile:  profile,
		Snapshot: &inventorySnapshotStub{raw: raw, profile: profile},
	})
	testutil.Require(t, !(err != nil), "BuildListRequestFromSnapshot() error = %v", err)
	{

		got := findInventoryItem(t, req.Sections, "tickets", 8)
		testutil.Require(t, !(got.IconPath != "asset/jp-assets/startapp/thumbnail/gacha_ticket/mission_ticket.png"), "gacha ticket icon path = %q", got.IconPath)
	}
	{

		got := findInventoryItem(t, req.Sections, "tickets", 9)
		testutil.Require(t, !(got.IconPath != "asset/jp-assets/startapp/thumbnail/gacha_item/ceil_item_limited.png"), "gacha ceil item icon path = %q", got.IconPath)
	}

}

func TestBuildListRequestFromSnapshotFiltersSpecialInventoryItems(t *testing.T) {
	dir := t.TempDir()
	masterDir := filepath.Join(dir, "haruki-sekai-master", "master")
	{
		err := os.MkdirAll(masterDir, 0o755)
		testutil.Require(t, !(err != nil), "MkdirAll() error = %v", err)
	}
	{

		err := os.WriteFile(filepath.Join(masterDir, "materials.json"), []byte(`[
		{"id":5,"seq":50,"materialType":"music","name":"音乐卡","flavorText":"用于解锁歌曲。"}
	]`), 0o644)
		testutil.Require(t, !(err != nil), "WriteFile(materials) error = %v", err)
	}
	{

		err := os.WriteFile(filepath.Join(masterDir, "boostItems.json"), []byte(`[
		{"id":2,"seq":2,"name":"演出能量饮料（大）","recoveryValue":10,"flavorText":"恢复10演出能量。"}
	]`), 0o644)
		testutil.Require(t, !(err != nil), "WriteFile(boostItems) error = %v", err)
	}
	{

		err := os.WriteFile(filepath.Join(masterDir, "mysekaiMaterials.json"), []byte(`[
		{"id":6,"seq":6,"mysekaiMaterialType":"mineral","name":"想いの石ころ","description":"岩から獲得できる。","iconAssetbundleName":"item_mineral_1"},
		{"id":35,"seq":601,"mysekaiMaterialType":"game_character","name":"スカイブルーメモリア","description":"星乃 一歌との思い出のかけら。","iconAssetbundleName":"item_memoria_1"}
	]`), 0o644)
		testutil.Require(t, !(err != nil), "WriteFile(mysekaiMaterials) error = %v", err)
	}

	profile := &drawing.DetailedProfileCardRequest{
		ID:              "123456789",
		Region:          "jp",
		Nickname:        "tester",
		Source:          "suite",
		UpdateTime:      1700000000000,
		LeaderImagePath: "asset/jp-assets/startapp/thumbnail/chara/chr_ts_01.png",
	}
	raw := &snapshot.RawUserData{
		UserGamedata:        snapshot.RawUserGamedata{Coin: 123456, VirtualCoin: 77},
		UserChargedCurrency: snapshot.RawUserChargedCurrency{Free: 300, Paid: 20},
		UserMaterials:       []snapshot.RawUserMaterial{{MaterialID: 5, Quantity: 3}},
		UserBoostItems:      []snapshot.RawUserBoostItem{{BoostItemID: 2, Quantity: 4}},
		UserMysekaiMaterials: []snapshot.RawUserMysekaiMaterial{
			{MysekaiMaterialID: 6, Quantity: 5},
			{MysekaiMaterialID: 35, Quantity: 6},
		},
	}
	ctrl := NewController(nil, nil, nil, renderregion.JP, MasterdataOptions{LocalDir: dir})

	tests := []struct {
		name       string
		filter     Filter
		sectionKey string
		wantIDs    []int
	}{
		{name: "jewel", filter: FilterJewel, sectionKey: "currency", wantIDs: []int{-1, -2}},
		{name: "boost", filter: FilterBoost, sectionKey: "boost", wantIDs: []int{2}},
		{name: "mysekai", filter: FilterMysekai, sectionKey: "mysekai", wantIDs: []int{6}},
		{name: "memory", filter: FilterMemory, sectionKey: "memory", wantIDs: []int{35}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := ctrl.BuildListRequestFromSnapshot(Query{
				Region:   renderregion.JP,
				Profile:  profile,
				Snapshot: &inventorySnapshotStub{raw: raw, profile: profile},
				Filter:   tt.filter,
			})
			testutil.Require(t, !(err != nil), "BuildListRequestFromSnapshot() error = %v", err)
			testutil.Require(t, !(req.TotalItems != len(tt.wantIDs)), "TotalItems = %d, want %d", req.TotalItems, len(tt.wantIDs))

			for _, id := range tt.wantIDs {
				findInventoryItem(t, req.Sections, tt.sectionKey, id)
			}
		})
	}
}

func TestBuildListRequestFromSnapshotRejectsCNMysekaiFilters(t *testing.T) {
	profile := &drawing.DetailedProfileCardRequest{
		ID:              "123456789",
		Region:          "cn",
		Nickname:        "tester",
		Source:          "suite",
		UpdateTime:      1700000000000,
		LeaderImagePath: "asset/jp-assets/startapp/thumbnail/chara/chr_ts_01.png",
	}
	ctrl := NewController(nil, nil, nil, renderregion.CN, MasterdataOptions{})

	for _, filter := range []Filter{FilterMysekai, FilterMemory} {
		{
			_, err := ctrl.BuildListRequestFromSnapshot(Query{
				Region:   renderregion.CN,
				Profile:  profile,
				Snapshot: &inventorySnapshotStub{raw: &snapshot.RawUserData{}, profile: profile},
				Filter:   filter,
			})
			testutil.Require(t, !(err == nil), "BuildListRequestFromSnapshot(filter=%q) error = nil, want error", filter)
		}

	}
}

func findInventoryItem(t *testing.T, sections []drawing.InventorySection, sectionKey string, id int) drawing.InventoryItem {
	t.Helper()
	for _, section := range sections {
		if section.Key != sectionKey {
			continue
		}
		for _, item := range section.Items {
			if item.ID == id {
				return item
			}
		}
	}
	t.Fatalf("item %s/%d not found in sections: %+v", sectionKey, id, sections)
	return drawing.InventoryItem{}
}

func assertInventoryItemMissing(t *testing.T, sections []drawing.InventorySection, sectionKey string, id int) {
	t.Helper()
	for _, section := range sections {
		if section.Key != sectionKey {
			continue
		}
		for _, item := range section.Items {
			testutil.Require(t, !(item.ID == id), "item %s/%d should be hidden, got %+v", sectionKey, id, item)

		}
	}
}

type inventorySnapshotStub struct {
	raw     *snapshot.RawUserData
	profile *drawing.DetailedProfileCardRequest
}

func (s *inventorySnapshotStub) Require() error { return nil }

func (s *inventorySnapshotStub) DetailedProfile(renderregion.Value) *drawing.DetailedProfileCardRequest {
	return s.profile
}

func (s *inventorySnapshotStub) ProfileCard(renderregion.Value) *drawing.ProfileCardRequest {
	return nil
}

func (s *inventorySnapshotStub) MusicResults(string) map[int]string { return nil }

func (s *inventorySnapshotStub) GetMusicResult(int, string) string { return "" }

func (s *inventorySnapshotStub) ChallengeLive() *snapshot.ChallengeLiveData { return nil }

func (s *inventorySnapshotStub) RawBytes() ([]byte, error) { return nil, nil }

func (s *inventorySnapshotStub) RawValue(string) ([]byte, error) { return nil, nil }

func (s *inventorySnapshotStub) RawFilePath() string { return "" }

func (s *inventorySnapshotStub) RawData() *snapshot.RawUserData { return s.raw }

func (s *inventorySnapshotStub) MusicMetaBytes() []byte { return nil }

func (s *inventorySnapshotStub) MusicMetaPath() string { return "" }
