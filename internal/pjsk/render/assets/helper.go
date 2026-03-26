package assets

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// AssetHelper resolves assets against a primary directory with legacy fallbacks.
type AssetHelper struct {
	roots []string
}

func NewAssetHelper(primary string, legacy []string) *AssetHelper {
	var roots []string
	seen := make(map[string]struct{})

	appendRoot := func(path string) {
		clean := strings.TrimSpace(path)
		if clean == "" {
			return
		}
		clean = normalizeAssetRoot(clean)
		if clean == "." {
			return
		}
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		roots = append(roots, clean)
	}

	appendRoot(primary)
	for _, dir := range legacy {
		appendRoot(dir)
	}
	if len(roots) == 0 {
		roots = []string{"."}
	}

	return &AssetHelper{roots: roots}
}

func (h *AssetHelper) Roots() []string {
	out := make([]string, len(h.roots))
	copy(out, h.roots)
	return out
}

func (h *AssetHelper) Primary() string {
	if len(h.roots) == 0 {
		return ""
	}
	return h.roots[0]
}

func (h *AssetHelper) Join(parts ...string) string {
	if len(h.roots) == 0 {
		return ""
	}
	return joinAssetPath(h.Primary(), parts...)
}

func (h *AssetHelper) FirstExisting(relPaths ...string) string {
	for _, rel := range relPaths {
		if strings.TrimSpace(rel) == "" {
			continue
		}
		for _, root := range h.roots {
			if isAssetURL(root) {
				continue
			}
			candidate := filepath.Join(root, rel)
			if _, err := os.Stat(candidate); err == nil {
				return filepath.ToSlash(candidate)
			}
		}
	}
	return ""
}

func ResolveAssetPath(helper *AssetHelper, assetDir string, relPaths ...string) string {
	if len(relPaths) == 0 {
		return ""
	}
	if helper != nil {
		if resolved := helper.FirstExisting(relPaths...); resolved != "" {
			return filepath.ToSlash(resolved)
		}
	}
	base := assetDir
	if base == "" && helper != nil {
		base = helper.Primary()
	}
	if base == "" {
		return filepath.ToSlash(relPaths[0])
	}
	return joinAssetPath(base, relPaths[0])
}

// RegionAssetDir returns the region-specific startapp asset subdirectory prefix
// used by the Haruki Drawing API, e.g. "asset/jp-assets/startapp" for "jp".
func RegionAssetDir(region string) string {
	return "asset/" + region + "-assets/startapp"
}

var CharacterIDToNickname = map[int]string{
	1:  "ick",
	2:  "saki",
	3:  "hnm",
	4:  "shiho",
	5:  "mnr",
	6:  "hrk",
	7:  "airi",
	8:  "szk",
	9:  "khn",
	10: "an",
	11: "akt",
	12: "toya",
	13: "tks",
	14: "emu",
	15: "nene",
	16: "rui",
	17: "knd",
	18: "mfy",
	19: "ena",
	20: "mzk",
	21: "miku",
	22: "rin",
	23: "len",
	24: "luka",
	25: "meiko",
	26: "kaito",
}

func MakeRelative(base, target string) string {
	if base == "" || target == "" {
		return target
	}

	cleanBase := normalizeAssetRoot(base)
	cleanTarget := normalizeAssetRoot(target)
	if strings.HasPrefix(cleanTarget, cleanBase) {
		rel := strings.TrimPrefix(cleanTarget, cleanBase)
		rel = strings.TrimPrefix(rel, "/")
		return rel
	}
	return cleanTarget
}

func normalizeAssetRoot(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	if isAssetURL(root) {
		return strings.TrimRight(root, "/")
	}
	return filepath.ToSlash(filepath.Clean(root))
}

func joinAssetPath(base string, parts ...string) string {
	base = normalizeAssetRoot(base)
	if base == "" {
		if len(parts) == 0 {
			return ""
		}
		return filepath.ToSlash(parts[0])
	}
	if isAssetURL(base) {
		parsed, err := url.Parse(base)
		if err != nil {
			return base
		}
		joined := make([]string, 0, len(parts)+1)
		if parsed.Path != "" {
			joined = append(joined, parsed.Path)
		}
		for _, part := range parts {
			if strings.TrimSpace(part) == "" {
				continue
			}
			joined = append(joined, filepath.ToSlash(part))
		}
		parsed.Path = path.Join(joined...)
		return parsed.String()
	}
	all := append([]string{base}, parts...)
	return filepath.ToSlash(filepath.Join(all...))
}

func isAssetURL(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}
