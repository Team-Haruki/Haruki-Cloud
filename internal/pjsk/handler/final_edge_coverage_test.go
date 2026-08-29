package handler

import (
	"context"
	"errors"
	"strings"
	"testing"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/masterdata"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func TestFinalCustomProfileCardEdges(t *testing.T) {
	target := mysekaiEdgeContext("1")
	target.uidArg = "@target"
	if _, err := buildProfileCustomProfileCardParams(target); err == nil {
		t.Fatal("custom profile self-only query accepted target")
	}
	for _, args := range []string{"", "1 2", "bad", "0", "-1"} {
		if _, err := buildProfileCustomProfileCardParams(mysekaiEdgeContext(args)); err == nil {
			t.Fatalf("custom profile page %q accepted", args)
		}
	}
	params, err := buildProfileCustomProfileCardParams(mysekaiEdgeContext(" 2 "))
	if err != nil || params.Seq != 2 || params.userQueryParams().Mode != "self" {
		t.Fatalf("custom profile params = %+v, %v", params, err)
	}
	explicit := profileCustomProfileCardParams{UserQueryParams: UserQueryParams{Mode: "uid", PJSKUserID: "123"}}
	if got := explicit.userQueryParams(); got.Mode != "uid" || got.PJSKUserID != "123" {
		t.Fatalf("explicit custom profile target = %+v", got)
	}

	cards := []sekaiapi.UserCustomProfileCard{
		{CustomProfileID: 2, CustomProfileCardID: 3, Seq: 2},
		{CustomProfileID: 1, CustomProfileCardID: 1, Seq: 1},
	}
	if _, err := resolveCustomProfileCard(nil, profileCustomProfileCardParams{}); err == nil {
		t.Fatal("empty custom profiles accepted")
	}
	card, err := resolveCustomProfileCard(cards, profileCustomProfileCardParams{CustomProfileID: 2, CustomProfileCardID: 3})
	if err != nil || card.CustomProfileID != 2 {
		t.Fatalf("explicit custom profile card = %+v, %v", card, err)
	}
	if _, err := resolveCustomProfileCard(cards, profileCustomProfileCardParams{CustomProfileID: 9, CustomProfileCardID: 9}); err == nil {
		t.Fatal("missing explicit custom profile accepted")
	}
	card, err = resolveCustomProfileCard(cards, profileCustomProfileCardParams{Seq: 0})
	if err != nil || card.Seq != 1 {
		t.Fatalf("default custom profile page = %+v, %v", card, err)
	}
	if value, ok := parsePositiveIntArg(" 12 "); !ok || value != 12 {
		t.Fatalf("positive integer = %d, %v", value, ok)
	}

	if _, err := executeProfileCustomProfileCard(nil); err == nil {
		t.Fatal("nil custom profile request accepted")
	}
	rc := &RequestContext{Ctx: context.Background(), Cmd: &CommandRequest{}, App: &renderapp.App{}}
	if _, err := executeProfileCustomProfileCard(rc); err == nil || !strings.Contains(err.Error(), "sekai api") {
		t.Fatalf("missing Sekai API error = %v", err)
	}
	rc.App.SekaiAPI = sekaiapi.NewSekaiAPIClient(nil)
	if _, err := executeProfileCustomProfileCard(rc); err == nil || !strings.Contains(err.Error(), "drawing") {
		t.Fatalf("missing drawing error = %v", err)
	}
	rc.App.Drawing = drawing.NewHarukiDrawingClient("")
	if _, err := executeProfileCustomProfileCard(rc); !errors.Is(err, accountdata.ErrBindingServiceUnavailable) {
		t.Fatalf("missing binding error = %v", err)
	}
}

