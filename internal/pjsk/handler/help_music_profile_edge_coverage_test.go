package handler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/masterdata"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
)

func TestCommandHelpPureFallbackBranches(t *testing.T) {
	if commandHelpRequestPath(nil) != "" || commandHelpDocKey(" ") != "" || commandHelpFamily(" ") != "generic" || commandHelpFamily("music") != "music" {
		t.Fatal("command help empty/family path mismatch")
	}
	if got := normalizeCommandHelpTrigger("/jp"); got != "/jp" {
		t.Fatalf("bare region trigger = %q", got)
	}
	if got := normalizeCommandHelpTrigger("/JP/music"); got != "/music" {
		t.Fatalf("regional trigger = %q", got)
	}
	if got := normalizeCommandHelpAlias("/cn歌曲列表"); got != "/歌曲列表" || normalizeCommandHelpAlias(" ") != "" {
		t.Fatalf("normalized alias = %q", got)
	}
	if _, ok, err := readCommandHelpMarkdown(""); err != nil || ok {
		t.Fatalf("empty help document = %v, %v", ok, err)
	}
	if _, ok, err := readCommandHelpMarkdown("definitely-missing"); err != nil || ok {
		t.Fatalf("missing help document = %v, %v", ok, err)
	}
	if keys := commandHelpLookupKeys(""); len(keys) != 1 || keys[0] != "generic" {
		t.Fatalf("empty lookup keys = %#v", keys)
	}

	resolved := &CommandRequest{CommandPath: "missing/child", TriggerCommand: "/fallback", HelpText: "usage details"}
	markdown, err := commandHelpMarkdown(resolved)
	if err != nil || !strings.Contains(markdown, "usage details") {
		t.Fatalf("fallback markdown = %q, %v", markdown, err)
	}
	if title := commandHelpTitle(nil, "plain text"); title != "指令帮助" {
		t.Fatalf("default title = %q", title)
	}
	if title := commandHelpTitle(&CommandRequest{CommandPath: "path"}, "plain text"); title != "path" {
		t.Fatalf("path title = %q", title)
	}
	if title := commandHelpTitle(&CommandRequest{TriggerCommand: "/trigger"}, "plain text"); title != "/trigger" {
		t.Fatalf("trigger title = %q", title)
	}
	if title := commandHelpTitle(nil, "## Heading\nbody"); title != "Heading" {
		t.Fatalf("markdown title = %q", title)
	}
	if got := fallbackCommandHelpMarkdown("", "", "body"); !strings.Contains(got, "指令帮助") {
		t.Fatalf("generic fallback = %q", got)
	}
	if got := fallbackCommandHelpMarkdown("", "path", "body"); !strings.Contains(got, "path") {
		t.Fatalf("path fallback = %q", got)
	}
	if got := withCommandHelpAliasSection("body", ""); got != "body" || missingCommandHelpAliases("body", "") != nil || commandHelpAliases("") != nil {
		t.Fatal("empty alias section mismatch")
	}
	if message, err := commandHelpMessage(context.Background(), nil, nil); err != nil || len(message) != 1 {
		t.Fatalf("generic text help = %#v, %v", message, err)
	}
}

