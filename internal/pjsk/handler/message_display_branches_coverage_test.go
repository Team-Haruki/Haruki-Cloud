package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	pjskenttest "haruki-cloud/database/pjsk/enttest"
	"haruki-cloud/ent/pjsk/schema"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/displaytime"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/releasecheck"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/internal/testutil"
)

func TestDisplayPreferenceAndTimeBranches(t *testing.T) {
	ctx := context.Background()
	{
		got := resolveHarukiUserChartStyle(ctx, nil, 1)
		testutil.Require(t, !(got != ""), "nil chart style = %q", got)
	}
	{

		got := resolveRequesterHarukiUserChartStyle(ctx, nil, "qq", "1")
		testutil.Require(t, !(got != ""), "nil requester chart style = %q", got)
	}
	{

		got := resolveHarukiUserTimeZone(ctx, nil, 1)
		testutil.Require(t, !(got != displaytime.DefaultTimeZone), "nil time zone = %q", got)
	}
	{

		got := resolveRequesterHarukiUserTimeZone(ctx, nil, "qq", "1")
		testutil.Require(t, !(got != displaytime.DefaultTimeZone), "nil requester time zone = %q", got)
	}

	db := pjskenttest.Open(t, "sqlite3", fmt.Sprintf("file:handler_display_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Close() })
	app := &renderapp.App{PJSK: db}
	{
		got := resolveHarukiUserChartStyle(ctx, app, 1)
		testutil.Require(t, !(got != ""), "missing chart style = %q", got)
	}
	{

		got := resolveHarukiUserTimeZone(ctx, app, 1)
		testutil.Require(t, !(got != displaytime.DefaultTimeZone), "missing settings time zone = %q", got)
	}
	{

		err := accountdata.UpsertUserSettings(ctx, db, 1, &schema.UserSettings{ChartStyle: " WHITE ", TimeZone: "Asia/Tokyo"})
		testutil.RequireArgs(t, !(err != nil), err)
	}
	{

		got := resolveHarukiUserChartStyle(ctx, app, 1)
		testutil.Require(t, !(got != "white"), "stored chart style = %q", got)
	}
	{

		got := resolveHarukiUserTimeZone(ctx, app, 1)
		testutil.Require(t, !(got != "Asia/Tokyo"), "stored time zone = %q", got)
	}

	_ = resolveRequesterHarukiUserChartStyle(ctx, app, "qq", "1")
	_ = resolveRequesterHarukiUserTimeZone(ctx, app, "qq", "1")
}

func TestMySekaiHousingAndMessageBranches(t *testing.T) {
	{
		_, err := executeMysekaiHousingSK(nil, "jp")
		testutil.RequireArgs(t, !(err == nil), "nil housing runtime unexpectedly succeeded")
	}

	rc := &RequestContext{Ctx: context.Background(), Cmd: &CommandRequest{}, App: &renderapp.App{}}
	{
		_, err := executeMysekaiHousingSK(rc, "jp")
		testutil.RequireArgs(t, !(err == nil), "missing housing controller unexpectedly succeeded")
	}

	app, _ := newExecutionCoverageApp(t)
	rc.App = app
	rc.Cmd.Params = []byte(`{"region":""}`)
	{
		_, err := executeMysekaiHousingSK(rc, "tw")
		testutil.RequireArgs(t, !(err == nil), "generic local API unexpectedly returned housing data")
	}

	for mode, want := range map[string]string{
		"mysekai-talk-list":      "/烤森对话列表",
		"mysekai-fixture-detail": "/msf",
		"mysekai-map":            "/烤森地图",
		"mysekai-door-upgrade":   "/msg",
		"unknown":                "/mysekai",
	} {
		{
			got := canonicalMySekaiTrigger(mode)
			testutil.Check(t, !(got != want), "canonical trigger %q = %q", mode, got)
		}

	}
	replay := onebot11.NewReplayError("already normalized")
	{
		testutil.RequireArgs(t, !(normalizeMySekaiUserFacingError(nil, "") != nil), "nil/replay MySekai error changed")
		testutil.RequireArgs(t, !(normalizeMySekaiUserFacingError(replay, "") != replay), "nil/replay MySekai error changed")
	}

	messages := []string{
		"mysekai service unavailable",
		"mysekai talk list requires character query",
		"mysekai talk list invalid character query: Miku",
		"mysekai talk list invalid character query:",
		"mysekai map query contains no valid map ids",
		"mysekai map contains no harvest map data",
		"mysekai fixture detail invalid query: bad",
		"mysekai fixture detail found no valid fixtures",
		"mysekai fixture detail render requires exactly one fixture id",
		"mysekai fixture category not found: chair",
		"mysekai fixture category not found:",
		"mysekai fixture category not found",
		"mysekai resource requires profile data",
		"mysekai music record requires profile data",
		"sekaiapi profile fetch failed: client not configured",
		"sekaiapi profile build failed: invalid",
		"queried gate already max level",
		"decode mysekai data:",
		"decode mysekai data: invalid json",
	}
	for _, message := range messages {
		{
			got := normalizeMySekaiUserFacingError(errors.New(message), "mysekai-talk-list")
			testutil.Check(t, !(got == nil || got.Error() == ""), "normalize MySekai %q = %v", message, got)
		}

	}
}

func TestProfileBackgroundPureBranchCoverage(t *testing.T) {
	for _, tt := range []struct {
		args string
		want *bool
	}{
		{"plain", nil},
		{"横屏 plain", commandBoolPtr(false)},
		{"竖屏 plain", commandBoolPtr(true)},
	} {
		got, _ := extractProfileVerticalArg(tt.args)
		testutil.Check(t, !((got == nil) != (tt.want == nil) || got != nil && *got != *tt.want), "extractProfileVerticalArg(%q) = %v", tt.args, got)

	}
	contexts := []HarrukiSekaiHandlerContext{
		{PjskHandlerContext: PjskHandlerContext{Message: onebot11.Message{onebot11.Text("x")}}},
		{PjskHandlerContext: PjskHandlerContext{Message: onebot11.Message{{Type: "image", Data: map[string]string{"url": "x"}}}}},
		{PjskHandlerContext: PjskHandlerContext{Message: onebot11.Message{onebot11.Image("", " https://example.invalid/a.png ")}}},
		{PjskHandlerContext: PjskHandlerContext{Message: onebot11.Message{onebot11.Image("https://example.invalid/b.png", "")}}},
		{PjskHandlerContext: PjskHandlerContext{Message: onebot11.Message{onebot11.Image("local.png", "")}}},
	}
	for _, ctx := range contexts {
		_ = extractFirstImageURL(ctx)
	}
	for _, args := range []string{
		"", "横屏", "模糊", "透明", "模糊 x", "透明 x", "模糊11", "透明101", "模糊5 透明80", "blur 2 alpha 20", "unknown",
	} {
		_, _ = parseProfileBGAdjustArgs(args)
	}
	for _, raw := range []string{"", "x", "0", "10", "11"} {
		_, _ = parseProfileBGInt(raw, 0, 10)
	}
	ctx := HarrukiSekaiHandlerContext{PjskHandlerContext: PjskHandlerContext{Context: context.Background()}, originalTriggerCmd: "/调整"}
	{
		selector, err := resolveProfileBGSelector(ctx)
		{
			testutil.Require(t, !(err != nil), "default BG selector = %q, %v", selector, err)
			testutil.Require(t, !(selector != ""), "default BG selector = %q, %v", selector, err)
		}
	}

	ctx.uidArg = "u2"
	{
		selector, err := resolveProfileBGSelector(ctx)
		{
			testutil.Require(t, !(err != nil), "indexed BG selector = %q, %v", selector, err)
			testutil.Require(t, !(selector != "u2"), "indexed BG selector = %q, %v", selector, err)
		}
	}

	ctx.uidArg = "@2"
	{
		_, err := resolveProfileBGSelector(ctx)
		testutil.RequireArgs(t, !(err == nil), "foreign BG selector unexpectedly accepted")
	}

	for _, handler := range []HarukiSekaiCommandHandler{
		(sekaiHandlers{}).ProfileUploadBGHandle(),
		(sekaiHandlers{}).ProfileClearBGHandle(),
		(sekaiHandlers{}).ProfileAdjustBGHandle(),
	} {
		ctx.uidArg = ""
		ctx.ArgText = ""
		if strings.Contains(handler.Path, "adjust") {
			ctx.ArgText = "模糊5"
		}
		request, err := handler.handleFunc(ctx)
		if strings.Contains(handler.Path, "upload") {
			testutil.CheckArgs(t, !(err == nil), "BG upload without image unexpectedly succeeded")

			ctx.Message = onebot11.Message{onebot11.Image("", "https://example.invalid/bg.png")}
			request, err = handler.handleFunc(ctx)
		}
		testutil.Check(t, !(err != nil || request == nil), "BG handler %s = %+v, %v", handler.Path, request, err)

	}
}

func TestCommonAndCostumeMessageBranchCoverage(t *testing.T) {
	{
		testutil.RequireArgs(t, !(bindingServiceUnavailableMessage() == ""), "common messages are empty")
		testutil.RequireArgs(t, !(newMySekaiDataNotFoundReplayError() == nil), "common messages are empty")
	}

	bindings := []*accountdata.ResolvedBinding{
		nil,
		{},
		{Server: "jp"},
		{PJSKUserID: "1234567890", Visible: true},
		{Server: "jp", PJSKUserID: "1234567890", Visible: false, SuiteVisible: false, MySekaiVisible: false},
	}
	for _, binding := range bindings {
		_ = newSuiteDataNotFoundReplayErrorForBinding(binding)
		_ = newMySekaiDataNotFoundReplayErrorForBinding(binding)
		_ = buildPrivateDataHiddenMessage("", binding)
		_ = buildPrivateDataHiddenMessage("mysekai", binding)
		_ = buildPrivateDataNotFoundMessage("", binding)
		_ = buildToolboxAccessDeniedMessage("suite", binding)
		_ = formatUserFacingBindingAccount(binding)
	}
	for _, err := range []error{
		sekaiapi.ErrAccountBindingNotFound,
		sekaiapi.ErrGameDataNotFound,
		sekaiapi.ErrInvalidPlatformUser,
		sekaiapi.ErrAccountOwnerBanned,
		errors.New("toolbox: request failed after retries"),
		errors.New("toolbox: zstd reader init failed"),
		&sekaiapi.ToolboxAPIError{StatusCode: 503},
		&sekaiapi.ToolboxAPIError{StatusCode: 403},
		&sekaiapi.ToolboxAPIError{StatusCode: 404},
		&sekaiapi.ToolboxAPIError{StatusCode: 500},
	} {
		_ = normalizeToolboxDataFetchError(err, "mysekai", bindings[len(bindings)-1])
	}
	for _, err := range []error{
		sekaiapi.ErrClientNotConfigured,
		sekaiapi.ErrServerMaintenance,
		sekaiapi.ErrUserNotFound,
		errors.New("sekai api: request failed after retries"),
		&sekaiapi.APIError{StatusCode: 500},
		&sekaiapi.APIError{StatusCode: 500, Message: "unknown"},
		errors.New("sekaiapi unknown failure"),
		errors.New("plain failure"),
	} {
		_ = normalizeSekaiAPIFetchError(err)
		_ = WrapDomainError(err)
	}
	for _, err := range []error{
		accountdata.ErrNoBinding,
		accountdata.ErrBindingServiceUnavailable,
		errors.New("local user snapshot is not configured"),
		errors.New("找不到用户的 MySekai 数据"),
		errors.New("drawing client is not configured"),
	} {
		_ = WrapDomainError(err)
	}
	{
		testutil.RequireArgs(t, !(stringPtr(" ") != nil), "stringPtr trimming mismatch")
		testutil.RequireArgs(t, !(stringPtr(" x ") == nil), "stringPtr trimming mismatch")
	}

	costumeMessages := []string{
		"3d preview service is not configured",
		"3d combo accessory legacy id",
		"3d combo anchor part not found",
		"3d preview part is missing runtime package",
		"3d preview part not found",
		"3d preview default role not found",
		"tuple incomplete",
		"3d preview capture fetch failed",
		"3d preview capture failed",
		"3d preview unknown",
		"plain error",
	}
	for _, message := range costumeMessages {
		_ = normalizeCostume3DError(errors.New(message))
	}
	{
		testutil.RequireArgs(t, !(trailingCostume3DID("bad") != 0), "costume ID fallback mismatch")
		testutil.RequireArgs(t, !(bracketedCostumeIDs("missing") != ""), "costume ID fallback mismatch")
	}

}

func TestReleaseMessageBranchCoverage(t *testing.T) {
	replay := onebot11.NewReplayError("existing")
	for _, normalize := range []func(error) error{normalizeCardUserFacingError, normalizeMusicUserFacingError, normalizeEventUserFacingError, normalizeGachaUserFacingError} {
		{
			testutil.RequireArgs(t, !(normalize(nil) != nil), "release normalizer changed nil/replay")
			testutil.RequireArgs(t, !(normalize(replay) != replay), "release normalizer changed nil/replay")
		}

	}
	for _, err := range []error{
		&releasecheck.UnreleasedError{Kind: releasecheck.KindCard, ID: 1},
		errors.New("card ids are required"), errors.New("no released cards found"), errors.New("card service unavailable"), errors.New("does not have original image assets"),
		errors.New("card not found"), errors.New("query card 123 failed"), errors.New("card sequence out of range"),
	} {
		_ = normalizeCardUserFacingErrorForLookup(err, "cn", "fallback")
	}
	for _, err := range []error{
		&releasecheck.UnreleasedError{Kind: releasecheck.KindMusic, ID: 1},
		errors.New("当前服务器暂未支持自定义谱面"), errors.New("music controller is not configured"), errors.New("music ids are required"), errors.New("no music matched the current filters"), errors.New("does not have jacket asset"), errors.New("does not have master chart"),
		errors.New("music 123 not found"), errors.New("wrapped music not found query music 456"), errors.New("ban event index out of range"), errors.New("no ban events found for character"),
	} {
		_ = normalizeMusicUserFacingErrorForLookup(err, "tw", "fallback")
	}
	for _, err := range []error{
		&releasecheck.UnreleasedError{Kind: releasecheck.KindEvent, ID: 1},
		errors.New("event id is required"), errors.New("no events matched filters"), errors.New("event service unavailable"), errors.New("event record requires profile"), errors.New("event not found"),
	} {
		_ = normalizeEventUserFacingErrorForRegion(err, "en")
	}
	for _, err := range []error{
		&releasecheck.UnreleasedError{Kind: releasecheck.KindGacha, ID: 1},
		errors.New("gacha id is required"), errors.New("gacha not found"), errors.New("gacha service unavailable"),
	} {
		_ = normalizeGachaUserFacingError(err)
	}
	for _, message := range []string{"", "query card 123", "card not found (filter): Miku", "card not found", "plain"} {
		_, _ = extractCardNotFoundQuery(errors.New(message))
	}
	for _, message := range []string{"", "music not found", "music not found: song", "music 99 not found", "query music 123 failed", "plain"} {
		_, _ = extractMusicNotFoundQuery(errors.New(message), "fallback")
	}
	_ = cardNotFoundQueryOrFallback("")
	_ = musicNotFoundQueryOrFallback("empty query", "")
	_ = extractQueryMusicIDFromError("plain")
	_ = readNumberAfter("abc")
	_ = isEventDataNotFoundMessage("")
}
