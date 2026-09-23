package app

import (
	"os"
	"path/filepath"
	"testing"
)

// The inventory tables read local files only under the fallback flag, so the
// composition root must not resolve an inventory directory without it.
func TestAppMasterdataDirsGateInventoryDirBehindFallbackFlag(t *testing.T) {
	root := t.TempDir()
	for _, region := range []string{"jp", "cn"} {
		if err := os.MkdirAll(filepath.Join(root, region), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	fallback, localDir, inventoryDir := appMasterdataDirs(Config{LocalMasterdata: LocalMasterdataConfig{Dir: root}})
	if fallback || localDir != "" || inventoryDir != "" {
		t.Fatalf("flag off: fallback=%v local=%q inventory=%q", fallback, localDir, inventoryDir)
	}
	fallback, localDir, inventoryDir = appMasterdataDirs(Config{LocalMasterdata: LocalMasterdataConfig{Enabled: true, AllowFallback: true, Dir: root}})
	if !fallback || localDir != root || inventoryDir != root {
		t.Fatalf("flag on: fallback=%v local=%q inventory=%q", fallback, localDir, inventoryDir)
	}
}
