package honor

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type testHonorSource struct {
	region  renderregion.Value
	honors  map[int]*masterdata.Honor
	groups  map[int]*masterdata.HonorGroup
	bonds   map[int]*masterdata.BondsHonor
	gcuByID map[int]*masterdata.GameCharacterUnit
}

func newTestHonorSource(region renderregion.Value) *testHonorSource {
	return &testHonorSource{
		region:  region,
		honors:  make(map[int]*masterdata.Honor),
		groups:  make(map[int]*masterdata.HonorGroup),
		bonds:   make(map[int]*masterdata.BondsHonor),
		gcuByID: make(map[int]*masterdata.GameCharacterUnit),
	}
}

func (s *testHonorSource) DefaultRegion() renderregion.Value { return s.region }

func (s *testHonorSource) GetHonorByID(id int) (*masterdata.Honor, error) {
	if item, ok := s.honors[id]; ok {
		copy := *item
		if item.Levels != nil {
			copy.Levels = append([]masterdata.HonorLevel(nil), item.Levels...)
		}
		return &copy, nil
	}
	return nil, fmt.Errorf("honor not found: %d", id)
}

func (s *testHonorSource) GetHonorGroupByID(id int) (*masterdata.HonorGroup, error) {
	if item, ok := s.groups[id]; ok {
		copy := *item
		return &copy, nil
	}
	return nil, fmt.Errorf("group not found: %d", id)
}

func (s *testHonorSource) GetBondsHonorByID(id int) (*masterdata.BondsHonor, error) {
	if item, ok := s.bonds[id]; ok {
		copy := *item
		return &copy, nil
	}
	return nil, fmt.Errorf("bonds not found: %d", id)
}

func (s *testHonorSource) GetGameCharacterUnitByID(id int) (*masterdata.GameCharacterUnit, bool) {
	if item, ok := s.gcuByID[id]; ok {
		copy := *item
		return &copy, true
	}
	return nil, false
}

func TestBuildHonorRequestNormalWorldLink(t *testing.T) {
	dir := t.TempDir()
	mustWriteHonorAsset(t, dir, filepath.Join("asset", "jp-assets", "ondemand", "honor", "honor_bg_001", "degree_main.png"))

	source := newTestHonorSource(renderregion.JP)
	bg := "honor_bg_001"
	source.honors[100] = &masterdata.Honor{
		ID:              100,
		GroupID:         200,
		HonorRarity:     "low",
		AssetBundleName: "honor_top_001",
		Levels: []masterdata.HonorLevel{
			{Level: 5, HonorRarity: "high", AssetBundleName: "honor_top_001"},
		},
	}
	source.groups[200] = &masterdata.HonorGroup{
		ID:                        200,
		HonorType:                 "world_link",
		BackgroundAssetBundleName: &bg,
	}

	builder := NewBuilder(source, assets.NewAssetHelper(dir, nil))
	req, err := builder.BuildHonorRequest(Query{
		Region:     renderregion.JP,
		HonorID:    100,
		HonorLevel: 5,
		IsMain:     true,
	})
	if err != nil {
		t.Fatalf("BuildHonorRequest failed: %v", err)
	}
	if req.GroupType == nil || *req.GroupType != "wl_event" {
		t.Fatalf("unexpected group type: %#v", req.GroupType)
	}
	expectedHonorPath := "asset/jp-assets/ondemand/honor/honor_bg_001/degree_main.png"
	if req.HonorImgPath == nil || *req.HonorImgPath != expectedHonorPath {
		t.Fatalf("unexpected honor image path: %#v", req.HonorImgPath)
	}
	if req.FrameImgPath == nil || *req.FrameImgPath != "static_images/honor/frame_degree_m_1.png" {
		t.Fatalf("unexpected frame image path: %#v", req.FrameImgPath)
	}
}

