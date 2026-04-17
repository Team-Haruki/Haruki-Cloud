package mysekai

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
)

func TestExtractMysekaiPhenomsIncludesBirthdayRefreshSlot(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 4, 12, 16, 37, 0, 0, loc)
	phenomIcons := map[int]string{
		1: "env_sunny",
		2: "env_evening",
		3: "env_night",
		4: "env_fine",
	}

	phenoms := extractMysekaiPhenoms(renderregion.JP, func(path string) string { return path }, phenomIcons, map[string]any{
		"now": now.UnixMilli(),
		"mysekaiPhenomenaSchedules": []any{
			map[string]any{"mysekaiPhenomenaId": 1},
			map[string]any{"mysekaiPhenomenaId": 2},
			map[string]any{"mysekaiPhenomenaId": 3},
			map[string]any{"mysekaiPhenomenaId": 4},
		},
	})

	if len(phenoms) != 5 {
		t.Fatalf("expected 5 phenom cards with birthday refresh, got %+v", phenoms)
	}

	gotTexts := make([]string, 0, len(phenoms))
	for _, item := range phenoms {
		gotTexts = append(gotTexts, time.UnixMilli(item.StartAt).In(loc).Format("15:04"))
	}
	wantTexts := []string{"05:00", "17:00", "05:00", "17:00", "00:00"}
	if !reflect.DeepEqual(gotTexts, wantTexts) {
		t.Fatalf("unexpected phenom card times: got=%v want=%v", gotTexts, wantTexts)
	}

	if phenoms[4].RefreshReason != "bdend_5" {
		t.Fatalf("unexpected birthday refresh reason: %+v", phenoms[4])
	}
	if phenoms[4].ImagePath != "thumbnail/material/material178.png" {
		t.Fatalf("unexpected birthday refresh image path: %+v", phenoms[4])
	}
}

func TestExtractMysekaiPhenomsPrefersFreshestSnapshotTime(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	staleNow := time.Date(2026, 4, 12, 16, 37, 0, 0, loc)
	freshUpload := time.Date(2026, 4, 14, 16, 1, 0, 0, loc)
	phenomIcons := map[int]string{
		1: "env_sunny",
		2: "env_evening",
		3: "env_night",
		4: "env_fine",
	}

	phenoms := extractMysekaiPhenoms(renderregion.JP, func(path string) string { return path }, phenomIcons, map[string]any{
		"now":         staleNow.UnixMilli(),
		"upload_time": freshUpload.UnixMilli(),
		"updatedResources": map[string]any{
			"now": staleNow.UnixMilli(),
		},
		"mysekaiPhenomenaSchedules": []any{
			map[string]any{"mysekaiPhenomenaId": 1},
			map[string]any{"mysekaiPhenomenaId": 2},
			map[string]any{"mysekaiPhenomenaId": 3},
			map[string]any{"mysekaiPhenomenaId": 4},
		},
	})

	if len(phenoms) != 4 {
		t.Fatalf("expected stale birthday slot to be suppressed, got %+v", phenoms)
	}

	gotTexts := make([]string, 0, len(phenoms))
	for _, item := range phenoms {
		gotTexts = append(gotTexts, time.UnixMilli(item.StartAt).In(loc).Format("15:04"))
	}
	wantTexts := []string{"05:00", "17:00", "05:00", "17:00"}
	if !reflect.DeepEqual(gotTexts, wantTexts) {
		t.Fatalf("unexpected phenom card times with fresh upload: got=%v want=%v", gotTexts, wantTexts)
	}
}

func TestExtractMysekaiPhenomsUsesMasterdataIconName(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 4, 16, 16, 37, 0, 0, loc)
	phenomIcons := map[int]string{
		17: "env_rainbow",
	}

	phenoms := extractMysekaiPhenoms(renderregion.JP, func(path string) string { return path }, phenomIcons, map[string]any{
		"now": now.UnixMilli(),
		"mysekaiPhenomenaSchedules": []any{
			map[string]any{"mysekaiPhenomenaId": 17},
			map[string]any{"mysekaiPhenomenaId": 1},
			map[string]any{"mysekaiPhenomenaId": 1},
			map[string]any{"mysekaiPhenomenaId": 1},
		},
	})

	if len(phenoms) == 0 {
		t.Fatal("expected phenom cards")
	}
	if got, want := phenoms[0].ImagePath, "mysekai/thumbnail/phenomena/env_rainbow.png"; got != want {
		t.Fatalf("unexpected icon path: got=%q want=%q", got, want)
	}
}

func TestSortKeysByResourceMovesRareEntriesToFront(t *testing.T) {
	counts := map[string]int{
		"material_1":             485,
		"mysekai_material_1":     9,
		"material_179":           1,
		"mysekai_material_24":    1,
		"mysekai_music_record_1": 1,
	}
	materialRarityMap := map[int]string{
		1:  "rarity_1",
		24: "rarity_3",
	}

	got := sortKeysByResource(counts, materialRarityMap)
	want := []string{
		"material_179",
		"mysekai_material_24",
		"mysekai_music_record_1",
		"material_1",
		"mysekai_material_1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected resource order: got=%v want=%v", got, want)
	}

	if rarity := resourceRarity("material_179", materialRarityMap); rarity != 2 {
		t.Fatalf("expected birthday material rarity 2, got %d", rarity)
	}
}

func TestResourceImagePathFallsBackToMaterialRip(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "jp-assets", "startapp", "thumbnail", "material_rip", "material179.png")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir asset dir: %v", err)
	}
	if err := os.WriteFile(target, []byte("png"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	controller := NewController(nil, nil, renderregion.JP, assets.NewAssetHelper(root, nil), MasterdataOptions{})
	got, hasRecord := controller.resourceImagePath(renderregion.JP, "material_179", nil, nil, nil, nil, nil)
	want := "asset/jp-assets/startapp/thumbnail/material_rip/material179.png"
	if got != want {
		t.Fatalf("unexpected material_rip fallback path: got=%q want=%q", got, want)
	}
	if hasRecord {
		t.Fatalf("material icon should not be marked as music record")
	}
}
