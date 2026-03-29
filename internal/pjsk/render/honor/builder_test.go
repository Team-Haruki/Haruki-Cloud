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
	expectedHonorPath := filepath.ToSlash(filepath.Join(dir, "asset", "jp-assets", "ondemand", "honor", "honor_bg_001", "degree_main.png"))
	if req.HonorImgPath == nil || *req.HonorImgPath != expectedHonorPath {
		t.Fatalf("unexpected honor image path: %#v", req.HonorImgPath)
	}
	if req.FrameImgPath == nil || *req.FrameImgPath != "static_images/honor/frame_degree_m_3.png" {
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
