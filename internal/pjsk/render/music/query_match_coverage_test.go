package music

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/releasecheck"
	"haruki-cloud/internal/testutil"
)

type queryMatchCoverageSource struct {
	*lookupTestSource
	localized    map[int][]string
	localizedErr map[int]error
	tags         map[int][]string
	tagErr       map[int]error
}

func (s *queryMatchCoverageSource) GetMusicLocalizedTitles(id int) ([]string, error) {
	if err := s.localizedErr[id]; err != nil {
		return nil, err
	}
	return append([]string(nil), s.localized[id]...), nil
}

func (s *queryMatchCoverageSource) GetMusicTags(id int) ([]string, error) {
	if err := s.tagErr[id]; err != nil {
		return nil, err
	}
	return append([]string(nil), s.tags[id]...), nil
}

func newQueryMatchSource(items ...*masterdata.Music) *queryMatchCoverageSource {
	musics := make(map[int]*masterdata.Music, len(items))
	for _, item := range items {
		if item != nil {
			musics[item.ID] = item
		}
	}
	return &queryMatchCoverageSource{
		lookupTestSource: &lookupTestSource{musics: musics},
		localized:        make(map[int][]string),
		localizedErr:     make(map[int]error),
		tags:             make(map[int][]string),
		tagErr:           make(map[int]error),
	}
}

func TestAmbiguousMusicErrorExtractionBranches(t *testing.T) {
	typed := &musicAmbiguousQueryError{
		sourceName: "alias",
		candidates: []musicQueryCandidate{{ID: 3, Title: "Three"}, {ID: -1, Title: "bad"}, {ID: 2, Title: "Two"}},
	}
	{
		testutil.Require(t, strings.Contains(typed.Error(), "music3/Three"), "ambiguous error was not recognized: %v", typed)
		testutil.Require(t, isMusicAmbiguousError(typed), "ambiguous error was not recognized: %v", typed)
		testutil.Require(t, isMusicAmbiguousError(errors.New("匹配到多个歌曲")), "ambiguous error was not recognized: %v", typed)
	}
	{
		testutil.RequireArgs(t, !(isMusicAmbiguousError(nil)), "non-ambiguous error was recognized")
		testutil.RequireArgs(t, !(isMusicAmbiguousError(errors.New("other"))), "non-ambiguous error was recognized")
	}
	{

		got := ExtractAmbiguousMusicIDs(typed)
		testutil.Require(t, reflect.DeepEqual(got, []int{3, 2}), "typed ambiguous IDs = %v", got)
	}

	textErr := errors.New("prefix\n music10/Ten \nMUSIC2/Two\nmusic10/Duplicate\nmusicx/Bad\nmusic0/Zero\nmusic7 no slash")
	{
		got := ExtractAmbiguousMusicIDs(textErr)
		testutil.Require(t, reflect.DeepEqual(got, []int{2, 10}), "text ambiguous IDs = %v", got)
	}
	{

		got := ExtractAmbiguousMusicIDs(nil)
		testutil.Require(t, !(got != nil), "nil ambiguous IDs = %v", got)
	}
	{

		got := ExtractAmbiguousMusicIDs(errors.New("no IDs"))
		testutil.Require(t, !(got != nil), "invalid ambiguous IDs = %v", got)
	}

}

