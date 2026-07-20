package music

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestFindMusicChartsByBPMTracesScanAndFileStages(t *testing.T) {
	root := t.TempDir()
	chartPath := filepath.Join(root, "music", "music_score", "0001_01", "expert.txt")
	if err := os.MkdirAll(filepath.Dir(chartPath), 0o755); err != nil {
		t.Fatalf("mkdir chart: %v", err)
	}
	if err := os.WriteFile(chartPath, []byte(strings.Join([]string{
		"#BPM01:200",
		"#00008:0100",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write chart: %v", err)
	}

	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_a"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {{MusicID: 1, MusicDifficulty: "expert"}},
		},
	}
	ctx, trace := commandtrace.WithTrace(context.Background())
	controller := NewController(source, nil, assets.NewAssetHelper(root, nil), nil, nil).WithContext(ctx)
	if _, err := controller.FindMusicChartsByBPM(BPMQuery{Region: "jp", BPM: 200}); err != nil {
		t.Fatalf("FindMusicChartsByBPM() error = %v", err)
	}

	operations := make(map[string]int)
	for _, operation := range trace.Snapshot().Operations {
		operations[operation.Name] = operation.Count
	}
	for _, name := range []string{"music.bpm_lookup", "music.chart_scan", "music.chart_read", "music.chart_parse"} {
		if operations[name] != 1 {
			t.Fatalf("%s count = %d, operations=%+v", name, operations[name], trace.Snapshot().Operations)
		}
	}
}

func TestFindMusicChartsByBPMHonorsRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	controller := NewController(&lookupTestSource{}, nil, assets.NewAssetHelper("", nil), nil, nil).WithContext(ctx)
	_, err := controller.FindMusicChartsByBPM(BPMQuery{Region: "jp", BPM: 200})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("FindMusicChartsByBPM() error = %v, want context.Canceled", err)
	}
}
