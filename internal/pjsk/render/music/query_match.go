package music

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/notfound"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/releasecheck"
	"haruki-cloud/utils/usererror"
)

type musicAmbiguousQueryError struct {
	sourceName string
	candidates []musicQueryCandidate
}

type musicQueryCandidate struct {
	ID    int
	Title string
}

// Error is the user reply (typed, see Unwrap): the candidates, one per line.
func (e *musicAmbiguousQueryError) Error() string { return e.userError().Error() }

// Unwrap exposes the typed user error, so the reply layer shows the
// candidate list from the catalog.
func (e *musicAmbiguousQueryError) Unwrap() error { return e.userError() }

// AmbiguousIDs lists the candidate song IDs.
func (e *musicAmbiguousQueryError) AmbiguousIDs() []int {
	return ambiguousMusicCandidateIDs(e.candidates)
}

func (e *musicAmbiguousQueryError) userError() *usererror.Error {
	return AmbiguousMusicError(e.candidates)
}

// AmbiguousMusicError is the reply listing several matching songs.
func AmbiguousMusicError(candidates []musicQueryCandidate) *usererror.Error {
	lines := make([]i18n.Message, 0, len(candidates))
	for _, item := range candidates {
		lines = append(lines, i18n.M("music.ambiguous_candidate", i18n.Data{"ID": item.ID, "Title": item.Title}))
	}
	return usererror.New(usererror.CodeAmbiguous, i18n.M("music.ambiguous", i18n.Data{"Candidates": lines}))
}

// ambiguousIDsError is any error that lists the IDs of several matches: the
// song search of this package and the alias service.
type ambiguousIDsError interface {
	error
	AmbiguousIDs() []int
}

func isMusicAmbiguousError(err error) bool {
	_, ok := errors.AsType[ambiguousIDsError](err)
	return ok
}

// ExtractAmbiguousMusicIDs returns the candidate song IDs of an ambiguous
// song query, judged by the error's type.
func ExtractAmbiguousMusicIDs(err error) []int {
	if ambiguous, ok := errors.AsType[ambiguousIDsError](err); ok {
		return slices.Clone(ambiguous.AmbiguousIDs())
	}
	return nil
}

func ambiguousMusicCandidateIDs(candidates []musicQueryCandidate) []int {
	ids := make([]int, 0, len(candidates))
	for _, item := range candidates {
		if item.ID > 0 {
			ids = append(ids, item.ID)
		}
	}
	return ids
}

func collectVisibleMusicMatchesByID(source DataSource, ids []int, now int64, allowUnreleased bool) []*masterdata.Music {
	if source == nil || len(ids) == 0 {
		return nil
	}
	matches := make([]*masterdata.Music, 0, len(ids))
	seen := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		musicInfo, err := source.GetMusicByID(id)
		if err != nil || !isMusicAccessibleAt(musicInfo, now, allowUnreleased) {
			continue
		}
		matches = append(matches, musicInfo)
	}
	return matches
}

func resolveUniqueMusicQuery(source DataSource, query string, allowUnreleased bool) (*masterdata.Music, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, notfound.Music("")
	}

	queryLower := strings.ToLower(query)
	now := currentMusicVisibilityTime()
	if matches := collectMusicMatches(source, func(musicInfo *masterdata.Music) bool {
		return strings.EqualFold(strings.TrimSpace(musicInfo.Title), query)
	}, now, allowUnreleased); len(matches) > 0 {
		return selectUniqueMusicMatch("title_or_alias", matches)
	}

	if matches := collectMusicMatches(source, func(musicInfo *masterdata.Music) bool {
		return strings.Contains(strings.ToLower(strings.TrimSpace(musicInfo.Title)), queryLower)
	}, now, allowUnreleased); len(matches) > 0 {
		return selectUniqueMusicMatch("title_or_alias", matches)
	}

	if matches := collectLocalizedMusicMatches(source, func(title string) bool {
		return strings.EqualFold(strings.TrimSpace(title), query)
	}, now, allowUnreleased); len(matches) > 0 {
		return selectUniqueMusicMatch("title_or_alias", matches)
	}

	if matches := collectLocalizedMusicMatches(source, func(title string) bool {
		return strings.Contains(strings.ToLower(strings.TrimSpace(title)), queryLower)
	}, now, allowUnreleased); len(matches) > 0 {
		return selectUniqueMusicMatch("title_or_alias", matches)
	}
	if allowUnreleased {
		return nil, notfound.Music(query)
	}
	if matches := collectUnreleasedMusicMatches(source, func(musicInfo *masterdata.Music) bool {
		return strings.EqualFold(strings.TrimSpace(musicInfo.Title), query)
	}, now); len(matches) > 0 {
		return nil, releasecheck.New(releasecheck.KindMusic, query, 0)
	}
	if matches := collectUnreleasedMusicMatches(source, func(musicInfo *masterdata.Music) bool {
		return strings.Contains(strings.ToLower(strings.TrimSpace(musicInfo.Title)), queryLower)
	}, now); len(matches) > 0 {
		return nil, releasecheck.New(releasecheck.KindMusic, query, 0)
	}
	if matches := collectUnreleasedLocalizedMusicMatches(source, func(title string) bool {
		return strings.EqualFold(strings.TrimSpace(title), query)
	}, now); len(matches) > 0 {
		return nil, releasecheck.New(releasecheck.KindMusic, query, 0)
	}
	if matches := collectUnreleasedLocalizedMusicMatches(source, func(title string) bool {
		return strings.Contains(strings.ToLower(strings.TrimSpace(title)), queryLower)
	}, now); len(matches) > 0 {
		return nil, releasecheck.New(releasecheck.KindMusic, query, 0)
	}

	return nil, notfound.Music(query)
}

