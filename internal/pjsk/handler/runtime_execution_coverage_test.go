package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	rendercostume "haruki-cloud/internal/pjsk/render/costume"
	rendereducation "haruki-cloud/internal/pjsk/render/education"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	rendergacha "haruki-cloud/internal/pjsk/render/gacha"
	rendermisc "haruki-cloud/internal/pjsk/render/misc"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	renderprofile "haruki-cloud/internal/pjsk/render/profile"
	renderscore "haruki-cloud/internal/pjsk/render/score"
	rendersk "haruki-cloud/internal/pjsk/render/sk"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/imagecache"
)

func newExecutionCoverageApp(t *testing.T) (*renderapp.App, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/profile") {
			_ = json.NewEncoder(w).Encode(sekaiapi.GetAnotherProfileResponse{
				User: sekaiapi.AnotherUser{UserID: 12345678901234, Name: "coverage-user", Rank: 88},
				UserMusicDifficultyClearCount: []sekaiapi.AnotherUserMusicDifficultyClearCount{
					{MusicDifficultyType: sekaiapi.MusicDifficultyMaster, LiveClear: 10, FullCombo: 8, AllPerfect: 3},
				},
				UserChallengeLiveSoloResult: sekaiapi.UserChallengeLiveSoloResult{CharacterID: 21, HighScore: 1234567},
			})
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("rendered-image"))
	}))

	drawingClient := drawing.NewHarukiDrawingClient(server.URL)
	assetHelper := assets.NewAssetHelper(t.TempDir(), nil)
	app := &renderapp.App{
		Drawing:    drawingClient,
		Assets:     assetHelper,
		ImageCache: imagecache.New("https://cache.invalid", t.TempDir()),
		Score:      renderscore.NewController(drawingClient),
		SK:         rendersk.NewController(drawingClient),
		Misc:       rendermisc.NewController(drawingClient),
		Edu:        rendereducation.NewController(drawingClient, assetHelper, nil, renderregion.JP),
		MySekai: rendermysekai.NewController(drawingClient, nil, renderregion.JP, assetHelper, rendermysekai.MasterdataOptions{
			LocalDir: t.TempDir(),
		}),
		Costumes: rendercostume.NewController(nil, drawingClient, assetHelper),
		Events:   renderevent.NewController(nil, drawingClient, assetHelper),
		Gachas:   rendergacha.NewController(nil, drawingClient, assetHelper),
		Profiles: renderprofile.NewController(nil, drawingClient, assetHelper, nil),
		SekaiAPI: sekaiapi.NewSekaiAPIClient(&harukiConfig.SekaiAPIConfig{BaseURL: server.URL}),
		Bindings: accountdata.NewBindingService(nil, nil, nil),
		Config: renderapp.Config{
			DefaultRegion: renderregion.JP,
		},
	}
	t.Cleanup(func() {
		_ = app.ImageCache.Close()
		server.Close()
		resetSekaiProfileCacheForTest()
	})
	return app, server
}

func executionCoverageParams(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return raw
}

func executionCoverageContext(t *testing.T, app *renderapp.App, mode string, params any) *RequestContext {
	t.Helper()
	cmd := &CommandRequest{Mode: mode, Region: "jp"}
	if params != nil {
		cmd.Params = executionCoverageParams(t, params)
	}
	return NewRequestContext(context.Background(), cmd, app)
}

func assertExecutionCoverageOutcome(t *testing.T, message any, err error) {
	t.Helper()
	if err == nil && message == nil {
		t.Fatal("execution returned neither a message nor an error")
	}
}

func TestExecuteSKDrawingModesCoverage(t *testing.T) {
	app, _ := newExecutionCoverageApp(t)
	for _, mode := range []string{
		"sk-line", "sk-query", "sk-check-room", "sk-csb", "sk-speed", "sk-daily-speed",
		"sk-player-trace", "sk-rank-trace", "sk-predict", "sk-winrate",
	} {
		t.Run(mode, func(t *testing.T) {
			message, err := executeSK(executionCoverageContext(t, app, mode, nil))
			assertExecutionCoverageOutcome(t, message, err)
		})
	}
	winRate := drawing.WinRateRequest{TeamInfo: []drawing.TeamInfo{{TeamID: 1, TeamName: "one", WinRate: 0.5}}}
	message, err := executeSK(executionCoverageContext(t, app, "sk-winrate", winRate))
	if err != nil || len(message) == 0 {
		t.Fatalf("executeSK winrate success = %#v, %v", message, err)
	}
	if _, err := executeSK(executionCoverageContext(t, app, "sk-unknown", nil)); err == nil {
		t.Fatal("expected unsupported SK mode error")
	}
	if _, err := executeSK(&RequestContext{Cmd: &CommandRequest{Mode: "sk-line"}}); err == nil {
		t.Fatal("expected unavailable SK error")
	}
}

