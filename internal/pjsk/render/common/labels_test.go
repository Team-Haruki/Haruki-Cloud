package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
)

// TestLabelsGolden locks the zh-CN output of the shared image labels. A
// wording change in render.toml shows up here as a reviewable diff;
// regenerate with HARUKI_UPDATE_GOLDEN=1 go test ./internal/pjsk/render/common/.
func TestLabelsGolden(t *testing.T) {
	var b strings.Builder
	line := func(name, value string) { fmt.Fprintf(&b, "%s\t%s\n", name, value) }
	for _, kind := range []drawing.DataSourceKind{drawing.DataSourceSuite, drawing.DataSourceMySekai, drawing.DataSourcePublic} {
		source := NewDataSource(kind)
		if source.Kind != kind {
			t.Fatalf("NewDataSource(%q).Kind = %q", kind, source.Kind)
		}
		line("DataSourceLabel("+string(kind)+")", source.Name)
	}
	for _, liveType := range []string{"solo", "multi", "auto", "mysekai", "challenge"} {
		line("LiveShortLabel("+liveType+")", LiveShortLabel(liveType))
	}
	line("CharacterFallbackName(7)", CharacterFallbackName(7))

	path := filepath.Join("testdata", "labels.zh-CN.golden")
	if os.Getenv("HARUKI_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (regenerate with HARUKI_UPDATE_GOLDEN=1)", path, err)
	}
	if string(want) != b.String() {
		t.Fatalf("%s is out of date; review the diff and regenerate with HARUKI_UPDATE_GOLDEN=1\n--- got ---\n%s", path, b.String())
	}
}
