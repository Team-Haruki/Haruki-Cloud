package music

import (
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestExtractMusicDifficultySupportsInlineForms(t *testing.T) {
	diff, cleaned := ExtractMusicDifficulty("テオex")
	if diff != "expert" || cleaned != "テオ" {
		t.Fatalf("unexpected inline expert diff: diff=%q cleaned=%q", diff, cleaned)
	}

	diff, cleaned = ExtractMusicDifficulty("music123紫谱")
	if diff != "master" || cleaned != "music123" {
		t.Fatalf("unexpected inline master diff: diff=%q cleaned=%q", diff, cleaned)
	}

	diff, cleaned = ExtractMusicDifficulty("红")
	if diff != "" || cleaned != "红" {
		t.Fatalf("single-char diff alias should not be recognized: diff=%q cleaned=%q", diff, cleaned)
	}

	diff, cleaned = ExtractMusicDifficulty("hnm")
	if diff != "" || cleaned != "hnm" {
		t.Fatalf("embedded normal alias should not be recognized inside hnm: diff=%q cleaned=%q", diff, cleaned)
	}

	diff, cleaned = ExtractMusicDifficulty("hnm ex")
	if diff != "expert" || cleaned != "hnm" {
		t.Fatalf("spaced expert alias should still be recognized: diff=%q cleaned=%q", diff, cleaned)
	}
}

func TestParserSupportsExplicitMusicID(t *testing.T) {
	parser := NewParser(nil)
	info, err := parser.Parse("music123ex")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if info.Type != QueryTypeID || info.Value != 123 {
		t.Fatalf("unexpected query info: %+v", info)
	}
	if info.Difficulty != "expert" || info.AllowTitleFallback {
		t.Fatalf("unexpected explicit id parsing: %+v", info)
	}
}

func TestSearchFallsBackFromImplicitMusicIDToTitle(t *testing.T) {
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "123"},
		},
	}
	searcher := NewSearchService(source, NewParser(nil))

	musicInfo, err := searcher.Search("123")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if musicInfo.ID != 1 {
		t.Fatalf("unexpected music from implicit id fallback: %+v", musicInfo)
	}
}

func TestSearchRejectsMissingExplicitMusicID(t *testing.T) {
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A"},
		},
	}
	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)

	if _, err := controller.ResolveMusicCover(Query{Query: "music999", Region: "jp"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestSearchNegativeSequenceIgnoresUnreleasedMusic(t *testing.T) {
	now := time.Now().UnixMilli()
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", PublishedAt: now - 30_000},
			2: {ID: 2, Title: "Song B", PublishedAt: now - 10_000},
			3: {ID: 3, Title: "Song C", PublishedAt: now + 60_000},
		},
	}
	searcher := NewSearchService(source, NewParser(nil))

	musicInfo, err := searcher.Search("-1")
	if err != nil {
		t.Fatalf("Search(-1) error = %v", err)
	}
	if musicInfo.ID != 2 {
		t.Fatalf("expected latest released music 2, got %+v", musicInfo)
	}

	musicInfo, err = searcher.Search("-2")
	if err != nil {
		t.Fatalf("Search(-2) error = %v", err)
	}
	if musicInfo.ID != 1 {
		t.Fatalf("expected second latest released music 1, got %+v", musicInfo)
	}
}

func TestSearchNegativeSequenceStillIgnoresUnreleasedMusicWhenLookupLeaksEnabled(t *testing.T) {
	now := time.Now().UnixMilli()
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", PublishedAt: now - 30_000},
			2: {ID: 2, Title: "Song B", PublishedAt: now - 10_000},
			3: {ID: 3, Title: "Song C", PublishedAt: now + 60_000},
		},
	}
	searcher := NewSearchService(source, NewParser(nil)).WithAllowUnreleased(true)

	musicInfo, err := searcher.Search("-1")
	if err != nil {
		t.Fatalf("Search(-1) error = %v", err)
	}
	if musicInfo.ID != 2 {
		t.Fatalf("expected latest released music 2 even with allowUnreleased, got %+v", musicInfo)
	}
}

func TestSearchBanLikeAliasPrefersTitleResolver(t *testing.T) {
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			5: {ID: 5, Title: "Mizuki Alias Song"},
		},
	}
	searcher := NewSearchService(source, NewParser(defaultBanCharacterNicknames)).
		WithTitleResolver(func(query string) (*masterdata.Music, error) {
			if query != "mzk5" {
				t.Fatalf("unexpected title resolver query: %q", query)
			}
			return source.GetMusicByID(5)
		})

	musicInfo, err := searcher.Search("mzk5")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if musicInfo == nil || musicInfo.ID != 5 {
		t.Fatalf("expected alias resolver result, got %+v", musicInfo)
	}
}
