package music

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/releasecheck"
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
	if !strings.Contains(typed.Error(), "music3/Three") || !isMusicAmbiguousError(typed) || !isMusicAmbiguousError(errors.New("匹配到多个歌曲")) {
		t.Fatalf("ambiguous error was not recognized: %v", typed)
	}
	if isMusicAmbiguousError(nil) || isMusicAmbiguousError(errors.New("other")) {
		t.Fatal("non-ambiguous error was recognized")
	}
	if got := ExtractAmbiguousMusicIDs(typed); !reflect.DeepEqual(got, []int{3, 2}) {
		t.Fatalf("typed ambiguous IDs = %v", got)
	}
	textErr := errors.New("prefix\n music10/Ten \nMUSIC2/Two\nmusic10/Duplicate\nmusicx/Bad\nmusic0/Zero\nmusic7 no slash")
	if got := ExtractAmbiguousMusicIDs(textErr); !reflect.DeepEqual(got, []int{2, 10}) {
		t.Fatalf("text ambiguous IDs = %v", got)
	}
	if got := ExtractAmbiguousMusicIDs(nil); got != nil {
		t.Fatalf("nil ambiguous IDs = %v", got)
	}
	if got := ExtractAmbiguousMusicIDs(errors.New("no IDs")); got != nil {
		t.Fatalf("invalid ambiguous IDs = %v", got)
	}
}

func TestCollectVisibleMusicMatchesByIDBranches(t *testing.T) {
	now := time.Now().UnixMilli()
	source := newQueryMatchSource(
		&masterdata.Music{ID: 1, Title: "Visible", PublishedAt: now - 1},
		&masterdata.Music{ID: 2, Title: "Future", PublishedAt: now + 60_000},
	)
	if got := collectVisibleMusicMatchesByID(nil, []int{1}, now, false); got != nil {
		t.Fatalf("nil source matches = %v", got)
	}
	if got := collectVisibleMusicMatchesByID(source, nil, now, false); got != nil {
		t.Fatalf("nil IDs matches = %v", got)
	}
	got := collectVisibleMusicMatchesByID(source, []int{0, 1, 1, 2, 99}, now, false)
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("visible matches = %+v", got)
	}
	got = collectVisibleMusicMatchesByID(source, []int{2}, now, true)
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("allow-unreleased matches = %+v", got)
	}
}

