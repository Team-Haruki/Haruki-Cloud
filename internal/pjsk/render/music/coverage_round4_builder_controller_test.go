package music

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

type musicControllerContextKey struct{}

type round4NoteFinderSource struct {
	*round4SearchSource
	items []*masterdata.MusicDifficulty
	err   error
}

func (s *round4NoteFinderSource) FindMusicDifficultiesByNoteCount(int) ([]*masterdata.MusicDifficulty, error) {
	return append([]*masterdata.MusicDifficulty(nil), s.items...), s.err
}

type round4ContextSource struct {
	*round4SearchSource
	ctx context.Context
}

func (s *round4ContextSource) WithContext(ctx context.Context) DataSource {
	clone := *s
	clone.ctx = ctx
	return &clone
}

func TestFindMusicChartsByNoteCountFinderBranches(t *testing.T) {
	if _, err := (*Controller)(nil).FindMusicChartsByNoteCount(NoteCountQuery{NoteCount: 1}); err == nil {
		t.Fatal("nil note-count controller error = nil")
	}
	controller := NewController(nil, nil, nil, nil, nil)
	if _, err := controller.FindMusicChartsByNoteCount(NoteCountQuery{}); err == nil {
		t.Fatal("invalid note count error = nil")
	}
	if _, err := controller.FindMusicChartsByNoteCount(NoteCountQuery{NoteCount: 10, Region: "jp"}); err == nil {
		t.Fatal("missing note-count source error = nil")
	}

	base := newRound4SearchSource()
	source := &round4NoteFinderSource{round4SearchSource: base, err: errors.New("finder failed")}
	controller = NewController(source, nil, nil, nil, nil)
	if _, err := controller.FindMusicChartsByNoteCount(NoteCountQuery{NoteCount: 10, Region: "jp"}); !errors.Is(err, source.err) {
		t.Fatalf("finder error = %v", err)
	}

	now := time.Now().UnixMilli()
	base.musics = map[int]*masterdata.Music{
		1:   {ID: 1, Title: "Visible", PublishedAt: now - 1},
		2:   {ID: 2, Title: "Future", PublishedAt: now + 100_000},
		241: {ID: 241, Title: "Hidden", PublishedAt: now - 1},
	}
	source.err = nil
	source.items = []*masterdata.MusicDifficulty{
		nil,
		{MusicID: 99, MusicDifficulty: "expert", TotalNoteCount: 10},
		{MusicID: 2, MusicDifficulty: "expert", TotalNoteCount: 10},
		{MusicID: 241, MusicDifficulty: "expert", TotalNoteCount: 10},
		{MusicID: 1, MusicDifficulty: "master", PlayLevel: 30, TotalNoteCount: 10},
		{MusicID: 1, MusicDifficulty: "expert", PlayLevel: 27, TotalNoteCount: 10},
	}
	matches, err := controller.FindMusicChartsByNoteCount(NoteCountQuery{NoteCount: 10, Region: "jp", Difficulty: "expert"})
	if err != nil || len(matches) != 1 || matches[0].Music.ID != 1 || matches[0].Difficulty != "expert" {
		t.Fatalf("finder matches = %#v, %v", matches, err)
	}
	if _, err := controller.FindMusicChartsByNoteCount(NoteCountQuery{NoteCount: 10, Region: "jp", Difficulty: "easy"}); err == nil || !strings.Contains(err.Error(), "easy") {
		t.Fatalf("filtered no-match error = %v", err)
	}
	source.items = nil
	if _, err := controller.FindMusicChartsByNoteCount(NoteCountQuery{NoteCount: 10, Region: "jp"}); err == nil || strings.Contains(err.Error(), "easy") {
		t.Fatalf("unfiltered no-match error = %v", err)
	}
}

