package music

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type lookupTestSource struct {
	musics       map[int]*masterdata.Music
	difficulties map[int][]*masterdata.MusicDifficulty
}

type lookupTestAliasResolver struct {
	ids      map[string]int
	approved map[int][]string
	err      error
}

type lookupContextKey string

type contextAwareLookupTestAliasResolver struct {
	wantKey   lookupContextKey
	wantValue string
	id        int
}

func (s *lookupTestSource) DefaultRegion() renderregion.Value { return renderregion.JP }

func (r *lookupTestAliasResolver) TryResolveMusicID(_ context.Context, token string) (int, bool, error) {
	if r == nil {
		return 0, false, nil
	}
	if r.err != nil {
		return 0, false, r.err
	}
	id, ok := r.ids[strings.ToLower(strings.TrimSpace(token))]
	return id, ok, nil
}

func (r *lookupTestAliasResolver) TryResolveMusicTitleOrAliasID(_ context.Context, token string) (int, bool, error) {
	if r == nil {
		return 0, false, nil
	}
	if r.err != nil {
		return 0, false, r.err
	}
	id, ok := r.ids[strings.ToLower(strings.TrimSpace(token))]
	return id, ok, nil
}

func (r *lookupTestAliasResolver) ListApprovedMusicAliases(_ context.Context, musicID int) ([]string, error) {
	if r == nil {
		return nil, nil
	}
	if r.err != nil {
		return nil, r.err
	}
	items := r.approved[musicID]
	if len(items) == 0 {
		return nil, nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *contextAwareLookupTestAliasResolver) TryResolveMusicID(ctx context.Context, token string) (int, bool, error) {
	return r.TryResolveMusicTitleOrAliasID(ctx, token)
}

func (r *contextAwareLookupTestAliasResolver) TryResolveMusicTitleOrAliasID(ctx context.Context, token string) (int, bool, error) {
	if r == nil {
		return 0, false, nil
	}
	if strings.ToLower(strings.TrimSpace(token)) != "ctx song" {
		return 0, false, nil
	}
	value, _ := ctx.Value(r.wantKey).(string)
	if value != r.wantValue {
		return 0, false, context.Canceled
	}
	return r.id, true, nil
}

func (s *lookupTestSource) SearchMusic(query string) (*masterdata.Music, error) {
	for _, item := range s.musics {
		if strings.EqualFold(item.Title, query) {
			copy := *item
			return &copy, nil
		}
	}
	return nil, errNotFound("music")
}

func (s *lookupTestSource) GetMusicByID(id int) (*masterdata.Music, error) {
	item := s.musics[id]
	if item == nil {
		return nil, errNotFound("music")
	}
	copy := *item
	return &copy, nil
}

func (s *lookupTestSource) GetMusicByEventID(int) (*masterdata.Music, error) {
	return nil, errNotFound("music")
}

func (s *lookupTestSource) GetMusics() []*masterdata.Music {
	out := make([]*masterdata.Music, 0, len(s.musics))
	for _, item := range s.musics {
		copy := *item
		out = append(out, &copy)
	}
	return out
}

func (s *lookupTestSource) GetBanEvents(int) []*masterdata.Event { return nil }

func (s *lookupTestSource) GetMusicLocalizedTitles(int) ([]string, error) { return nil, nil }

func (s *lookupTestSource) GetMusicDifficulties(musicID int) ([]*masterdata.MusicDifficulty, error) {
	items := s.difficulties[musicID]
	out := make([]*masterdata.MusicDifficulty, 0, len(items))
	for _, item := range items {
		copy := *item
		out = append(out, &copy)
	}
	return out, nil
}

func (s *lookupTestSource) GetMusicVocals(int) ([]*masterdata.MusicVocal, error) { return nil, nil }

func (s *lookupTestSource) GetMusicTags(int) ([]string, error) { return nil, nil }

func (s *lookupTestSource) GetCharacterByID(int) (*masterdata.Character, error) {
	return nil, errNotFound("character")
}

func (s *lookupTestSource) GetOutsideCharacterByID(int) (string, error) {
	return "", errNotFound("outside character")
}

func (s *lookupTestSource) GetPrimaryEventByMusicID(int) (*masterdata.Event, error) {
	return nil, errNotFound("event")
}

func (s *lookupTestSource) GetLimitedTimeMusics(int) []*masterdata.LimitedTimeMusic { return nil }

func TestFindMusicChartsByNoteCount(t *testing.T) {
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1:   {ID: 1, Title: "Song A"},
			2:   {ID: 2, Title: "Song B"},
			241: {ID: 241, Title: "Hidden Song"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31, TotalNoteCount: 777},
				{MusicID: 1, MusicDifficulty: "expert", PlayLevel: 27, TotalNoteCount: 777},
			},
			2: {
				{MusicID: 2, MusicDifficulty: "append", PlayLevel: 34, TotalNoteCount: 777},
			},
			241: {
				{MusicID: 241, MusicDifficulty: "master", PlayLevel: 99, TotalNoteCount: 777},
			},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	matches, err := controller.FindMusicChartsByNoteCount(NoteCountQuery{NoteCount: 777, Region: "jp"})
	if err != nil {
		t.Fatalf("FindMusicChartsByNoteCount() error = %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matches))
	}
	if matches[0].Music.ID != 1 || matches[0].Difficulty != "expert" {
		t.Fatalf("unexpected first match: %+v", matches[0])
	}
	if matches[1].Music.ID != 1 || matches[1].Difficulty != "master" {
		t.Fatalf("unexpected second match: %+v", matches[1])
	}
	if matches[2].Music.ID != 2 || matches[2].Difficulty != "append" {
		t.Fatalf("unexpected third match: %+v", matches[2])
	}
}

func TestFindMusicChartsByNoteCountSupportsDifficultyFilter(t *testing.T) {
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A"},
			2: {ID: 2, Title: "Song B"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31, TotalNoteCount: 777},
				{MusicID: 1, MusicDifficulty: "expert", PlayLevel: 27, TotalNoteCount: 777},
			},
			2: {
				{MusicID: 2, MusicDifficulty: "append", PlayLevel: 34, TotalNoteCount: 777},
			},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	matches, err := controller.FindMusicChartsByNoteCount(NoteCountQuery{
		NoteCount:  777,
		Difficulty: "expert",
		Region:     "jp",
	})
	if err != nil {
		t.Fatalf("FindMusicChartsByNoteCount() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Music.ID != 1 || matches[0].Difficulty != "expert" {
		t.Fatalf("unexpected match: %+v", matches[0])
	}
}

func TestResolveMusicCoverUsesControllerRequestContext(t *testing.T) {
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			7: {ID: 7, Title: "Ctx Song", AssetBundleName: "ctx_song"},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	controller.SetAliasResolver(&contextAwareLookupTestAliasResolver{
		wantKey:   lookupContextKey("trace"),
		wantValue: "music-cover",
		id:        7,
	})

	ctx := context.WithValue(context.Background(), lookupContextKey("trace"), "music-cover")
	result, err := controller.WithContext(ctx).ResolveMusicCover(Query{Query: "ctx song", Region: "jp"})
	if err != nil {
		t.Fatalf("ResolveMusicCover() error = %v", err)
	}
	if result == nil || result.Music == nil || result.Music.ID != 7 {
		t.Fatalf("unexpected cover result: %+v", result)
	}
}

func TestResolveMusicCoverHidesUnreleasedMusic(t *testing.T) {
	now := time.Now().UnixMilli()
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			9: {ID: 9, Title: "Future Song", AssetBundleName: "future_song", PublishedAt: now + 60_000},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	if _, err := controller.ResolveMusicCover(Query{Query: "music9", Region: "jp"}); err == nil {
		t.Fatal("expected unreleased music query to fail")
	}
}

func TestResolveMusicCoverAndBPM(t *testing.T) {
	root := t.TempDir()
	jacketPath := filepath.Join(root, "music", "jacket", "jacket_test", "jacket_test.png")
	chartPath := filepath.Join(root, "music", "music_score", "0001_01", "expert.txt")
	if err := os.MkdirAll(filepath.Dir(jacketPath), 0o755); err != nil {
		t.Fatalf("mkdir jacket: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(chartPath), 0o755); err != nil {
		t.Fatalf("mkdir chart: %v", err)
	}
	if err := os.WriteFile(jacketPath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write jacket: %v", err)
	}
	chart := strings.Join([]string{
		"#BPM01:120",
		"#BPM02:180",
		"#00008:0100",
		"#00108:0200",
	}, "\n")
	if err := os.WriteFile(chartPath, []byte(chart), 0o644); err != nil {
		t.Fatalf("write chart: %v", err)
	}

	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_test"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "expert", PlayLevel: 27, TotalNoteCount: 777},
			},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper(root, nil), nil, nil)
	cover, err := controller.ResolveMusicCover(Query{Query: "Song A", Region: "jp"})
	if err != nil {
		t.Fatalf("ResolveMusicCover() error = %v", err)
	}
	if filepath.Clean(cover.JacketPath) != filepath.Clean(jacketPath) {
		t.Fatalf("unexpected jacket path: %q", cover.JacketPath)
	}

	bpm, err := controller.ResolveMusicBPM(Query{Query: "Song A", Region: "jp"})
	if err != nil {
		t.Fatalf("ResolveMusicBPM() error = %v", err)
	}
	if bpm.Difficulty != "expert" {
		t.Fatalf("unexpected difficulty: %q", bpm.Difficulty)
	}
	if bpm.MainBPM != 120 {
		t.Fatalf("unexpected main bpm: %v", bpm.MainBPM)
	}
	if len(bpm.Events) != 2 || bpm.Events[0].BPM != 120 || bpm.Events[1].BPM != 180 {
		t.Fatalf("unexpected events: %+v", bpm.Events)
	}
}

