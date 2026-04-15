package mysekai

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/internal/pjsk/drawing"
)

func newPhotoTestController(t *testing.T, mysekaiJSON string) *Controller {
	t.Helper()

	root := t.TempDir()
	userPath := filepath.Join(root, "user.json")
	mysekaiPath := filepath.Join(root, "mysekai.json")

	userJSON := `{
  "now": 1700000000,
  "userGamedata": {"userId": 12345678901234, "name": "Tester", "deck": 1},
  "userProfile": {},
  "userDecks": [{"deckId": 1}],
  "userCards": []
}`

	if err := os.WriteFile(userPath, []byte(userJSON), 0o644); err != nil {
		t.Fatalf("write user snapshot: %v", err)
	}
	if err := os.WriteFile(mysekaiPath, []byte(mysekaiJSON), 0o644); err != nil {
		t.Fatalf("write mysekai snapshot: %v", err)
	}

	service := userdata.NewLocalFileService(nil, nil, userdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userPath,
		MySekaiJSON:   mysekaiPath,
	})
	return NewController(nil, service, renderregion.JP, nil, MasterdataOptions{AllowFallback: true})
}

func TestResolvePhotoSupportsPositiveAndNegativeSequence(t *testing.T) {
	controller := newPhotoTestController(t, `{
  "updatedResources": {
    "userMysekaiPhotos": [
      {"seq": 1, "obtainedAt": 1700000000000, "imagePath": "photos/one"},
      {"seq": 2, "obtainedAt": 1700003600000, "imagePath": "photos/two"}
    ]
  }
}`)

	first, err := controller.ResolvePhoto(PhotoQuery{Seq: 1})
	if err != nil {
		t.Fatalf("ResolvePhoto(1): %v", err)
	}
	if first.Region != "jp" || first.ImagePath != "photos/one" {
		t.Fatalf("unexpected first photo: %+v", first)
	}
	if !first.ObtainedAt.Equal(time.UnixMilli(1700000000000)) {
		t.Fatalf("unexpected first obtainedAt: %s", first.ObtainedAt)
	}

	last, err := controller.ResolvePhoto(PhotoQuery{Seq: -1})
	if err != nil {
		t.Fatalf("ResolvePhoto(-1): %v", err)
	}
	if last.Seq != 2 || last.Total != 2 || last.ImagePath != "photos/two" {
		t.Fatalf("unexpected last photo: %+v", last)
	}
}

func TestResolvePhotoValidatesInputAndImagePath(t *testing.T) {
	controller := newPhotoTestController(t, `{
  "updatedResources": {
    "userMysekaiPhotos": [
      {"seq": 1, "obtainedAt": 1700000000000}
    ]
  }
}`)

	if _, err := controller.ResolvePhoto(PhotoQuery{Seq: 0}); err == nil || err.Error() != "请输入正确的照片编号（从1或-1开始）" {
		t.Fatalf("unexpected seq=0 error: %v", err)
	}
	if _, err := controller.ResolvePhoto(PhotoQuery{Seq: 2}); err == nil || err.Error() != "照片编号大于照片数量(1)" {
		t.Fatalf("unexpected seq=2 error: %v", err)
	}
	if _, err := controller.ResolvePhoto(PhotoQuery{Seq: 1}); err == nil || err.Error() != "该照片缺少 imagePath，无法下载" {
		t.Fatalf("unexpected missing imagePath error: %v", err)
	}
}

func TestBuildFixtureListRequestSupportsOnlyCraftable(t *testing.T) {
	root := t.TempDir()
	userPath := filepath.Join(root, "user.json")
	mysekaiPath := filepath.Join(root, "mysekai.json")
	masterdataDir := filepath.Join(root, "masterdata")

	userJSON := `{
  "now": 1700000000,
  "userGamedata": {"userId": 12345678901234, "name": "Tester", "deck": 1},
  "userProfile": {},
  "userDecks": [{"deckId": 1}],
  "userCards": []
}`
	mysekaiJSON := `{
  "updatedResources": {
    "userMysekaiBlueprints": [{"mysekaiBlueprintId": 1001}]
  }
}`

	if err := os.WriteFile(userPath, []byte(userJSON), 0o644); err != nil {
		t.Fatalf("write user snapshot: %v", err)
	}
	if err := os.WriteFile(mysekaiPath, []byte(mysekaiJSON), 0o644); err != nil {
		t.Fatalf("write mysekai snapshot: %v", err)
	}
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiFixtures.json"), []map[string]any{
		{
			"id":                        2001,
			"name":                      "Wood Chair",
			"assetbundleName":           "wood_chair",
			"mysekaiFixtureType":        "furniture",
			"mysekaiFixtureMainGenreId": 1,
			"mysekaiFixtureSubGenreId":  11,
		},
		{
			"id":                        2002,
			"name":                      "Garden Lamp",
			"assetbundleName":           "garden_lamp",
			"mysekaiFixtureType":        "furniture",
			"mysekaiFixtureMainGenreId": 1,
			"mysekaiFixtureSubGenreId":  11,
		},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiFixtureMainGenres.json"), []map[string]any{
		{"id": 1, "name": "Main A", "assetbundleName": "main_a"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiFixtureSubGenres.json"), []map[string]any{
		{"id": 11, "name": "Sub A", "assetbundleName": "sub_a"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiBlueprints.json"), []map[string]any{
		{"id": 1001, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 2001},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacters.json"), []map[string]any{})

	service := userdata.NewLocalFileService(nil, nil, userdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userPath,
		MySekaiJSON:   mysekaiPath,
	})
	controller := NewController(nil, service, renderregion.JP, nil, MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true})

	onlyCraftable := true
	req, err := controller.BuildFixtureListRequest(FixtureListQuery{
		Region:        "jp",
		OnlyCraftable: &onlyCraftable,
		Profile:       &drawing.ProfileCardRequest{},
	})
	if err != nil {
		t.Fatalf("BuildFixtureListRequest() error = %v", err)
	}
	if len(req.MainGenres) != 1 || len(req.MainGenres[0].SubGenres) != 1 {
		t.Fatalf("unexpected genre layout: %+v", req.MainGenres)
	}
	fixtures := req.MainGenres[0].SubGenres[0].Fixtures
	if len(fixtures) != 1 {
		t.Fatalf("expected 1 craftable fixture, got %+v", fixtures)
	}
	if fixtures[0].ID != 2001 || !fixtures[0].Obtained {
		t.Fatalf("unexpected craftable fixture: %+v", fixtures[0])
	}
}

