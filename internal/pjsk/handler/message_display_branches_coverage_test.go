package handler

import (
	"context"
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