func TestResolveMusicBPMSupportsRegionAssetChartPath(t *testing.T) {
	root := t.TempDir()
	chartPath := filepath.Join(root, "asset", "jp-assets", "startapp", "music", "music_score", "0001_01", "expert.txt")
	if err := os.MkdirAll(filepath.Dir(chartPath), 0o755); err != nil {
		t.Fatalf("mkdir chart: %v", err)
	}
	chart := strings.Join([]string{
		"#BPM01:128",
		"#BPM02:196",
		"#00008:0100",
		"#00108:0200",
	}, "\n")
	if err := os.WriteFile(chartPath, []byte(chart), 0o644); err != nil {
		t.Fatalf("write chart: %v", err)
	}

	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_test"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "expert", PlayLevel: 27, TotalNoteCount: 777},
			},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper(root, nil), nil, nil)
	bpm, err := controller.ResolveMusicBPM(Query{Query: "Song A", Region: "jp", Difficulty: "expert"})
	if err != nil {
		t.Fatalf("ResolveMusicBPM() error = %v", err)
	}
	if bpm.Difficulty != "expert" {
		t.Fatalf("unexpected difficulty: %q", bpm.Difficulty)
	}
	if bpm.MainBPM != 128 {
		t.Fatalf("unexpected main bpm: %v", bpm.MainBPM)
	}
	if len(bpm.Events) != 2 || bpm.Events[0].BPM != 128 || bpm.Events[1].BPM != 196 {
		t.Fatalf("unexpected events: %+v", bpm.Events)
	}
}