func TestBuildHonorRequestBondsMain(t *testing.T) {
	source := newTestHonorSource(renderregion.JP)
	source.bonds[900] = &masterdata.BondsHonor{
		ID:                   900,
		GameCharacterUnitID1: 11,
		GameCharacterUnitID2: 22,
		HonorRarity:          "highest",
	}
	source.gcuByID[11] = &masterdata.GameCharacterUnit{ID: 11, GameCharacterID: 5}
	source.gcuByID[22] = &masterdata.GameCharacterUnit{ID: 22, GameCharacterID: 7}

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	req, err := builder.BuildHonorRequest(Query{
		Region:     renderregion.JP,
		HonorID:    900,
		HonorLevel: 3,
		IsMain:     true,
	})
	if err != nil {
		t.Fatalf("BuildHonorRequest failed: %v", err)
	}
	if req.HonorType == nil || *req.HonorType != "bonds" {
		t.Fatalf("unexpected honor type: %#v", req.HonorType)
	}
	if req.WordImgPath == nil || *req.WordImgPath == "" {
		t.Fatalf("expected word image path, got %#v", req.WordImgPath)
	}
	if req.CharaID == nil || *req.CharaID != "11" || req.CharaID2 == nil || *req.CharaID2 != "22" {
		t.Fatalf("unexpected chara ids: %#v %#v", req.CharaID, req.CharaID2)
	}
}

func TestBuildHonorRequestBirthdayUsesDerivedBirthdayType(t *testing.T) {
	dir := t.TempDir()
	mustWriteHonorAsset(t, dir, filepath.Join("asset", "jp-assets", "ondemand", "honor", "honor_bg_birthday_01_06", "degree_sub.png"))
	mustWriteHonorAsset(t, dir, filepath.Join("asset", "jp-assets", "ondemand", "honor_frame", "honor_frame_birthday_01_06", "frame_degree_s_4.png"))
	mustWriteHonorAsset(t, dir, filepath.Join("asset", "jp-assets", "ondemand", "honor_frame", "honor_frame_birthday_01_06", "frame_degree_level_4.png"))

	bg := "honor_bg_birthday_01_06"
	frame := "honor_frame_birthday_01_06"
	source := newTestHonorSource(renderregion.JP)
	source.honors[6833] = &masterdata.Honor{
		ID:              6833,
		GroupID:         544,
		HonorRarity:     "highest",
		AssetBundleName: "honor_6833",
	}
	source.groups[544] = &masterdata.HonorGroup{
		ID:                        544,
		HonorType:                 "birthday",
		BackgroundAssetBundleName: &bg,
		FrameName:                 &frame,
	}

	builder := NewBuilder(source, assets.NewAssetHelper(dir, nil))
	req, err := builder.BuildHonorRequest(Query{
		Region:     renderregion.JP,
		HonorID:    6833,
		HonorLevel: 3,
		IsMain:     false,
	})
	if err != nil {
		t.Fatalf("BuildHonorRequest failed: %v", err)
	}
	if req.HonorType == nil || *req.HonorType != "birthday" {
		t.Fatalf("unexpected honor type: %#v", req.HonorType)
	}
	if req.FrameDegreeLevelImgPath == nil || *req.FrameDegreeLevelImgPath == "" {
		t.Fatalf("expected birthday level frame path, got %#v", req.FrameDegreeLevelImgPath)
	}
}

func TestBuildHonorRequestFallsBackToLevelAssetWhenTopLevelAssetIsEmpty(t *testing.T) {
	dir := t.TempDir()
	mustWriteHonorAsset(t, dir, filepath.Join("asset", "jp-assets", "ondemand", "honor", "honor_3009_100", "degree_main.png"))

	source := newTestHonorSource(renderregion.JP)
	source.honors[3009] = &masterdata.Honor{
		ID:      3009,
		GroupID: 300,
		Levels: []masterdata.HonorLevel{
			{Level: 1, AssetBundleName: "honor_3009_100", HonorRarity: "low"},
		},
	}
	source.groups[300] = &masterdata.HonorGroup{
		ID:        300,
		HonorType: "achievement",
	}

	builder := NewBuilder(source, assets.NewAssetHelper(dir, nil))
	req, err := builder.BuildHonorRequest(Query{
		Region:     renderregion.JP,
		HonorID:    3009,
		HonorLevel: 0,
		IsMain:     true,
	})
	if err != nil {
		t.Fatalf("BuildHonorRequest failed: %v", err)
	}
	expectedHonorPath := "asset/jp-assets/ondemand/honor/honor_3009_100/degree_main.png"
	if req.HonorImgPath == nil || *req.HonorImgPath != expectedHonorPath {
		t.Fatalf("unexpected honor image path: %#v", req.HonorImgPath)
	}
	if req.HonorRarity == nil || *req.HonorRarity != "low" {
		t.Fatalf("unexpected honor rarity: %#v", req.HonorRarity)
	}
}

