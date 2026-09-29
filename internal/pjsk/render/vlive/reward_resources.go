package vlive

import (
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/provider"
)

// masterRowDataSource is the optional DataSource extension serving generic
// master rows (honorBackgrounds, honorWords, virtualItems).
type masterRowDataSource interface {
	MasterRows(filename string) (map[int]map[string]any, bool)
}

// MasterRows implements masterRowDataSource.
func (a *ProviderAdapter) MasterRows(filename string) (map[int]map[string]any, bool) {
	if a == nil {
		return nil, false
	}
	return provider.LoadMasterRows(a.Context(), a.P, filename)
}

// newResourceRewardImagePath resolves the icon of a JP 7.0.0 resource type
// (honor_background, honor_word, virtual_item); "" on regions without the
// rows, so such a reward is skipped as before.
func (c *Controller) newResourceRewardImagePath(source DataSource, resourceType string, resourceID int) string {
	rowSource, ok := source.(masterRowDataSource)
	if !ok {
		return ""
	}
	rows := func(filename string) map[int]map[string]any {
		items, _ := rowSource.MasterRows(filename)
		return items
	}
	rel := provider.NewResourceIconRelPath(resourceType, resourceID, rows)
	if rel == "" {
		return ""
	}
	return assets.ResolveRegionAssetPath(c.assets, "jp", rel)
}
