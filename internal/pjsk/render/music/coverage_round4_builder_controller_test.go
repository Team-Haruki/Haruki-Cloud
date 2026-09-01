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
	"haruki-cloud/internal/testutil"
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
	{
		_, err := (*Controller)(nil).FindMusicChartsByNoteCount(NoteCountQuery{NoteCount: 1})
		testutil.RequireArgs(t, !(err == nil), "nil note-count controller error = nil")
	}

	controller := NewController(nil, nil, nil, nil, nil)
	{
		_, err := controller.FindMusicChartsByNoteCount(NoteCountQuery{})
		testutil.RequireArgs(t, !(err == nil), "invalid note count error = nil")
	}
	{

		_, err := controller.FindMusicChartsByNoteCount(NoteCountQuery{NoteCount: 10, Region: "jp"})
		testutil.RequireArgs(t, !(err == nil), "missing note-count source error = nil")
	}

	base := newRound4SearchSource()
	source := &round4NoteFinderSource{round4SearchSource: base, err: errors.New("finder failed")}
	controller = NewController(source, nil, nil, nil, nil)
	{
		_, err := controller.FindMusicChartsByNoteCount(NoteCountQuery{NoteCount: 10, Region: "jp"})
		testutil.Require(t, errors.Is(err, source.err), "finder error = %v", err)
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
	{
		testutil.Require(t, !(err != nil), "finder matches = %#v, %v", matches, err)
		testutil.Require(t, !(len(matches) != 1), "finder matches = %#v, %v", matches, err)
		testutil.Require(t, !(matches[0].Music.ID != 1), "finder matches = %#v, %v", matches, err)
		testutil.Require(t, !(matches[0].Difficulty != "expert"), "finder matches = %#v, %v", matches, err)
	}
	{

		_, err := controller.FindMusicChartsByNoteCount(NoteCountQuery{NoteCount: 10, Region: "jp", Difficulty: "easy"})
		{
			testutil.Require(t, !(err == nil), "filtered no-match error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "easy"), "filtered no-match error = %v", err)
		}
	}

	source.items = nil
	{
		_, err := controller.FindMusicChartsByNoteCount(NoteCountQuery{NoteCount: 10, Region: "jp"})
		{
			testutil.Require(t, !(err == nil), "unfiltered no-match error = %v", err)
			testutil.Require(t, !(strings.Contains(err.Error(), "easy")), "unfiltered no-match error = %v", err)
		}
	}

}

func TestBuilderRequestValidationAndMixedItems(t *testing.T) {
	source := newRound4SearchSource()
	builder := NewBuilder(source, nil, assets.NewAssetHelper("", nil))
	{
		_, err := builder.BuildMusicDetailRequest(nil, renderregion.Unknown)
		testutil.RequireArgs(t, !(err == nil), "nil music detail error = nil")
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
	{
		testutil.Require(t, !(err != nil), "detail event/limited payload = %#v, %v", detail, err)
		testutil.Require(t, !(detail.EventID == nil), "detail event/limited payload = %#v, %v", detail, err)
		testutil.Require(t, !(*detail.EventID != 9), "detail event/limited payload = %#v, %v", detail, err)
		testutil.Require(t, !(detail.EventBannerPath == nil), "detail event/limited payload = %#v, %v", detail, err)
		testutil.Require(t, !(len(detail.LimitedTimes) != 1), "detail event/limited payload = %#v, %v", detail, err)
	}
	{

		_, err := builder.BuildMusicBriefListRequest(nil, "master", renderregion.JP)
		testutil.RequireArgs(t, !(err == nil), "empty brief IDs error = nil")
	}
	{

		_, err := builder.BuildMusicBriefListRequest([]int{99}, "master", renderregion.JP)
		testutil.RequireArgs(t, !(err == nil), "missing brief music error = nil")
	}
	{

		_, err := builder.BuildMusicBriefListRequest([]int{2}, "append", renderregion.JP)
		testutil.RequireArgs(t, !(err == nil), "brief music without level error = nil")
	}

	brief, err := builder.BuildMusicBriefListRequest([]int{99, 1}, "expert", renderregion.Unknown)
	{
		testutil.Require(t, !(err != nil), "brief request = %#v, %v", brief, err)
		testutil.Require(t, !(len(brief.MusicList) != 1), "brief request = %#v, %v", brief, err)
		testutil.Require(t, !(brief.MusicList[0].ID != 1), "brief request = %#v, %v", brief, err)
	}
	{

		_, err := builder.BuildMusicBriefListRequestFromItems(nil, renderregion.JP)
		testutil.RequireArgs(t, !(err == nil), "empty brief items error = nil")
	}
	{

		_, err := builder.BuildMusicBriefListRequestFromItems([]BriefListItemQuery{{MusicID: 0}, {MusicID: 99}}, renderregion.JP)
		testutil.RequireArgs(t, !(err == nil), "invalid brief items error = nil")
	}

	items, err := builder.BuildMusicBriefListRequestFromItems([]BriefListItemQuery{
		{MusicID: 0},
		{MusicID: 99},
		{MusicID: 1},
		{MusicID: 1, Difficulty: "expert"},
		{MusicID: 1, Difficulty: "master"},
		{MusicID: 2, Difficulty: "append"},
	}, renderregion.Unknown)
	{
		testutil.Require(t, !(err != nil), "mixed brief items = %#v, %v", items, err)
		testutil.Require(t, !(len(items.MusicList) != 3), "mixed brief items = %#v, %v", items, err)
		testutil.Require(t, !(items.RequiredDifficulty != ""), "mixed brief items = %#v, %v", items, err)
	}

	same, err := builder.BuildMusicBriefListRequestFromItems([]BriefListItemQuery{{MusicID: 1, Difficulty: "expert"}}, renderregion.JP)
	{
		testutil.Require(t, !(err != nil), "same-difficulty brief items = %#v, %v", same, err)
		testutil.Require(t, !(same.RequiredDifficulty != "expert"), "same-difficulty brief items = %#v, %v", same, err)
		testutil.Require(t, !(same.RequiredDifficulties != "expert"), "same-difficulty brief items = %#v, %v", same, err)
	}
	{

		_, err := builder.BuildMusicChartRequest(ChartQuery{}, nil, renderregion.JP)
		testutil.RequireArgs(t, !(err == nil), "nil chart music error = nil")
	}
	{

		_, err := builder.BuildMusicChartRequest(ChartQuery{Difficulty: "append"}, source.musics[1], renderregion.JP)
		testutil.RequireArgs(t, !(err == nil), "missing chart difficulty error = nil")
	}
	testutil.RequireArgs(t, !(builder.BuildChartArtist(nil) != ""), "nil chart artist is non-empty")

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
		{
			got := builder.BuildChartArtist(tt.music)
			testutil.Check(t, !(got != tt.want), "BuildChartArtist(%+v) = %q, want %q", tt.music, got, tt.want)
		}

	}
}

func TestMusicControllerContextAndResolutionEdges(t *testing.T) {
	var nilController *Controller
	{
		testutil.RequireArgs(t, !(nilController.WithContext(context.Background()) != nil), "nil controller clone returned non-nil")
		testutil.RequireArgs(t, !(nilController.WithSnapshot(nil) != nil), "nil controller clone returned non-nil")
	}

	nilController.SetCustomMusicScoreClient(nil)
	nilController.SetAliasResolver(nil)
	{
		got := nilController.resolveRegion("cn")
		testutil.Require(t, !(got != renderregion.CN), "nil controller region = %s", got)
	}

	source := newRound4SearchSource()
	contextSource := &round4ContextSource{round4SearchSource: source}
	controller := NewController(contextSource, nil, nil, nil, nil)
	controller.SetCustomMusicScoreClient(sekaiapi.NewSekaiAPIClient(nil))
	ctx := context.WithValue(context.Background(), musicControllerContextKey{}, "music-context")
	clone := controller.WithContext(ctx)
	{
		testutil.Require(t, !(clone == controller), "context clone = %#v", clone)
		testutil.Require(t, !(clone.requestCtx != ctx), "context clone = %#v", clone)
	}

	clonedSource, ok := clone.sources.SourceForRegion(renderregion.JP)
	{
		testutil.Require(t, ok, "contextual source = %#v, ok=%v", clonedSource, ok)
		testutil.Require(t, !(clonedSource.(*round4ContextSource).ctx != ctx), "contextual source = %#v, ok=%v", clonedSource, ok)
	}
	{

		_, ok := clone.customScores.(*sekaiapi.HarukiSekaiAPIClient)
		testutil.Require(t, ok, "contextual custom score client = %T", clone.customScores)
	}

	stub := &musicSnapshotStub{}
	{
		got := controller.WithSnapshot(stub)
		{
			testutil.Require(t, !(got == controller), "snapshot clone = %#v", got)
			testutil.Require(t, !(got.snapshot != stub), "snapshot clone = %#v", got)
		}
	}

	wantErr := errors.New("lookup failed")
	source.getByID = func(int) (*masterdata.Music, error) { return nil, wantErr }
	{
		_, err := controller.resolveMusicTitleQuery(contextSource, "music1", false)
		testutil.Require(t, errors.Is(err, wantErr), "explicit title source error = %v", err)
	}

	plainAliasError := errors.New("alias unavailable")
	controller.SetAliasResolver(&lookupTestAliasResolver{err: plainAliasError})
	{
		_, err := controller.resolveMusicTitleQuery(contextSource, "alias", false)
		testutil.Require(t, errors.Is(err, plainAliasError), "alias error = %v", err)
	}

	controller.SetAliasResolver(&lookupTestAliasResolver{ids: map[string]int{"alias": 2}})
	{
		_, err := controller.resolveMusicTitleQuery(contextSource, "alias", false)
		testutil.Require(t, errors.Is(err, wantErr), "alias get error = %v", err)
	}
	{

		_, _, err := controller.resolveMusicListKeywordFilter(nil, "music1", false)
		testutil.RequireArgs(t, !(err == nil), "nil explicit keyword source error = nil")
	}
	{

		_, _, err := controller.resolveMusicListKeywordFilter(contextSource, "music1", false)
		testutil.Require(t, errors.Is(err, wantErr), "explicit keyword source error = %v", err)
	}

	future := &masterdata.Music{ID: 1, PublishedAt: time.Now().Add(time.Hour).UnixMilli()}
	source.getByID = func(int) (*masterdata.Music, error) { return future, nil }
	{
		_, _, err := controller.resolveMusicListKeywordFilter(contextSource, "music1", false)
		testutil.RequireArgs(t, !(err == nil), "unreleased explicit keyword error = nil")
	}

	controller.SetAliasResolver(&lookupTestAliasResolver{err: plainAliasError})
	{
		_, _, err := controller.resolveMusicListKeywordFilter(contextSource, "alias", false)
		testutil.Require(t, errors.Is(err, plainAliasError), "keyword alias error = %v", err)
	}

	controller.SetAliasResolver(&lookupTestAliasResolver{ids: map[string]int{"alias": 2}})
	source.getByID = func(int) (*masterdata.Music, error) { return nil, wantErr }
	{
		_, _, err := controller.resolveMusicListKeywordFilter(contextSource, "alias", false)
		testutil.Require(t, errors.Is(err, wantErr), "keyword alias get error = %v", err)
	}
	{
		testutil.RequireArgs(t, !(controller.fallbackSource(renderregion.JP) != nil), "fallback source resolution mismatch")
		testutil.RequireArgs(t, !(controller.fallbackSource(renderregion.CN) == nil), "fallback source resolution mismatch")
	}
	testutil.RequireArgs(t, !(NewController(nil, nil, nil, nil, nil).fallbackSource(renderregion.CN) != nil), "missing JP fallback returned a source")
	{

		_, _, _, err := NewController(nil, nil, nil, nil, nil).resolveBuilder("cn")
		testutil.RequireArgs(t, !(err == nil), "missing regional builder error = nil")
	}

}

func TestMusicControllerHelperResidualBranches(t *testing.T) {
	controller := NewController(newRound4SearchSource(), nil, nil, nil, nil)
	testutil.RequireArgs(t, !(controller.currentSnapshot() != nil), "missing current snapshot is non-nil")

	invalidSnapshot := &rendersnapshot.Service{}
	controller = controller.WithSnapshot(invalidSnapshot)
	testutil.RequireArgs(t, !(controller.currentSnapshot() != nil), "invalid current snapshot is non-nil")

	override := &drawing.DetailedProfileCardRequest{ID: "override"}
	{
		got := controller.resolveMusicListProfile(override, renderregion.JP)
		{
			testutil.Require(t, !(got == override), "profile override = %#v", got)
			testutil.Require(t, !(got.ID != "override"), "profile override = %#v", got)
		}
	}
	{

		got := controller.resolveMusicListProfile(nil, renderregion.JP)
		testutil.Require(t, !(got != nil), "missing list profile = %#v", got)
	}

	card := convertDetailedProfileToCard(drawing.DetailedProfileCardRequest{ID: "x"})
	{
		testutil.Require(t, !(card.Profile == nil), "default converted profile = %#v", card)
		testutil.Require(t, !(card.DataSources[0].Source == nil), "default converted profile = %#v", card)
		testutil.Require(t, !(*card.DataSources[0].Source != "lunabot-service"), "default converted profile = %#v", card)
		testutil.Require(t, !(card.DataSources[0].UpdateTime == nil), "default converted profile = %#v", card)
		testutil.Require(t, !(*card.DataSources[0].UpdateTime == 0), "default converted profile = %#v", card)
	}
	{

		got := controller.buildUserResults("master")
		testutil.Require(t, !(got != nil), "invalid snapshot user results = %#v", got)
	}
	{

		got := controller.resolveStaticIcon(nil, "missing.png")
		{
			testutil.Require(t, !(got == nil), "fallback static icon = %#v", got)
			testutil.Require(t, !(*got == ""), "fallback static icon = %#v", got)
		}
	}

}

var _ rendersnapshot.Snapshot = (*musicSnapshotStub)(nil)
