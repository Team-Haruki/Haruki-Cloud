package mysekai

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
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
	return NewController(nil, service, "", renderregion.JP)
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
