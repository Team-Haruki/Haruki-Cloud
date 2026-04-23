package music

import (
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestBuildMusicChartRequestUsesConfiguredStylePath(t *testing.T) {
	source := &vocalBuilderTestSource{
		music: &masterdata.Music{
			ID:              199,
			Title:           "test chart",
			Composer:        "composer",
			Arranger:        "arranger",
			AssetBundleName: "jacket_s_199",
		},
		difficulties: []*masterdata.MusicDifficulty{
			{MusicID: 199, MusicDifficulty: "expert", PlayLevel: 27},
		},
	}

	builder := NewBuilder(source, nil, assets.NewAssetHelper("", nil))

	tests := []struct {
		name      string
		style     string
		wantStyle string
	}{
		{
			name:      "explicit white style",
			style:     "white",
			wantStyle: "static_images/chart_asset/css/white.css",
		},
		{
			name:      "default black style",
			style:     "",
			wantStyle: "static_images/chart_asset/css/black.css",
		},
		{
			name:      "invalid style falls back to black",
			style:     "blue",
			wantStyle: "static_images/chart_asset/css/black.css",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := builder.BuildMusicChartRequest(ChartQuery{
				Query:      "music199",
				Region:     "jp",
				Difficulty: "expert",
				Style:      tt.style,
			}, source.music, renderregion.JP)
			if err != nil {
				t.Fatalf("BuildMusicChartRequest() error = %v", err)
			}
			if req.StylePath == nil {
				t.Fatal("expected style path to be set")
			}
			if got := *req.StylePath; got != tt.wantStyle {
				t.Fatalf("StylePath = %q, want %q", got, tt.wantStyle)
			}
		})
	}
}
