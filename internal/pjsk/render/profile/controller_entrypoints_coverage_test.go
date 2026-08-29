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
	if err != nil {
		t.Fatalf("BuildModularProfileRequestFromAPIWithSnapshot() error = %v", err)
	}
	if payload.Profile.ID != "42" || payload.Profile.IsHideUID || payload.Preset.ID == "" || len(payload.Preset.Widgets) != 7 {
		t.Fatalf("unexpected modular payload: %+v", payload)
	}
	if payload.BgSettings == nil || payload.BgSettings.ImgPath == nil || *payload.BgSettings.ImgPath != "asset/background.png" {
		t.Fatalf("background settings = %+v", payload.BgSettings)
	}
	widget := modularWidget("id", "type", "family", "Title", 1, 2, 3, 4, map[string]any{"a": 1}, map[string]any{"b": 2})
	if widget.Title == nil || *widget.Title != "Title" || widget.Frame.W != 3 {
		t.Fatalf("widget = %+v", widget)
	}
	if firstCharacterRank(nil) != nil || firstDeckCard(nil) != nil {
		t.Fatal("empty focus helpers returned an item")
	}
	ranks := []drawing.CharacterRank{{}, {CharacterID: 3}}
	cards := []drawing.CardFullThumbnailRequest{{}, {CardID: 4}}
	if firstCharacterRank(ranks).CharacterID != 3 || firstDeckCard(cards).CardID != 4 {
		t.Fatal("focus helpers did not skip empty entries")
	}
}

func TestProfilePublicBuildEntrypointsAndValidation(t *testing.T) {
	controller := minimalProfileController(nil)
	resp := minimalProfileResponse()
	if detail, err := controller.BuildDetailedProfileCardFromAPI(Query{Region: "jp"}, resp, []byte(`[]`)); err != nil || detail.ID != "42" {
		t.Fatalf("detailed profile = %+v, %v", detail, err)
	}
	if card, err := controller.BuildProfileCardFromAPI(Query{Region: "jp"}, resp, []byte(`[]`)); err != nil || card.Profile == nil || card.Profile.ID != "42" {
		t.Fatalf("profile card = %+v, %v", card, err)
	}

	if _, err := (*Controller)(nil).BuildModularProfileRequestFromAPIWithSnapshot(Query{}, resp, nil); err == nil {
		t.Fatal("nil modular controller unexpectedly succeeded")
	}
	if _, err := controller.BuildModularProfileRequestFromAPIWithSnapshot(Query{}, nil, nil); err == nil {
		t.Fatal("nil modular response unexpectedly succeeded")
	}
	if _, err := NewController(nil, nil, nil, nil).BuildModularProfileRequestFromAPIWithSnapshot(Query{}, resp, nil); err == nil {
		t.Fatal("missing modular source unexpectedly succeeded")
	}
	if _, err := controller.BuildDetailedProfileCardFromAPI(Query{}, nil, nil); err == nil {
		t.Fatal("nil detailed response unexpectedly succeeded")
	}
	if _, err := controller.BuildProfileCardFromAPI(Query{}, nil, nil); err == nil {
		t.Fatal("nil card response unexpectedly succeeded")
	}
	if controller.SnapshotDetailedProfile(renderregion.JP) != nil || (*Controller)(nil).SnapshotDetailedProfile(renderregion.JP) != nil {
		t.Fatal("missing snapshot returned profile data")
	}
	controller.SetCensor(nil)
	(*Controller)(nil).SetCensor(nil)
	controller.RegisterSource(nil)
	(*Controller)(nil).RegisterSource(nil)
	if (*Controller)(nil).WithContext(context.Background()) != nil {
		t.Fatal("nil controller WithContext returned a controller")
	}
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
			if err != nil || !bytes.Equal(got, []byte("profile-image")) {
				t.Fatalf("render = %q, %v", got, err)
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
			if _, err := render(); err == nil {
				t.Fatal("render unexpectedly succeeded")
			}
		})
	}
	if _, err := controller.RenderProfileFromAPI(Query{}, nil, nil); err == nil {
		t.Fatal("render with invalid payload unexpectedly succeeded")
	}
	if _, err := controller.RenderModularProfileFromAPIWithSnapshot(Query{}, nil, nil); err == nil {
		t.Fatal("modular render with invalid payload unexpectedly succeeded")
	}
}