func TestCollectVisibleMusicMatchesByIDBranches(t *testing.T) {
	now := time.Now().UnixMilli()
	source := newQueryMatchSource(
		&masterdata.Music{ID: 1, Title: "Visible", PublishedAt: now - 1},
		&masterdata.Music{ID: 2, Title: "Future", PublishedAt: now + 60_000},
	)
	{
		got := collectVisibleMusicMatchesByID(nil, []int{1}, now, false)
		testutil.Require(t, !(got != nil), "nil source matches = %v", got)
	}
	{

		got := collectVisibleMusicMatchesByID(source, nil, now, false)
		testutil.Require(t, !(got != nil), "nil IDs matches = %v", got)
	}

	got := collectVisibleMusicMatchesByID(source, []int{0, 1, 1, 2, 99}, now, false)
	{
		testutil.Require(t, !(len(got) != 1), "visible matches = %+v", got)
		testutil.Require(t, !(got[0].ID != 1), "visible matches = %+v", got)
	}

	got = collectVisibleMusicMatchesByID(source, []int{2}, now, true)
	{
		testutil.Require(t, !(len(got) != 1), "allow-unreleased matches = %+v", got)
		testutil.Require(t, !(got[0].ID != 2), "allow-unreleased matches = %+v", got)
	}

}

