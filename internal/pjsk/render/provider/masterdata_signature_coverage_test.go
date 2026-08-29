package provider

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestLocalMasterdataDirSignatureFindsContentAndHashesJSONFiles(t *testing.T) {
	root := t.TempDir()
	content := filepath.Join(root, localMasterdataRepoDirs[renderregion.JP], "master")
	if err := os.MkdirAll(filepath.Join(content, "nested"), 0o755); err != nil {
		t.Fatalf("create content directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(content, ".git"), 0o755); err != nil {
		t.Fatalf("create git directory: %v", err)
	}
	writeLocalCoverageFile(t, content, "cards.json", `[{"id":1}]`)
	writeLocalCoverageFile(t, content, "nested/musics.JSON", `[{"id":2}]`)
	writeLocalCoverageFile(t, content, "notes.txt", "ignored")
	writeLocalCoverageFile(t, filepath.Join(content, ".git"), "ignored.json", `[{"id":3}]`)

	dir, ok := ResolveLocalMasterdataContentDir(root, renderregion.JP)
	if !ok || dir != content {
		t.Fatalf("ResolveLocalMasterdataContentDir() = %q, %v; want %q, true", dir, ok, content)
	}
	sig, err := LocalMasterdataDirSignature(root, renderregion.JP)
	if err != nil {
		t.Fatalf("LocalMasterdataDirSignature() error = %v", err)
	}
	if sig.Dir != content || sig.Files != 2 || len(sig.Hash) != 64 {
		t.Fatalf("signature = %+v, want content directory, two files, SHA-256 hash", sig)
	}

	writeLocalCoverageFile(t, content, "notes.txt", "changed but still ignored")
	unchanged, err := LocalMasterdataDirSignature(root, renderregion.JP)
	if err != nil || unchanged.Hash != sig.Hash {
		t.Fatalf("non-JSON change affected signature: before=%+v after=%+v err=%v", sig, unchanged, err)
	}

	musicPath := filepath.Join(content, "nested", "musics.JSON")
	stamp := time.Unix(1_800_000_000, 123)
	if err := os.Chtimes(musicPath, stamp, stamp); err != nil {
		t.Fatalf("change JSON timestamp: %v", err)
	}
	changed, err := LocalMasterdataDirSignature(root, renderregion.JP)
	if err != nil || changed.Hash == sig.Hash {
		t.Fatalf("JSON change did not affect signature: before=%+v after=%+v err=%v", sig, changed, err)
	}
}

func TestLocalMasterdataDirSignatureRejectsMissingOrEmptyContent(t *testing.T) {
	if dir, ok := ResolveLocalMasterdataContentDir("  ", renderregion.JP); ok || dir != "" {
		t.Fatalf("blank root resolved to %q, %v", dir, ok)
	}
	if _, err := LocalMasterdataDirSignature(t.TempDir(), renderregion.JP); err == nil {
		t.Fatal("missing masterdata should return an error")
	}

	root := t.TempDir()
	markerDir := filepath.Join(root, renderregion.JP.String())
	if err := os.MkdirAll(filepath.Join(markerDir, "cards.json"), 0o755); err != nil {
		t.Fatalf("create directory marker: %v", err)
	}
	if hasLocalMasterdataMarker(markerDir) {
		t.Fatal("a directory named cards.json must not count as a masterdata marker")
	}

	writeLocalCoverageFile(t, markerDir, "cards.json/placeholder.txt", "not JSON")
	if _, err := LocalMasterdataDirSignature(root, renderregion.JP); err == nil {
		t.Fatal("content without regular JSON files should return an error")
	}
}
