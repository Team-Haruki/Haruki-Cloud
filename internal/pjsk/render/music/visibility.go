package music

import (
	"fmt"
	"sort"
	"time"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/releasecheck"
)

func currentMusicVisibilityTime() int64 {
	return time.Now().UnixMilli()
}

func isMusicVisibleAt(musicInfo *masterdata.Music, now int64) bool {
	return isMusicAccessibleAt(musicInfo, now, false)
}

func isMusicAccessibleAt(musicInfo *masterdata.Music, now int64, allowUnreleased bool) bool {
	if musicInfo == nil {
		return false
	}
	if _, blocked := hiddenMusicIDs[musicInfo.ID]; blocked {
		return false
	}
	return allowUnreleased || musicInfo.PublishedAt <= now
}

func ensureVisibleMusic(musicInfo *masterdata.Music, now int64, fallback any) (*masterdata.Music, error) {
	return ensureAccessibleMusic(musicInfo, now, fallback, false)
}

func ensureAccessibleMusic(musicInfo *masterdata.Music, now int64, fallback any, allowUnreleased bool) (*masterdata.Music, error) {
	if isMusicAccessibleAt(musicInfo, now, allowUnreleased) {
		return musicInfo, nil
	}
	switch value := fallback.(type) {
	case int:
		if musicInfo != nil {
			return nil, releasecheck.New(releasecheck.KindMusic, "", value)
		}
		return nil, fmt.Errorf("music not found: %d", value)
	case string:
		value = stringsTrimSpace(value)
		if value != "" {
			if musicInfo != nil {
				return nil, releasecheck.New(releasecheck.KindMusic, value, 0)
			}
			return nil, fmt.Errorf("music not found: %s", value)
		}
	}
	if musicInfo != nil {
		return nil, releasecheck.New(releasecheck.KindMusic, "", musicInfo.ID)
	}
	return nil, fmt.Errorf("music not found")
}

func filterVisibleMusics(items []*masterdata.Music, now int64) []*masterdata.Music {
	return filterAccessibleMusics(items, now, false)
}

func filterAccessibleMusics(items []*masterdata.Music, now int64, allowUnreleased bool) []*masterdata.Music {
	if len(items) == 0 {
		return nil
	}
	filtered := make([]*masterdata.Music, 0, len(items))
	for _, item := range items {
		if !isMusicAccessibleAt(item, now, allowUnreleased) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func visibleMusicsSortedByPublishedAt(source DataSource, now int64) []*masterdata.Music {
	return accessibleMusicsSortedByPublishedAt(source, now, false)
}

func accessibleMusicsSortedByPublishedAt(source DataSource, now int64, allowUnreleased bool) []*masterdata.Music {
	if source == nil {
		return nil
	}
	musics := filterAccessibleMusics(source.GetMusics(), now, allowUnreleased)
	sort.Slice(musics, func(i, j int) bool {
		if musics[i].PublishedAt == musics[j].PublishedAt {
			return musics[i].ID < musics[j].ID
		}
		return musics[i].PublishedAt < musics[j].PublishedAt
	})
	return musics
}

func musicListDisplayOrder(musicInfo *masterdata.Music) int64 {
	if musicInfo == nil {
		return 0
	}
	if musicInfo.Seq > 0 {
		return int64(musicInfo.Seq)
	}
	if musicInfo.PublishedAt > 0 {
		return musicInfo.PublishedAt
	}
	return int64(musicInfo.ID)
}

func stringsTrimSpace(value string) string {
	start := 0
	for start < len(value) && (value[start] == ' ' || value[start] == '\t' || value[start] == '\n' || value[start] == '\r') {
		start++
	}
	end := len(value)
	for end > start && (value[end-1] == ' ' || value[end-1] == '\t' || value[end-1] == '\n' || value[end-1] == '\r') {
		end--
	}
	return value[start:end]
}