func TestResolveUniqueMusicQueryAllMatchKinds(t *testing.T) {
	now := time.Now().UnixMilli()
	visibleA := &masterdata.Music{ID: 1, Title: "Alpha Song", PublishedAt: now - 1}
	visibleB := &masterdata.Music{ID: 2, Title: "Alpha Remix", PublishedAt: now - 1}
	future := &masterdata.Music{ID: 3, Title: "Future Song", PublishedAt: now + 60_000}
	{

		_, err := resolveUniqueMusicQuery(newQueryMatchSource(), " ", false)
		testutil.RequireArgs(t, !(err == nil), "empty unique query succeeded")
	}
	{

		got, err := resolveUniqueMusicQuery(newQueryMatchSource(visibleA), " alpha song ", false)
		{
			testutil.Require(t, !(err != nil), "exact title match = %+v, %v", got, err)
			testutil.Require(t, !(got.ID != 1), "exact title match = %+v, %v", got, err)
		}
	}
	{

		_, err := resolveUniqueMusicQuery(newQueryMatchSource(visibleA, visibleB), "alpha", false)
		testutil.Require(t, isMusicAmbiguousError(err), "substring ambiguity = %v", err)
	}

	localizedExact := newQueryMatchSource(visibleA)
	localizedExact.localized[1] = []string{"阿尔法之歌"}
	{
		got, err := resolveUniqueMusicQuery(localizedExact, "阿尔法之歌", false)
		{
			testutil.Require(t, !(err != nil), "localized exact match = %+v, %v", got, err)
			testutil.Require(t, !(got.ID != 1), "localized exact match = %+v, %v", got, err)
		}
	}

	localizedContains := newQueryMatchSource(visibleA)
	localizedContains.localized[1] = []string{"Localized Alpha"}
	{
		got, err := resolveUniqueMusicQuery(localizedContains, "localized", false)
		{
			testutil.Require(t, !(err != nil), "localized substring match = %+v, %v", got, err)
			testutil.Require(t, !(got.ID != 1), "localized substring match = %+v, %v", got, err)
		}
	}
	{

		_, err := resolveUniqueMusicQuery(newQueryMatchSource(), "missing", true)
		{
			testutil.Require(t, !(err == nil), "allow-unreleased missing error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "not found"), "allow-unreleased missing error = %v", err)
		}
	}

	for name, sourceAndQuery := range map[string]struct {
		source *queryMatchCoverageSource
		query  string
	}{
		"title exact":    {newQueryMatchSource(future), "Future Song"},
		"title contains": {newQueryMatchSource(&masterdata.Music{ID: 4, Title: "Brand New Future", PublishedAt: now + 60_000}), "new future"},
	} {
		t.Run(name, func(t *testing.T) {
			{
				_, err := resolveUniqueMusicQuery(sourceAndQuery.source, sourceAndQuery.query, false)
				{
					testutil.Require(t, !(err == nil), "unreleased query error = %v", err)
					testutil.Require(t, isUnreleasedMusicTestError(err), "unreleased query error = %v", err)
				}
			}

		})
	}
	futureLocalizedExact := newQueryMatchSource(future)
	futureLocalizedExact.localized[3] = []string{"未来之歌"}
	{
		_, err := resolveUniqueMusicQuery(futureLocalizedExact, "未来之歌", false)
		{
			testutil.Require(t, !(err == nil), "unreleased localized exact error = %v", err)
			testutil.Require(t, isUnreleasedMusicTestError(err), "unreleased localized exact error = %v", err)
		}
	}

	futureLocalizedContains := newQueryMatchSource(future)
	futureLocalizedContains.localized[3] = []string{"Localized Future Song"}
	{
		_, err := resolveUniqueMusicQuery(futureLocalizedContains, "localized future", false)
		{
			testutil.Require(t, !(err == nil), "unreleased localized substring error = %v", err)
			testutil.Require(t, isUnreleasedMusicTestError(err), "unreleased localized substring error = %v", err)
		}
	}
	{

		_, err := resolveUniqueMusicQuery(newQueryMatchSource(), "absent", false)
		{
			testutil.Require(t, !(err == nil), "missing query error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "not found"), "missing query error = %v", err)
		}
	}

}

func isUnreleasedMusicTestError(err error) bool {
	var unreleased *releasecheck.UnreleasedError
	return errors.As(err, &unreleased)
}

func TestUniqueKeywordAndCollectionHelpers(t *testing.T) {
	now := time.Now().UnixMilli()
	a := &masterdata.Music{ID: 1, Title: "Alpha", Pronunciation: "arufa", PublishedAt: now - 1}
	b := &masterdata.Music{ID: 2, Title: "Beta", PublishedAt: now - 1}
	source := newQueryMatchSource(a, b)
	source.tags[1] = []string{"vocaloid"}
	source.localized[2] = []string{"贝塔歌曲"}
	{

		_, err := resolveUniqueMusicKeyword(source, " ", false)
		testutil.RequireArgs(t, !(err == nil), "empty keyword succeeded")
	}
	{

		got, err := resolveUniqueMusicKeyword(source, "missing", false)
		{
			testutil.Require(t, !(err != nil), "missing keyword = %+v, %v", got, err)
			testutil.Require(t, !(got != nil), "missing keyword = %+v, %v", got, err)
		}
	}
	{

		got, err := resolveUniqueMusicKeyword(source, "vocaloid", false)
		{
			testutil.Require(t, !(err != nil), "tag keyword = %+v, %v", got, err)
			testutil.Require(t, !(got.ID != 1), "tag keyword = %+v, %v", got, err)
		}
	}
	{

		got, err := resolveUniqueMusicKeyword(source, "贝塔", false)
		{
			testutil.Require(t, !(err != nil), "localized keyword = %+v, %v", got, err)
			testutil.Require(t, !(got.ID != 2), "localized keyword = %+v, %v", got, err)
		}
	}

	source.tags[2] = []string{"vocaloid"}
	{
		_, err := resolveUniqueMusicKeyword(source, "vocaloid", false)
		testutil.Require(t, isMusicAmbiguousError(err), "ambiguous keyword = %v", err)
	}
	{
		testutil.RequireArgs(t, !(collectMusicMatches(nil, func(*masterdata.Music) bool { return true }, now, false) != nil), "collectMusicMatches nil guards failed")
		testutil.RequireArgs(t, !(collectMusicMatches(source, nil, now, false) != nil), "collectMusicMatches nil guards failed")
	}
	{

		got := collectMusicMatches(source, func(item *masterdata.Music) bool { return item.ID == 1 }, now, false)
		testutil.Require(t, !(len(got) != 1), "collected music matches = %+v", got)
	}
	{
		testutil.RequireArgs(t, !(collectLocalizedMusicMatches(nil, func(string) bool { return true }, now, false) != nil), "collectLocalizedMusicMatches nil guards failed")
		testutil.RequireArgs(t, !(collectLocalizedMusicMatches(source, nil, now, false) != nil), "collectLocalizedMusicMatches nil guards failed")
	}

	source.localizedErr[1] = errors.New("localized")
	{
		got := collectLocalizedMusicMatches(source, func(string) bool { return true }, now, false)
		{
			testutil.Require(t, !(len(got) != 1), "localized error filtering = %+v", got)
			testutil.Require(t, !(got[0].ID != 2), "localized error filtering = %+v", got)
		}
	}

	future := &masterdata.Music{ID: 3, Title: "Future", PublishedAt: now + 60_000}
	futureSource := newQueryMatchSource(a, future)
	{
		testutil.RequireArgs(t, !(collectUnreleasedMusicMatches(nil, func(*masterdata.Music) bool { return true }, now) != nil), "collectUnreleasedMusicMatches nil guards failed")
		testutil.RequireArgs(t, !(collectUnreleasedMusicMatches(futureSource, nil, now) != nil), "collectUnreleasedMusicMatches nil guards failed")
	}
	{

		got := collectUnreleasedMusicMatches(futureSource, func(item *masterdata.Music) bool { return true }, now)
		{
			testutil.Require(t, !(len(got) != 1), "unreleased matches = %+v", got)
			testutil.Require(t, !(got[0].ID != 3), "unreleased matches = %+v", got)
		}
	}
	{
		testutil.RequireArgs(t, !(collectUnreleasedLocalizedMusicMatches(nil, func(string) bool { return true }, now) != nil), "collectUnreleasedLocalizedMusicMatches nil guards failed")
		testutil.RequireArgs(t, !(collectUnreleasedLocalizedMusicMatches(futureSource, nil, now) != nil), "collectUnreleasedLocalizedMusicMatches nil guards failed")
	}

	futureSource.localizedErr[3] = errors.New("localized")
	{
		got := collectUnreleasedLocalizedMusicMatches(futureSource, func(string) bool { return true }, now)
		testutil.Require(t, !(len(got) != 0), "unreleased localized error matches = %+v", got)
	}

	futureSource.localizedErr[3] = nil
	futureSource.localized[3] = []string{"", "Future Local"}
	{
		got := collectUnreleasedLocalizedMusicMatches(futureSource, func(title string) bool { return strings.Contains(title, "Local") }, now)
		testutil.Require(t, !(len(got) != 1), "unreleased localized matches = %+v", got)
	}

}

func TestSelectUniqueMusicMatchBranches(t *testing.T) {
	{
		got, err := selectUniqueMusicMatch("source", []*masterdata.Music{nil, {ID: 0}})
		{
			testutil.Require(t, !(err != nil), "empty unique selection = %+v, %v", got, err)
			testutil.Require(t, !(got != nil), "empty unique selection = %+v, %v", got, err)
		}
	}

	single := &masterdata.Music{ID: 2, Title: " Song "}
	{
		got, err := selectUniqueMusicMatch("source", []*masterdata.Music{single, single})
		{
			testutil.Require(t, !(err != nil), "single unique selection = %+v, %v", got, err)
			testutil.Require(t, !(got.ID != 2), "single unique selection = %+v, %v", got, err)
			testutil.Require(t, !(got.Title != " Song "), "single unique selection = %+v, %v", got, err)
			testutil.Require(t, !(got == single), "single unique selection = %+v, %v", got, err)
		}
	}

	_, err := selectUniqueMusicMatch("aliases", []*masterdata.Music{{ID: 9}, {ID: 3, Title: "Three"}})
	var ambiguous *musicAmbiguousQueryError
	{
		testutil.Require(t, errors.As(err, &ambiguous), "ambiguous selection = %+v, %v", ambiguous, err)
		testutil.Require(t, !(len(ambiguous.candidates) != 2), "ambiguous selection = %+v, %v", ambiguous, err)
		testutil.Require(t, !(ambiguous.candidates[0].ID != 3), "ambiguous selection = %+v, %v", ambiguous, err)
		testutil.Require(t, !(ambiguous.candidates[1].Title != "music9"), "ambiguous selection = %+v, %v", ambiguous, err)
	}

}
