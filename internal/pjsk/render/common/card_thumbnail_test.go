package common

import (
	"os"
	"path/filepath"
	"testing"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

func TestResolveCardThumbnailPathFallsBackToRipMemberAsset(t *testing.T) {
	root := t.TempDir()
	helper := assets.NewAssetHelper(root, nil)
	target := filepath.Join(root, "asset", "cn-assets", "startapp", "character", "member", "card_test_rip", "card_after_training.png")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("png"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	got := ResolveCardThumbnailPath(helper, renderregion.CN, "card_test", true)
	want := filepath.ToSlash(filepath.Join("asset", "cn-assets", "startapp", "character", "member", "card_test_rip", "card_after_training.png"))
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBuildCardThumbnailUsesRipMemberAssetWhenThumbnailMissing(t *testing.T) {
	root := t.TempDir()
	helper := assets.NewAssetHelper(root, nil)
	target := filepath.Join(root, "asset", "cn-assets", "startapp", "character", "member", "card_test_rip", "card_after_training.png")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("png"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	card := &masterdata.Card{
		ID:              1001,
		CardRarityType:  "rarity_4",
		Attr:            "cool",
		AssetBundleName: "card_test",
	}

	got := BuildCardThumbnail(helper, card, renderregion.CN, ThumbnailOptions{
		AfterTraining: true,
		TrainedArt:    true,
	})

	want := filepath.ToSlash(filepath.Join("asset", "cn-assets", "startapp", "character", "member", "card_test_rip", "card_after_training.png"))
	if got.CardThumbnailPath != want {
		t.Fatalf("expected %q, got %q", want, got.CardThumbnailPath)
	}
}

func TestResolveCardThumbnailPathSupportsOnDemandMemberAsset(t *testing.T) {
	root := t.TempDir()
	helper := assets.NewAssetHelper(root, nil)
	target := filepath.Join(root, "asset", "en-assets", "ondemand", "character", "member", "card_test", "card_after_training.png")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("png"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	got := ResolveCardThumbnailPath(helper, renderregion.EN, "card_test", true)
	want := filepath.ToSlash(filepath.Join("asset", "en-assets", "ondemand", "character", "member", "card_test", "card_after_training.png"))
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
