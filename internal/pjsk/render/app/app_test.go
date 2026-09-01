package app

import (
	"haruki-cloud/internal/testutil"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRenderProviderMasterdataDir(t *testing.T) {
	t.Run("prefers local masterdata", func(t *testing.T) {
		root := t.TempDir()
		masterdataRoot := filepath.Join(root, "render-masterdata")
		for _, region := range []string{"jp", "cn"} {
			{
				err := os.MkdirAll(filepath.Join(masterdataRoot, region), 0o755)
				testutil.Require(t, !(err != nil), "mkdir region dir: %v", err)
			}

		}
		cfg := Config{
			LocalMasterdata: LocalMasterdataConfig{Enabled: true, AllowFallback: true, Dir: " " + masterdataRoot + " "},
			DeckRecommend:   DeckRecommendConfig{MasterdataDir: "/srv/deck-masterdata"},
		}
		{

			got := resolveRenderProviderMasterdataDir(cfg)
			testutil.Require(t, !(got != masterdataRoot), "expected local masterdata dir, got %q", got)
		}

	})

	t.Run("does not fall back to deck masterdata", func(t *testing.T) {
		root := t.TempDir()
		masterdataRoot := filepath.Join(root, "deck-masterdata")
		for _, region := range []string{"jp", "cn"} {
			{
				err := os.MkdirAll(filepath.Join(masterdataRoot, region), 0o755)
				testutil.Require(t, !(err != nil), "mkdir region dir: %v", err)
			}

		}
		cfg := Config{
			LocalMasterdata: LocalMasterdataConfig{Enabled: true, AllowFallback: true, Dir: "   "},
			DeckRecommend:   DeckRecommendConfig{MasterdataDir: " " + masterdataRoot + " "},
		}
		{

			got := resolveRenderProviderMasterdataDir(cfg)
			testutil.Require(t, !(got != ""), "expected no deck masterdata fallback, got %q", got)
		}

	})

	t.Run("does not scan working directory for fallback roots", func(t *testing.T) {
		wd := t.TempDir()
		deckRoot := filepath.Join(wd, "deckrec", "masterdata")
		for _, region := range []string{"jp", "cn", "tw", "kr", "en"} {
			{
				err := os.MkdirAll(filepath.Join(deckRoot, region), 0o755)
				testutil.Require(t, !(err != nil), "mkdir deck region dir: %v", err)
			}

		}

		cfg := Config{
			LocalMasterdata: LocalMasterdataConfig{Enabled: true, AllowFallback: true, Dir: "/masterdata/jp"},
		}
		{

			got := resolveRenderProviderMasterdataDirFromWD(cfg, wd)
			testutil.Require(t, !(got != ""), "expected no working-directory fallback, got %q", got)
		}

	})

	t.Run("detects mounted multi-repo masterdata root from broken region config", func(t *testing.T) {
		root := t.TempDir()
		repoRoot := filepath.Join(root, "masterdata")
		for _, repoDir := range []string{
			"haruki-sekai-master",
			"haruki-sekai-sc-master",
			"haruki-sekai-tc-master",
			"haruki-sekai-kr-master",
			"haruki-sekai-en-master",
		} {
			masterDir := filepath.Join(repoRoot, repoDir, "master")
			{
				err := os.MkdirAll(masterDir, 0o755)
				testutil.Require(t, !(err != nil), "mkdir repo master dir: %v", err)
			}
			{

				err := os.WriteFile(filepath.Join(masterDir, "resourceBoxes.json"), []byte(`[]`), 0o644)
				testutil.Require(t, !(err != nil), "write repo resourceBoxes.json: %v", err)
			}

		}

		cfg := Config{
			LocalMasterdata: LocalMasterdataConfig{Enabled: true, AllowFallback: true, Dir: filepath.Join(repoRoot, "jp")},
		}
		{

			got := resolveRenderProviderMasterdataDirFromWD(cfg, root)
			testutil.Require(t, !(got != repoRoot), "expected repo-root masterdata dir, got %q", got)
		}

	})

	t.Run("accepts inventory-only masterdata files", func(t *testing.T) {
		root := t.TempDir()
		masterdataRoot := filepath.Join(root, "inventory-masterdata")
		{
			err := os.MkdirAll(masterdataRoot, 0o755)
			testutil.Require(t, !(err != nil), "mkdir inventory masterdata root: %v", err)
		}
		{

			err := os.WriteFile(filepath.Join(masterdataRoot, "materials.json"), []byte(`[]`), 0o644)
			testutil.Require(t, !(err != nil), "write materials.json: %v", err)
		}

		cfg := Config{
			LocalMasterdata: LocalMasterdataConfig{Enabled: true, AllowFallback: true, Dir: masterdataRoot},
		}
		{

			got := resolveRenderProviderMasterdataDir(cfg)
			testutil.Require(t, !(got != masterdataRoot), "expected inventory masterdata dir, got %q", got)
		}

	})

	t.Run("returns empty when both are unset", func(t *testing.T) {
		{
			got := resolveRenderProviderMasterdataDirFromWD(Config{}, t.TempDir())
			testutil.Require(t, !(got != ""), "expected empty masterdata dir, got %q", got)
		}

	})

	t.Run("returns empty when local fallback is disabled", func(t *testing.T) {
		root := t.TempDir()
		masterdataRoot := filepath.Join(root, "render-masterdata")
		for _, region := range []string{"jp", "cn"} {
			{
				err := os.MkdirAll(filepath.Join(masterdataRoot, region), 0o755)
				testutil.Require(t, !(err != nil), "mkdir region dir: %v", err)
			}

		}
		cfg := Config{
			LocalMasterdata: LocalMasterdataConfig{Dir: masterdataRoot},
		}
		{

			got := resolveRenderProviderMasterdataDir(cfg)
			testutil.Require(t, !(got != ""), "expected disabled local masterdata fallback, got %q", got)
		}

	})
}
