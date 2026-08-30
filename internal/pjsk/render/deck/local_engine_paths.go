package deck

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	json "haruki-cloud/internal/jsonutil"
)

var deckMasterdataRegions = map[string]struct{}{
	"jp": {},
	"cn": {},
	"tw": {},
	"kr": {},
	"en": {},
}

var deckMasterdataRepoDirs = map[string]string{
	"jp": "haruki-sekai-master",
	"cn": "haruki-sekai-sc-master",
	"tw": "haruki-sekai-tc-master",
	"kr": "haruki-sekai-kr-master",
	"en": "haruki-sekai-en-master",
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

func resolveDeckMasterdataContentDir(configured, region string) (string, bool) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return "", false
	}
	root := filepath.Clean(configured)
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		region = "jp"
	}

	candidates := []string{
		filepath.Join(root, region),
		filepath.Join(root, region, "master"),
		root,
		filepath.Join(root, "master"),
	}
	if repoDir := deckMasterdataRepoDirs[region]; repoDir != "" {
		candidates = append(candidates,
			filepath.Join(root, repoDir),
			filepath.Join(root, repoDir, "master"),
		)
	}
	for _, candidate := range candidates {
		if fileExists(filepath.Join(candidate, "areaItemLevels.json")) {
			return candidate, true
		}
	}
	return "", false
}

func deckMasterdataContainsEvent(configured, region string, eventID int) (bool, bool) {
	if eventID <= 0 {
		return false, false
	}
	eventsFile, ok := resolveDeckMasterdataEventsFile(configured, region)
	if !ok {
		return false, false
	}
	data, err := os.ReadFile(eventsFile)
	if err != nil {
		return false, false
	}
	var events []struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(data, &events); err != nil {
		return false, false
	}
	for _, eventInfo := range events {
		if eventInfo.ID == eventID {
			return true, true
		}
	}
	return false, true
}

func resolveDeckMasterdataEventsFile(configured, region string) (string, bool) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return "", false
	}
	root := filepath.Clean(configured)
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		region = "jp"
	}

	candidates := []string{
		filepath.Join(root, region, eventsFileName),
		filepath.Join(root, eventsFileName),
	}
	if repoDir := deckMasterdataRepoDirs[region]; repoDir != "" {
		candidates = append(candidates,
			filepath.Join(root, repoDir, "master", eventsFileName),
			filepath.Join(root, "master", eventsFileName),
		)
	}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate, true
		}
	}
	return "", false
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
