package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	renderregion "haruki-cloud/internal/pjsk/region"
)

type LocalMasterdataSignature struct {
	Dir   string
	Hash  string
	Files int
}

func LocalMasterdataDirSignature(root string, region renderregion.Value) (LocalMasterdataSignature, error) {
	dir, ok := ResolveLocalMasterdataContentDir(root, region)
	if !ok {
		return LocalMasterdataSignature{}, fmt.Errorf("local masterdata dir not found for region %s under %s", region, strings.TrimSpace(root))
	}

	hasher := sha256.New()
	files := 0
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			rel = path
		}
		files++
		fmt.Fprintf(hasher, "%s\x00%d\x00%d\x00", filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		return LocalMasterdataSignature{}, err
	}
	if files == 0 {
		return LocalMasterdataSignature{}, fmt.Errorf("local masterdata dir %s has no json files", dir)
	}
	return LocalMasterdataSignature{
		Dir:   dir,
		Hash:  hex.EncodeToString(hasher.Sum(nil)),
		Files: files,
	}, nil
}

func ResolveLocalMasterdataContentDir(root string, region renderregion.Value) (string, bool) {
	for _, dir := range localMasterdataCandidateDirs(root, region) {
		if hasLocalMasterdataMarker(dir) {
			return dir, true
		}
	}
	return "", false
}

func hasLocalMasterdataMarker(dir string) bool {
	for _, name := range []string{"areaItemLevels.json", "resourceBoxes.json", "cards.json", "musics.json"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}
