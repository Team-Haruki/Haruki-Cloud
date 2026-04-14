package app

import "testing"

func TestResolveRenderProviderMasterdataDir(t *testing.T) {
	t.Run("prefers local masterdata", func(t *testing.T) {
		cfg := Config{
			LocalMasterdata: LocalMasterdataConfig{Dir: " /srv/render-masterdata "},
			DeckRecommend:   DeckRecommendConfig{MasterdataDir: "/srv/deck-masterdata"},
		}

		if got := resolveRenderProviderMasterdataDir(cfg); got != "/srv/render-masterdata" {
			t.Fatalf("expected local masterdata dir, got %q", got)
		}
	})

	t.Run("falls back to deck masterdata", func(t *testing.T) {
		cfg := Config{
			LocalMasterdata: LocalMasterdataConfig{Dir: "   "},
			DeckRecommend:   DeckRecommendConfig{MasterdataDir: " /srv/deck-masterdata "},
		}

		if got := resolveRenderProviderMasterdataDir(cfg); got != "/srv/deck-masterdata" {
			t.Fatalf("expected deck masterdata dir fallback, got %q", got)
		}
	})

	t.Run("returns empty when both are unset", func(t *testing.T) {
		if got := resolveRenderProviderMasterdataDir(Config{}); got != "" {
			t.Fatalf("expected empty masterdata dir, got %q", got)
		}
	})
}
