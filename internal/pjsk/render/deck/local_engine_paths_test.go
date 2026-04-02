package deck

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDeckMasterdataDirUsesRegionSubdir(t *testing.T) {
	root := t.TempDir()
	regionDir := filepath.Join(root, "jp")
	if err := os.Mkdir(regionDir, 0o755); err != nil {
		t.Fatalf("mkdir region dir: %v", err)
	}

	resolved, err := resolveDeckMasterdataDir(root, "jp")
	if err != nil {
		t.Fatalf("resolveDeckMasterdataDir returned error: %v", err)
	}
	if resolved != regionDir {
		t.Fatalf("unexpected masterdata dir: %s", resolved)
	}
}

func TestResolveDeckMasterdataDirErrorsWhenRegionMissing(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "jp"), 0o755); err != nil {
		t.Fatalf("mkdir jp dir: %v", err)
	}

	_, err := resolveDeckMasterdataDir(root, "en")
	if err == nil {
		t.Fatalf("expected missing region error")
	}
}

func TestResolveDeckStaticDataDirUsesMasterdataRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "worldBloomSupportDeckBonusesWL1.json"), []byte("[]"), 0o644); err != nil {
		t.Fatalf("write wl1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "worldBloomSupportDeckBonusesWL2.json"), []byte("[]"), 0o644); err != nil {
		t.Fatalf("write wl2: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "jp"), 0o755); err != nil {
		t.Fatalf("mkdir jp dir: %v", err)
	}

	resolved := resolveDeckStaticDataDir("", root)
	if resolved != root {
		t.Fatalf("unexpected static data dir from root: %s", resolved)
	}

	resolved = resolveDeckStaticDataDir("", filepath.Join(root, "jp"))
	if resolved != root {
		t.Fatalf("unexpected static data dir from region dir: %s", resolved)
	}
}

func TestResolveDeckRemoteMasterdataDirStripsRegionSuffix(t *testing.T) {
	resolved := resolveDeckRemoteMasterdataDir("/masterdata/jp")
	if resolved != filepath.Clean("/masterdata") {
		t.Fatalf("unexpected remote masterdata dir: %s", resolved)
	}
}

func TestResolveDeckRemoteMasterdataDirKeepsRoot(t *testing.T) {
	resolved := resolveDeckRemoteMasterdataDir("/masterdata")
	if resolved != filepath.Clean("/masterdata") {
		t.Fatalf("unexpected remote masterdata root: %s", resolved)
	}
}