func TestBuildFixtureListRequestSortsFixturesByIDWithinGroup(t *testing.T) {
	root := t.TempDir()
	userPath := filepath.Join(root, "user.json")
	mysekaiPath := filepath.Join(root, "mysekai.json")
	masterdataDir := filepath.Join(root, "masterdata")

	userJSON := `{
  "now": 1700000000,
  "userGamedata": {"userId": 12345678901234, "name": "Tester", "deck": 1},
  "userProfile": {},
  "userDecks": [{"deckId": 1}],
  "userCards": []
}`
	mysekaiJSON := `{
  "updatedResources": {
    "userMysekaiBlueprints": [{"mysekaiBlueprintId": 1001}, {"mysekaiBlueprintId": 1002}]
  }
}`

	if err := os.WriteFile(userPath, []byte(userJSON), 0o644); err != nil {
		t.Fatalf("write user snapshot: %v", err)
	}
	if err := os.WriteFile(mysekaiPath, []byte(mysekaiJSON), 0o644); err != nil {
		t.Fatalf("write mysekai snapshot: %v", err)
	}
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiFixtures.json"), []map[string]any{
		{
			"id":                        2002,
			"name":                      "Garden Lamp",
			"assetbundleName":           "garden_lamp",
			"mysekaiFixtureType":        "furniture",
			"mysekaiFixtureMainGenreId": 1,
			"mysekaiFixtureSubGenreId":  11,
		},
		{
			"id":                        2001,
			"name":                      "Wood Chair",
			"assetbundleName":           "wood_chair",
			"mysekaiFixtureType":        "furniture",
			"mysekaiFixtureMainGenreId": 1,
			"mysekaiFixtureSubGenreId":  11,
		},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiFixtureMainGenres.json"), []map[string]any{
		{"id": 1, "name": "Main A", "assetbundleName": "main_a"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiFixtureSubGenres.json"), []map[string]any{
		{"id": 11, "name": "Sub A", "assetbundleName": "sub_a"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiBlueprints.json"), []map[string]any{
		{"id": 1001, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 2001},
		{"id": 1002, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 2002},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacters.json"), []map[string]any{})

	service := userdata.NewLocalFileService(nil, nil, userdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userPath,
		MySekaiJSON:   mysekaiPath,
	})
	controller := NewController(nil, service, renderregion.JP, nil, MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true})

	req, err := controller.BuildFixtureListRequest(FixtureListQuery{
		Region:  "jp",
		Profile: &drawing.ProfileCardRequest{},
	})
	if err != nil {
		t.Fatalf("BuildFixtureListRequest() error = %v", err)
	}
	if len(req.MainGenres) != 1 || len(req.MainGenres[0].SubGenres) != 1 {
		t.Fatalf("unexpected genre layout: %+v", req.MainGenres)
	}
	fixtures := req.MainGenres[0].SubGenres[0].Fixtures
	if len(fixtures) != 2 {
		t.Fatalf("expected 2 fixtures, got %+v", fixtures)
	}
	if fixtures[0].ID != 2001 || fixtures[1].ID != 2002 {
		t.Fatalf("expected fixture IDs sorted ascending, got %+v", fixtures)
	}
}

func TestMysekaiProfileCardAppendsMySekaiDataSource(t *testing.T) {
	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{AllowFallback: true})
	profile := &drawing.ProfileCardRequest{
		Profile: &drawing.BasicProfile{
			ID:              "12345678901234567",
			Region:          "JP",
			Nickname:        "Tester",
			LeaderImagePath: "user/leader.png",
		},
		DataSources: []drawing.ProfileDataSource{
			{Name: "Suite数据"},
		},
	}
	merged := map[string]any{
		"upload_time":  float64(1776000000123),
		"source":       "toolbox_live",
		"local_source": "haruki",
		"userMysekaiGamedata": map[string]any{
			"mysekaiRank": 10,
		},
	}

	got := controller.mysekaiProfileCard(renderregion.JP, merged, profile, true)
	if got == nil {
		t.Fatal("expected profile card")
	}
	if got.MysekaiLevel == nil || *got.MysekaiLevel != 10 {
		t.Fatalf("unexpected mysekai level: %+v", got.MysekaiLevel)
	}
	if len(got.DataSources) != 2 {
		t.Fatalf("expected 2 data sources, got %+v", got.DataSources)
	}
	if got.DataSources[1].Name != "Mysekai数据" {
		t.Fatalf("unexpected mysekai data source: %+v", got.DataSources[1])
	}
	if got.DataSources[1].UpdateTime == nil || *got.DataSources[1].UpdateTime != 1776000000123 {
		t.Fatalf("unexpected mysekai update time: %+v", got.DataSources[1].UpdateTime)
	}
	if got.DataSources[1].Source == nil || *got.DataSources[1].Source != "toolbox_live(haruki)" {
		t.Fatalf("unexpected mysekai source: %+v", got.DataSources[1].Source)
	}
}

func TestMysekaiProfileCardReplacesSingleSourceWhenUsingRawMySekaiOnly(t *testing.T) {
	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{AllowFallback: true})
	controller = controller.WithMySekaiData([]byte(`{"updatedResources":{"userMysekaiGamedata":{"mysekaiRank":8}},"upload_time":1776000000,"source":"toolbox_live"}`))
	profile := &drawing.ProfileCardRequest{
		Profile: &drawing.BasicProfile{
			ID:              "GAME_USER_ID_REDACTED",
			Region:          "JP",
			Nickname:        "Tester",
			LeaderImagePath: "user/leader.png",
		},
		DataSources: []drawing.ProfileDataSource{
			{Name: "Sekai API"},
		},
	}
	merged := map[string]any{
		"upload_time": float64(1776000000),
		"source":      "toolbox_live",
		"userMysekaiGamedata": map[string]any{
			"mysekaiRank": 8,
		},
	}

	got := controller.mysekaiProfileCard(renderregion.JP, merged, profile, false)
	if got == nil || len(got.DataSources) != 1 {
		t.Fatalf("expected one mysekai data source, got %+v", got)
	}
	if got.DataSources[0].Name != "Mysekai数据" {
		t.Fatalf("expected single source renamed to Mysekai数据, got %+v", got.DataSources[0])
	}
	if got.DataSources[0].UpdateTime == nil || *got.DataSources[0].UpdateTime != 1776000000000 {
		t.Fatalf("unexpected mysekai update time: %+v", got.DataSources[0].UpdateTime)
	}
	if got.MysekaiLevel == nil || *got.MysekaiLevel != 8 {
		t.Fatalf("unexpected mysekai level: %+v", got.MysekaiLevel)
	}
}