func resolveUniqueMusicKeyword(source DataSource, keyword string, allowUnreleased bool) (*masterdata.Music, error) {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return nil, usererror.Misuse(i18n.M("music.query_required"))
	}

	now := currentMusicVisibilityTime()
	matches := make([]*masterdata.Music, 0)
	for _, item := range source.GetMusics() {
		if !isMusicAccessibleAt(item, now, allowUnreleased) || !matchesMusicKeyword(source, item, keyword) {
			continue
		}
		matches = append(matches, item)
	}
	if len(matches) == 0 {
		return nil, nil
	}
	return selectUniqueMusicMatch("title_or_alias", matches)
}

func collectMusicMatches(source DataSource, matcher func(*masterdata.Music) bool, now int64, allowUnreleased bool) []*masterdata.Music {
	if source == nil || matcher == nil {
		return nil
	}
	matches := make([]*masterdata.Music, 0)
	for _, item := range source.GetMusics() {
		if !isMusicAccessibleAt(item, now, allowUnreleased) || !matcher(item) {
			continue
		}
		matches = append(matches, item)
	}
	return matches
}

func collectLocalizedMusicMatches(source DataSource, matcher func(string) bool, now int64, allowUnreleased bool) []*masterdata.Music {
	if source == nil || matcher == nil {
		return nil
	}
	matches := make([]*masterdata.Music, 0)
	for _, item := range source.GetMusics() {
		if !isMusicAccessibleAt(item, now, allowUnreleased) {
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

func collectUnreleasedMusicMatches(source DataSource, matcher func(*masterdata.Music) bool, now int64) []*masterdata.Music {
	if source == nil || matcher == nil {
		return nil
	}
	matches := make([]*masterdata.Music, 0)
	for _, item := range source.GetMusics() {
		if item == nil || isMusicVisibleAt(item, now) || !matcher(item) {
			continue
		}
		matches = append(matches, item)
	}
	return matches
}

func collectUnreleasedLocalizedMusicMatches(source DataSource, matcher func(string) bool, now int64) []*masterdata.Music {
	if source == nil || matcher == nil {
		return nil
	}
	matches := make([]*masterdata.Music, 0)
	for _, item := range source.GetMusics() {
		if item == nil || isMusicVisibleAt(item, now) {
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
	deduped := dedupeMusicMatchTitles(matches)
	if len(deduped) == 0 {
		return nil, nil
	}
	ids := sortedMusicMatchIDs(deduped)
	if len(ids) == 1 {
		return copyMusicMatch(matches, ids[0], deduped[ids[0]]), nil
	}
	return nil, &musicAmbiguousQueryError{
		sourceName: sourceName,
		candidates: musicMatchCandidates(ids, deduped),
	}
}

func dedupeMusicMatchTitles(matches []*masterdata.Music) map[int]string {
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
	return deduped
}

func sortedMusicMatchIDs(deduped map[int]string) []int {
	ids := make([]int, 0, len(deduped))
	for id := range deduped {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func copyMusicMatch(matches []*masterdata.Music, id int, fallbackTitle string) *masterdata.Music {
	for _, item := range matches {
		if item != nil && item.ID == id {
			return new(*item)
		}
	}
	return &masterdata.Music{ID: id, Title: fallbackTitle}
}

func musicMatchCandidates(ids []int, titles map[int]string) []musicQueryCandidate {
	candidates := make([]musicQueryCandidate, 0, len(ids))
	for _, id := range ids {
		candidates = append(candidates, musicQueryCandidate{
			ID:    id,
			Title: titles[id],
		})
	}
	return candidates
}
