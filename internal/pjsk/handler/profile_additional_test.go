package handler

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/accountdata"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/internal/testutil"
)

func additionalProfileContext(args, uidArg string) HarrukiSekaiHandlerContext {
	return HarrukiSekaiHandlerContext{
		PjskHandlerContext: PjskHandlerContext{
			Context:    context.Background(),
			Platform:   "qq",
			UserId:     "10001",
			TriggerCmd: "/jp测试",
			ArgText:    args,
		},
		region:             renderregion.JP,
		explicitRegion:     true,
		originalTriggerCmd: "/jp测试",
		uidArg:             uidArg,
	}
}

func TestProfileParameterAndDifficultyHelpers(t *testing.T) {
	ctx := additionalProfileContext("", "")
	binding := newProfileBindingParams(ctx, " u2 ", " global ")
	{
		testutil.Require(t, !(binding.Platform != "qq"), "binding params = %+v", binding)
		testutil.Require(t, !(binding.PlatformUserID != "10001"), "binding params = %+v", binding)
		testutil.Require(t, !(binding.Selector != "u2"), "binding params = %+v", binding)
		testutil.Require(t, !(binding.Server != "jp"), "binding params = %+v", binding)
		testutil.Require(t, !(binding.Scope != "global"), "binding params = %+v", binding)
	}

	settings := newProfileSettingsParams(ctx, "u3")
	{
		testutil.Require(t, !(settings.Platform != "qq"), "settings params = %+v", settings)
		testutil.Require(t, !(settings.Selector != "u3"), "settings params = %+v", settings)
		testutil.Require(t, !(settings.Server != "jp"), "settings params = %+v", settings)
		testutil.Require(t, settings.RegionExplicit, "settings params = %+v", settings)
	}

	settings = newProfileSettingsParams(ctx, "")
	testutil.Require(t, !(settings.Selector != ""), "empty selector retained: %+v", settings)
	{

		selector, err := resolveSettingsSelector(ctx)
		{
			testutil.Require(t, !(err != nil), "empty selector = %q, %v", selector, err)
			testutil.Require(t, !(selector != ""), "empty selector = %q, %v", selector, err)
		}
	}

	ctx.uidArg = "u4"
	{
		selector, err := resolveSettingsSelector(ctx)
		{
			testutil.Require(t, !(err != nil), "valid selector = %q, %v", selector, err)
			testutil.Require(t, !(selector != "u4"), "valid selector = %q, %v", selector, err)
		}
	}

	ctx.uidArg = "@2"
	{
		_, err := resolveSettingsSelector(ctx)
		testutil.RequireArgs(t, !(err == nil), "expected invalid target selector error")
	}

	ctx = additionalProfileContext("unexpected", "")
	{
		_, err := resolveSettingsSelector(ctx)
		testutil.RequireArgs(t, !(err == nil), "expected extra argument selector error")
	}

	commands := profileTimeZoneCommands()
	testutil.Require(t, !(len(commands) <= len(profileTimeZoneBaseCommands)), "timezone command aliases missing: %v", commands)

	ctx = additionalProfileContext(" Asia/Shanghai ", "")
	{
		got := extractProfileTimeZoneArg(ctx)
		testutil.Require(t, !(got != "Asia/Shanghai"), "timezone arg = %q", got)
	}

	ctx = additionalProfileContext("", "")
	ctx.TriggerCmd = "/unrelated"
	{
		got := extractProfileTimeZoneArg(ctx)
		testutil.Require(t, !(got != ""), "unrelated timezone trigger = %q", got)
	}

	ctx.TriggerCmd = "/pjsktz"
	{
		got := extractProfileTimeZoneArg(ctx)
		testutil.Require(t, !(got != ""), "bare timezone trigger = %q", got)
	}

	ctx.TriggerCmd = "/pjsktzUTC"
	{
		got := extractProfileTimeZoneArg(ctx)
		testutil.Require(t, !(got != "UTC"), "compact timezone trigger = %q", got)
	}

	difficulties := map[string]sekaiapi.MusicDifficultyType{
		"easy": sekaiapi.MusicDifficultyEasy, "normal": sekaiapi.MusicDifficultyNormal,
		"hard": sekaiapi.MusicDifficultyHard, "HD": sekaiapi.MusicDifficultyHard,
		"expert": sekaiapi.MusicDifficultyExpert, "ex": sekaiapi.MusicDifficultyExpert,
		"master": sekaiapi.MusicDifficultyMaster, "append": sekaiapi.MusicDifficultyAppend,
		"apd": sekaiapi.MusicDifficultyAppend,
	}
	for raw, want := range difficulties {
		{
			got := parseProfileDifficultyToken(raw)
			testutil.Check(t, !(got != want), "parseProfileDifficultyToken(%q) = %q", raw, got)
		}

	}
	testutil.RequireArgs(t, !(parseProfileDifficultyToken("bad") != ""), "invalid difficulty parsed")

	for _, raw := range []string{"开启", "开", "on", "enable", "enabled", "true", "1"} {
		{
			enabled, ok := parseProfileDifficultyState(raw)
			testutil.Check(t, !(!ok || !enabled), "enabled state %q = %v %v", raw, enabled, ok)
		}

	}
	for _, raw := range []string{"关闭", "关", "off", "disable", "disabled", "false", "0"} {
		{
			enabled, ok := parseProfileDifficultyState(raw)
			testutil.Check(t, !(!ok || enabled), "disabled state %q = %v %v", raw, enabled, ok)
		}

	}
	{
		_, ok := parseProfileDifficultyState("bad")
		testutil.RequireArgs(t, !(ok), "invalid state parsed")
	}
	{

		_, ok := parseProfileDifficultyCompactToggle("")
		testutil.RequireArgs(t, !(ok), "empty compact toggle parsed")
	}
	{

		_, ok := parseProfileDifficultyCompactToggle("bad开启")
		testutil.RequireArgs(t, !(ok), "invalid compact difficulty parsed")
	}
	{

		_, ok := parseProfileDifficultyCompactToggle("master")
		testutil.RequireArgs(t, !(ok), "missing compact state parsed")
	}
	{

		toggle, ok := parseProfileDifficultyCompactToggle("master关")
		{
			testutil.Require(t, ok, "compact toggle = %+v %v", toggle, ok)
			testutil.Require(t, !(toggle.Difficulty != sekaiapi.MusicDifficultyMaster), "compact toggle = %+v %v", toggle, ok)
			testutil.Require(t, !(toggle.Enabled), "compact toggle = %+v %v", toggle, ok)
		}
	}

	for _, raw := range []string{"", "master", "bad on", "master maybe"} {
		{
			_, err := parseProfileDifficultyToggles(raw)
			testutil.Check(t, !(err == nil), "parseProfileDifficultyToggles(%q) unexpectedly succeeded", raw)
		}

	}
	toggles, err := parseProfileDifficultyToggles("master 开启，expert关闭\nhard on")
	{
		testutil.Require(t, !(err != nil), "difficulty toggles = %+v, %v", toggles, err)
		testutil.Require(t, !(len(toggles) != 3), "difficulty toggles = %+v, %v", toggles, err)
		testutil.Require(t, toggles[0].Enabled, "difficulty toggles = %+v, %v", toggles, err)
		testutil.Require(t, !(toggles[1].Enabled), "difficulty toggles = %+v, %v", toggles, err)
		testutil.Require(t, toggles[2].Enabled, "difficulty toggles = %+v, %v", toggles, err)
	}

}

