package provider

import (
	"fmt"
	"strings"
)

// NewResourceIconRelPath returns the region-relative icon path of a reward
// whose resource type JP 7.0.0 added, or "" when the type is not one of them
// or rows does not know the id (a region without the table). rows serves a
// whole generic master table by file name.
//
// Assumed asset layout (not yet verified against the exported asset tree):
//   - honor_background: honor_background/<assetbundleName>/degree_sub.png,
//     the same bundle the honor badge uses;
//   - honor_word: honor_word/<assetbundleName>_1.png, the lowest-rarity
//     variant (a reward is not tied to an honor rarity);
//   - virtual_item: thumbnail/virtual_live_item/<assetbundleName>.png, next to
//     the client's other thumbnail/<type> folders.
func NewResourceIconRelPath(resourceType string, resourceID int, rows func(filename string) map[int]map[string]any) string {
	if rows == nil || resourceID <= 0 {
		return ""
	}
	var filename, format string
	switch strings.ToLower(strings.TrimSpace(resourceType)) {
	case "honor_background":
		filename, format = "honorBackgrounds.json", "honor_background/%s/degree_sub.png"
	case "honor_word":
		filename, format = "honorWords.json", "honor_word/%s_1.png"
	case "virtual_item":
		filename, format = "virtualItems.json", "thumbnail/virtual_live_item/%s.png"
	default:
		return ""
	}
	row := rows(filename)[resourceID]
	if row == nil {
		return ""
	}
	bundle, _ := row["assetbundleName"].(string)
	bundle = strings.TrimSpace(bundle)
	if bundle == "" {
		return ""
	}
	return fmt.Sprintf(format, bundle)
}
