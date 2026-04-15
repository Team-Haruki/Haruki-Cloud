package deck

import (
	"path/filepath"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/region"
)

type MusicSource interface {
	DefaultRegion() renderregion.Value
	GetMusicByID(id int) (*masterdata.Music, error)
}

func (c *Controller) resolveMusicSource(requested renderregion.Value) (renderregion.Value, MusicSource, bool) {
	if c == nil || c.musicSources == nil {
		return renderregion.WithDefault(requested), nil, false
	}
	normalized := renderregion.Normalize(requested.String())
	if !normalized.IsZero() {
		source, ok := c.musicSources.SourceForRegion(normalized)
		return normalized, source, ok
	}
	source, ok := c.musicSources.SourceForRegion(renderregion.Unknown)
	if !ok {
		return c.musicSources.ResolveRegion(renderregion.Unknown), nil, false
	}
	return renderregion.WithDefault(source.DefaultRegion()), source, true
}

func (c *Controller) resolveCompareMusicMetadata(region renderregion.Value, musicID int) (title string, coverPath string) {
	if musicID <= 0 {
		return "", ""
	}
	_, source, ok := c.resolveMusicSource(region)
	if !ok || source == nil {
		return "", ""
	}

	musicInfo, err := source.GetMusicByID(musicID)
	if err != nil || musicInfo == nil {
		return "", ""
	}

	title = strings.TrimSpace(musicInfo.Title)
	if title == "" {
		title = musicInfo.Title
	}
	if c.assets != nil && strings.TrimSpace(musicInfo.AssetBundleName) != "" {
		coverPath = assets.ResolveRegionAssetPath(
			c.assets,
			region.String(),
			filepath.Join("music", "jacket", musicInfo.AssetBundleName, musicInfo.AssetBundleName+".png"),
		)
	}
	return title, coverPath
}