func TestProfileSettingsHandlersRejectInvalidSelectors(t *testing.T) {
	ctx := additionalProfileContext("", "@2")
	handlers := []HarukiSekaiCommandHandler{
		sekaiHandlers{}.ProfileHideSuiteHandle(),
		sekaiHandlers{}.ProfileShowSuiteHandle(),
		sekaiHandlers{}.ProfileHideMySekaiHandle(),
		sekaiHandlers{}.ProfileShowMySekaiHandle(),
		sekaiHandlers{}.ProfileHideIDHandle(),
		sekaiHandlers{}.ProfileShowIDHandle(),
		sekaiHandlers{}.ProfileVerifyHandle(),
	}
	for _, handler := range handlers {
		{
			_, err := handler.handleFunc(ctx)
			testutil.Check(t, !(err == nil), "handler %s accepted invalid selector", handler.Path)
		}

	}

	ctx = additionalProfileContext("unexpected", "")
	for _, handler := range []HarukiSekaiCommandHandler{
		sekaiHandlers{}.ProfileEnableModularHandle(),
		sekaiHandlers{}.ProfileDisableModularHandle(),
		sekaiHandlers{}.ProfileVerifyListHandle(),
	} {
		{
			_, err := handler.handleFunc(ctx)
			testutil.Check(t, !(err == nil), "handler %s accepted extra arguments", handler.Path)
		}

	}
	{
		_, err := (sekaiHandlers{}).ProfileArrestDifficultyHandle().handleFunc(ctx)
		testutil.RequireArgs(t, !(err == nil), "arrest difficulty accepted invalid arguments")
	}

	ctx = additionalProfileContext("", "@2")
	{
		_, err := (sekaiHandlers{}).ProfileCheckDataHandle().handleFunc(ctx)
		testutil.RequireArgs(t, !(err == nil), "suite data handler accepted another user")
	}
	{

		_, err := (sekaiHandlers{}).MsdHandle().handleFunc(ctx)
		testutil.RequireArgs(t, !(err == nil), "MySekai data handler accepted another user")
	}

}