func TestMysekaiProfileCardKeepsBothSourcesWhenRequested(t *testing.T) {
	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{AllowFallback: true})
	profile := &drawing.ProfileCardRequest{
		Profile: &drawing.BasicProfile{
			ID:              "GAME_USER_ID_REDACTED",
			Region:          "JP",
			Nickname:        "Tester",
			LeaderImagePath: "user/leader.png",
		},
		DataSources: []drawing.ProfileDataSource{
			{Name: "Suite数据"},
		},
	}
	merged := map[string]any{
		"upload_time": float64(1776000000),
		"source":      "toolbox_live",
		"userMysekaiGamedata": map[string]any{
			"mysekaiRank": 8,
		},
	}

	got := controller.mysekaiProfileCard(renderregion.JP, merged, profile, true)
	if got == nil || len(got.DataSources) != 2 {
		t.Fatalf("expected suite + mysekai data sources, got %+v", got)
	}
	if got.DataSources[0].Name != "Suite数据" || got.DataSources[1].Name != "Mysekai数据" {
		t.Fatalf("unexpected data source order: %+v", got.DataSources)
	}
}

func TestResolveTalkCharacterHandlesVirtualSingerUnits(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacters.json"), []map[string]any{
		{"id": 1, "firstName": "星乃", "givenName": "一歌", "firstNameEnglish": "Hoshino", "givenNameEnglish": "Ichika"},
		{"id": 21, "firstName": "初音", "givenName": "未来", "firstNameEnglish": "Hatsune", "givenNameEnglish": "Miku"},
		{"id": 22, "firstName": "镜音", "givenName": "铃", "firstNameEnglish": "Kagamine", "givenNameEnglish": "Rin"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacterUnits.json"), []map[string]any{
		{"id": 1, "gameCharacterId": 1, "unit": "light_sound"},
		{"id": 21, "gameCharacterId": 21, "unit": "piapro"},
		{"id": 27, "gameCharacterId": 21, "unit": "light_sound"},
		{"id": 28, "gameCharacterId": 21, "unit": "idol"},
		{"id": 32, "gameCharacterId": 22, "unit": "piapro"},
		{"id": 33, "gameCharacterId": 22, "unit": "light_sound"},
		{"id": 34, "gameCharacterId": 22, "unit": "idol"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiGateCharacterLotteries.json"), []map[string]any{
		{"id": 1, "gameCharacterUnitId": 27},
		{"id": 2, "gameCharacterUnitId": 28},
		{"id": 3, "gameCharacterUnitId": 34},
	})

	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true})

	if _, _, err := controller.resolveTalkCharacter("miku"); err == nil || !strings.Contains(err.Error(), "需要同时指定组合") {
		t.Fatalf("resolveTalkCharacter(miku) error = %v", err)
	}

	characterID, unitID, err := controller.resolveTalkCharacter("light_sound miku")
	if err != nil {
		t.Fatalf("resolveTalkCharacter(light_sound miku): %v", err)
	}
	if characterID != 21 || unitID != 27 {
		t.Fatalf("unexpected light_sound miku result: characterID=%d unitID=%d", characterID, unitID)
	}

	characterID, unitID, err = controller.resolveTalkCharacter("rin")
	if err != nil {
		t.Fatalf("resolveTalkCharacter(rin): %v", err)
	}
	if characterID != 22 || unitID != 34 {
		t.Fatalf("unexpected rin result: characterID=%d unitID=%d", characterID, unitID)
	}
}