func TestMusicHandlerAndParserEdgeBranches(t *testing.T) {
	selfTarget := mysekaiEdgeContext("")
	selfTarget.uidArg = "@target"
	for _, h := range []HarukiSekaiCommandHandler{
		sekaiHandlers{}.MusicListHandle(), sekaiHandlers{}.MusicRewardsHandle(), sekaiHandlers{}.MusicProgressHandle(),
	} {
		if _, err := h.handleFunc(selfTarget); err == nil {
			t.Fatalf("handler %s accepted target query", h.Path)
		}
	}
	for _, tc := range []struct {
		handler HarukiSekaiCommandHandler
		args    string
	}{
		{sekaiHandlers{}.SongHandle(), ""},
		{sekaiHandlers{}.NoteNumHandle(), "not-a-number"},
		{sekaiHandlers{}.BPMHandle(), ""},
		{sekaiHandlers{}.BPMHandle(), "master"},
		{sekaiHandlers{}.BPMSearchHandle(), "bad"},
		{sekaiHandlers{}.BPMSearchHandle(), "0"},
		{sekaiHandlers{}.MusicCoverHandle(), ""},
	} {
		if _, err := tc.handler.handleFunc(mysekaiEdgeContext(tc.args)); err == nil {
			t.Fatalf("handler %s accepted %q", tc.handler.Path, tc.args)
		}
	}
	if request, err := (sekaiHandlers{}).SongHandle().handleFunc(mysekaiEdgeContext("song expert")); err != nil || request == nil || !strings.Contains(string(request.Params), "expert") {
		t.Fatalf("song difficulty request = %#v, %v", request, err)
	}

	for _, token := range []string{"not_fc", "未完成"} {
		filter, _, ok := extractMusicListResultFilter("prefix " + token + " suffix")
		if !ok || filter == "" {
			t.Fatalf("result filter %q = %q, %v", token, filter, ok)
		}
	}
	if filter, rest, ok := extractMusicListResultFilter("none"); ok || filter != "" || rest != "none" {
		t.Fatalf("missing result filter = %q, %q, %v", filter, rest, ok)
	}
	if full, rest := extractMusicListFullFlag(""); full || rest != "" {
		t.Fatalf("empty full flag = %v, %q", full, rest)
	}

	for _, token := range []string{"<=30", ">=31", "=32"} {
		if _, ok := parseMusicListLevelToken(token); !ok {
			t.Fatalf("level token %q rejected", token)
		}
	}
	for _, token := range []string{"<=0", ">=-1", "=0", "<1"} {
		if _, ok := parseMusicListLevelToken(token); ok {
			t.Fatalf("invalid level token %q accepted", token)
		}
	}
	if params, rest, ok := extractMusicListLevelArgs("prefix 32 30 suffix"); !ok || params["level_min"] != 30 || rest != "prefix suffix" {
		t.Fatalf("adjacent levels = %#v, %q, %v", params, rest, ok)
	}
	if params, rest, ok := extractMusicListLevelArgs("prefix 32 到 30 suffix"); !ok || params["level_max"] != 32 || rest != "prefix suffix" {
		t.Fatalf("separated levels = %#v, %q, %v", params, rest, ok)
	}
	if _, rest, ok := extractMusicListLevelArgs("nothing"); ok || rest != "nothing" {
		t.Fatalf("missing levels = %q, %v", rest, ok)
	}
	if left, right, ok := parseMusicListRangeToken("【32～30】"); !ok || left != 32 || right != 30 {
		t.Fatalf("bracketed level range = %d, %d, %v", left, right, ok)
	}
	if _, _, ok := parseMusicListRangeToken("bad"); ok || isMusicListRangeSeparatorToken("bad") {
		t.Fatal("invalid range accepted")
	}
	if joinMusicListTokensExcluding(nil, 0) != "" {
		t.Fatal("empty token join should be empty")
	}

	if got := formatMusicDuration(-1); got != "0:00" {
		t.Fatalf("negative duration = %q", got)
	}
	if got := formatMusicBPMSequence([]rendermusic.BPMEvent{{BPM: 0}, {BPM: 120}, {BPM: 120}, {BPM: 130}}); got != "0 / 120 / 130" {
		t.Fatalf("BPM sequence = %q", got)
	}
	if got := dedupeBPMMatchesByMusic([]rendermusic.BPMMatch{{}, {Music: &masterdata.Music{ID: 1}}, {Music: &masterdata.Music{ID: 1}}}); len(got) != 1 {
		t.Fatalf("deduped BPM matches = %#v", got)
	}
	if dedupeBPMMatchesByMusic(nil) != nil {
		t.Fatal("nil BPM matches should stay nil")
	}
	for _, err := range []error{errors.New(""), errors.New("匹配到多个歌曲 1,2"), errors.New("custom title")} {
		if buildAmbiguousMusicDetailListTitle(err) == "" || buildAmbiguousMusicBPMListTitle(err) == "" {
			t.Fatal("ambiguous title should never be empty")
		}
	}
	if eventPlannerBoostMultiplier(99) != 1 {
		t.Fatal("unknown event-planner boost multiplier should be one")
	}
}