func TestProfileBindingHandlerErrorBranches(t *testing.T) {
	ctx := additionalProfileContext("", "")
	{
		_, err := (sekaiHandlers{}).ProfileBindHandle().handleFunc(ctx)
		testutil.RequireArgs(t, !(err == nil), "bind accepted empty arguments")
	}
	{

		request, handled, err := tryRerouteProfileBindCommand(ctx, "")
		{
			testutil.Require(t, !(request != nil), "empty bind reroute = %+v %v %v", request, handled, err)
			testutil.Require(t, !(handled), "empty bind reroute = %+v %v %v", request, handled, err)
			testutil.Require(t, !(err != nil), "empty bind reroute = %+v %v %v", request, handled, err)
		}
	}
	{

		_, handled, err := tryRerouteProfileBindCommand(ctx, "list extra")
		{
			testutil.Require(t, handled, "invalid list reroute = %v %v", handled, err)
			testutil.Require(t, !(err == nil), "invalid list reroute = %v %v", handled, err)
		}
	}
	{

		_, handled, err := tryRerouteProfileBindCommand(ctx, "swap u1")
		{
			testutil.Require(t, handled, "invalid swap reroute = %v %v", handled, err)
			testutil.Require(t, !(err == nil), "invalid swap reroute = %v %v", handled, err)
		}
	}
	{

		request, handled, err := tryRerouteProfileBindCommand(ctx, "123456")
		{
			testutil.Require(t, !(request != nil), "normal bind reroute = %+v %v %v", request, handled, err)
			testutil.Require(t, !(handled), "normal bind reroute = %+v %v %v", request, handled, err)
			testutil.Require(t, !(err != nil), "normal bind reroute = %+v %v %v", request, handled, err)
		}
	}

	for _, tt := range []struct {
		mode string
		want string
	}{{"list", "/jp绑定列表"}, {"swap", "/jp绑定交换"}, {"other", "/jp测试"}} {
		{
			got := buildProfileBindDerivedTrigger(ctx, tt.mode)
			testutil.Check(t, !(got != tt.want), "derived trigger %q = %q", tt.mode, got)
		}

	}
	ctx.explicitRegion = false
	{
		testutil.RequireArgs(t, !(buildProfileBindDerivedTrigger(ctx, "list") != "/绑定列表"), "implicit derived trigger mismatch")
		testutil.RequireArgs(t, !(buildProfileBindDerivedTrigger(ctx, "swap") != "/绑定交换"), "implicit derived trigger mismatch")
	}

	ctx = additionalProfileContext("extra", "")
	{
		_, err := (sekaiHandlers{}).ProfileBindListHandle().handleFunc(ctx)
		testutil.RequireArgs(t, !(err == nil), "bind list accepted arguments")
	}

	ctx = additionalProfileContext("u1", "")
	{
		_, err := (sekaiHandlers{}).ProfileBindSwapHandle().handleFunc(ctx)
		testutil.RequireArgs(t, !(err == nil), "bind swap accepted one selector")
	}

	ctx = additionalProfileContext("", "")
	{
		_, err := (sekaiHandlers{}).ProfileUnbindHandle().handleFunc(ctx)
		testutil.RequireArgs(t, !(err == nil), "unbind accepted empty target")
	}
	{

		_, err := (sekaiHandlers{}).ProfileSetMainHandle().handleFunc(ctx)
		testutil.RequireArgs(t, !(err == nil), "set main accepted empty target")
	}

}