func TestBuildHonorRequestDetectsWorldLinkEventByAssetName(t *testing.T) {
	dir := t.TempDir()
	mustWriteHonorAsset(t, dir, filepath.Join("asset", "jp-assets", "ondemand", "honor", "honor_bg_event_wl_2nd_idol_cp1", "degree_main.png"))
	mustWriteHonorAsset(t, dir, filepath.Join("asset", "jp-assets", "ondemand", "honor", "honor_top_000100_event_wl_2nd_idol_cp1", "rank_main.png"))

	source := newTestHonorSource(renderregion.JP)
	bg := "honor_bg_event_wl_2nd_idol_cp1"
	source.honors[5746] = &masterdata.Honor{
		ID:              5746,
		GroupID:         485,
		HonorRarity:     "high",
		AssetBundleName: "honor_top_000100_event_wl_2nd_idol_cp1",
	}
	source.groups[485] = &masterdata.HonorGroup{
		ID:                        485,
		HonorType:                 "event",
		BackgroundAssetBundleName: &bg,
	}

	builder := NewBuilder(source, assets.NewAssetHelper(dir, nil))
	req, err := builder.BuildHonorRequest(Query{
		Region:     renderregion.JP,
		HonorID:    5746,
		HonorLevel: 1,
		IsMain:     true,
	})
	if err != nil {
		t.Fatalf("BuildHonorRequest failed: %v", err)
	}
	if req.GroupType == nil || *req.GroupType != "wl_event" {
		t.Fatalf("unexpected group type: %#v", req.GroupType)
	}
	expectedRankPath := "asset/jp-assets/ondemand/honor/honor_top_000100_event_wl_2nd_idol_cp1/rank_main.png"
	if req.RankImgPath == nil || *req.RankImgPath != expectedRankPath {
		t.Fatalf("unexpected rank image path: %#v", req.RankImgPath)
	}
}

func TestBuildHonorRequestEventUsesResolvedAbsoluteRankPath(t *testing.T) {
	dir := t.TempDir()
	mustWriteHonorAsset(t, dir, filepath.Join("asset", "cn-assets", "startapp", "honor", "honor_bg_event_demo", "degree_sub.png"))
	mustWriteHonorAsset(t, dir, filepath.Join("asset", "cn-assets", "startapp", "honor", "honor_top_000100_event_demo", "rank_sub.png"))

	source := newTestHonorSource(renderregion.CN)
	bg := "honor_bg_event_demo"
	source.honors[6201] = &masterdata.Honor{
		ID:              6201,
		GroupID:         601,
		HonorRarity:     "high",
		AssetBundleName: "honor_top_000100_event_demo",
	}
	source.groups[601] = &masterdata.HonorGroup{
		ID:                        601,
		HonorType:                 "event",
		BackgroundAssetBundleName: &bg,
	}

	builder := NewBuilder(source, assets.NewAssetHelper(dir, nil))
	req, err := builder.BuildHonorRequest(Query{
		Region:     renderregion.CN,
		HonorID:    6201,
		HonorLevel: 1,
		IsMain:     false,
	})
	if err != nil {
		t.Fatalf("BuildHonorRequest failed: %v", err)
	}
	expectedRankPath := "asset/cn-assets/startapp/honor/honor_top_000100_event_demo/rank_sub.png"
	if req.RankImgPath == nil || *req.RankImgPath != expectedRankPath {
		t.Fatalf("unexpected rank image path: %#v", req.RankImgPath)
	}
}

