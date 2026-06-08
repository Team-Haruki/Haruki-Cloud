package server

import (
	"testing"

	harukiConfig "haruki-cloud/config"
)

func TestResolveDeckRecommendMasterdataDirPrefersDeckRecommend(t *testing.T) {
	original := harukiConfig.Cfg
	t.Cleanup(func() {
		harukiConfig.Cfg = original
	})

	harukiConfig.Cfg.PJSKRender.LocalMasterdata.Dir = "/masterdata/jp"
	harukiConfig.Cfg.PJSKRender.DeckRecommend.MasterdataDir = "/data/deck-masterdata"

	if got := resolveDeckRecommendMasterdataDir(); got != "/data/deck-masterdata" {
		t.Fatalf("expected deck recommend masterdata dir, got %q", got)
	}
}

func TestResolveDeckRecommendMasterdataDirDoesNotFallbackToLocalMasterdata(t *testing.T) {
	original := harukiConfig.Cfg
	t.Cleanup(func() {
		harukiConfig.Cfg = original
	})

	harukiConfig.Cfg.PJSKRender.LocalMasterdata.Dir = "/masterdata/jp"
	harukiConfig.Cfg.PJSKRender.DeckRecommend.MasterdataDir = "   "

	if got := resolveDeckRecommendMasterdataDir(); got != "" {
		t.Fatalf("expected empty deck masterdata dir without explicit config, got %q", got)
	}
}