func TestProfileBindingHandlerSuccessBranches(t *testing.T) {
	base := additionalProfileContext("12345678901234", "")
	for _, h := range []HarukiSekaiCommandHandler{
		sekaiHandlers{}.ProfileBindHandle(), sekaiHandlers{}.ProfileUnbindHandle(), sekaiHandlers{}.ProfileSetMainHandle(),
	} {
		if request, err := h.handleFunc(base); err != nil || request == nil {
			t.Fatalf("handler %s = %#v, %v", h.Path, request, err)
		}
	}
	if request, handled, err := tryRerouteProfileBindCommand(base, "list"); err != nil || !handled || request == nil {
		t.Fatalf("rerouted list = %#v, %v, %v", request, handled, err)
	}
	if request, handled, err := tryRerouteProfileBindCommand(base, "swap u1 u2"); err != nil || !handled || request == nil {
		t.Fatalf("rerouted swap = %#v, %v, %v", request, handled, err)
	}
	if request, handled, err := tryRerouteProfileBindCommand(base, "123"); err != nil || handled || request != nil {
		t.Fatalf("ordinary bind reroute = %#v, %v, %v", request, handled, err)
	}
	if request, handled, err := tryRerouteProfileBindCommand(base, ""); err != nil || handled || request != nil {
		t.Fatalf("empty bind reroute = %#v, %v, %v", request, handled, err)
	}

	explicit := base
	explicit.explicitRegion = true
	explicit.region = renderregion.CN
	for _, h := range []HarukiSekaiCommandHandler{
		sekaiHandlers{}.ProfileBindListHandle(), sekaiHandlers{}.ProfileBindSwapHandle(), sekaiHandlers{}.ProfileClearDefaultBindingHandle(),
	} {
		ctx := explicit
		if h.Path == "profile/bind/list" || h.Path == "profile/default/clear" {
			ctx.SetArgs("")
		} else {
			ctx.SetArgs("u1 u2")
		}
		if request, err := h.handleFunc(ctx); err != nil || request == nil {
			t.Fatalf("handler %s = %#v, %v", h.Path, request, err)
		}
	}
	if buildProfileBindDerivedTrigger(explicit, "list") != "/cn绑定列表" || buildProfileBindDerivedTrigger(explicit, "swap") != "/cn绑定交换" ||
		buildProfileBindDerivedTrigger(explicit, "other") != explicit.originalTriggerCmd {
		t.Fatal("derived profile bind trigger mismatch")
	}
}

func TestEventPlannerAdditionalPureEdges(t *testing.T) {
	if _, err := parseEventPlannerParams("", "/planner"); err == nil {
		t.Fatal("empty planner params unexpectedly succeeded")
	}
	if err := parseEventPlannerDeckParams("anything", nil, "/planner"); err != nil {
		t.Fatalf("nil deck params = %v", err)
	}
	if point, rest := parseEventPlannerBareTargetPoint(" , 10000 #1 20000"); point != 10_000 || !strings.Contains(rest, "#1") {
		t.Fatalf("bare planner target = %d, %q", point, rest)
	}
	if value, ok := parseEventPlannerPointWithRE(eventPlannerTargetPointRE, "none"); ok || value != 0 {
		t.Fatalf("missing planner point = %d, %v", value, ok)
	}
	if songs, rest := parseEventPlannerSongs("no marker"); songs != nil || rest != "no marker" {
		t.Fatalf("planner songs without marker = %#v, %q", songs, rest)
	}
	selections, rest := eventPlannerSongSelectionsFromText("#1 5火")
	if len(selections) != 0 || !strings.Contains(rest, "#1") {
		t.Fatalf("non-song selections = %#v, %q", selections, rest)
	}
	songs := []eventPlannerSongSelection{{Query: "虾"}, {Query: "龙", Difficulty: "expert"}, {Query: "野车"}}
	applyEventPlannerDefaultSongDifficulties(songs)
	if songs[0].Difficulty != "expert" || songs[1].Difficulty != "expert" || songs[2].MusicID != eventPlannerOmakaseMusicID {
		t.Fatalf("default planner songs = %#v", songs)
	}
	if !eventPlannerContainsAny("abc", "", "b") || eventPlannerContainsAny("abc", "x") {
		t.Fatal("planner contains-any mismatch")
	}
	if query := buildEventPlannerBaseDeckQuery(renderregion.JP, deckAutoQueryParams{}); query.Algorithm != "rl" || query.LiveType != "multi" || query.Target != "score" {
		t.Fatalf("default planner deck query = %+v", query)
	}
	if got := eventPlannerDailyPoint(1_000, 100, 0, int64(2*24*60*60*1000), int64(24*60*60*1000), true); got != 900 {
		t.Fatalf("daily planner point = %d", got)
	}
	if got := eventPlannerEventBannerPath(&renderapp.App{}, renderregion.JP, &masterdata.Event{AssetBundleName: "bundle"}); got == "" {
		t.Fatal("event banner path should be derived")
	}
}