func TestBuildHonorRequestLegacyWLEventUsesResolvedAbsoluteRankPath(t *testing.T) {
	dir := t.TempDir()
	mustWriteHonorAsset(t, dir, filepath.Join("asset", "cn-assets", "startapp", "honor", "honor_bg_event_beginning_cp6", "degree_sub.png"))
	mustWriteHonorAsset(t, dir, filepath.Join("asset", "cn-assets", "startapp", "honor", "honor_top_001000_event_beginning_cp6", "rank_sub.png"))

	source := newTestHonorSource(renderregion.CN)
	bg := "honor_bg_event_beginning_cp6"
	source.honors[6101] = &masterdata.Honor{
		ID:              6101,
		GroupID:         501,
		HonorRarity:     "highest",
		AssetBundleName: "honor_top_001000_event_beginning_cp6",
	}
	source.groups[501] = &masterdata.HonorGroup{
		ID:                        501,
		HonorType:                 "event",
		BackgroundAssetBundleName: &bg,
	}

	builder := NewBuilder(source, assets.NewAssetHelper(dir, nil))
	req, err := builder.BuildHonorRequest(Query{
		Region:     renderregion.CN,
		HonorID:    6101,
		HonorLevel: 1,
		IsMain:     false,
	})
	if err != nil {
		t.Fatalf("BuildHonorRequest failed: %v", err)
	}
	expectedRankPath := "asset/cn-assets/startapp/honor/honor_top_001000_event_beginning_cp6/rank_sub.png"
	if req.RankImgPath == nil || *req.RankImgPath != expectedRankPath {
		t.Fatalf("unexpected rank image path: %#v", req.RankImgPath)
	}
}

func TestBuildHonorRequestFcApUsesResolvedAbsoluteScrollPath(t *testing.T) {
	dir := t.TempDir()
	mustWriteHonorAsset(t, dir, filepath.Join("asset", "cn-assets", "startapp", "honor", "honor_3012_600", "degree_sub.png"))
	mustWriteHonorAsset(t, dir, filepath.Join("asset", "cn-assets", "startapp", "honor", "honor_3012_600", "scroll.png"))

	source := newTestHonorSource(renderregion.CN)
	source.honors[3012] = &masterdata.Honor{
		ID:          3012,
		GroupID:     701,
		HonorRarity: "high",
		Levels: []masterdata.HonorLevel{
			{Level: 56, AssetBundleName: "honor_3012_600", HonorRarity: "high"},
		},
	}
	source.groups[701] = &masterdata.HonorGroup{
		ID:        701,
		HonorType: "achievement",
	}

	builder := NewBuilder(source, assets.NewAssetHelper(dir, nil))
	req, err := builder.BuildHonorRequest(Query{
		Region:     renderregion.CN,
		HonorID:    3012,
		HonorLevel: 56,
		IsMain:     false,
	})
	if err != nil {
		t.Fatalf("BuildHonorRequest failed: %v", err)
	}
	expectedScrollPath := "asset/cn-assets/startapp/honor/honor_3012_600/scroll.png"
	if req.ScrollImgPath == nil || *req.ScrollImgPath != expectedScrollPath {
		t.Fatalf("unexpected scroll image path: %#v", req.ScrollImgPath)
	}
	if req.GroupType == nil || *req.GroupType != "fc_ap" {
		t.Fatalf("unexpected group type: %#v", req.GroupType)
	}
}