func TestFindMusicChartsByBPM(t *testing.T) {
	root := t.TempDir()
	writeChart := func(relPath string, lines ...string) {
		path := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir chart: %v", err)
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			t.Fatalf("write chart: %v", err)
		}
	}

	writeChart(filepath.Join("music", "music_score", "0001_01", "expert.txt"),
		"#BPM01:120",
		"#BPM02:180",
		"#00008:0100",
		"#00108:0200",
	)
	writeChart(filepath.Join("music", "music_score", "0001_01", "master.txt"),
		"#BPM01:200",
		"#00008:0100",
	)
	writeChart(filepath.Join("music", "music_score", "0002_01", "append.txt"),
		"#BPM01:200",
		"#00008:0100",
	)

	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_a"},
			2: {ID: 2, Title: "Song B", AssetBundleName: "jacket_b"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "expert"},
				{MusicID: 1, MusicDifficulty: "master"},
			},
			2: {
				{MusicID: 2, MusicDifficulty: "append"},
			},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper(root, nil), nil, nil)
	matches, err := controller.FindMusicChartsByBPM(BPMQuery{Region: "jp", BPM: 200})
	if err != nil {
		t.Fatalf("FindMusicChartsByBPM() error = %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %+v", matches)
	}
	if matches[0].Music.ID != 1 || matches[0].Difficulty != "master" {
		t.Fatalf("unexpected first bpm match: %+v", matches[0])
	}
	if matches[1].Music.ID != 2 || matches[1].Difficulty != "append" {
		t.Fatalf("unexpected second bpm match: %+v", matches[1])
	}

	filtered, err := controller.FindMusicChartsByBPM(BPMQuery{Region: "jp", BPM: 200, Difficulty: "append"})
	if err != nil {
		t.Fatalf("FindMusicChartsByBPM() filtered error = %v", err)
	}
	if len(filtered) != 1 || filtered[0].Music.ID != 2 || filtered[0].Difficulty != "append" {
		t.Fatalf("unexpected filtered matches: %+v", filtered)
	}
}

func TestResolveMusicCoverUsesApprovedAlias(t *testing.T) {
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_test"},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	controller.SetAliasResolver(&lookupTestAliasResolver{
		ids: map[string]int{"blue song": 1},
	})

	cover, err := controller.ResolveMusicCover(Query{Query: "blue song", Region: "jp"})
	if err != nil {
		t.Fatalf("ResolveMusicCover() error = %v", err)
	}
	if cover.Music.ID != 1 || cover.Music.Title != "Song A" {
		t.Fatalf("unexpected music from alias: %+v", cover.Music)
	}
}

func TestResolveMusicCoverRejectsAmbiguousTitleQuery(t *testing.T) {
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_a"},
			2: {ID: 2, Title: "Song Alpha", AssetBundleName: "jacket_b"},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	_, err := controller.ResolveMusicCover(Query{Query: "Song", Region: "jp"})
	if err == nil {
		t.Fatal("expected ambiguous title query to fail")
	}
	if !strings.Contains(err.Error(), "匹配到多个歌曲") {
		t.Fatalf("expected ambiguous error, got %v", err)
	}
	if !strings.Contains(err.Error(), "music1/Song A") || !strings.Contains(err.Error(), "music2/Song Alpha") {
		t.Fatalf("expected music id hints in error, got %v", err)
	}
}

func errNotFound(kind string) error {
	return &lookupTestError{kind: kind}
}

type lookupTestError struct {
	kind string
}

func (e *lookupTestError) Error() string {
	return e.kind + " not found"
}
