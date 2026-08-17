package app

import (
	"os"
	"path/filepath"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/provider"
)

type countingMasterdataResetter struct {
	count int
}

func (r *countingMasterdataResetter) ResetMasterdataCache() {
	r.count++
}

func TestLocalMasterdataRefreshResetsProviderAndAdditionalCaches(t *testing.T) {
	root := t.TempDir()
	masterDir := filepath.Join(root, "haruki-sekai-master", "master")
	if err := os.MkdirAll(masterDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(masterDir, "cards.json"), []byte(`[]`), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	fixturesPath := filepath.Join(masterDir, "mysekaiFixtures.json")
	if err := os.WriteFile(fixturesPath, []byte(`[{"id":1,"name":"Old"}]`), 0o644); err != nil {
		t.Fatalf("write initial fixtures: %v", err)
	}

	local := provider.NewLocalProvider(masterDir, renderregion.JP)
	if got := local.MySekai().LoadMapByID("mysekaiFixtures.json")[1]["name"]; got != "Old" {
		t.Fatalf("initial fixture name = %v", got)
	}
	additional := &countingMasterdataResetter{}
	state := newLocalMasterdataRefreshState(root, map[renderregion.Value]provider.MasterDataProvider{
		renderregion.JP: local,
	}, additional)
	state.captureInitial()

	if err := os.WriteFile(fixturesPath, []byte(`[{"id":1,"name":"Updated"}]`), 0o644); err != nil {
		t.Fatalf("write updated fixtures: %v", err)
	}
	state.refresh()

	if additional.count != 1 {
		t.Fatalf("additional reset count = %d", additional.count)
	}
	if got := local.MySekai().LoadMapByID("mysekaiFixtures.json")[1]["name"]; got != "Updated" {
		t.Fatalf("reloaded fixture name = %v", got)
	}

	state.refresh()
	if additional.count != 1 {
		t.Fatalf("unchanged signature reset count = %d", additional.count)
	}
}