func TestBuildEventPlannerDrawingRequestSuccess(t *testing.T) {
	deckCtrl := newHandlerTestDeckController(t)
	app := &renderapp.App{Decks: deckCtrl}
	rc := NewRequestContext(context.Background(), &CommandRequest{Region: "jp"}, app)
	now := time.Now()
	eventInfo := &masterdata.Event{
		ID:              7,
		Name:            "Planner Event",
		AssetBundleName: "event_7",
		StartAt:         now.Add(-24 * time.Hour).UnixMilli(),
		AggregateAt:     now.Add(5 * 24 * time.Hour).UnixMilli(),
	}
	query := buildEventPlannerBaseDeckQuery(renderregion.JP, deckAutoQueryParams{MaxProfile: true})
	request, err := buildEventPlannerDrawingRequest(
		rc,
		renderregion.JP,
		eventInfo,
		&runtimeSnapshotStub{},
		query,
		eventPlannerCommandParams{Boosts: []int{1, 3, 10}},
		[]eventPlannerSongSelection{{Query: "野车", Difficulty: "master", MusicID: eventPlannerOmakaseMusicID}},
		10_000,
		"manual",
		1_000,
		true,
	)
	if err != nil {
		t.Fatalf("build planner drawing request: %v", err)
	}
	if len(request.Songs) != 1 || len(request.Songs[0].Rows) != 3 || len(request.DeckCards) == 0 {
		t.Fatalf("planner drawing request = %+v", request)
	}
	if request.RemainingPoint != 9_000 || request.DeckTotalPower <= 0 || request.DeckSummary == "" {
		t.Fatalf("planner totals = %+v", request)
	}
}

func TestExecuteEventPlannerSuccessfulCoverage(t *testing.T) {
	ctx := context.Background()
	app, _ := newExecutionCoverageApp(t)
	app.Decks = newHandlerTestDeckControllerForRegion(t, renderregion.JP, 100, "Current WL")
	app.Music = newHandlerTestMusicController(t)
	eventApp := newDeckEventCoverageApp(t)
	app.Provider = eventApp.Provider
	app.Providers = eventApp.Providers
	app.Bindings = newHandlerTestBindingService(t)
	if _, err := app.Bindings.Bind(ctx, "qq", "planner", "12345678901234"); err != nil {
		t.Fatalf("bind planner user: %v", err)
	}
	app.Snapshots = rendersnapshot.NewStaticSnapshotProvider(&runtimeSnapshotStub{})
	app.Config.UserSnapshot.AllowFallback = true

	params := eventPlannerCommandParams{
		TargetPoint:     25_000,
		CurrentPoint:    1_000,
		CurrentPointSet: true,
		TotalRanking:    true,
		Deck: deckAutoQueryParams{
			EventID:    drawing.IntPtr(100),
			MaxProfile: true,
		},
		Songs:  []eventPlannerSongSelection{{Query: "野车", Difficulty: "master", MusicID: eventPlannerOmakaseMusicID}},
		Boosts: []int{5, 10},
	}
	rc := executionCoverageContext(t, app, "event-planner", params)
	rc.Platform = "qq"
	rc.PlatformUserID = "planner"
	rc.Cmd.RequesterPlatform = "qq"
	rc.Cmd.RequesterUserID = "planner"
	message, err := executeEventPlanner(rc)
	if err != nil {
		t.Fatalf("execute planner: %v", err)
	}
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("planner message = %#v", message)
	}
}