func TestBuildTalkListRequestSortsSingleTalkFixturesByGroupSizeAndFixtureID(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}

	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacters.json"), []map[string]any{
		{"id": 1, "firstName": "星乃", "givenName": "一歌"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacterUnits.json"), []map[string]any{
		{"id": 101, "gameCharacterId": 1, "unit": "light_sound"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiFixtureMainGenres.json"), []map[string]any{
		{"id": 1, "name": "Main A", "assetbundleName": "main_a"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiFixtures.json"), []map[string]any{
		{"id": 11, "assetbundleName": "fixture_11", "mysekaiFixtureType": "furniture", "mysekaiFixtureMainGenreId": 1},
		{"id": 12, "assetbundleName": "fixture_12", "mysekaiFixtureType": "furniture", "mysekaiFixtureMainGenreId": 1},
		{"id": 13, "assetbundleName": "fixture_13", "mysekaiFixtureType": "furniture", "mysekaiFixtureMainGenreId": 1},
		{"id": 14, "assetbundleName": "fixture_14", "mysekaiFixtureType": "furniture", "mysekaiFixtureMainGenreId": 1},
		{"id": 15, "assetbundleName": "fixture_15", "mysekaiFixtureType": "furniture", "mysekaiFixtureMainGenreId": 1},
		{"id": 16, "assetbundleName": "fixture_16", "mysekaiFixtureType": "furniture", "mysekaiFixtureMainGenreId": 1},
		{"id": 17, "assetbundleName": "fixture_17", "mysekaiFixtureType": "furniture", "mysekaiFixtureMainGenreId": 1},
		{"id": 18, "assetbundleName": "fixture_18", "mysekaiFixtureType": "furniture", "mysekaiFixtureMainGenreId": 1},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiBlueprints.json"), []map[string]any{
		{"id": 10011, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 11},
		{"id": 10012, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 12},
		{"id": 10013, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 13},
		{"id": 10014, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 14},
		{"id": 10015, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 15},
		{"id": 10016, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 16},
		{"id": 10017, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 17},
		{"id": 10018, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 18},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiGameCharacterUnitGroups.json"), []map[string]any{
		{"id": 1, "gameCharacterUnitId1": 101},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "characterArchiveMysekaiCharacterTalkGroups.json"), []map[string]any{
		{"id": 100, "archiveDisplayType": "normal"},
		{"id": 101, "archiveDisplayType": "normal"},
		{"id": 102, "archiveDisplayType": "normal"},
		{"id": 103, "archiveDisplayType": "normal"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiCharacterTalkConditions.json"), []map[string]any{
		{"id": 2011, "mysekaiCharacterTalkConditionType": "mysekai_fixture_id", "mysekaiCharacterTalkConditionTypeValue": 11},
		{"id": 2012, "mysekaiCharacterTalkConditionType": "mysekai_fixture_id", "mysekaiCharacterTalkConditionTypeValue": 12},
		{"id": 2013, "mysekaiCharacterTalkConditionType": "mysekai_fixture_id", "mysekaiCharacterTalkConditionTypeValue": 13},
		{"id": 2014, "mysekaiCharacterTalkConditionType": "mysekai_fixture_id", "mysekaiCharacterTalkConditionTypeValue": 14},
		{"id": 2015, "mysekaiCharacterTalkConditionType": "mysekai_fixture_id", "mysekaiCharacterTalkConditionTypeValue": 15},
		{"id": 2016, "mysekaiCharacterTalkConditionType": "mysekai_fixture_id", "mysekaiCharacterTalkConditionTypeValue": 16},
		{"id": 2017, "mysekaiCharacterTalkConditionType": "mysekai_fixture_id", "mysekaiCharacterTalkConditionTypeValue": 17},
		{"id": 2018, "mysekaiCharacterTalkConditionType": "mysekai_fixture_id", "mysekaiCharacterTalkConditionTypeValue": 18},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiCharacterTalkConditionGroups.json"), []map[string]any{
		{"id": 3011, "mysekaiCharacterTalkConditionId": 2011},
		{"id": 3012, "mysekaiCharacterTalkConditionId": 2012},
		{"id": 3013, "mysekaiCharacterTalkConditionId": 2013},
		{"id": 3014, "mysekaiCharacterTalkConditionId": 2014},
		{"id": 3015, "mysekaiCharacterTalkConditionId": 2015},
		{"id": 3016, "mysekaiCharacterTalkConditionId": 2016},
		{"id": 3017, "mysekaiCharacterTalkConditionId": 2017},
		{"id": 3018, "mysekaiCharacterTalkConditionId": 2018},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiCharacterTalks.json"), []map[string]any{
		{"id": 4011, "mysekaiCharacterTalkConditionGroupId": 3011, "mysekaiGameCharacterUnitGroupId": 1, "characterArchiveMysekaiCharacterTalkGroupId": 100},
		{"id": 4012, "mysekaiCharacterTalkConditionGroupId": 3012, "mysekaiGameCharacterUnitGroupId": 1, "characterArchiveMysekaiCharacterTalkGroupId": 100},
		{"id": 4013, "mysekaiCharacterTalkConditionGroupId": 3013, "mysekaiGameCharacterUnitGroupId": 1, "characterArchiveMysekaiCharacterTalkGroupId": 101},
		{"id": 4014, "mysekaiCharacterTalkConditionGroupId": 3014, "mysekaiGameCharacterUnitGroupId": 1, "characterArchiveMysekaiCharacterTalkGroupId": 102},
		{"id": 4015, "mysekaiCharacterTalkConditionGroupId": 3015, "mysekaiGameCharacterUnitGroupId": 1, "characterArchiveMysekaiCharacterTalkGroupId": 102},
		{"id": 4016, "mysekaiCharacterTalkConditionGroupId": 3016, "mysekaiGameCharacterUnitGroupId": 1, "characterArchiveMysekaiCharacterTalkGroupId": 102},
		{"id": 4017, "mysekaiCharacterTalkConditionGroupId": 3017, "mysekaiGameCharacterUnitGroupId": 1, "characterArchiveMysekaiCharacterTalkGroupId": 103},
		{"id": 4018, "mysekaiCharacterTalkConditionGroupId": 3018, "mysekaiGameCharacterUnitGroupId": 1, "characterArchiveMysekaiCharacterTalkGroupId": 103},
	})

	mysekaiJSON := `{
  "updatedResources": {
    "userMysekaiBlueprints": [
      {"mysekaiBlueprintId": 10011},
      {"mysekaiBlueprintId": 10012},
      {"mysekaiBlueprintId": 10013},
      {"mysekaiBlueprintId": 10014},
      {"mysekaiBlueprintId": 10015},
      {"mysekaiBlueprintId": 10016},
      {"mysekaiBlueprintId": 10017},
      {"mysekaiBlueprintId": 10018}
    ],
    "userMysekaiCharacterTalks": []
  }
}`

	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true}).WithMySekaiData([]byte(mysekaiJSON))
	req, err := controller.BuildTalkListRequest(TalkListQuery{Region: "jp", Query: "101"})
	if err != nil {
		t.Fatalf("BuildTalkListRequest() error = %v", err)
	}
	if len(req.SingleMainGenres) != 1 || len(req.SingleMainGenres[0].SubGenres) != 1 {
		t.Fatalf("unexpected single talk genres: %+v", req.SingleMainGenres)
	}

	got := make([][]int, 0, len(req.SingleMainGenres[0].SubGenres[0]))
	for _, group := range req.SingleMainGenres[0].SubGenres[0] {
		ids := make([]int, 0, len(group.Fixtures))
		for _, fixture := range group.Fixtures {
			ids = append(ids, fixture.ID)
		}
		got = append(got, ids)
	}

	want := [][]int{
		{14, 15, 16},
		{17, 18},
		{11, 12},
		{13},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected single talk fixture order: got=%v want=%v", got, want)
	}
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture json: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture json: %v", err)
	}
}

func TestBuildResourceRequestUsesGateLargeThumbnailPath(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}

	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiGates.json"), []map[string]any{
		{"id": 4, "assetbundleName": "mdl_non0006_gate_wns1"},
	})

	mysekaiJSON := `{
  "userMysekaiGateCharacterVisit": {
    "userMysekaiGate": {
      "mysekaiGateId": 4,
      "mysekaiGateSkinId": 0,
      "mysekaiGateLevel": 30
    }
  }
}`

	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true}).WithMySekaiData([]byte(mysekaiJSON))
	req, err := controller.BuildResourceRequest(ResourceQuery{
		Region:  "jp",
		Profile: &drawing.ProfileCardRequest{},
	})
	if err != nil {
		t.Fatalf("BuildResourceRequest() error = %v", err)
	}

	want := "asset/jp-assets/ondemand/mysekai/thumbnail/gate_large/mdl_non0006_gate_wns1.png"
	if req.GateIconPath != want {
		t.Fatalf("unexpected gate icon path: got=%q want=%q", req.GateIconPath, want)
	}
}