func TestBuildHonorRequestFcApUsesOverrideLevelForDisplayedCount(t *testing.T) {
	source := newTestHonorSource(renderregion.JP)
	source.honors[3013] = &masterdata.Honor{
		ID:          3013,
		GroupID:     711,
		HonorRarity: "high",
		Levels: []masterdata.HonorLevel{
			{Level: 20, AssetBundleName: "honor_3013_700", HonorRarity: "high"},
		},
	}
	source.groups[711] = &masterdata.HonorGroup{
		ID:        711,
		HonorType: "achievement",
	}

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	displayLevel := 15
	req, err := builder.BuildHonorRequest(Query{
		Region:              renderregion.JP,
		HonorID:             3013,
		HonorLevel:          20,
		IsMain:              true,
		FcOrApLevelOverride: &displayLevel,
	})
	if err != nil {
		t.Fatalf("BuildHonorRequest failed: %v", err)
	}
	if req.HonorLevel == nil || *req.HonorLevel != 20 {
		t.Fatalf("expected visual honor level 20, got %#v", req.HonorLevel)
	}
	if req.FcOrApLevel == nil || *req.FcOrApLevel != "15" {
		t.Fatalf("expected displayed FC/AP level 15, got %#v", req.FcOrApLevel)
	}
}

func TestBuildHonorRequestEventFrameFallsBackToStaticForLowRarity(t *testing.T) {
	dir := t.TempDir()
	mustWriteHonorAsset(t, dir, filepath.Join("asset", "cn-assets", "startapp", "honor", "honor_bg_event_underwater_cp1", "degree_sub.png"))

	source := newTestHonorSource(renderregion.CN)
	bg := "honor_bg_event_underwater_cp1"
	frame := "event_underwater_cp1"
	source.honors[4301] = &masterdata.Honor{
		ID:              4301,
		GroupID:         801,
		HonorRarity:     "middle",
		AssetBundleName: "honor_top_000100_event_underwater_cp1",
	}
	source.groups[801] = &masterdata.HonorGroup{
		ID:                        801,
		HonorType:                 "event",
		BackgroundAssetBundleName: &bg,
		FrameName:                 &frame,
	}

	builder := NewBuilder(source, assets.NewAssetHelper(dir, nil))
	req, err := builder.BuildHonorRequest(Query{
		Region:     renderregion.CN,
		HonorID:    4301,
		HonorLevel: 1,
		IsMain:     false,
	})
	if err != nil {
		t.Fatalf("BuildHonorRequest failed: %v", err)
	}
	if req.FrameImgPath == nil || *req.FrameImgPath != "static_images/honor/frame_degree_s_2.png" {
		t.Fatalf("expected static fallback frame, got %#v", req.FrameImgPath)
	}
}

func TestBuildHonorRequestRankMatchUsesRankLiveBackground(t *testing.T) {
	dir := t.TempDir()
	mustWriteHonorAsset(t, dir, filepath.Join("asset", "jp-assets", "startapp", "rank_live", "honor", "season_2025_winter", "degree_sub.png"))
	mustWriteHonorAsset(t, dir, filepath.Join("asset", "jp-assets", "startapp", "rank_live", "honor", "common", "tier_11", "sub.png"))

	source := newTestHonorSource(renderregion.JP)
	bg := "season_2025_winter"
	source.honors[9001] = &masterdata.Honor{
		ID:              9001,
		GroupID:         901,
		HonorRarity:     "low",
		AssetBundleName: "common/tier_11",
	}
	source.groups[901] = &masterdata.HonorGroup{
		ID:                        901,
		HonorType:                 "rank_match",
		BackgroundAssetBundleName: &bg,
	}

	builder := NewBuilder(source, assets.NewAssetHelper(dir, nil))
	req, err := builder.BuildHonorRequest(Query{
		Region:     renderregion.JP,
		HonorID:    9001,
		HonorLevel: 1,
		IsMain:     false,
	})
	if err != nil {
		t.Fatalf("BuildHonorRequest failed: %v", err)
	}
	if req.HonorImgPath == nil || *req.HonorImgPath != "asset/jp-assets/startapp/rank_live/honor/season_2025_winter/degree_sub.png" {
		t.Fatalf("unexpected honor image path: %#v", req.HonorImgPath)
	}
	if req.RankImgPath == nil || *req.RankImgPath != "asset/jp-assets/startapp/rank_live/honor/common/tier_11/sub.png" {
		t.Fatalf("unexpected rank image path: %#v", req.RankImgPath)
	}
}

func mustWriteHonorAsset(t *testing.T, root, rel string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte("png"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