func TestFinalContextDataMapEdges(t *testing.T) {
	if got := stripInlineCQTags(" \t "); got != " \t " {
		t.Fatalf("blank CQ text = %q", got)
	}
	if value, ok := extractSegmentDataField(map[string]any{"text": 123}, "text"); !ok || value != "123" {
		t.Fatalf("map[string]any field = %q, %v", value, ok)
	}
	if value, ok := extractSegmentDataField(map[any]any{"qq": 456}, "qq"); !ok || value != "456" {
		t.Fatalf("map[any]any direct field = %q, %v", value, ok)
	}
	if value, ok := extractSegmentDataField(map[any]any{any("text"): "value"}, "text"); !ok || value != "value" {
		t.Fatalf("map[any]any scanned field = %q, %v", value, ok)
	}
	if value, ok := extractSegmentDataField(123, "text"); ok || value != "" {
		t.Fatalf("unsupported segment data = %q, %v", value, ok)
	}
	message := onebot11.Message{
		{Type: onebot11.TypeAt, Data: map[string]any{"qq": 42}},
		{Type: onebot11.TypeText, Data: map[any]any{"text": "hello[CQ:at,qq=1]world"}},
	}
	ctx, err := BuildContext(context.Background(), Event{Message: message})
	if err != nil || len(ctx.AtIds) != 1 || ctx.AtIds[0] != "42" || !strings.Contains(ctx.ArgText, "hello world") {
		t.Fatalf("built context = %+v, %v", ctx, err)
	}
}

func TestFinalProfileCloneAndGuardEdges(t *testing.T) {
	if resolveCardBoxDetailedProfile(nil) != nil || resolveCardBoxDetailedProfile(&RequestContext{}) != nil {
		t.Fatal("nil card-box profile should stay nil")
	}
	if resolveCardCatalogTitle(nil) != nil || resolveCardCatalogTitle(&RequestContext{App: &renderapp.App{}}) != nil {
		t.Fatal("unscoped card catalog title should stay nil")
	}
	if detail, card := buildPublicMusicProfiles(nil); detail != nil || card != nil {
		t.Fatal("nil public music profiles should stay nil")
	}
	if detail, card := buildPublicMusicProfiles(&RequestContext{App: &renderapp.App{}}); detail != nil || card != nil {
		t.Fatal("unconfigured public music profiles should stay nil")
	}
	if detail, card := resolveCurrentTargetPublicProfiles(nil); detail != nil || card != nil {
		t.Fatal("nil current-target profiles should stay nil")
	}
	if detail, card := resolveCurrentTargetPublicProfiles(&RequestContext{}); detail != nil || card != nil {
		t.Fatal("unresolved current-target profiles should stay nil")
	}
	if cloneDetailedProfileForTarget(nil, ResolvedGameTarget{}, "jp") != nil {
		t.Fatal("nil detailed profile clone should stay nil")
	}
	detail := &drawing.DetailedProfileCardRequest{}
	if cloneDetailedProfileForCurrentTarget(nil, detail) != detail || cloneDetailedProfileForCurrentTarget(&RequestContext{}, detail) != detail {
		t.Fatal("unresolved detailed profile should not be cloned")
	}
	card := &drawing.ProfileCardRequest{}
	if cloneProfileCardForCurrentTarget(nil, card) != card || cloneProfileCardForCurrentTarget(&RequestContext{}, card) != card {
		t.Fatal("unresolved profile card should not be cloned")
	}
	if cloneProfileCardForTarget(nil, ResolvedGameTarget{}, "jp") != nil {
		t.Fatal("nil profile card clone should stay nil")
	}
	level := 7
	cloned := cloneProfileCardForTarget(&drawing.ProfileCardRequest{MysekaiLevel: &level}, ResolvedGameTarget{Visible: true}, "jp")
	if cloned.MysekaiLevel == &level || *cloned.MysekaiLevel != level {
		t.Fatal("MySekai level was not deeply cloned")
	}
	if detail, compact := buildPublicMusicProfilesFromResolvedTarget(context.Background(), ResolvedGameTarget{}, "jp", "", "", nil, nil); detail != nil || compact != nil {
		t.Fatal("nil public profile dependencies should stay nil")
	}
	if card, err := buildPublicProfileCardForTarget(context.Background(), ResolvedGameTarget{}, "jp", nil, nil); err != nil || card != nil {
		t.Fatalf("nil profile controller = %+v, %v", card, err)
	}
}

