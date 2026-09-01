package profile

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/internal/testutil"

	_ "github.com/mattn/go-sqlite3"
)

func minimalProfileController(drawingClient *drawing.HarukiDrawingClient) *Controller {
	return NewController(&testProfileSource{
		region:      renderregion.JP,
		cards:       map[int]*masterdata.Card{},
		honors:      map[int]*masterdata.Honor{},
		honorGroups: map[int]*masterdata.HonorGroup{},
		frames:      map[int]*masterdata.PlayerFrame{},
		frameGroups: map[int]*masterdata.PlayerFrameGroup{},
	}, drawingClient, assets.NewAssetHelper("", nil), nil)
}

func minimalProfileResponse() *sekai.GetAnotherProfileResponse {
	return &sekai.GetAnotherProfileResponse{
		User:        sekai.AnotherUser{UserID: 42, Name: "Coverage User", Rank: 88},
		UserProfile: sekai.UserProfile{Word: "hello"},
	}
}

func TestModularProfileBuildAndPureHelpers(t *testing.T) {
	controller := minimalProfileController(nil)
	img := "background.png"
	payload, err := controller.BuildModularProfileRequestFromAPIWithSnapshot(Query{
		Region:     "jp",
		Visible:    true,
		BgSettings: &drawing.ProfileBgSettings{ImgPath: &img},
	}, minimalProfileResponse(), nil)
	testutil.Require(t, !(err != nil), "BuildModularProfileRequestFromAPIWithSnapshot() error = %v", err)
	{
		testutil.Require(t, !(payload.Profile.ID != "42"), "unexpected modular payload: %+v", payload)
		testutil.Require(t, !(payload.Profile.IsHideUID), "unexpected modular payload: %+v", payload)
		testutil.Require(t, !(payload.Preset.ID == ""), "unexpected modular payload: %+v", payload)
		testutil.Require(t, !(len(payload.Preset.Widgets) != 7), "unexpected modular payload: %+v", payload)
	}
	{
		testutil.Require(t, !(payload.BgSettings == nil), "background settings = %+v", payload.BgSettings)
		testutil.Require(t, !(payload.BgSettings.ImgPath == nil), "background settings = %+v", payload.BgSettings)
		testutil.Require(t, !(*payload.BgSettings.ImgPath != "asset/background.png"), "background settings = %+v", payload.BgSettings)
	}

	widget := modularWidget("id", "type", "family", "Title", 1, 2, 3, 4, map[string]any{"a": 1}, map[string]any{"b": 2})
	{
		testutil.Require(t, !(widget.Title == nil), "widget = %+v", widget)
		testutil.Require(t, !(*widget.Title != "Title"), "widget = %+v", widget)
		testutil.Require(t, !(widget.Frame.W != 3), "widget = %+v", widget)
	}
	{
		testutil.RequireArgs(t, !(firstCharacterRank(nil) != nil), "empty focus helpers returned an item")
		testutil.RequireArgs(t, !(firstDeckCard(nil) != nil), "empty focus helpers returned an item")
	}

	ranks := []drawing.CharacterRank{{}, {CharacterID: 3}}
	cards := []drawing.CardFullThumbnailRequest{{}, {CardID: 4}}
	{
		testutil.RequireArgs(t, !(firstCharacterRank(ranks).CharacterID != 3), "focus helpers did not skip empty entries")
		testutil.RequireArgs(t, !(firstDeckCard(cards).CardID != 4), "focus helpers did not skip empty entries")
	}

}