func TestExecuteScorePrefilledRequestsCoverage(t *testing.T) {
	app, _ := newExecutionCoverageApp(t)
	cases := []struct {
		mode   string
		params any
	}{
		{
			mode: "score-control",
			params: drawing.ScoreControlRequest{
				MusicID: 1, TargetPoint: 1000, ValidScores: []drawing.ScoreData{{EventBonus: 100, Boost: 5, ScoreMin: 1, ScoreMax: 2}},
			},
		},
		{
			mode: "score-custom-room",
			params: drawing.CustomRoomScoreRequest{
				TargetPoint: 1000, CandidatePairs: [][]int{{100, 200}}, MusicListMap: map[int][]map[string]any{1: {{"music_cover": "jacket/a/a.png"}}},
			},
		},
		{
			mode:   "score-music-board",
			params: drawing.MusicBoardRequest{Items: []drawing.MusicBoardItem{{MusicID: 1, Difficulty: "master"}}},
		},
	}
	for _, tc := range cases {
		message, err := executeScore(executionCoverageContext(t, app, tc.mode, tc.params))
		if err != nil {
			t.Fatalf("executeScore(%s): %v", tc.mode, err)
		}
		if len(message) == 0 {
			t.Fatalf("executeScore(%s) returned empty message", tc.mode)
		}
	}
	invalidMusicMeta := executionCoverageContext(t, app, "score-music-meta", nil)
	invalidMusicMeta.Cmd.Params = []byte("{")
	if _, err := executeScore(invalidMusicMeta); err == nil {
		t.Fatal("expected invalid music-meta params error")
	}
	if _, err := executeScore(executionCoverageContext(t, app, "score-unknown", nil)); err == nil {
		t.Fatal("expected unsupported score mode")
	}
	if got := splitScoreMusicMetaQueries(" a / b || c "); len(got) != 3 {
		t.Fatalf("split score queries = %#v", got)
	}
}

func TestExecuteProfileArrestAndRegTimeCoverage(t *testing.T) {
	app, _ := newExecutionCoverageApp(t)
	const userID = "12345678901234"
	query := userQueryParams{Mode: "uid", PJSKUserID: userID}

	arrestMessage, err := executeArrest(executionCoverageContext(t, app, "arrest", query))
	if err != nil {
		t.Fatalf("executeArrest: %v", err)
	}
	if len(arrestMessage) != 1 {
		t.Fatalf("arrest message = %#v", arrestMessage)
	}
	arrestText, arrestTextOK := arrestMessage[0].Data.(onebot11.TextData)
	if !arrestTextOK || !strings.Contains(arrestText.Text, "coverage-user") {
		t.Fatalf("arrest message = %#v", arrestMessage)
	}

	regMessage, err := executeRegTime(executionCoverageContext(t, app, "reg-time", query))
	if err != nil {
		t.Fatalf("executeRegTime: %v", err)
	}
	if len(regMessage) != 1 {
		t.Fatalf("registration message = %#v", regMessage)
	}
	regText, regTextOK := regMessage[0].Data.(onebot11.TextData)
	if !regTextOK || !strings.Contains(regText.Text, "注册时间") {
		t.Fatalf("registration message = %#v", regMessage)
	}

	profileMessage, err := executeProfile(executionCoverageContext(t, app, "profile", query))
	assertExecutionCoverageOutcome(t, profileMessage, err)

	for _, mode := range []string{
		"profile-bind", "profile-hide-id", "profile-bg-adjust", "profile-unknown",
	} {
		message, err := executeProfile(executionCoverageContext(t, app, mode, nil))
		assertExecutionCoverageOutcome(t, message, err)
	}
	customMessage, customErr := executeProfile(executionCoverageContext(t, app, profileModeCustomProfileCard, query))
	assertExecutionCoverageOutcome(t, customMessage, customErr)
}

func TestExecuteControllerGuardAndModeCoverage(t *testing.T) {
	app, _ := newExecutionCoverageApp(t)

	for _, mode := range []string{"education-challenge", "education-bonds", "education-leader", "education-character-mission", "education-power", "education-area", "education-unknown"} {
		message, err := executeEducation(executionCoverageContext(t, app, mode, nil))
		assertExecutionCoverageOutcome(t, message, err)
	}
	fullArea := map[string]any{"show_full": true, "cid": 1, "region": "jp"}
	message, err := executeEducation(executionCoverageContext(t, app, "education-area", fullArea))
	assertExecutionCoverageOutcome(t, message, err)
	if _, err := executeEducation(&RequestContext{Cmd: &CommandRequest{Mode: "education-area"}}); err == nil {
		t.Fatal("expected unavailable education error")
	}

	for _, mode := range []string{"costume-detail", "costume-list", "costume-combo", "costume-unknown"} {
		message, err := executeCostume(executionCoverageContext(t, app, mode, nil))
		assertExecutionCoverageOutcome(t, message, err)
	}
	for _, mode := range []string{"gacha-list", "gacha-detail", "gacha-unknown"} {
		message, err := executeGacha(executionCoverageContext(t, app, mode, nil))
		assertExecutionCoverageOutcome(t, message, err)
	}
	for _, mode := range []string{"event-planner-help", "event-detail", "event-list", "event-record", "event-unknown"} {
		message, err := executeEvent(executionCoverageContext(t, app, mode, nil))
		assertExecutionCoverageOutcome(t, message, err)
	}
	message, err = executeMisc(executionCoverageContext(t, app, "misc-birthday", miscBirthdayParams{UpcomingIndex: 1}))
	assertExecutionCoverageOutcome(t, message, err)
	if _, err := executeMisc(executionCoverageContext(t, app, "misc-unknown", nil)); err == nil {
		t.Fatal("expected unsupported misc mode")
	}
}
