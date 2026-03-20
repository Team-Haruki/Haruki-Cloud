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
