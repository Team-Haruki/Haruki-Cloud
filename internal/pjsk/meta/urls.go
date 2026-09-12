package meta

import (
	"fmt"
	"strings"
)

// Music metas sources. "legacy" fetches the community feed (or any host that
// mirrors its file layout); "registry" fetches the Haruki master registry,
// which owns the feed and already injects the omakase rows.
const (
	SourceLegacy   = "legacy"
	SourceRegistry = "registry"

	defaultLegacyBaseURL = "https://sekai-data.3-3.dev"
)

// ResolveURL builds the music_metas URL for a region under the given source.
// An empty source means legacy; an empty base URL under legacy keeps the
// hardcoded community host so existing deployments are unaffected.
func ResolveURL(source, baseURL, region string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "", SourceLegacy:
		if base == "" {
			base = defaultLegacyBaseURL
		}
		filename, ok := regionFilenames[region]
		if !ok {
			return "", fmt.Errorf("meta: unknown region %q", region)
		}
		return base + "/" + filename, nil
	case SourceRegistry:
		if base == "" {
			return "", fmt.Errorf("meta: registry source requires a base URL")
		}
		if _, ok := regionFilenames[region]; !ok {
			return "", fmt.Errorf("meta: unknown region %q", region)
		}
		return base + "/v1/metas/" + region + "/music_metas.json", nil
	default:
		return "", fmt.Errorf("meta: unknown music metas source %q", source)
	}
}

// regionFilenames maps SekaiServerRegion strings to the legacy community
// feed's file names (sekai-data.3-3.dev; "tw" uses the "-tc" suffix).
var regionFilenames = map[string]string{
	"jp": "music_metas.json",
	"en": "music_metas-en.json",
	"tw": "music_metas-tc.json",
	"kr": "music_metas-kr.json",
	"cn": "music_metas-cn.json",
}

// Regions returns all supported region keys in a stable order.
func Regions() []string {
	return []string{"jp", "en", "tw", "kr", "cn"}
}
