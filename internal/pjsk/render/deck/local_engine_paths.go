package deck

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var deckMasterdataRegions = map[string]struct{}{
	"jp": {},
	"cn": {},
	"tw": {},
	"kr": {},
	"en": {},
}

func resolveDeckMasterdataDir(configured, region string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return "", nil
	}

	root := filepath.Clean(configured)
	if !dirExists(root) {
		return root, nil
	}

	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		region = "jp"
	}

	candidate := filepath.Join(root, region)
	if dirExists(candidate) {
		return candidate, nil
	}
	if fileExists(filepath.Join(root, "cards.json")) {
		return root, nil
	}
	if hasDeckRegionSubdirs(root) {
		return "", fmt.Errorf("deck local engine: masterdata root %s missing region dir %s", root, region)
	}
	return root, nil
}

func resolveDeckStaticDataDir(configured, masterdataDir string) string {
	if configured = strings.TrimSpace(configured); configured != "" && dirExists(configured) {
		return filepath.Clean(configured)
	}

	if candidate := resolveDeckStaticDataDirFromMasterdata(masterdataDir); candidate != "" {
		return candidate
	}

	candidates := []string{
		filepath.Join("data", "sekai_deck_recommend"),
		"data",
	}

	if wd, err := os.Getwd(); err == nil {
		for _, suffix := range candidates {
			candidate := filepath.Join(wd, suffix)
			if dirExists(candidate) {
				return candidate
			}
		}
	}

	if exePath, err := os.Executable(); err == nil {
		for _, suffix := range candidates {
			candidate := filepath.Join(filepath.Dir(exePath), suffix)
			if dirExists(candidate) {
				return candidate
			}
		}
	}

	return ""
}

func resolveDeckStaticDataDirFromMasterdata(masterdataDir string) string {
	masterdataDir = strings.TrimSpace(masterdataDir)
	if masterdataDir == "" {
		return ""
	}

	root := filepath.Clean(masterdataDir)
	if !dirExists(root) {
		return ""
	}
	if hasDeckStaticDataFiles(root) {
		return root
	}

	base := strings.ToLower(filepath.Base(root))
	if _, ok := deckMasterdataRegions[base]; ok {
		parent := filepath.Dir(root)
		if dirExists(parent) && hasDeckStaticDataFiles(parent) {
			return parent
		}
	}

	return ""
}

func hasDeckRegionSubdirs(root string) bool {
	for region := range deckMasterdataRegions {
		if dirExists(filepath.Join(root, region)) {
			return true
		}
	}
	return false
}

func hasDeckStaticDataFiles(root string) bool {
	return fileExists(filepath.Join(root, "worldBloomSupportDeckBonusesWL1.json")) &&
		fileExists(filepath.Join(root, "worldBloomSupportDeckBonusesWL2.json"))
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
