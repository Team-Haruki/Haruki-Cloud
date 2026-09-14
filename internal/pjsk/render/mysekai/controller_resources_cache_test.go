package mysekai

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestMysekaiBirthdayIconCandidates(t *testing.T) {
	now := time.Date(2026, time.July, 15, 0, 0, 0, 0, time.UTC)
	icon := func(region string, year int) string {
		return fmt.Sprintf("asset/%s-assets/ondemand/mysekai/birthday/haruka_%d/icon_refresh.png", region, year)
	}
	// Frozen order [Y, Y-1, Y-2, Y+1] (addendum A4).
	want := []string{icon("jp", 2026), icon("jp", 2025), icon("jp", 2024), icon("jp", 2027)}
	if got := mysekaiBirthdayIconCandidates("jp", "haruka", now); !slices.Equal(got, want) {
		t.Fatalf("jp candidates = %v, want %v", got, want)
	}
	wantTW := []string{icon("tw", 2026), icon("tw", 2025), icon("tw", 2024), icon("tw", 2027)}
	if got := mysekaiBirthdayIconCandidates("tw", " haruka ", now); !slices.Equal(got, wantTW) {
		t.Fatalf("tw candidates = %v, want %v", got, wantTW)
	}
	// The window follows the clock only; nothing on disk is consulted.
	newYear := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)
	if got := mysekaiBirthdayIconCandidates("jp", "haruka", newYear); got[0] != icon("jp", 2027) || got[3] != icon("jp", 2028) {
		t.Fatalf("new year candidates = %v", got)
	}
	if got := mysekaiBirthdayIconCandidates("jp", "  ", now); got != nil {
		t.Fatalf("blank image name candidates = %v, want nil", got)
	}
}

func TestMysekaiHarvestPointImageSendsCandidatesAndFallback(t *testing.T) {
	controller := &Controller{}
	characters := map[int]map[string]any{21: {"givenNameEnglish": "Haruka"}}
	birthdays := map[string]int{mysekaiHarvestPosKey(3, 4): 21}
	year := time.Now().Year()
	wantFallback := "static_images/mysekai/harvest_fixture_icon/rarity_1/mdl_site_wood_common_fieldtree01.png"

	image, fallback, size, offsetX, offsetZ := controller.mysekaiHarvestPointImage(renderregion.JP, "birthday_plant", "rarity_2", "plant", 3, 4, birthdays, characters)
	want := drawing.AssetKey(mysekaiBirthdayIconCandidates("jp", "haruka", time.Now()))
	if !slices.Equal(image, want) || len(image) != 4 || image.First() != fmt.Sprintf("asset/jp-assets/ondemand/mysekai/birthday/haruka_%d/icon_refresh.png", year) {
		t.Fatalf("birthday candidates = %v, want %v", image, want)
	}
	if fallback == nil || *fallback != wantFallback || size == nil || *size != 50 || offsetX != 7.5 || offsetZ != 0 {
		t.Fatalf("birthday fallback = %v size = %v offsets = %v,%v", fallback, size, offsetX, offsetZ)
	}
	point := drawing.MysekaiMsrMapHarvestPoint{ImagePath: image, FallbackImagePath: fallback}
	body, err := json.Marshal(point)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		ImagePath []string `json:"image_path"`
		Fallback  string   `json:"fallback_image_path"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil || !slices.Equal(decoded.ImagePath, want) || decoded.Fallback != wantFallback {
		t.Fatalf("wire payload = %s (%v)", body, err)
	}

	// No birthday character at this position: today's single static path.
	image, fallback, _, _, _ = controller.mysekaiHarvestPointImage(renderregion.JP, "birthday_plant", "rarity_2", "plant", 9, 9, birthdays, characters)
	if !slices.Equal(image, drawing.AssetKey{"static_images/mysekai/harvest_fixture_icon/rarity_2/plant.png"}) || fallback == nil {
		t.Fatalf("birthday plant without character = %v, %v", image, fallback)
	}
	// A character without an English given name has no candidates either.
	image, _, _, _, _ = controller.mysekaiHarvestPointImage(renderregion.JP, "birthday_plant", "rarity_2", "plant", 3, 4, birthdays, map[int]map[string]any{})
	if image.First() != "static_images/mysekai/harvest_fixture_icon/rarity_2/plant.png" || len(image) != 1 {
		t.Fatalf("birthday plant without name = %v", image)
	}
}
