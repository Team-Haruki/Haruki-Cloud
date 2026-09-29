package honor

import (
	"strings"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// rowHonorSource adds generic master rows to testHonorSource.
type rowHonorSource struct {
	*testHonorSource
	rows map[string]map[int]map[string]any
}

func (s *rowHonorSource) HonorMasterRows(filename string) (map[int]map[string]any, bool) {
	rows, ok := s.rows[filename]
	return rows, ok
}

func characterHonorSource(region renderregion.Value) *testHonorSource {
	source := newTestHonorSource(region)
	levels := make([]masterdata.HonorLevel, 0, 20)
	for level := 1; level <= 20; level++ {
		levels = append(levels, masterdata.HonorLevel{Level: level})
	}
	source.honors[4] = &masterdata.Honor{ID: 4, GroupID: 1, HonorRarity: "highest", AssetBundleName: "honor_0004", Levels: levels}
	source.groups[1] = &masterdata.HonorGroup{ID: 1, HonorType: "character"}
	return source
}

func jp700HonorRows() map[string]map[int]map[string]any {
	return map[string]map[int]map[string]any{
		"honorBackgrounds.json": {
			10101: {"id": 10101, "seq": 1, "honorGroupId": 1, "assetbundleName": "honor_bg_style_01_01"},
			10102: {"id": 10102, "seq": 2, "honorGroupId": 1, "assetbundleName": "honor_bg_style_02_01"},
			10201: {"id": 10201, "seq": 1, "honorGroupId": 2, "assetbundleName": "honor_bg_style_01_02"},
		},
		"honorWords.json": {
			10101: {"id": 10101, "seq": 1, "honorGroupId": 1, "assetbundleName": "honor_word_01_01"},
			10102: {"id": 10102, "seq": 2, "honorGroupId": 1, "assetbundleName": "honor_word_01_02"},
		},
		"honorGroups.json": {
			1: {"id": 1, "honorType": "character", "isMedalDisplayed": true},
		},
	}
}

func TestBuildHonorRequestAppliesSelectedBackgroundWordAndMedal(t *testing.T) {
	source := &rowHonorSource{testHonorSource: characterHonorSource(renderregion.JP), rows: jp700HonorRows()}
	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	req, err := builder.BuildHonorRequest(Query{
		Region: renderregion.JP, HonorID: 4, HonorLevel: 14, IsMain: true,
		HonorBackgroundID: new(10102), HonorWordID: new(10102),
	})
	if err != nil {
		t.Fatalf("BuildHonorRequest() error = %v", err)
	}
	if len(req.HonorImgPath) != 2 || !strings.HasSuffix(req.HonorImgPath[0], "honor_background/honor_bg_style_02_01/degree_main.png") || !strings.HasSuffix(req.HonorImgPath[1], "honor/honor_0004/degree_main.png") {
		t.Fatalf("honor_img_path = %#v, want the background then the classic art", req.HonorImgPath)
	}
	if req.WordImgPath == nil || !strings.HasSuffix(*req.WordImgPath, "honor_word/honor_word_01_02_4.png") {
		t.Fatalf("word_img_path = %v", req.WordImgPath)
	}
	if req.MedalImgPath == nil || !strings.HasSuffix(*req.MedalImgPath, "honor_medal/medal/icon_degree_medal1.png") {
		t.Fatalf("medal_img_path = %v", req.MedalImgPath)
	}
}

func TestBuildHonorRequestDefaultsToGroupFirstRowsAndSubHidesWord(t *testing.T) {
	source := &rowHonorSource{testHonorSource: characterHonorSource(renderregion.JP), rows: jp700HonorRows()}
	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	// Background 10201 belongs to another group: the group's first row wins.
	req, err := builder.BuildHonorRequest(Query{Region: renderregion.JP, HonorID: 4, HonorLevel: 10, HonorBackgroundID: new(10201)})
	if err != nil {
		t.Fatalf("BuildHonorRequest() error = %v", err)
	}
	if !strings.HasSuffix(req.HonorImgPath.First(), "honor_background/honor_bg_style_01_01/degree_sub.png") {
		t.Fatalf("honor_img_path = %#v", req.HonorImgPath)
	}
	if req.WordImgPath != nil {
		t.Fatalf("sub honor must not carry a word: %v", *req.WordImgPath)
	}
	if req.MedalImgPath != nil {
		t.Fatalf("level 10 must not show a medal: %v", *req.MedalImgPath)
	}
}

func TestHonorMedalTier(t *testing.T) {
	honor := &masterdata.Honor{Levels: []masterdata.HonorLevel{{Level: 1}}}
	cases := []struct {
		level     int
		displayed bool
		want      int
	}{{10, true, 0}, {11, true, 1}, {20, true, 1}, {21, true, 2}, {200, true, 9}, {50, false, 0}}
	for _, tc := range cases {
		if got := honorMedalTier(honor, tc.level, tc.displayed); got != tc.want {
			t.Errorf("honorMedalTier(%d, %v) = %d, want %d", tc.level, tc.displayed, got, tc.want)
		}
	}
	if honorMedalTier(&masterdata.Honor{}, 50, true) != 0 {
		t.Error("an honor without levels never shows a medal")
	}
}

// Regions without the JP 7.0.0 tables (EN 6.0.0, TW/KR/CN 6.4) render
// exactly as before, whether the source lacks row support or the tables.
func TestBuildHonorRequestOldRegionUnchanged(t *testing.T) {
	base := characterHonorSource(renderregion.EN)
	plain, err := NewBuilder(base, assets.NewAssetHelper("", nil)).BuildHonorRequest(Query{Region: renderregion.EN, HonorID: 4, HonorLevel: 14, IsMain: true})
	if err != nil {
		t.Fatalf("BuildHonorRequest() error = %v", err)
	}
	withoutTables := &rowHonorSource{testHonorSource: base, rows: map[string]map[int]map[string]any{}}
	got, err := NewBuilder(withoutTables, assets.NewAssetHelper("", nil)).BuildHonorRequest(Query{
		Region: renderregion.EN, HonorID: 4, HonorLevel: 14, IsMain: true, HonorBackgroundID: new(10101), HonorWordID: new(10101),
	})
	if err != nil {
		t.Fatalf("BuildHonorRequest() error = %v", err)
	}
	if len(got.HonorImgPath) != 1 || got.HonorImgPath.First() != plain.HonorImgPath.First() {
		t.Fatalf("honor_img_path = %#v, want %#v", got.HonorImgPath, plain.HonorImgPath)
	}
	if got.WordImgPath != nil || got.MedalImgPath != nil || plain.WordImgPath != nil || plain.MedalImgPath != nil {
		t.Fatalf("old region must not get word/medal layers: %+v / %+v", got, plain)
	}
}

func TestBuildHonorRequestNonCharacterGroupIgnoresLayers(t *testing.T) {
	source := &rowHonorSource{testHonorSource: characterHonorSource(renderregion.JP), rows: jp700HonorRows()}
	source.groups[1] = &masterdata.HonorGroup{ID: 1, HonorType: "achievement"}
	req, err := NewBuilder(source, assets.NewAssetHelper("", nil)).BuildHonorRequest(Query{Region: renderregion.JP, HonorID: 4, HonorLevel: 5, IsMain: true})
	if err != nil {
		t.Fatalf("BuildHonorRequest() error = %v", err)
	}
	if len(req.HonorImgPath) != 1 || req.WordImgPath != nil {
		t.Fatalf("non-character group must keep classic art: %#v word=%v", req.HonorImgPath, req.WordImgPath)
	}
}
