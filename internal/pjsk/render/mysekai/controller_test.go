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

	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
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
	return NewController(nil, service, "", renderregion.JP, nil)
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
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiFixtures.json"), []map[string]interface{}{
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
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiFixtureMainGenres.json"), []map[string]interface{}{
		{"id": 1, "name": "Main A", "assetbundleName": "main_a"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiFixtureSubGenres.json"), []map[string]interface{}{
		{"id": 11, "name": "Sub A", "assetbundleName": "sub_a"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiBlueprints.json"), []map[string]interface{}{
		{"id": 1001, "mysekaiCraftType": "mysekai_fixture", "craftTargetId": 2001},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacters.json"), []map[string]interface{}{})

	service := userdata.NewLocalFileService(nil, nil, userdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userPath,
		MySekaiJSON:   mysekaiPath,
	})
	controller := NewController(nil, service, masterdataDir, renderregion.JP, nil)

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

func TestResolveTalkCharacterHandlesVirtualSingerUnits(t *testing.T) {
	root := t.TempDir()
	masterdataDir := filepath.Join(root, "masterdata")
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacters.json"), []map[string]interface{}{
		{"id": 1, "firstName": "星乃", "givenName": "一歌", "firstNameEnglish": "Hoshino", "givenNameEnglish": "Ichika"},
		{"id": 21, "firstName": "初音", "givenName": "未来", "firstNameEnglish": "Hatsune", "givenNameEnglish": "Miku"},
		{"id": 22, "firstName": "镜音", "givenName": "铃", "firstNameEnglish": "Kagamine", "givenNameEnglish": "Rin"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacterUnits.json"), []map[string]interface{}{
		{"id": 1, "gameCharacterId": 1, "unit": "light_sound"},
		{"id": 21, "gameCharacterId": 21, "unit": "piapro"},
		{"id": 27, "gameCharacterId": 21, "unit": "light_sound"},
		{"id": 28, "gameCharacterId": 21, "unit": "idol"},
		{"id": 32, "gameCharacterId": 22, "unit": "piapro"},
		{"id": 33, "gameCharacterId": 22, "unit": "light_sound"},
		{"id": 34, "gameCharacterId": 22, "unit": "idol"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiGateCharacterLotteries.json"), []map[string]interface{}{
		{"id": 1, "gameCharacterUnitId": 27},
		{"id": 2, "gameCharacterUnitId": 28},
		{"id": 3, "gameCharacterUnitId": 34},
	})

	controller := NewController(nil, nil, masterdataDir, renderregion.JP, nil)

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

func writeTestJSON(t *testing.T, path string, value interface{}) {
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

	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiSiteHarvestFixtures.json"), []map[string]interface{}{
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
	writeTestJSON(t, filepath.Join(masterdataDir, "gameCharacters.json"), []map[string]interface{}{
		{"id": 6, "givenNameEnglish": "Haruka"},
	})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiMaterials.json"), []map[string]interface{}{})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiItems.json"), []map[string]interface{}{})
	writeTestJSON(t, filepath.Join(masterdataDir, "mysekaiMusicRecords.json"), []map[string]interface{}{})
	writeTestJSON(t, filepath.Join(masterdataDir, "musics.json"), []map[string]interface{}{})

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
          }
        ]
      }
    ]
  }
}`

	controller := NewController(nil, nil, masterdataDir, renderregion.JP, nil).WithMySekaiData([]byte(mysekaiJSON))
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
	if birthdayPoint.Size == nil || *birthdayPoint.Size != 50 {
		t.Fatalf("unexpected birthday point size: %+v", birthdayPoint.Size)
	}
	if birthdayPoint.OffsetX != 7.5 || birthdayPoint.OffsetZ != 0 {
		t.Fatalf("unexpected birthday point offsets: x=%v z=%v", birthdayPoint.OffsetX, birthdayPoint.OffsetZ)
	}
}
