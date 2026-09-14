package honor

import (
	"context"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestResolveHonorLevelVisualFallbacks(t *testing.T) {
	levels := []masterdata.HonorLevel{
		{Level: 1},
		{Level: 2, AssetBundleName: "level_two"},
		{Level: 4, HonorRarity: "highest"},
	}

	tests := []struct {
		name      string
		requested int
		wantLevel int
	}{
		{name: "exact", requested: 4, wantLevel: 4},
		{name: "best at or below", requested: 3, wantLevel: 2},
		{name: "first usable for zero", requested: 0, wantLevel: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := resolveHonorLevelVisual(levels, tt.requested)
			if !ok || got.Level != tt.wantLevel {
				t.Fatalf("resolveHonorLevelVisual(%d) = %#v, %v; want level %d", tt.requested, got, ok, tt.wantLevel)
			}
		})
	}

	if got, ok := resolveHonorLevelVisual([]masterdata.HonorLevel{{Level: 1}}, 1); ok || got != nil {
		t.Fatalf("expected no usable level, got %#v, %v", got, ok)
	}
}

func TestHonorPureHelpers(t *testing.T) {
	if difficulty, score, ok := LookupFcApCounter(3009); !ok || difficulty != "easy" || score != "fullCombo" {
		t.Fatalf("unexpected counter lookup: %q, %q, %v", difficulty, score, ok)
	}
	if difficulty, score, ok := LookupFcApCounter(-1); ok || difficulty != "" || score != "" {
		t.Fatalf("unexpected missing counter lookup: %q, %q, %v", difficulty, score, ok)
	}
	if got := absInt(-7); got != 7 {
		t.Fatalf("absInt(-7) = %d", got)
	}
	if got := absInt(7); got != 7 {
		t.Fatalf("absInt(7) = %d", got)
	}

	for input, want := range map[string]string{
		" honor_top_event_123 ": "honor_bg_123",
		"honor_top_invalid":     "",
		"other_event_123":       "",
	} {
		if got := deriveHonorBackgroundAssetName(input); got != want {
			t.Errorf("deriveHonorBackgroundAssetName(%q) = %q; want %q", input, got, want)
		}
	}
}

func TestControllerNilAndContextBranches(t *testing.T) {
	var nilController *Controller
	if got := nilController.WithContext(context.Background()); got != nil {
		t.Fatalf("nil controller WithContext returned %#v", got)
	}

	source := newTestHonorSource(renderregion.JP)
	controller := NewController(source, nil, nil)
	clone := controller.WithContext(context.Background())
	if clone == nil || clone == controller || clone.assets == nil {
		t.Fatalf("unexpected contextual clone: %#v", clone)
	}
	if _, err := controller.RenderHonor(Query{}); err == nil {
		t.Fatal("expected missing drawing client error")
	}

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	if builder.assetExists(" ") {
		t.Fatal("empty asset path unexpectedly exists")
	}
	builder.assets = nil
	if builder.assetExists("honor/example.png") {
		t.Fatal("asset unexpectedly exists without an asset helper")
	}
}