func TestBuildResourceRequestGateSkinOverridesGateDefaultIcon(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}

	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiGates.json"), []map[string]any{
		{"id": 1, "assetbundleName": "mdl_non0006_gate_lon1"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiGateSkins.json"), []map[string]any{
		{"id": 7, "mysekaiGateSkinType": "unit", "mysekaiGateSkinTypeId": 4},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiGateUnitSkins.json"), []map[string]any{
		{"id": 4, "assetbundleName": "mdl_non0006_gate_wns1"},
	})

	mysekaiJSON := `{
  "userMysekaiGateCharacterVisit": {
    "userMysekaiGate": {
      "mysekaiGateId": 1,
      "mysekaiGateSkinId": 7,
      "mysekaiGateLevel": 30
    }
  }
}`

	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true}).WithMySekaiData([]byte(mysekaiJSON))
	req, err := controller.BuildResourceRequest(ResourceQuery{
		Region:  "jp",
		Profile: &drawing.ProfileCardRequest{},
	})
	if err != nil {
		t.Fatalf("BuildResourceRequest() error = %v", err)
	}

	want := "asset/jp-assets/ondemand/mysekai/thumbnail/gate_large/mdl_non0006_gate_wns1.png"
	if req.GateIconPath != want {
		t.Fatalf("unexpected gate icon path with skin: got=%q want=%q", req.GateIconPath, want)
	}
}

func TestBuildDoorUpgradeRequestSupportsShowAll(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}

	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiGateMaterialGroups.json"), []map[string]any{
		{"groupId": 1001, "mysekaiMaterialId": 1, "quantity": 2},
		{"groupId": 2001, "mysekaiMaterialId": 1, "quantity": 3},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiMaterials.json"), []map[string]any{
		{"id": 1, "iconAssetbundleName": "mat_1"},
	})

	mysekaiJSON := `{
  "updatedResources": {
    "userMysekaiMaterials": [{"mysekaiMaterialId": 1, "quantity": 5}],
    "userMysekaiGates": [
      {"mysekaiGateId": 1, "mysekaiGateLevel": 5},
      {"mysekaiGateId": 2, "mysekaiGateLevel": 7}
    ]
  }
}`

	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{
		LocalDir:      masterdataDir,
		AllowFallback: true,
	}).WithMySekaiData([]byte(mysekaiJSON))

	showAll := true
	req, err := controller.BuildDoorUpgradeRequest(DoorUpgradeQuery{
		Region:  "jp",
		ShowAll: &showAll,
		Profile: &drawing.ProfileCardRequest{},
	})
	if err != nil {
		t.Fatalf("BuildDoorUpgradeRequest() error = %v", err)
	}
	if len(req.GateMaterials) != 2 {
		t.Fatalf("expected all gate materials, got %+v", req.GateMaterials)
	}
	if req.GateMaterials[0].ID != 1 || req.GateMaterials[1].ID != 2 {
		t.Fatalf("unexpected gate material order: %+v", req.GateMaterials)
	}
}