func TestFinalTargetSnapshotAndPlannerEdges(t *testing.T) {
	ctx := context.Background()
	if _, err := resolveGameTarget(ctx, userQueryParams{Mode: "uid", PJSKUserID: "1"}, "jp", true, nil); !errors.Is(err, accountdata.ErrBindingServiceUnavailable) {
		t.Fatalf("nil binding service error = %v", err)
	}
	service := newHandlerTestBindingService(t)
	app := &renderapp.App{Bindings: service}
	uidTarget, err := resolveGameTarget(ctx, userQueryParams{Mode: "uid", PJSKUserID: "123"}, "jp", true, app)
	if err != nil || uidTarget.PJSKUserID != "123" || !uidTarget.Visible {
		t.Fatalf("UID target = %+v, %v", uidTarget, err)
	}
	if _, err := resolveGameTarget(ctx, userQueryParams{Mode: "unknown"}, "jp", true, app); err == nil {
		t.Fatal("unknown target mode accepted")
	}
	if _, err := resolveGameTarget(ctx, userQueryParams{Mode: "at_user", Platform: "qq", AtUserID: "missing"}, "jp", true, app); err == nil {
		t.Fatal("missing at-user binding resolved")
	}
	if got := resolveRegionFromDefaultBinding(ctx, &CommandRequest{Region: "tw", RegionExplicit: true}, nil); got != "tw" {
		t.Fatalf("explicit target region = %q", got)
	}
	if got := resolveRegionFromDefaultBinding(ctx, &CommandRequest{Region: "en"}, app); got != "en" {
		t.Fatalf("unscoped target region = %q", got)
	}

	if defaultSnapshotProviderFactory(nil) != nil || defaultSnapshotProviderFactory(&renderapp.App{}) != nil {
		t.Fatal("empty snapshot app unexpectedly produced provider")
	}
	static := rendersnapshot.NewStaticSnapshotProvider(&runtimeSnapshotStub{})
	if defaultSnapshotProviderFactory(&renderapp.App{Config: renderapp.Config{UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true}}, Snapshots: static}) == nil {
		t.Fatal("static fallback snapshot provider missing")
	}
	liveApp := &renderapp.App{
		Bindings:  service,
		Toolbox:   sekaiapi.NewToolboxClient(nil),
		Snapshots: static,
		Config: renderapp.Config{UserSnapshot: renderapp.UserSnapshotConfig{
			Provider:      "toolbox",
			AllowFallback: true,
		}},
	}
	if liveSnapshotProvider(liveApp) == nil || defaultSnapshotProviderFactory(liveApp) == nil {
		t.Fatal("live/fallback snapshot providers missing")
	}
	if platform, userID := platformCredentials(userQueryParams{Mode: "at_user", Platform: "qq", AtUserID: "42"}); platform != "qq" || userID != "42" {
		t.Fatalf("at-user credentials = %q, %q", platform, userID)
	}
	if platform, userID := platformCredentials(userQueryParams{Mode: "uid"}); platform != "" || userID != "" {
		t.Fatalf("UID credentials = %q, %q", platform, userID)
	}

	if _, _, err := resolveEventPlannerEvent(ctx, nil, renderregion.JP, 0); err == nil {
		t.Fatal("planner without provider resolved event")
	}
	simulated, warning, err := resolveEventPlannerEventFromQuery(ctx, nil, renderregion.JP, renderDeckQueryWithSimulation())
	if err != nil || simulated.ID != 0 || warning == "" {
		t.Fatalf("simulated planner event = %+v, %q, %v", simulated, warning, err)
	}
	if _, _, err := resolveEventPlannerTargetPoint(nil, renderregion.JP, nil, renderDeckQueryWithSimulation(), eventPlannerCommandParams{TargetRank: 10}); err == nil {
		t.Fatal("rank target accepted without real event")
	}
	trackerApp := &renderapp.App{Tracker: sekaiapi.NewTrackerClient(nil)}
	if _, _, err := resolveEventPlannerTargetPoint(&RequestContext{App: trackerApp}, renderregion.JP, &masterdata.Event{ID: 1}, renderDeckQueryWithSimulation(), eventPlannerCommandParams{TargetRank: 10}); err == nil {
		t.Fatal("WL rank target accepted without chapter character")
	}
	if point, known, warning := resolveEventPlannerCurrentPoint(nil, nil, renderregion.JP, nil, renderDeckQueryWithSimulation(), eventPlannerCommandParams{}); point != 0 || !known || warning == "" {
		t.Fatalf("planner current point fallback = %d, %v, %q", point, known, warning)
	}
}

func renderDeckQueryWithSimulation() renderdeck.AutoQuery {
	return renderdeck.AutoQuery{EventAttr: "cute"}
}
