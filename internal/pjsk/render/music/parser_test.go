package music

import (
	"testing"

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
