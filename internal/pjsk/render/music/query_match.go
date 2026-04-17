package music

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

type musicAmbiguousQueryError struct {
	sourceName string
	candidates []musicQueryCandidate
}

type musicQueryCandidate struct {
	ID    int
	Title string
}

func (e *musicAmbiguousQueryError) Error() string {
	parts := make([]string, 0, len(e.candidates))
	for _, item := range e.candidates {
		parts = append(parts, fmt.Sprintf("music%d/%s", item.ID, item.Title))
	}
	return fmt.Sprintf("%s匹配到多个歌曲，请改用 music<id> 查询：\n%s", e.sourceName, strings.Join(parts, "\n"))
}

func isMusicAmbiguousError(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := errors.AsType[*musicAmbiguousQueryError](err); ok {
		return true
	}
	return strings.Contains(err.Error(), "匹配到多个歌曲")
}

func resolveUniqueMusicQuery(source DataSource, query string) (*masterdata.Music, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("music not found: empty query")
	}

	queryLower := strings.ToLower(query)
	now := currentMusicVisibilityTime()
	if matches := collectMusicMatches(source, func(musicInfo *masterdata.Music) bool {
		return strings.EqualFold(strings.TrimSpace(musicInfo.Title), query)
	}, now); len(matches) > 0 {
		return selectUniqueMusicMatch("曲名/别名", matches)
	}

	if matches := collectMusicMatches(source, func(musicInfo *masterdata.Music) bool {
		return strings.Contains(strings.ToLower(strings.TrimSpace(musicInfo.Title)), queryLower)
	}, now); len(matches) > 0 {
		return selectUniqueMusicMatch("曲名/别名", matches)
	}

	if matches := collectLocalizedMusicMatches(source, func(title string) bool {
		return strings.EqualFold(strings.TrimSpace(title), query)
	}, now); len(matches) > 0 {
		return selectUniqueMusicMatch("曲名/别名", matches)
	}

	if matches := collectLocalizedMusicMatches(source, func(title string) bool {
		return strings.Contains(strings.ToLower(strings.TrimSpace(title)), queryLower)
	}, now); len(matches) > 0 {
		return selectUniqueMusicMatch("曲名/别名", matches)
	}

	return nil, fmt.Errorf("music not found: %s", query)
}

func resolveUniqueMusicKeyword(source DataSource, keyword string) (*masterdata.Music, error) {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return nil, fmt.Errorf("music query is empty")
	}

	now := currentMusicVisibilityTime()
	matches := make([]*masterdata.Music, 0)
	for _, item := range source.GetMusics() {
		if !isMusicVisibleAt(item, now) || !matchesMusicKeyword(source, item, keyword) {
			continue
		}
		matches = append(matches, item)
	}
	if len(matches) == 0 {
		return nil, nil
	}
	return selectUniqueMusicMatch("曲名/别名", matches)
}

func collectMusicMatches(source DataSource, matcher func(*masterdata.Music) bool, now int64) []*masterdata.Music {
	if source == nil || matcher == nil {
		return nil
	}
	matches := make([]*masterdata.Music, 0)
	for _, item := range source.GetMusics() {
		if !isMusicVisibleAt(item, now) || !matcher(item) {
			continue
		}
		matches = append(matches, item)
	}
	return matches
}

func collectLocalizedMusicMatches(source DataSource, matcher func(string) bool, now int64) []*masterdata.Music {
	if source == nil || matcher == nil {
		return nil
	}
	matches := make([]*masterdata.Music, 0)
	for _, item := range source.GetMusics() {
		if !isMusicVisibleAt(item, now) {
			continue
		}
		titles, err := source.GetMusicLocalizedTitles(item.ID)
		if err != nil {
			continue
		}
		for _, title := range titles {
			if !matcher(title) {
				continue
			}
			matches = append(matches, item)
			break
		}
	}
	return matches
}

func selectUniqueMusicMatch(sourceName string, matches []*masterdata.Music) (*masterdata.Music, error) {
	deduped := make(map[int]string, len(matches))
	for _, item := range matches {
		if item == nil || item.ID <= 0 {
			continue
		}
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = fmt.Sprintf("music%d", item.ID)
		}
		deduped[item.ID] = title
	}
	if len(deduped) == 0 {
		return nil, nil
	}

	ids := make([]int, 0, len(deduped))
	for id := range deduped {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	if len(ids) == 1 {
		for _, item := range matches {
			if item == nil || item.ID != ids[0] {
				continue
			}
			return new(*item), nil
		}
		return &masterdata.Music{ID: ids[0], Title: deduped[ids[0]]}, nil
	}

	candidates := make([]musicQueryCandidate, 0, len(ids))
	for _, id := range ids {
		candidates = append(candidates, musicQueryCandidate{
			ID:    id,
			Title: deduped[id],
		})
	}
	return nil, &musicAmbiguousQueryError{
		sourceName: sourceName,
		candidates: candidates,
	}
}