func TestProfilePublicBuildEntrypointsAndValidation(t *testing.T) {
	controller := minimalProfileController(nil)
	resp := minimalProfileResponse()
	{
		detail, err := controller.BuildDetailedProfileCardFromAPI(Query{Region: "jp"}, resp, []byte(`[]`))
		{
			testutil.Require(t, !(err != nil), "detailed profile = %+v, %v", detail, err)
			testutil.Require(t, !(detail.ID != "42"), "detailed profile = %+v, %v", detail, err)
		}
	}
	{

		card, err := controller.BuildProfileCardFromAPI(Query{Region: "jp"}, resp, []byte(`[]`))
		{
			testutil.Require(t, !(err != nil), "profile card = %+v, %v", card, err)
			testutil.Require(t, !(card.Profile == nil), "profile card = %+v, %v", card, err)
			testutil.Require(t, !(card.Profile.ID != "42"), "profile card = %+v, %v", card, err)
		}
	}
	{

		_, err := (*Controller)(nil).BuildModularProfileRequestFromAPIWithSnapshot(Query{}, resp, nil)
		testutil.RequireArgs(t, !(err == nil), "nil modular controller unexpectedly succeeded")
	}
	{

		_, err := controller.BuildModularProfileRequestFromAPIWithSnapshot(Query{}, nil, nil)
		testutil.RequireArgs(t, !(err == nil), "nil modular response unexpectedly succeeded")
	}
	{

		_, err := NewController(nil, nil, nil, nil).BuildModularProfileRequestFromAPIWithSnapshot(Query{}, resp, nil)
		testutil.RequireArgs(t, !(err == nil), "missing modular source unexpectedly succeeded")
	}
	{

		_, err := controller.BuildDetailedProfileCardFromAPI(Query{}, nil, nil)
		testutil.RequireArgs(t, !(err == nil), "nil detailed response unexpectedly succeeded")
	}
	{

		_, err := controller.BuildProfileCardFromAPI(Query{}, nil, nil)
		testutil.RequireArgs(t, !(err == nil), "nil card response unexpectedly succeeded")
	}
	{
		testutil.RequireArgs(t, !(controller.SnapshotDetailedProfile(renderregion.JP) != nil), "missing snapshot returned profile data")
		testutil.RequireArgs(t, !((*Controller)(nil).SnapshotDetailedProfile(renderregion.JP) != nil), "missing snapshot returned profile data")
	}

	controller.SetCensor(nil)
	(*Controller)(nil).SetCensor(nil)
	controller.RegisterSource(nil)
	(*Controller)(nil).RegisterSource(nil)
	testutil.RequireArgs(t, !((*Controller)(nil).WithContext(context.Background()) != nil), "nil controller WithContext returned a controller")

}

func TestProfileRenderEntrypoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("profile-image"))
	}))
	defer server.Close()
	controller := minimalProfileController(drawing.NewHarukiDrawingClient(server.URL))
	resp := minimalProfileResponse()
	for name, render := range map[string]func() ([]byte, error){
		"profile": func() ([]byte, error) { return controller.RenderProfileFromAPI(Query{Region: "jp"}, resp, nil) },
		"profile snapshot": func() ([]byte, error) {
			return controller.RenderProfileFromAPIWithSnapshot(Query{Region: "jp"}, resp, nil)
		},
		"modular": func() ([]byte, error) {
			return controller.RenderModularProfileFromAPIWithSnapshot(Query{Region: "jp"}, resp, nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := render()
			{
				testutil.Require(t, !(err != nil), "render = %q, %v", got, err)
				testutil.Require(t, bytes.Equal(got, []byte("profile-image")), "render = %q, %v", got, err)
			}

		})
	}

	withoutDrawing := minimalProfileController(nil)
	for name, render := range map[string]func() ([]byte, error){
		"profile":          func() ([]byte, error) { return withoutDrawing.RenderProfileFromAPI(Query{}, resp, nil) },
		"profile snapshot": func() ([]byte, error) { return withoutDrawing.RenderProfileFromAPIWithSnapshot(Query{}, resp, nil) },
		"modular": func() ([]byte, error) {
			return withoutDrawing.RenderModularProfileFromAPIWithSnapshot(Query{}, resp, nil)
		},
		"local": func() ([]byte, error) { return withoutDrawing.RenderProfile(Query{}) },
	} {
		t.Run("missing drawing "+name, func(t *testing.T) {
			{
				_, err := render()
				testutil.RequireArgs(t, !(err == nil), "render unexpectedly succeeded")
			}

		})
	}
	{
		_, err := controller.RenderProfileFromAPI(Query{}, nil, nil)
		testutil.RequireArgs(t, !(err == nil), "render with invalid payload unexpectedly succeeded")
	}
	{

		_, err := controller.RenderModularProfileFromAPIWithSnapshot(Query{}, nil, nil)
		testutil.RequireArgs(t, !(err == nil), "modular render with invalid payload unexpectedly succeeded")
	}

}

