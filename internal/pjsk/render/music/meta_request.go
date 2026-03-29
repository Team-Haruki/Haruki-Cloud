package music

import (
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/utils/drawing"
)

func (c *Controller) ResolveMusicMetaRequests(region string, queries []string) ([]drawing.MusicMetaRequest, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}
	if len(queries) == 0 {
		return nil, fmt.Errorf("music meta request is empty")
	}

	resolvedRegion, source, builder, err := c.resolveBuilder(region)
	if err != nil {
		return nil, err
	}

	result := make([]drawing.MusicMetaRequest, 0, len(queries))
	for _, rawQuery := range queries {
		query := strings.TrimSpace(rawQuery)
		if query == "" {
			continue
		}

		musicInfo, err := c.resolveMusicMetaQuery(source, query)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve music meta query %q: %w", query, err)
		}

		metas := c.resolveAllMusicMetas(resolvedRegion.String(), musicInfo.ID)
		if len(metas) == 0 {
			return nil, fmt.Errorf("music %d has no meta data", musicInfo.ID)
		}

		result = append(result, drawing.MusicMetaRequest{
			MusicID:        musicInfo.ID,
			MusicTitle:     builder.buildDisplayMusicTitle(musicInfo, resolvedRegion),
			MusicCoverPath: builder.BuildMusicJacketPath(musicInfo.AssetBundleName, resolvedRegion),
			Metas:          metas,
		})
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("music meta request is empty")
	}
	return result, nil
}

func (c *Controller) resolveMusicMetaQuery(source DataSource, query string) (*masterdata.Music, error) {
	searcher := c.newSearchService(source)
	musicInfo, err := searcher.Search(query)
	if err == nil {
		return musicInfo, nil
	}

	lower := strings.ToLower(strings.TrimSpace(query))
	if lower == "" {
		return nil, fmt.Errorf("music query is empty")
	}

	var best *masterdata.Music
	for _, item := range source.GetMusics() {
		if item == nil || !matchesMusicKeyword(source, item, lower) {
			continue
		}
		if best == nil || len(strings.TrimSpace(item.Title)) < len(strings.TrimSpace(best.Title)) {
			best = item
		}
	}
	if best != nil {
		copy := *best
		return &copy, nil
	}

	return nil, err
}

func (c *Controller) resolveAllMusicMetas(region string, musicID int) []drawing.MusicMetaInfo {
	if musicID <= 0 {
		return nil
	}

	if c != nil && c.metaLoader != nil {
		if payload := c.metaLoader.Get(region); len(payload) > 0 {
			if items := musicMetaInfosFromPayload(payload, musicID); len(items) > 0 {
				return items
			}
		}
	}

	if snapshot := c.currentSnapshot(); snapshot != nil {
		if payload := snapshot.MusicMetaBytes(); len(payload) > 0 {
			if items := musicMetaInfosFromPayload(payload, musicID); len(items) > 0 {
				return items
			}
		}
	}

	return nil
}
