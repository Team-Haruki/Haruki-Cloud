package app

import (
	"os"
	"path/filepath"
	"strings"

	renderregion "haruki-cloud/internal/pjsk/region"
)

func resolveRenderProviderMasterdataDir(cfg Config) string {
	if dir := resolveRenderProviderMasterdataDirFromWD(cfg, currentWorkingDir()); dir != "" {
		return dir
	}
	return ""
}

func resolveRenderProviderMasterdataDirFromWD(cfg Config, wd string) string {
	flatCandidates := make([]string, 0, 6)
	rootCandidates := make([]string, 0, 6)
	seen := make(map[string]struct{}, 12)

	addCandidate := func(raw string) {
		dir := strings.TrimSpace(raw)
		if dir == "" {
			return
		}
		dir = filepath.Clean(dir)
		if _, ok := seen[dir]; ok {
			return
		}
		seen[dir] = struct{}{}

		switch classifyRenderMasterdataDir(dir) {
		case masterdataDirRegionRoot:
			rootCandidates = append(rootCandidates, dir)
		case masterdataDirRepoRoot:
			rootCandidates = append(rootCandidates, dir)
		case masterdataDirFlat:
			flatCandidates = append(flatCandidates, dir)
		default:
			// masterdataDirInvalid: skip unrecognised directories
		}
	}

	if dir := strings.TrimSpace(cfg.LocalMasterdata.Dir); dir != "" {
		addCandidate(dir)
		if parent, ok := regionParentDir(dir); ok {
			addCandidate(parent)
		}
	}

	if dir := strings.TrimSpace(cfg.DeckRecommend.MasterdataDir); dir != "" {
		addCandidate(dir)
		if parent, ok := regionParentDir(dir); ok {
			addCandidate(parent)
		}
	}

	wd = strings.TrimSpace(wd)
	if wd != "" {
		addCandidate(filepath.Join(wd, "deckrec", "masterdata"))
		addCandidate(filepath.Join(wd, "data", "masterdata"))
		addCandidate(filepath.Join(wd, "Data", "master", "haruki-sekai-master"))
		addCandidate(filepath.Join(wd, "Data", "master", "haruki-sekai-master", "master"))
	}

	if len(rootCandidates) > 0 {
		return rootCandidates[0]
	}
	if len(flatCandidates) > 0 {
		return flatCandidates[0]
	}
	return ""
}

func classifyRenderMasterdataDir(dir string) renderMasterdataDirKind {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return masterdataDirInvalid
	}
	if hasRenderMasterdataRegionDirs(dir) {
		return masterdataDirRegionRoot
	}
	if hasRenderMasterdataRepoDirs(dir) {
		return masterdataDirRepoRoot
	}
	if hasRenderMasterdataFiles(dir) {
		return masterdataDirFlat
	}
	return masterdataDirInvalid
}

func hasRenderMasterdataFiles(dir string) bool {
	for _, name := range []string{"resourceBoxes.json", "resourceBoxDetails.json", "cards.json"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func hasRenderMasterdataRegionDirs(dir string) bool {
	found := 0
	for _, region := range []string{"jp", "cn", "tw", "kr", "en"} {
		info, err := os.Stat(filepath.Join(dir, region))
		if err == nil && info.IsDir() {
			found++
		}
	}
	return found > 0
}

func hasRenderMasterdataRepoDirs(dir string) bool {
	for _, repoDir := range []string{
		"haruki-sekai-master",
		"haruki-sekai-sc-master",
		"haruki-sekai-tc-master",
		"haruki-sekai-kr-master",
		"haruki-sekai-en-master",
	} {
		if hasRenderMasterdataFiles(filepath.Join(dir, repoDir, "master")) {
			return true
		}
	}
	return false
}

func regionParentDir(path string) (string, bool) {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	base := strings.ToLower(filepath.Base(cleaned))
	switch renderregion.Value(base) {
	case renderregion.JP, renderregion.CN, renderregion.TW, renderregion.KR, renderregion.EN:
		parent := filepath.Dir(cleaned)
		if parent != "" && parent != "." && parent != cleaned {
			return parent, true
		}
	}
	return "", false
}

func currentWorkingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}
