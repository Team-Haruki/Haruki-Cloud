package mysekai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
