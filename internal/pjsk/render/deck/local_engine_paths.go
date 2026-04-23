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
		return "", fmt.Errorf("deck recommend: masterdata root %s missing region dir %s", root, region)
	}
	return root, nil
}

func resolveDeckRemoteMasterdataDir(configured string) string {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return ""
	}

	root := filepath.Clean(configured)
	base := strings.ToLower(filepath.Base(root))
	if _, ok := deckMasterdataRegions[base]; ok {
		parent := filepath.Dir(root)
		if parent != "" && parent != "." && parent != root {
			return parent
		}
	}
	return root
}

func hasDeckRegionSubdirs(root string) bool {
	for region := range deckMasterdataRegions {
		if dirExists(filepath.Join(root, region)) {
			return true
		}
	}
	return false
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
