package mysekai

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalMasterdataStoreResetReloadsListsAndMaps(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "mysekaiFixtures.json")
	if err := os.WriteFile(path, []byte(`[{"id":1,"name":"Old"}]`), 0o644); err != nil {
		t.Fatalf("write initial fixture: %v", err)
	}
	store := newLocalMasterdataStore(root)
	if got := stringValue(store.loadList("mysekaiFixtures.json")[0]["name"]); got != "Old" {
		t.Fatalf("initial fixture name = %q", got)
	}
	if got := stringValue(store.loadMapByID("mysekaiFixtures.json")[1]["name"]); got != "Old" {
		t.Fatalf("initial fixture map name = %q", got)
	}

	if err := os.WriteFile(path, []byte(`[{"id":1,"name":"Updated"}]`), 0o644); err != nil {
		t.Fatalf("write updated fixture: %v", err)
	}
	store.resetCache()

	if got := stringValue(store.loadList("mysekaiFixtures.json")[0]["name"]); got != "Updated" {
		t.Fatalf("reloaded fixture name = %q", got)
	}
	if got := stringValue(store.loadMapByID("mysekaiFixtures.json")[1]["name"]); got != "Updated" {
		t.Fatalf("reloaded fixture map name = %q", got)
	}
}
