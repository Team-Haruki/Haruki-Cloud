package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRenderProviderMasterdataDir(t *testing.T) {
	t.Run("prefers local masterdata", func(t *testing.T) {
		root := t.TempDir()
		masterdataRoot := filepath.Join(root, "render-masterdata")
		for _, region := range []string{"jp", "cn"} {
			if err := os.MkdirAll(filepath.Join(masterdataRoot, region), 0o755); err != nil {
				t.Fatalf("mkdir region dir: %v", err)
			}
		}
		cfg := Config{
			LocalMasterdata: LocalMasterdataConfig{Dir: " " + masterdataRoot + " "},
			DeckRecommend:   DeckRecommendConfig{MasterdataDir: "/srv/deck-masterdata"},
		}

		if got := resolveRenderProviderMasterdataDir(cfg); got != masterdataRoot {
			t.Fatalf("expected local masterdata dir, got %q", got)
		}
	})

	t.Run("falls back to deck masterdata", func(t *testing.T) {
		root := t.TempDir()
		masterdataRoot := filepath.Join(root, "deck-masterdata")
		for _, region := range []string{"jp", "cn"} {
			if err := os.MkdirAll(filepath.Join(masterdataRoot, region), 0o755); err != nil {
				t.Fatalf("mkdir region dir: %v", err)
			}
		}
		cfg := Config{
			LocalMasterdata: LocalMasterdataConfig{Dir: "   "},
			DeckRecommend:   DeckRecommendConfig{MasterdataDir: " " + masterdataRoot + " "},
		}

		if got := resolveRenderProviderMasterdataDir(cfg); got != masterdataRoot {
			t.Fatalf("expected deck masterdata dir fallback, got %q", got)
		}
	})

	t.Run("prefers region root over broken jp-only config", func(t *testing.T) {
		wd := t.TempDir()
		deckRoot := filepath.Join(wd, "deckrec", "masterdata")
		for _, region := range []string{"jp", "cn", "tw", "kr", "en"} {
			if err := os.MkdirAll(filepath.Join(deckRoot, region), 0o755); err != nil {
				t.Fatalf("mkdir deck region dir: %v", err)
			}
		}

		cfg := Config{
			LocalMasterdata: LocalMasterdataConfig{Dir: "/masterdata/jp"},
		}

		if got := resolveRenderProviderMasterdataDirFromWD(cfg, wd); got != deckRoot {
			t.Fatalf("expected deckrec region root fallback, got %q", got)
		}
	})

	t.Run("returns empty when both are unset", func(t *testing.T) {
		if got := resolveRenderProviderMasterdataDirFromWD(Config{}, t.TempDir()); got != "" {
			t.Fatalf("expected empty masterdata dir, got %q", got)
		}
	})
}