func TestProfileProviderAdapterEmptyDatabase(t *testing.T) {
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:profile_adapter_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = client.Close() })
	adapter := NewProviderAdapter(provider.NewDatabaseProvider(client, renderregion.JP))
	ctx := context.WithValue(context.Background(), profileContextKey("adapter"), "request")
	withContext := adapter.WithContext(ctx)
	{
		testutil.RequireArgs(t, !(withContext == nil), "adapter did not retain its request context")
		testutil.RequireArgs(t, !(withContext.(*ProviderAdapter).Context() != ctx), "adapter did not retain its request context")
	}
	testutil.RequireArgs(t, !((*ProviderAdapter)(nil).WithContext(ctx) != nil), "nil adapter returned a data source")
	{

		_, err := adapter.GetHonorByID(1)
		testutil.RequireArgs(t, !(err == nil), "missing honor returned no error")
	}
	{

		_, err := adapter.GetHonorGroupByID(1)
		testutil.RequireArgs(t, !(err == nil), "missing honor group returned no error")
	}
	{

		_, err := adapter.GetBondsHonorByID(1)
		testutil.RequireArgs(t, !(err == nil), "missing bonds honor returned no error")
	}
	{

		_, err := adapter.GetBondsHonorWordByID(1)
		testutil.RequireArgs(t, !(err == nil), "missing bonds honor word returned no error")
	}
	{

		_, ok := adapter.GetGameCharacterUnitByID(1)
		testutil.RequireArgs(t, !(ok), "missing game-character unit was found")
	}
	{

		_, err := adapter.GetPlayerFrameByID(1)
		testutil.RequireArgs(t, !(err == nil), "missing frame returned no error")
	}
	{

		_, err := adapter.GetPlayerFrameGroupByID(1)
		testutil.RequireArgs(t, !(err == nil), "missing frame group returned no error")
	}
	{

		_, err := adapter.GetCardByID(1)
		testutil.RequireArgs(t, !(err == nil), "missing card returned no error")
	}
	{

		got := adapter.GetEventIDByHonorID(1)
		testutil.Require(t, !(got != 0), "missing honor event ID = %d", got)
	}

}