func TestProfileProviderAdapterEmptyDatabase(t *testing.T) {
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:profile_adapter_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = client.Close() })
	adapter := NewProviderAdapter(provider.NewDatabaseProvider(client, renderregion.JP))
	ctx := context.WithValue(context.Background(), profileContextKey("adapter"), "request")
	withContext := adapter.WithContext(ctx)
	if withContext == nil || withContext.(*ProviderAdapter).Context() != ctx {
		t.Fatal("adapter did not retain its request context")
	}
	if (*ProviderAdapter)(nil).WithContext(ctx) != nil {
		t.Fatal("nil adapter returned a data source")
	}
	if _, err := adapter.GetHonorByID(1); err == nil {
		t.Fatal("missing honor returned no error")
	}
	if _, err := adapter.GetHonorGroupByID(1); err == nil {
		t.Fatal("missing honor group returned no error")
	}
	if _, err := adapter.GetBondsHonorByID(1); err == nil {
		t.Fatal("missing bonds honor returned no error")
	}
	if _, err := adapter.GetBondsHonorWordByID(1); err == nil {
		t.Fatal("missing bonds honor word returned no error")
	}
	if _, ok := adapter.GetGameCharacterUnitByID(1); ok {
		t.Fatal("missing game-character unit was found")
	}
	if _, err := adapter.GetPlayerFrameByID(1); err == nil {
		t.Fatal("missing frame returned no error")
	}
	if _, err := adapter.GetPlayerFrameGroupByID(1); err == nil {
		t.Fatal("missing frame group returned no error")
	}
	if _, err := adapter.GetCardByID(1); err == nil {
		t.Fatal("missing card returned no error")
	}
	if got := adapter.GetEventIDByHonorID(1); got != 0 {
		t.Fatalf("missing honor event ID = %d", got)
	}
}

func TestProfileAggregationHelperBranches(t *testing.T) {
	counts := buildMusicCounts(nil, []snapshot.RawMusicResult{
		{MusicID: 1, MusicDifficultyType: "master", FullComboFlg: true, FullPerfectFlg: true},
		{MusicID: 1, MusicDifficultyType: "MASTER", FullComboFlg: true},
		{MusicID: 2, MusicDifficultyType: "master"},
		{MusicID: 3, MusicDifficultyType: "expert", FullComboFlg: true},
	})
	if len(counts) != 6 || counts[4].Clear != 2 || counts[4].Fc != 1 || counts[4].Ap != 1 {
		t.Fatalf("music counts = %#v", counts)
	}
	levels := buildHonorFcApLevels([]drawing.MusicClearCount{
		{Difficulty: "master", Fc: 12, Ap: 3},
		{Difficulty: " ", Fc: 99},
	})
	if len(levels) == 0 {
		t.Fatal("FC/AP honor levels are empty")
	}
	if buildHonorFcApLevels(nil) != nil || buildHonorFcApLevels([]drawing.MusicClearCount{{Difficulty: " "}}) != nil {
		t.Fatal("empty FC/AP inputs returned levels")
	}

	solo := buildSoloLive(
		[]snapshot.RawChallengeLiveResult{{CharacterID: 1, HighScore: 10}, {CharacterID: 2, HighScore: 30}},
		[]snapshot.RawChallengeLiveStage{{CharacterID: 1, Rank: 9}, {CharacterID: 2, Rank: 2}, {CharacterID: 2, Rank: 5}},
	)
	if solo == nil || solo.CharacterID != 2 || solo.Score != 30 || solo.Rank != 5 {
		t.Fatalf("solo-live rank = %#v", solo)
	}
	if buildSoloLive(nil, nil) != nil {
		t.Fatal("empty solo-live results returned a rank")
	}
	if got := adaptAPIChallengeLiveResult(sekai.UserChallengeLiveSoloResult{CharacterID: 2, HighScore: 40}); len(got) != 1 || got[0].HighScore != 40 {
		t.Fatalf("adapted solo-live result = %#v", got)
	}
	stages := adaptAPIChallengeLiveStages([]sekai.AnotherUserChallengeLiveSoloStage{{CharacterID: 2, Rank: 7}})
	if len(stages) != 1 || stages[0].Rank != 7 {
		t.Fatalf("adapted solo-live stages = %#v", stages)
	}

	vertical := true
	if settings := applyProfileBGVerticalOverride(nil, &vertical); settings == nil || !settings.Vertical {
		t.Fatalf("vertical background settings = %#v", settings)
	}
	entries := buildAPIUserCardEntries([]sekai.AnotherUserCard{
		{},
		{CardID: 8, Level: 1},
		{CardID: 8, Level: 2},
	}, sekai.UserDeck{})
	if len(entries) != 1 {
		t.Fatalf("deduplicated API card entries = %#v", entries)
	}
	if ranks := buildCharacterRanks([]snapshot.RawUserCharacter{{CharacterID: 9, CharacterRank: 10}}); len(ranks) != 1 || ranks[0].Rank != 10 {
		t.Fatalf("character ranks = %#v", ranks)
	}
	if chars := adaptAPICharacters([]sekai.AnotherUserCharacter{{CharacterID: 11, CharacterRank: 12}}); len(chars) != 1 || chars[0].CharacterRank != 12 {
		t.Fatalf("adapted characters = %#v", chars)
	}
	if parseFramesJSON([]byte(`{`)) != nil || isSnapshotCardTrainedArt(nil) {
		t.Fatal("invalid frame data or nil snapshot card was accepted")
	}
	if _, err := minimalProfileController(nil).cardByIDWithFallback(nil, renderregion.JP, 1); err == nil {
		t.Fatal("nil card source unexpectedly succeeded")
	}
	if got := minimalProfileController(nil).buildLeaderImagePathFromSource(&testProfileSource{}, 99, false, renderregion.JP); got == "" {
		t.Fatal("missing leader card returned an empty placeholder")
	}
}