func TestBuildDoorUpgradeRequestRenamesTopSourceToSuite(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}

	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiGateMaterialGroups.json"), []map[string]any{
		{"groupId": 1001, "mysekaiMaterialId": 1, "quantity": 2},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiMaterials.json"), []map[string]any{
		{"id": 1, "iconAssetbundleName": "mat_1"},
	})

	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{
		LocalDir:      masterdataDir,
		AllowFallback: true,
	}).WithMySekaiData([]byte(`{
  "upload_time": 1776000000,
  "source": "toolbox_live",
  "updatedResources": {
    "userMysekaiMaterials": [{"mysekaiMaterialId": 1, "quantity": 5}],
    "userMysekaiGates": [{"mysekaiGateId": 1, "mysekaiGateLevel": 5}],
    "userMysekaiGamedata": {"mysekaiRank": 8}
  }
}`))

	req, err := controller.BuildDoorUpgradeRequest(DoorUpgradeQuery{
		Region: "jp",
		Profile: &drawing.ProfileCardRequest{
			Profile: &drawing.BasicProfile{
				ID:              "GAME_USER_ID_REDACTED",
				Region:          "JP",
				Nickname:        "Tester",
				LeaderImagePath: "user/leader.png",
			},
			DataSources: []drawing.ProfileDataSource{
				{Name: "Suite数据"},
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildDoorUpgradeRequest() error = %v", err)
	}
	if req.Profile == nil || len(req.Profile.DataSources) != 1 {
		t.Fatalf("expected one top data source, got %+v", req.Profile)
	}
	if req.Profile.DataSources[0].Name != "Suite数据" {
		t.Fatalf("expected top source to be renamed to Suite数据, got %+v", req.Profile.DataSources)
	}
}

func TestBuildMusicRecordRequestUsesRegionScopedMasterdata(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")

	writeTestJSON(t, filepath.Join(masterdataDir, "jp", "mysekaiMusicRecords.json"), []map[string]any{
		{"id": 596, "mysekaiMusicTrackType": "music", "externalId": 641},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "jp", "musicTags.json"), []map[string]any{
		{"id": 1, "musicId": 641, "musicTag": "light_music_club"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "jp", "musics.json"), []map[string]any{
		{"id": 641, "assetbundleName": "jacket_s_641", "publishedAt": 1},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "jp", "limitedTimeMusics.json"), []map[string]any{})

	writeTestJSON(t, filepath.Join(masterdataDir, "cn", "mysekaiMusicRecords.json"), []map[string]any{
		{"id": 15, "mysekaiMusicTrackType": "music", "externalId": 15},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "cn", "musicTags.json"), []map[string]any{
		{"id": 2, "musicId": 15, "musicTag": "light_music_club"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "cn", "musics.json"), []map[string]any{
		{"id": 15, "assetbundleName": "jacket_s_015", "publishedAt": 1},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "cn", "limitedTimeMusics.json"), []map[string]any{})

	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{
		LocalDir:      masterdataDir,
		AllowFallback: true,
	}).WithMySekaiData([]byte(`{"updatedResources":{"userMysekaiMusicRecords":[]}}`))

	req, err := controller.BuildMusicRecordRequest(MusicRecordQuery{
		Region: "cn",
		Profile: &drawing.ProfileCardRequest{
			Profile: &drawing.BasicProfile{
				ID:              "12345678901234567",
				Region:          "CN",
				Nickname:        "Tester",
				LeaderImagePath: "user/leader.png",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildMusicRecordRequest() error = %v", err)
	}
	if len(req.CategoryMusicrecords) != 1 {
		t.Fatalf("expected 1 category, got %+v", req.CategoryMusicrecords)
	}
	if len(req.CategoryMusicrecords[0].Musicrecords) != 1 {
		t.Fatalf("expected 1 music record, got %+v", req.CategoryMusicrecords[0].Musicrecords)
	}

	got := req.CategoryMusicrecords[0].Musicrecords[0].ImagePath
	want := "asset/cn-assets/startapp/music/jacket/jacket_s_015/jacket_s_015.png"
	if got != want {
		t.Fatalf("unexpected CN music record path: got=%q want=%q", got, want)
	}
	if strings.Contains(got, "641") {
		t.Fatalf("expected CN request to avoid JP-only jacket, got %q", got)
	}
}

func TestBuildResourceRequestUsesRegionScopedMasterdata(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")

	writeTestJSON(t, filepath.Join(masterdataDir, "jp", "mysekaiGates.json"), []map[string]any{
		{"id": 4, "assetbundleName": "mdl_non0006_gate_jp1"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "cn", "mysekaiGates.json"), []map[string]any{
		{"id": 4, "assetbundleName": "mdl_non0006_gate_cn1"},
	})

	mysekaiJSON := `{
  "userMysekaiGateCharacterVisit": {
    "userMysekaiGate": {
      "mysekaiGateId": 4,
      "mysekaiGateSkinId": 0,
      "mysekaiGateLevel": 30
    }
  }
}`

	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{
		LocalDir:      masterdataDir,
		AllowFallback: true,
	}).WithMySekaiData([]byte(mysekaiJSON))

	req, err := controller.BuildResourceRequest(ResourceQuery{
		Region: "cn",
		Profile: &drawing.ProfileCardRequest{
			Profile: &drawing.BasicProfile{
				ID:              "12345678901234567",
				Region:          "CN",
				Nickname:        "Tester",
				LeaderImagePath: "user/leader.png",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildResourceRequest() error = %v", err)
	}

	want := "asset/cn-assets/ondemand/mysekai/thumbnail/gate_large/mdl_non0006_gate_cn1.png"
	if req.GateIconPath != want {
		t.Fatalf("unexpected region-scoped gate icon path: got=%q want=%q", req.GateIconPath, want)
	}
	if strings.Contains(req.GateIconPath, "_jp1") {
		t.Fatalf("expected CN request to avoid JP gate assetbundle, got %q", req.GateIconPath)
	}
}

func TestResolveMysekaiMapSiteIDs(t *testing.T) {
	if got, want := resolveMysekaiMapSiteIDs(nil), []int{5, 6, 7, 8}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveMysekaiMapSiteIDs(nil) = %+v, want %+v", got, want)
	}

	if got, want := resolveMysekaiMapSiteIDs([]int{5, 7, 5, 8}), []int{5, 7, 8}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveMysekaiMapSiteIDs(valid) = %+v, want %+v", got, want)
	}

	if got, want := resolveMysekaiMapSiteIDs([]int{9, 10}), []int{}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveMysekaiMapSiteIDs(invalid) = %+v, want %+v", got, want)
	}
}

func TestBuildMapRequestHarvestPointsMatchFixtureSemantics(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}

	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiSiteHarvestFixtures.json"), []map[string]any{
		{
			"id":                                  1001,
			"assetbundleName":                     "mdl_site_wood_common_fieldtree01",
			"mysekaiSiteHarvestFixtureType":       "wood",
			"mysekaiSiteHarvestFixtureRarityType": "rarity_1",
		},
		{
			"id":                                  8001,
			"assetbundleName":                     "mdl_site_dewdrop_birthday_plant106",
			"mysekaiSiteHarvestFixtureType":       "birthday_plant",
			"mysekaiSiteHarvestFixtureRarityType": "rarity_2",
		},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacters.json"), []map[string]any{
		{"id": 6, "givenNameEnglish": "Haruka"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiMaterials.json"), []map[string]any{
		{"id": 179, "iconAssetbundleName": "birthday_drop_179", "mysekaiMaterialRarityType": "rarity_2"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiItems.json"), []map[string]any{
		{"id": 501, "iconAssetbundleName": "side_drop_501"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiMusicRecords.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(masterdataDir, "musics.json"), []map[string]any{})

	mysekaiJSON := `{
  "updatedResources": {
    "userMysekaiHarvestMaps": [
      {
        "mysekaiSiteId": 5,
        "userMysekaiSiteHarvestFixtures": [
          {
            "mysekaiSiteHarvestFixtureId": 1001,
            "userMysekaiSiteHarvestFixtureStatus": "spawned",
            "positionX": 1,
            "positionZ": 2
          },
          {
            "mysekaiSiteHarvestFixtureId": 8001,
            "userMysekaiSiteHarvestFixtureStatus": "spawned",
            "positionX": 3,
            "positionZ": 4
          }
        ],
        "userMysekaiSiteHarvestResourceDrops": [
          {
            "resourceType": "mysekai_material",
            "resourceId": 179,
            "mysekaiSiteHarvestResourceDropStatus": "before_drop",
            "positionX": 3,
            "positionZ": 4,
            "quantity": 20
          },
          {
            "resourceType": "mysekai_item",
            "resourceId": 501,
            "mysekaiSiteHarvestResourceDropStatus": "before_drop",
            "positionX": 3,
            "positionZ": 4,
            "quantity": 1
          },
          {
            "resourceType": "mysekai_item",
            "resourceId": 501,
            "mysekaiSiteHarvestResourceDropStatus": "before_drop",
            "positionX": 3,
            "positionZ": 4,
            "quantity": 2
          }
        ]
      }
    ]
  }
}`

	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true}).WithMySekaiData([]byte(mysekaiJSON))
	req, err := controller.BuildMapRequest(MapQuery{Region: "jp", MapIDs: []int{5}})
	if err != nil {
		t.Fatalf("BuildMapRequest() error = %v", err)
	}
	if len(req.Maps) != 1 {
		t.Fatalf("expected 1 map, got %d", len(req.Maps))
	}
	if len(req.Maps[0].HarvestPoints) != 2 {
		t.Fatalf("expected 2 harvest points, got %+v", req.Maps[0].HarvestPoints)
	}

	var normalPoint, birthdayPoint *drawing.MysekaiMsrMapHarvestPoint
	for i := range req.Maps[0].HarvestPoints {
		point := &req.Maps[0].HarvestPoints[i]
		if point.ID != nil && *point.ID == 1001 {
			normalPoint = point
		}
		if point.ID != nil && *point.ID == 8001 {
			birthdayPoint = point
		}
	}
	if normalPoint == nil || birthdayPoint == nil {
		t.Fatalf("missing expected harvest points: %+v", req.Maps[0].HarvestPoints)
	}

	if normalPoint.ImagePath != "static_images/mysekai/harvest_fixture_icon/rarity_1/mdl_site_wood_common_fieldtree01.png" {
		t.Fatalf("unexpected normal point image path: %q", normalPoint.ImagePath)
	}
	if normalPoint.Size != nil {
		t.Fatalf("expected normal point size nil, got %+v", normalPoint.Size)
	}
	if normalPoint.OffsetZ != -48 {
		t.Fatalf("unexpected normal point offset_z: %v", normalPoint.OffsetZ)
	}

	if birthdayPoint.ImagePath != fmt.Sprintf("static_images/mysekai/birthday/haruka_%d/icon_refresh.png", time.Now().Year()) {
		t.Fatalf("unexpected birthday point image path: %q", birthdayPoint.ImagePath)
	}
	gotFallback := "<nil>"
	if birthdayPoint.FallbackImagePath != nil {
		gotFallback = *birthdayPoint.FallbackImagePath
	}
	if gotFallback != "static_images/mysekai/harvest_fixture_icon/rarity_1/mdl_site_wood_common_fieldtree01.png" {
		t.Fatalf("unexpected birthday point fallback image path: %q", gotFallback)
	}
	if birthdayPoint.Size == nil || *birthdayPoint.Size != 50 {
		t.Fatalf("unexpected birthday point size: %+v", birthdayPoint.Size)
	}
	if birthdayPoint.OffsetX != 7.5 || birthdayPoint.OffsetZ != 0 {
		t.Fatalf("unexpected birthday point offsets: x=%v z=%v", birthdayPoint.OffsetX, birthdayPoint.OffsetZ)
	}

	if len(req.Maps[0].ResourceDrops) != 2 {
		t.Fatalf("expected 2 grouped resource drops, got %+v", req.Maps[0].ResourceDrops)
	}
	var birthdayDrop *drawing.MysekaiMsrMapResourceDrop
	var sideDrop *drawing.MysekaiMsrMapResourceDrop
	for i := range req.Maps[0].ResourceDrops {
		drop := &req.Maps[0].ResourceDrops[i]
		if drop.Type == "mysekai_material" && drop.ID == 179 {
			birthdayDrop = drop
		}
		if drop.Type == "mysekai_item" && drop.ID == 501 {
			sideDrop = drop
		}
	}
	if birthdayDrop == nil || sideDrop == nil {
		t.Fatalf("missing expected resource drops: %+v", req.Maps[0].ResourceDrops)
	}
	if birthdayDrop.Hide {
		t.Fatalf("birthday sapling drop should stay visible: %+v", birthdayDrop)
	}
	if sideDrop.Quantity != 3 {
		t.Fatalf("expected grouped side-drop quantity 3, got %+v", sideDrop)
	}
	if sideDrop.SmallIcon == nil || !*sideDrop.SmallIcon {
		t.Fatalf("expected side-drop to be rendered as small icon, got %+v", sideDrop.SmallIcon)
	}
}

func TestBuildMapRequestSkipsHarvestPointWhenStaticIconMissing(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")
	assetRoot := filepath.Join(root, "assets")
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(assetRoot, "static_images", "mysekai", "harvest_fixture_icon", "rarity_1"), 0o755); err != nil {
		t.Fatalf("mkdir static icon dir: %v", err)
	}
	// Keep one normal icon so we can assert only the missing-icon fixture is skipped.
	if err := os.WriteFile(
		filepath.Join(assetRoot, "static_images", "mysekai", "harvest_fixture_icon", "rarity_1", "mdl_site_wood_common_fieldtree01.png"),
		[]byte("ok"),
		0o644,
	); err != nil {
		t.Fatalf("write static icon: %v", err)
	}

	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiSiteHarvestFixtures.json"), []map[string]any{
		{
			"id":                                  1001,
			"assetbundleName":                     "mdl_site_wood_common_fieldtree01",
			"mysekaiSiteHarvestFixtureType":       "wood",
			"mysekaiSiteHarvestFixtureRarityType": "rarity_1",
		},
		{
			"id":                                  7001,
			"assetbundleName":                     "tone_gust",
			"mysekaiSiteHarvestFixtureType":       "tone",
			"mysekaiSiteHarvestFixtureRarityType": "rarity_2",
		},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacters.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiMaterials.json"), []map[string]any{
		{"id": 1, "iconAssetbundleName": "wood_mat", "mysekaiMaterialRarityType": "rarity_1"},
		{"id": 24, "iconAssetbundleName": "item_tone_8", "mysekaiMaterialRarityType": "rarity_1"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiItems.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiMusicRecords.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(masterdataDir, "musics.json"), []map[string]any{})

	mysekaiJSON := `{
  "updatedResources": {
    "userMysekaiHarvestMaps": [
      {
        "mysekaiSiteId": 8,
        "userMysekaiSiteHarvestFixtures": [
          {
            "mysekaiSiteHarvestFixtureId": 1001,
            "userMysekaiSiteHarvestFixtureStatus": "spawned",
            "positionX": 1,
            "positionZ": 2
          },
          {
            "mysekaiSiteHarvestFixtureId": 7001,
            "userMysekaiSiteHarvestFixtureStatus": "spawned",
            "positionX": 6,
            "positionZ": 13
          }
        ],
        "userMysekaiSiteHarvestResourceDrops": [
          {
            "resourceType": "mysekai_material",
            "resourceId": 1,
            "mysekaiSiteHarvestResourceDropStatus": "before_drop",
            "positionX": 1,
            "positionZ": 2,
            "quantity": 1
          },
          {
            "resourceType": "mysekai_material",
            "resourceId": 24,
            "mysekaiSiteHarvestResourceDropStatus": "before_drop",
            "positionX": 6,
            "positionZ": 13,
            "quantity": 1
          }
        ]
      }
    ]
  }
}`

	controller := NewController(
		nil,
		nil,
		renderregion.JP,
		assets.NewAssetHelper(assetRoot, nil),
		MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true},
	).WithMySekaiData([]byte(mysekaiJSON))
	req, err := controller.BuildMapRequest(MapQuery{Region: "jp", MapIDs: []int{8}})
	if err != nil {
		t.Fatalf("BuildMapRequest() error = %v", err)
	}
	if len(req.Maps) != 1 {
		t.Fatalf("expected 1 map, got %d", len(req.Maps))
	}

	has1001 := false
	for _, point := range req.Maps[0].HarvestPoints {
		if point.ID == nil {
			continue
		}
		if *point.ID == 7001 {
			t.Fatalf("tone fixture should be skipped when icon file is missing, got %+v", point)
		}
		if *point.ID == 1001 {
			has1001 = true
		}
	}
	if !has1001 {
		t.Fatalf("expected normal fixture point to remain, got %+v", req.Maps[0].HarvestPoints)
	}

	hasToneDrop := false
	for _, drop := range req.Maps[0].ResourceDrops {
		if drop.Type == "mysekai_material" && drop.ID == 24 {
			hasToneDrop = true
			break
		}
	}
	if !hasToneDrop {
		t.Fatalf("expected tone material drop to remain, got %+v", req.Maps[0].ResourceDrops)
	}
}

func TestBuildMapRequestSkipsToneGustHarvestPoint(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}

	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiSiteHarvestFixtures.json"), []map[string]any{
		{
			"id":                                  1001,
			"assetbundleName":                     "mdl_site_wood_common_fieldtree01",
			"mysekaiSiteHarvestFixtureType":       "wood",
			"mysekaiSiteHarvestFixtureRarityType": "rarity_1",
		},
		{
			"id":                                  9001,
			"assetbundleName":                     "mdl_site_rock_tone_gust01",
			"mysekaiSiteHarvestFixtureType":       "tone_gust",
			"mysekaiSiteHarvestFixtureRarityType": "rarity_1",
		},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacters.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiMaterials.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiItems.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiMusicRecords.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(masterdataDir, "musics.json"), []map[string]any{})

	mysekaiJSON := `{
  "updatedResources": {
    "userMysekaiHarvestMaps": [
      {
        "mysekaiSiteId": 5,
        "userMysekaiSiteHarvestFixtures": [
          {
            "mysekaiSiteHarvestFixtureId": 1001,
            "userMysekaiSiteHarvestFixtureStatus": "spawned",
            "positionX": 1,
            "positionZ": 2
          },
          {
            "mysekaiSiteHarvestFixtureId": 9001,
            "userMysekaiSiteHarvestFixtureStatus": "spawned",
            "positionX": 3,
            "positionZ": 4
          }
        ],
        "userMysekaiSiteHarvestResourceDrops": []
      }
    ]
  }
}`

	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true}).WithMySekaiData([]byte(mysekaiJSON))
	req, err := controller.BuildMapRequest(MapQuery{Region: "jp", MapIDs: []int{5}})
	if err != nil {
		t.Fatalf("BuildMapRequest() error = %v", err)
	}
	if len(req.Maps) != 1 {
		t.Fatalf("expected 1 map, got %d", len(req.Maps))
	}

	if len(req.Maps[0].HarvestPoints) != 1 {
		t.Fatalf("expected 1 harvest point after tone_gust skip, got %+v", req.Maps[0].HarvestPoints)
	}
	if req.Maps[0].HarvestPoints[0].ID == nil || *req.Maps[0].HarvestPoints[0].ID != 1001 {
		t.Fatalf("unexpected remaining harvest point: %+v", req.Maps[0].HarvestPoints[0])
	}
}