func TestProfileAndCheckDataExecutionGuards(t *testing.T) {
	ctx := context.Background()
	app := &renderapp.App{}
	rc := &RequestContext{Ctx: ctx, App: app, Cmd: &CommandRequest{Region: "jp"}}

	for _, mode := range []string{
		accountdata.ProfileModeBind,
		accountdata.ProfileModeHideID,
		accountdata.ProfileModeBGAdjust,
	} {
		rc.Cmd.Mode = mode
		{
			_, err := executeProfile(rc)
			testutil.Check(t, errors.Is(err, accountdata.ErrBindingServiceUnavailable), "executeProfile(%q) error = %v", mode, err)
		}

	}
	rc.Cmd.Mode = accountdata.ProfileModeRender
	{
		_, err := executeProfile(rc)
		{
			testutil.Require(t, !(err == nil), "profile render guard error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "profile service unavailable"), "profile render guard error = %v", err)
		}
	}

	rc.Cmd.Mode = "unknown"
	{
		_, err := executeProfile(rc)
		{
			testutil.Require(t, !(err == nil), "unsupported profile error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "unsupported profile mode"), "unsupported profile error = %v", err)
		}
	}
	{

		_, _, err := renderProfileMessageForQuery(nil, userQueryParams{}, "jp", false)
		testutil.RequireArgs(t, !(err == nil), "nil profile context unexpectedly rendered")
	}
	{

		_, _, err := renderProfileMessageForQuery(rc, userQueryParams{}, "jp", false)
		testutil.RequireArgs(t, !(err == nil), "missing profile services unexpectedly rendered")
	}
	{
		testutil.RequireArgs(t, !(isRequesterModularProfileEnabled(nil, userQueryParams{})), "missing binding service reported modular profile")
		testutil.RequireArgs(t, !(isRequesterModularProfileEnabled(rc, userQueryParams{})), "missing binding service reported modular profile")
	}

	for _, tt := range []struct {
		mode string
		p    userQueryParams
		want string
	}{
		{mode: "mysekai", p: userQueryParams{Mode: "uid"}, want: "MySekai抓包相关内容仅支持"},
		{mode: "suite", p: userQueryParams{Mode: "uid"}, want: "suite抓包相关内容仅支持"},
	} {
		params, _ := json.Marshal(tt.p)
		rc.Cmd.Mode = tt.mode
		rc.Cmd.Params = params
		{
			_, err := executeCheckData(rc)
			testutil.Check(t, !(err == nil || !strings.Contains(err.Error(), tt.want)), "executeCheckData(%q) error = %v", tt.mode, err)
		}

	}
	for _, mode := range []string{"mysekai", "suite"} {
		params, _ := json.Marshal(userQueryParams{Mode: "self", Platform: "qq", PlatformUserID: "1"})
		rc.Cmd.Mode = mode
		rc.Cmd.Params = params
		{
			_, err := executeCheckData(rc)
			testutil.Check(t, !(err == nil), "executeCheckData(%q) unexpectedly resolved without bindings", mode)
		}

	}
}

func TestProfileDifficultyToggleValueSemantics(t *testing.T) {
	toggles, err := parseProfileDifficultyToggles("easy on normal off")
	testutil.Require(t, !(err != nil), "parse toggles: %v", err)

	want := []accountdata.ProfileDifficultyToggle{
		{Difficulty: sekaiapi.MusicDifficultyEasy, Enabled: true},
		{Difficulty: sekaiapi.MusicDifficultyNormal, Enabled: false},
	}
	testutil.Require(t, reflect.DeepEqual(toggles, want), "toggles = %+v, want %+v", toggles, want)

}