func TestBuilderRequestValidationAndMixedItems(t *testing.T) {
	source := newRound4SearchSource()
	builder := NewBuilder(source, nil, assets.NewAssetHelper("", nil))
	if _, err := builder.BuildMusicDetailRequest(nil, renderregion.Unknown); err == nil {
		t.Fatal("nil music detail error = nil")
	}

	now := time.Now().UnixMilli()
	source.musics = map[int]*masterdata.Music{
		1: {ID: 1, Title: "One", Composer: "Same", Arranger: "Same", AssetBundleName: "one", PublishedAt: now - 1},
		2: {ID: 2, Title: "Two", Composer: "Comp", Arranger: "Arr", AssetBundleName: "two", PublishedAt: now - 1},
	}
	source.difficulties = []*masterdata.MusicDifficulty{
		{MusicID: 1, MusicDifficulty: "expert", PlayLevel: 27},
		{MusicID: 1, MusicDifficulty: "master", PlayLevel: 30},
	}
	source.primaryEvent = &masterdata.Event{ID: 9, AssetBundleName: "event_9"}
	source.limited = []*masterdata.LimitedTimeMusic{{StartAt: 10, EndAt: 20}}
	detail, err := builder.BuildMusicDetailRequest(source.musics[1], renderregion.Unknown)
	if err != nil || detail.EventID == nil || *detail.EventID != 9 || detail.EventBannerPath == nil || len(detail.LimitedTimes) != 1 {
		t.Fatalf("detail event/limited payload = %#v, %v", detail, err)
	}

	if _, err := builder.BuildMusicBriefListRequest(nil, "master", renderregion.JP); err == nil {
		t.Fatal("empty brief IDs error = nil")
	}
	if _, err := builder.BuildMusicBriefListRequest([]int{99}, "master", renderregion.JP); err == nil {
		t.Fatal("missing brief music error = nil")
	}
	if _, err := builder.BuildMusicBriefListRequest([]int{2}, "append", renderregion.JP); err == nil {
		t.Fatal("brief music without level error = nil")
	}
	brief, err := builder.BuildMusicBriefListRequest([]int{99, 1}, "expert", renderregion.Unknown)
	if err != nil || len(brief.MusicList) != 1 || brief.MusicList[0].ID != 1 {
		t.Fatalf("brief request = %#v, %v", brief, err)
	}

	if _, err := builder.BuildMusicBriefListRequestFromItems(nil, renderregion.JP); err == nil {
		t.Fatal("empty brief items error = nil")
	}
	if _, err := builder.BuildMusicBriefListRequestFromItems([]BriefListItemQuery{{MusicID: 0}, {MusicID: 99}}, renderregion.JP); err == nil {
		t.Fatal("invalid brief items error = nil")
	}
	items, err := builder.BuildMusicBriefListRequestFromItems([]BriefListItemQuery{
		{MusicID: 0},
		{MusicID: 99},
		{MusicID: 1},
		{MusicID: 1, Difficulty: "expert"},
		{MusicID: 1, Difficulty: "master"},
		{MusicID: 2, Difficulty: "append"},
	}, renderregion.Unknown)
	if err != nil || len(items.MusicList) != 3 || items.RequiredDifficulty != "" {
		t.Fatalf("mixed brief items = %#v, %v", items, err)
	}
	same, err := builder.BuildMusicBriefListRequestFromItems([]BriefListItemQuery{{MusicID: 1, Difficulty: "expert"}}, renderregion.JP)
	if err != nil || same.RequiredDifficulty != "expert" || same.RequiredDifficulties != "expert" {
		t.Fatalf("same-difficulty brief items = %#v, %v", same, err)
	}

	if _, err := builder.BuildMusicChartRequest(ChartQuery{}, nil, renderregion.JP); err == nil {
		t.Fatal("nil chart music error = nil")
	}
	if _, err := builder.BuildMusicChartRequest(ChartQuery{Difficulty: "append"}, source.musics[1], renderregion.JP); err == nil {
		t.Fatal("missing chart difficulty error = nil")
	}
	if builder.BuildChartArtist(nil) != "" {
		t.Fatal("nil chart artist is non-empty")
	}
	artistCases := []struct {
		music *masterdata.Music
		want  string
	}{
		{music: &masterdata.Music{Composer: "Same", Arranger: "Same"}, want: "Same"},
		{music: &masterdata.Music{Composer: "-", Arranger: "Arr"}, want: "Arr"},
		{music: &masterdata.Music{Composer: "Comp", Arranger: "Comp feat."}, want: "Comp feat."},
		{music: &masterdata.Music{Composer: "Comp", Arranger: "-"}, want: "Comp"},
		{music: &masterdata.Music{Composer: "Comp feat.", Arranger: "Comp"}, want: "Comp feat."},
		{music: &masterdata.Music{}, want: ""},
		{music: &masterdata.Music{Composer: "Comp", Arranger: "Arr"}, want: "Comp / Arr"},
	}
	for _, tt := range artistCases {
		if got := builder.BuildChartArtist(tt.music); got != tt.want {
			t.Errorf("BuildChartArtist(%+v) = %q, want %q", tt.music, got, tt.want)
		}
	}
}