func TestBuildMapRequestMixedMaterialAndFixtureKeepsMaterialLarge(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")
	if err := os.MkdirAll(masterdataDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}

	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiSiteHarvestFixtures.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiMaterials.json"), []map[string]any{
		{"id": 1, "iconAssetbundleName": "mat_1", "mysekaiMaterialRarityType": "rarity_1"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiFixtures.json"), []map[string]any{
		{"id": 118, "assetbundleName": "mdl_site_wood_common_conifer01"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiItems.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiMusicRecords.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(masterdataDir, "musics.json"), []map[string]any{})
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacters.json"), []map[string]any{})

	mysekaiJSON := `{
  "updatedResources": {
    "userMysekaiHarvestMaps": [
      {
        "mysekaiSiteId": 5,
        "userMysekaiSiteHarvestFixtures": [],
        "userMysekaiSiteHarvestResourceDrops": [
          {
            "resourceType": "mysekai_material",
            "resourceId": 1,
            "mysekaiSiteHarvestResourceDropStatus": "before_drop",
            "positionX": 1,
            "positionZ": 2,
            "quantity": 2
          },
          {
            "resourceType": "mysekai_fixture",
            "resourceId": 118,
            "mysekaiSiteHarvestResourceDropStatus": "before_drop",
            "positionX": 1,
            "positionZ": 2,
            "quantity": 1
          }
        ]
      }
    ]
  }
}`

	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: masterdataDir, AllowFallback: true}).WithMySekaiData([]byte(mysekaiJSON))
	req, err := controller.BuildMapRequest(MapQuery{Region: "jp", MapIDs: []int{5}})
	if err != nil {
		t.Fatalf("BuildMapRequest() error = %v", err)
	}
	if len(req.Maps) != 1 {
		t.Fatalf("expected 1 map, got %d", len(req.Maps))
	}
	if len(req.Maps[0].ResourceDrops) != 2 {
		t.Fatalf("expected 2 resource drops, got %+v", req.Maps[0].ResourceDrops)
	}

	var materialDrop *drawing.MysekaiMsrMapResourceDrop
	var fixtureDrop *drawing.MysekaiMsrMapResourceDrop
	for i := range req.Maps[0].ResourceDrops {
		drop := &req.Maps[0].ResourceDrops[i]
		if drop.Type == "mysekai_material" && drop.ID == 1 {
			materialDrop = drop
		}
		if drop.Type == "mysekai_fixture" && drop.ID == 118 {
			fixtureDrop = drop
		}
	}
	if materialDrop == nil || fixtureDrop == nil {
		t.Fatalf("missing expected mixed drops: %+v", req.Maps[0].ResourceDrops)
	}
	if materialDrop.SmallIcon == nil || *materialDrop.SmallIcon {
		t.Fatalf("expected material drop to stay large, got small=%+v", materialDrop.SmallIcon)
	}
	if fixtureDrop.SmallIcon == nil || !*fixtureDrop.SmallIcon {
		t.Fatalf("expected fixture drop to be small, got small=%+v", fixtureDrop.SmallIcon)
	}
}