func TestProfileAggregationHelperBranches(t *testing.T) {
	counts := buildMusicCounts(nil, []snapshot.RawMusicResult{
		{MusicID: 1, MusicDifficultyType: "master", FullComboFlg: true, FullPerfectFlg: true},
		{MusicID: 1, MusicDifficultyType: "MASTER", FullComboFlg: true},
		{MusicID: 2, MusicDifficultyType: "master"},
		{MusicID: 3, MusicDifficultyType: "expert", FullComboFlg: true},
	})
	{
		testutil.Require(t, !(len(counts) != 6), "music counts = %#v", counts)
		testutil.Require(t, !(counts[4].Clear != 2), "music counts = %#v", counts)
		testutil.Require(t, !(counts[4].Fc != 1), "music counts = %#v", counts)
		testutil.Require(t, !(counts[4].Ap != 1), "music counts = %#v", counts)
	}

	levels := buildHonorFcApLevels([]drawing.MusicClearCount{
		{Difficulty: "master", Fc: 12, Ap: 3},
		{Difficulty: " ", Fc: 99},
	})
	testutil.RequireArgs(t, !(len(levels) == 0), "FC/AP honor levels are empty")
	{
		testutil.RequireArgs(t, !(buildHonorFcApLevels(nil) != nil), "empty FC/AP inputs returned levels")
		testutil.RequireArgs(t, !(buildHonorFcApLevels([]drawing.MusicClearCount{{Difficulty: " "}}) != nil), "empty FC/AP inputs returned levels")
	}

	solo := buildSoloLive(
		[]snapshot.RawChallengeLiveResult{{CharacterID: 1, HighScore: 10}, {CharacterID: 2, HighScore: 30}},
		[]snapshot.RawChallengeLiveStage{{CharacterID: 1, Rank: 9}, {CharacterID: 2, Rank: 2}, {CharacterID: 2, Rank: 5}},
	)
	{
		testutil.Require(t, !(solo == nil), "solo-live rank = %#v", solo)
		testutil.Require(t, !(solo.CharacterID != 2), "solo-live rank = %#v", solo)
		testutil.Require(t, !(solo.Score != 30), "solo-live rank = %#v", solo)
		testutil.Require(t, !(solo.Rank != 5), "solo-live rank = %#v", solo)
	}
	testutil.RequireArgs(t, !(buildSoloLive(nil, nil) != nil), "empty solo-live results returned a rank")
	{

		got := adaptAPIChallengeLiveResult(sekai.UserChallengeLiveSoloResult{CharacterID: 2, HighScore: 40})
		{
			testutil.Require(t, !(len(got) != 1), "adapted solo-live result = %#v", got)
			testutil.Require(t, !(got[0].HighScore != 40), "adapted solo-live result = %#v", got)
		}
	}

	stages := adaptAPIChallengeLiveStages([]sekai.AnotherUserChallengeLiveSoloStage{{CharacterID: 2, Rank: 7}})
	{
		testutil.Require(t, !(len(stages) != 1), "adapted solo-live stages = %#v", stages)
		testutil.Require(t, !(stages[0].Rank != 7), "adapted solo-live stages = %#v", stages)
	}

	vertical := true
	{
		settings := applyProfileBGVerticalOverride(nil, &vertical)
		{
			testutil.Require(t, !(settings == nil), "vertical background settings = %#v", settings)
			testutil.Require(t, settings.Vertical, "vertical background settings = %#v", settings)
		}
	}

	entries := buildAPIUserCardEntries([]sekai.AnotherUserCard{
		{},
		{CardID: 8, Level: 1},
		{CardID: 8, Level: 2},
	}, sekai.UserDeck{})
	testutil.Require(t, !(len(entries) != 1), "deduplicated API card entries = %#v", entries)
	{

		ranks := buildCharacterRanks([]snapshot.RawUserCharacter{{CharacterID: 9, CharacterRank: 10}})
		{
			testutil.Require(t, !(len(ranks) != 1), "character ranks = %#v", ranks)
			testutil.Require(t, !(ranks[0].Rank != 10), "character ranks = %#v", ranks)
		}
	}
	{

		chars := adaptAPICharacters([]sekai.AnotherUserCharacter{{CharacterID: 11, CharacterRank: 12}})
		{
			testutil.Require(t, !(len(chars) != 1), "adapted characters = %#v", chars)
			testutil.Require(t, !(chars[0].CharacterRank != 12), "adapted characters = %#v", chars)
		}
	}
	{
		testutil.RequireArgs(t, !(parseFramesJSON([]byte(`{`)) != nil), "invalid frame data or nil snapshot card was accepted")
		testutil.RequireArgs(t, !(isSnapshotCardTrainedArt(nil)), "invalid frame data or nil snapshot card was accepted")
	}
	{

		_, err := minimalProfileController(nil).cardByIDWithFallback(nil, renderregion.JP, 1)
		testutil.RequireArgs(t, !(err == nil), "nil card source unexpectedly succeeded")
	}
	{

		got := minimalProfileController(nil).buildLeaderImagePathFromSource(&testProfileSource{}, 99, false, renderregion.JP)
		testutil.RequireArgs(t, !(got == ""), "missing leader card returned an empty placeholder")
	}

}
