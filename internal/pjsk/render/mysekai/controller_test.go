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
