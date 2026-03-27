package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAssetHelperFirstExistingFallsBackToLegacyRoots(t *testing.T) {
	tmpDir := t.TempDir()
	primary := filepath.Join(tmpDir, "primary")
	legacy := filepath.Join(tmpDir, "legacy")

	if err := os.MkdirAll(primary, 0o755); err != nil {
		t.Fatalf("mkdir primary: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(legacy, "icons"), 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	target := filepath.Join(legacy, "icons", "card.png")
	if err := os.WriteFile(target, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	helper := NewAssetHelper(primary, []string{legacy})
	got := helper.FirstExisting("icons/card.png")
	if got != filepath.ToSlash(target) {
		t.Fatalf("expected %q, got %q", filepath.ToSlash(target), got)
	}
}

func TestResolveAssetPathFallsBackToPrimaryPath(t *testing.T) {
	helper := NewAssetHelper("/srv/assets", []string{"/srv/assets-legacy"})
	got := ResolveAssetPath(helper, "", "cards/1.webp")
	if want := "/srv/assets/cards/1.webp"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveAssetPathSupportsURLRoots(t *testing.T) {
	helper := NewAssetHelper("https://sekai-assets.haruki.seiunx.com/jp-assets", nil)
	got := ResolveAssetPath(helper, "", "music/jacket/jacket_s_001/jacket_s_001.png")
	want := "https://sekai-assets.haruki.seiunx.com/jp-assets/music/jacket/jacket_s_001/jacket_s_001.png"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestMakeRelativeSupportsURLRoots(t *testing.T) {
	base := "https://sekai-assets.haruki.seiunx.com/jp-assets"
	target := "https://sekai-assets.haruki.seiunx.com/jp-assets/music/jacket/jacket_s_001/jacket_s_001.png"
	got := MakeRelative(base, target)
	want := "music/jacket/jacket_s_001/jacket_s_001.png"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveRegionAssetPathFallsBackToOnDemand(t *testing.T) {
	tmpDir := t.TempDir()
	helper := NewAssetHelper(tmpDir, nil)

	rel := filepath.Join("asset", "jp-assets", "ondemand", "gacha", "ab_gacha_1", "logo", "logo.png")
	full := filepath.Join(tmpDir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got := ResolveRegionAssetPath(helper, "jp", filepath.Join("gacha", "ab_gacha_1", "logo", "logo.png"))
	if want := filepath.ToSlash(full); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveRegionAssetPathPrefersStartAppOverOnDemand(t *testing.T) {
	tmpDir := t.TempDir()
	helper := NewAssetHelper(tmpDir, nil)
	rel := filepath.Join("honor", "honor_6881", "degree_sub.png")

	startApp := filepath.Join(tmpDir, "asset", "jp-assets", "startapp", rel)
	if err := os.MkdirAll(filepath.Dir(startApp), 0o755); err != nil {
		t.Fatalf("mkdir startapp: %v", err)
	}
	if err := os.WriteFile(startApp, []byte("startapp"), 0o644); err != nil {
		t.Fatalf("write startapp file: %v", err)
	}

	onDemand := filepath.Join(tmpDir, "asset", "jp-assets", "ondemand", rel)
	if err := os.MkdirAll(filepath.Dir(onDemand), 0o755); err != nil {
		t.Fatalf("mkdir ondemand: %v", err)
	}
	if err := os.WriteFile(onDemand, []byte("ondemand"), 0o644); err != nil {
		t.Fatalf("write ondemand file: %v", err)
	}

	got := ResolveRegionAssetPath(helper, "jp", rel)
	if want := filepath.ToSlash(startApp); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveRegionAssetPathPrefersOnDemandForGacha(t *testing.T) {
	tmpDir := t.TempDir()
	helper := NewAssetHelper(tmpDir, nil)
	rel := filepath.Join("gacha", "ab_gacha_1", "logo", "logo.png")

	startApp := filepath.Join(tmpDir, "asset", "jp-assets", "startapp", rel)
	if err := os.MkdirAll(filepath.Dir(startApp), 0o755); err != nil {
		t.Fatalf("mkdir startapp: %v", err)
	}
	if err := os.WriteFile(startApp, []byte("startapp"), 0o644); err != nil {
		t.Fatalf("write startapp file: %v", err)
	}

	onDemand := filepath.Join(tmpDir, "asset", "jp-assets", "ondemand", rel)
	if err := os.MkdirAll(filepath.Dir(onDemand), 0o755); err != nil {
		t.Fatalf("mkdir ondemand: %v", err)
	}
	if err := os.WriteFile(onDemand, []byte("ondemand"), 0o644); err != nil {
		t.Fatalf("write ondemand file: %v", err)
	}

	got := ResolveRegionAssetPath(helper, "jp", rel)
	if want := filepath.ToSlash(onDemand); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveRegionAssetPathPrefersStartAppForBondsHonor(t *testing.T) {
	tmpDir := t.TempDir()
	helper := NewAssetHelper(tmpDir, nil)
	rel := filepath.Join("bonds_honor", "character", "chr_sd_11_01.png")

	startApp := filepath.Join(tmpDir, "asset", "jp-assets", "startapp", rel)
	if err := os.MkdirAll(filepath.Dir(startApp), 0o755); err != nil {
		t.Fatalf("mkdir startapp: %v", err)
	}
	if err := os.WriteFile(startApp, []byte("startapp"), 0o644); err != nil {
		t.Fatalf("write startapp file: %v", err)
	}

	onDemand := filepath.Join(tmpDir, "asset", "jp-assets", "ondemand", rel)
	if err := os.MkdirAll(filepath.Dir(onDemand), 0o755); err != nil {
		t.Fatalf("mkdir ondemand: %v", err)
	}
	if err := os.WriteFile(onDemand, []byte("ondemand"), 0o644); err != nil {
		t.Fatalf("write ondemand file: %v", err)
	}

	got := ResolveRegionAssetPath(helper, "jp", rel)
	if want := filepath.ToSlash(startApp); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