func TestResolveUniqueMusicQueryAllMatchKinds(t *testing.T) {
	now := time.Now().UnixMilli()
	visibleA := &masterdata.Music{ID: 1, Title: "Alpha Song", PublishedAt: now - 1}
	visibleB := &masterdata.Music{ID: 2, Title: "Alpha Remix", PublishedAt: now - 1}
	future := &masterdata.Music{ID: 3, Title: "Future Song", PublishedAt: now + 60_000}

	if _, err := resolveUniqueMusicQuery(newQueryMatchSource(), " ", false); err == nil {
		t.Fatal("empty unique query succeeded")
	}
	if got, err := resolveUniqueMusicQuery(newQueryMatchSource(visibleA), " alpha song ", false); err != nil || got.ID != 1 {
		t.Fatalf("exact title match = %+v, %v", got, err)
	}
	if _, err := resolveUniqueMusicQuery(newQueryMatchSource(visibleA, visibleB), "alpha", false); !isMusicAmbiguousError(err) {
		t.Fatalf("substring ambiguity = %v", err)
	}

	localizedExact := newQueryMatchSource(visibleA)
	localizedExact.localized[1] = []string{"阿尔法之歌"}
	if got, err := resolveUniqueMusicQuery(localizedExact, "阿尔法之歌", false); err != nil || got.ID != 1 {
		t.Fatalf("localized exact match = %+v, %v", got, err)
	}
	localizedContains := newQueryMatchSource(visibleA)
	localizedContains.localized[1] = []string{"Localized Alpha"}
	if got, err := resolveUniqueMusicQuery(localizedContains, "localized", false); err != nil || got.ID != 1 {
		t.Fatalf("localized substring match = %+v, %v", got, err)
	}

	if _, err := resolveUniqueMusicQuery(newQueryMatchSource(), "missing", true); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("allow-unreleased missing error = %v", err)
	}
	for name, sourceAndQuery := range map[string]struct {
		source *queryMatchCoverageSource
		query  string
	}{
		"title exact":    {newQueryMatchSource(future), "Future Song"},
		"title contains": {newQueryMatchSource(&masterdata.Music{ID: 4, Title: "Brand New Future", PublishedAt: now + 60_000}), "new future"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := resolveUniqueMusicQuery(sourceAndQuery.source, sourceAndQuery.query, false); err == nil || !isUnreleasedMusicTestError(err) {
				t.Fatalf("unreleased query error = %v", err)
			}
		})
	}
	futureLocalizedExact := newQueryMatchSource(future)
	futureLocalizedExact.localized[3] = []string{"未来之歌"}
	if _, err := resolveUniqueMusicQuery(futureLocalizedExact, "未来之歌", false); err == nil || !isUnreleasedMusicTestError(err) {
		t.Fatalf("unreleased localized exact error = %v", err)
	}
	futureLocalizedContains := newQueryMatchSource(future)
	futureLocalizedContains.localized[3] = []string{"Localized Future Song"}
	if _, err := resolveUniqueMusicQuery(futureLocalizedContains, "localized future", false); err == nil || !isUnreleasedMusicTestError(err) {
		t.Fatalf("unreleased localized substring error = %v", err)
	}
	if _, err := resolveUniqueMusicQuery(newQueryMatchSource(), "absent", false); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing query error = %v", err)
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

	if _, err := resolveUniqueMusicKeyword(source, " ", false); err == nil {
		t.Fatal("empty keyword succeeded")
	}
	if got, err := resolveUniqueMusicKeyword(source, "missing", false); err != nil || got != nil {
		t.Fatalf("missing keyword = %+v, %v", got, err)
	}
	if got, err := resolveUniqueMusicKeyword(source, "vocaloid", false); err != nil || got.ID != 1 {
		t.Fatalf("tag keyword = %+v, %v", got, err)
	}
	if got, err := resolveUniqueMusicKeyword(source, "贝塔", false); err != nil || got.ID != 2 {
		t.Fatalf("localized keyword = %+v, %v", got, err)
	}
	source.tags[2] = []string{"vocaloid"}
	if _, err := resolveUniqueMusicKeyword(source, "vocaloid", false); !isMusicAmbiguousError(err) {
		t.Fatalf("ambiguous keyword = %v", err)
	}

	if collectMusicMatches(nil, func(*masterdata.Music) bool { return true }, now, false) != nil || collectMusicMatches(source, nil, now, false) != nil {
		t.Fatal("collectMusicMatches nil guards failed")
	}
	if got := collectMusicMatches(source, func(item *masterdata.Music) bool { return item.ID == 1 }, now, false); len(got) != 1 {
		t.Fatalf("collected music matches = %+v", got)
	}
	if collectLocalizedMusicMatches(nil, func(string) bool { return true }, now, false) != nil || collectLocalizedMusicMatches(source, nil, now, false) != nil {
		t.Fatal("collectLocalizedMusicMatches nil guards failed")
	}
	source.localizedErr[1] = errors.New("localized")
	if got := collectLocalizedMusicMatches(source, func(string) bool { return true }, now, false); len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("localized error filtering = %+v", got)
	}

	future := &masterdata.Music{ID: 3, Title: "Future", PublishedAt: now + 60_000}
	futureSource := newQueryMatchSource(a, future)
	if collectUnreleasedMusicMatches(nil, func(*masterdata.Music) bool { return true }, now) != nil || collectUnreleasedMusicMatches(futureSource, nil, now) != nil {
		t.Fatal("collectUnreleasedMusicMatches nil guards failed")
	}
	if got := collectUnreleasedMusicMatches(futureSource, func(item *masterdata.Music) bool { return true }, now); len(got) != 1 || got[0].ID != 3 {
		t.Fatalf("unreleased matches = %+v", got)
	}
	if collectUnreleasedLocalizedMusicMatches(nil, func(string) bool { return true }, now) != nil || collectUnreleasedLocalizedMusicMatches(futureSource, nil, now) != nil {
		t.Fatal("collectUnreleasedLocalizedMusicMatches nil guards failed")
	}
	futureSource.localizedErr[3] = errors.New("localized")
	if got := collectUnreleasedLocalizedMusicMatches(futureSource, func(string) bool { return true }, now); len(got) != 0 {
		t.Fatalf("unreleased localized error matches = %+v", got)
	}
	futureSource.localizedErr[3] = nil
	futureSource.localized[3] = []string{"", "Future Local"}
	if got := collectUnreleasedLocalizedMusicMatches(futureSource, func(title string) bool { return strings.Contains(title, "Local") }, now); len(got) != 1 {
		t.Fatalf("unreleased localized matches = %+v", got)
	}
}

func TestSelectUniqueMusicMatchBranches(t *testing.T) {
	if got, err := selectUniqueMusicMatch("source", []*masterdata.Music{nil, {ID: 0}}); err != nil || got != nil {
		t.Fatalf("empty unique selection = %+v, %v", got, err)
	}
	single := &masterdata.Music{ID: 2, Title: " Song "}
	if got, err := selectUniqueMusicMatch("source", []*masterdata.Music{single, single}); err != nil || got.ID != 2 || got.Title != " Song " || got == single {
		t.Fatalf("single unique selection = %+v, %v", got, err)
	}
	_, err := selectUniqueMusicMatch("aliases", []*masterdata.Music{{ID: 9}, {ID: 3, Title: "Three"}})
	var ambiguous *musicAmbiguousQueryError
	if !errors.As(err, &ambiguous) || len(ambiguous.candidates) != 2 || ambiguous.candidates[0].ID != 3 || ambiguous.candidates[1].Title != "music9" {
		t.Fatalf("ambiguous selection = %+v, %v", ambiguous, err)
	}
}