func TestMusicControllerContextAndResolutionEdges(t *testing.T) {
	var nilController *Controller
	if nilController.WithContext(context.Background()) != nil || nilController.WithSnapshot(nil) != nil {
		t.Fatal("nil controller clone returned non-nil")
	}
	nilController.SetCustomMusicScoreClient(nil)
	nilController.SetAliasResolver(nil)
	if got := nilController.resolveRegion("cn"); got != renderregion.CN {
		t.Fatalf("nil controller region = %s", got)
	}

	source := newRound4SearchSource()
	contextSource := &round4ContextSource{round4SearchSource: source}
	controller := NewController(contextSource, nil, nil, nil, nil)
	controller.SetCustomMusicScoreClient(sekaiapi.NewSekaiAPIClient(nil))
	ctx := context.WithValue(context.Background(), musicControllerContextKey{}, "music-context")
	clone := controller.WithContext(ctx)
	if clone == controller || clone.requestCtx != ctx {
		t.Fatalf("context clone = %#v", clone)
	}
	clonedSource, ok := clone.sources.SourceForRegion(renderregion.JP)
	if !ok || clonedSource.(*round4ContextSource).ctx != ctx {
		t.Fatalf("contextual source = %#v, ok=%v", clonedSource, ok)
	}
	if _, ok := clone.customScores.(*sekaiapi.HarukiSekaiAPIClient); !ok {
		t.Fatalf("contextual custom score client = %T", clone.customScores)
	}
	stub := &musicSnapshotStub{}
	if got := controller.WithSnapshot(stub); got == controller || got.snapshot != stub {
		t.Fatalf("snapshot clone = %#v", got)
	}

	wantErr := errors.New("lookup failed")
	source.getByID = func(int) (*masterdata.Music, error) { return nil, wantErr }
	if _, err := controller.resolveMusicTitleQuery(contextSource, "music1", false); !errors.Is(err, wantErr) {
		t.Fatalf("explicit title source error = %v", err)
	}
	plainAliasError := errors.New("alias unavailable")
	controller.SetAliasResolver(&lookupTestAliasResolver{err: plainAliasError})
	if _, err := controller.resolveMusicTitleQuery(contextSource, "alias", false); !errors.Is(err, plainAliasError) {
		t.Fatalf("alias error = %v", err)
	}
	controller.SetAliasResolver(&lookupTestAliasResolver{ids: map[string]int{"alias": 2}})
	if _, err := controller.resolveMusicTitleQuery(contextSource, "alias", false); !errors.Is(err, wantErr) {
		t.Fatalf("alias get error = %v", err)
	}

	if _, _, err := controller.resolveMusicListKeywordFilter(nil, "music1", false); err == nil {
		t.Fatal("nil explicit keyword source error = nil")
	}
	if _, _, err := controller.resolveMusicListKeywordFilter(contextSource, "music1", false); !errors.Is(err, wantErr) {
		t.Fatalf("explicit keyword source error = %v", err)
	}
	future := &masterdata.Music{ID: 1, PublishedAt: time.Now().Add(time.Hour).UnixMilli()}
	source.getByID = func(int) (*masterdata.Music, error) { return future, nil }
	if _, _, err := controller.resolveMusicListKeywordFilter(contextSource, "music1", false); err == nil {
		t.Fatal("unreleased explicit keyword error = nil")
	}
	controller.SetAliasResolver(&lookupTestAliasResolver{err: plainAliasError})
	if _, _, err := controller.resolveMusicListKeywordFilter(contextSource, "alias", false); !errors.Is(err, plainAliasError) {
		t.Fatalf("keyword alias error = %v", err)
	}
	controller.SetAliasResolver(&lookupTestAliasResolver{ids: map[string]int{"alias": 2}})
	source.getByID = func(int) (*masterdata.Music, error) { return nil, wantErr }
	if _, _, err := controller.resolveMusicListKeywordFilter(contextSource, "alias", false); !errors.Is(err, wantErr) {
		t.Fatalf("keyword alias get error = %v", err)
	}

	if controller.fallbackSource(renderregion.JP) != nil || controller.fallbackSource(renderregion.CN) == nil {
		t.Fatal("fallback source resolution mismatch")
	}
	if NewController(nil, nil, nil, nil, nil).fallbackSource(renderregion.CN) != nil {
		t.Fatal("missing JP fallback returned a source")
	}
	if _, _, _, err := NewController(nil, nil, nil, nil, nil).resolveBuilder("cn"); err == nil {
		t.Fatal("missing regional builder error = nil")
	}
}

func TestMusicControllerHelperResidualBranches(t *testing.T) {
	controller := NewController(newRound4SearchSource(), nil, nil, nil, nil)
	if controller.currentSnapshot() != nil {
		t.Fatal("missing current snapshot is non-nil")
	}
	invalidSnapshot := &rendersnapshot.Service{}
	controller = controller.WithSnapshot(invalidSnapshot)
	if controller.currentSnapshot() != nil {
		t.Fatal("invalid current snapshot is non-nil")
	}
	override := &drawing.DetailedProfileCardRequest{ID: "override"}
	if got := controller.resolveMusicListProfile(override, renderregion.JP); got == override || got.ID != "override" {
		t.Fatalf("profile override = %#v", got)
	}
	if got := controller.resolveMusicListProfile(nil, renderregion.JP); got != nil {
		t.Fatalf("missing list profile = %#v", got)
	}
	card := convertDetailedProfileToCard(drawing.DetailedProfileCardRequest{ID: "x"})
	if card.Profile == nil || card.DataSources[0].Source == nil || *card.DataSources[0].Source != "lunabot-service" || card.DataSources[0].UpdateTime == nil || *card.DataSources[0].UpdateTime == 0 {
		t.Fatalf("default converted profile = %#v", card)
	}
	if got := controller.buildUserResults("master"); got != nil {
		t.Fatalf("invalid snapshot user results = %#v", got)
	}
	if got := controller.resolveStaticIcon(nil, "missing.png"); got == nil || *got == "" {
		t.Fatalf("fallback static icon = %#v", got)
	}
}

var _ rendersnapshot.Snapshot = (*musicSnapshotStub)(nil)
