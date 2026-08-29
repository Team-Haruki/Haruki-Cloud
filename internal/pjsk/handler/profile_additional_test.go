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
	if binding.Platform != "qq" || binding.PlatformUserID != "10001" || binding.Selector != "u2" || binding.Server != "jp" || binding.Scope != "global" {
		t.Fatalf("binding params = %+v", binding)
	}
	settings := newProfileSettingsParams(ctx, "u3")
	if settings.Platform != "qq" || settings.Selector != "u3" || settings.Server != "jp" || !settings.RegionExplicit {
		t.Fatalf("settings params = %+v", settings)
	}
	settings = newProfileSettingsParams(ctx, "")
	if settings.Selector != "" {
		t.Fatalf("empty selector retained: %+v", settings)
	}

	if selector, err := resolveSettingsSelector(ctx); err != nil || selector != "" {
		t.Fatalf("empty selector = %q, %v", selector, err)
	}
	ctx.uidArg = "u4"
	if selector, err := resolveSettingsSelector(ctx); err != nil || selector != "u4" {
		t.Fatalf("valid selector = %q, %v", selector, err)
	}
	ctx.uidArg = "@2"
	if _, err := resolveSettingsSelector(ctx); err == nil {
		t.Fatal("expected invalid target selector error")
	}
	ctx = additionalProfileContext("unexpected", "")
	if _, err := resolveSettingsSelector(ctx); err == nil {
		t.Fatal("expected extra argument selector error")
	}

	commands := profileTimeZoneCommands()
	if len(commands) <= len(profileTimeZoneBaseCommands) {
		t.Fatalf("timezone command aliases missing: %v", commands)
	}
	ctx = additionalProfileContext(" Asia/Shanghai ", "")
	if got := extractProfileTimeZoneArg(ctx); got != "Asia/Shanghai" {
		t.Fatalf("timezone arg = %q", got)
	}
	ctx = additionalProfileContext("", "")
	ctx.TriggerCmd = "/unrelated"
	if got := extractProfileTimeZoneArg(ctx); got != "" {
		t.Fatalf("unrelated timezone trigger = %q", got)
	}
	ctx.TriggerCmd = "/pjsktz"
	if got := extractProfileTimeZoneArg(ctx); got != "" {
		t.Fatalf("bare timezone trigger = %q", got)
	}
	ctx.TriggerCmd = "/pjsktzUTC"
	if got := extractProfileTimeZoneArg(ctx); got != "UTC" {
		t.Fatalf("compact timezone trigger = %q", got)
	}

	difficulties := map[string]sekaiapi.MusicDifficultyType{
		"easy": sekaiapi.MusicDifficultyEasy, "normal": sekaiapi.MusicDifficultyNormal,
		"hard": sekaiapi.MusicDifficultyHard, "HD": sekaiapi.MusicDifficultyHard,
		"expert": sekaiapi.MusicDifficultyExpert, "ex": sekaiapi.MusicDifficultyExpert,
		"master": sekaiapi.MusicDifficultyMaster, "append": sekaiapi.MusicDifficultyAppend,
		"apd": sekaiapi.MusicDifficultyAppend,
	}
	for raw, want := range difficulties {
		if got := parseProfileDifficultyToken(raw); got != want {
			t.Errorf("parseProfileDifficultyToken(%q) = %q", raw, got)
		}
	}
	if parseProfileDifficultyToken("bad") != "" {
		t.Fatal("invalid difficulty parsed")
	}
	for _, raw := range []string{"开启", "开", "on", "enable", "enabled", "true", "1"} {
		if enabled, ok := parseProfileDifficultyState(raw); !ok || !enabled {
			t.Errorf("enabled state %q = %v %v", raw, enabled, ok)
		}
	}
	for _, raw := range []string{"关闭", "关", "off", "disable", "disabled", "false", "0"} {
		if enabled, ok := parseProfileDifficultyState(raw); !ok || enabled {
			t.Errorf("disabled state %q = %v %v", raw, enabled, ok)
		}
	}
	if _, ok := parseProfileDifficultyState("bad"); ok {
		t.Fatal("invalid state parsed")
	}
	if _, ok := parseProfileDifficultyCompactToggle(""); ok {
		t.Fatal("empty compact toggle parsed")
	}
	if _, ok := parseProfileDifficultyCompactToggle("bad开启"); ok {
		t.Fatal("invalid compact difficulty parsed")
	}
	if _, ok := parseProfileDifficultyCompactToggle("master"); ok {
		t.Fatal("missing compact state parsed")
	}
	if toggle, ok := parseProfileDifficultyCompactToggle("master关"); !ok || toggle.Difficulty != sekaiapi.MusicDifficultyMaster || toggle.Enabled {
		t.Fatalf("compact toggle = %+v %v", toggle, ok)
	}

	for _, raw := range []string{"", "master", "bad on", "master maybe"} {
		if _, err := parseProfileDifficultyToggles(raw); err == nil {
			t.Errorf("parseProfileDifficultyToggles(%q) unexpectedly succeeded", raw)
		}
	}
	toggles, err := parseProfileDifficultyToggles("master 开启，expert关闭\nhard on")
	if err != nil || len(toggles) != 3 || !toggles[0].Enabled || toggles[1].Enabled || !toggles[2].Enabled {
		t.Fatalf("difficulty toggles = %+v, %v", toggles, err)
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
		if _, err := handler.handleFunc(ctx); err == nil {
			t.Errorf("handler %s accepted invalid selector", handler.Path)
		}
	}

	ctx = additionalProfileContext("unexpected", "")
	for _, handler := range []HarukiSekaiCommandHandler{
		sekaiHandlers{}.ProfileEnableModularHandle(),
		sekaiHandlers{}.ProfileDisableModularHandle(),
		sekaiHandlers{}.ProfileVerifyListHandle(),
	} {
		if _, err := handler.handleFunc(ctx); err == nil {
			t.Errorf("handler %s accepted extra arguments", handler.Path)
		}
	}
	if _, err := (sekaiHandlers{}).ProfileArrestDifficultyHandle().handleFunc(ctx); err == nil {
		t.Fatal("arrest difficulty accepted invalid arguments")
	}

	ctx = additionalProfileContext("", "@2")
	if _, err := (sekaiHandlers{}).ProfileCheckDataHandle().handleFunc(ctx); err == nil {
		t.Fatal("suite data handler accepted another user")
	}
	if _, err := (sekaiHandlers{}).MsdHandle().handleFunc(ctx); err == nil {
		t.Fatal("MySekai data handler accepted another user")
	}
}

func TestProfileBindingHandlerErrorBranches(t *testing.T) {
	ctx := additionalProfileContext("", "")
	if _, err := (sekaiHandlers{}).ProfileBindHandle().handleFunc(ctx); err == nil {
		t.Fatal("bind accepted empty arguments")
	}
	if request, handled, err := tryRerouteProfileBindCommand(ctx, ""); request != nil || handled || err != nil {
		t.Fatalf("empty bind reroute = %+v %v %v", request, handled, err)
	}
	if _, handled, err := tryRerouteProfileBindCommand(ctx, "list extra"); !handled || err == nil {
		t.Fatalf("invalid list reroute = %v %v", handled, err)
	}
	if _, handled, err := tryRerouteProfileBindCommand(ctx, "swap u1"); !handled || err == nil {
		t.Fatalf("invalid swap reroute = %v %v", handled, err)
	}
	if request, handled, err := tryRerouteProfileBindCommand(ctx, "123456"); request != nil || handled || err != nil {
		t.Fatalf("normal bind reroute = %+v %v %v", request, handled, err)
	}

	for _, tt := range []struct {
		mode string
		want string
	}{{"list", "/jp绑定列表"}, {"swap", "/jp绑定交换"}, {"other", "/jp测试"}} {
		if got := buildProfileBindDerivedTrigger(ctx, tt.mode); got != tt.want {
			t.Errorf("derived trigger %q = %q", tt.mode, got)
		}
	}
	ctx.explicitRegion = false
	if buildProfileBindDerivedTrigger(ctx, "list") != "/绑定列表" || buildProfileBindDerivedTrigger(ctx, "swap") != "/绑定交换" {
		t.Fatal("implicit derived trigger mismatch")
	}

	ctx = additionalProfileContext("extra", "")
	if _, err := (sekaiHandlers{}).ProfileBindListHandle().handleFunc(ctx); err == nil {
		t.Fatal("bind list accepted arguments")
	}
	ctx = additionalProfileContext("u1", "")
	if _, err := (sekaiHandlers{}).ProfileBindSwapHandle().handleFunc(ctx); err == nil {
		t.Fatal("bind swap accepted one selector")
	}
	ctx = additionalProfileContext("", "")
	if _, err := (sekaiHandlers{}).ProfileUnbindHandle().handleFunc(ctx); err == nil {
		t.Fatal("unbind accepted empty target")
	}
	if _, err := (sekaiHandlers{}).ProfileSetMainHandle().handleFunc(ctx); err == nil {
		t.Fatal("set main accepted empty target")
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
		if _, err := executeProfile(rc); !errors.Is(err, accountdata.ErrBindingServiceUnavailable) {
			t.Errorf("executeProfile(%q) error = %v", mode, err)
		}
	}
	rc.Cmd.Mode = accountdata.ProfileModeRender
	if _, err := executeProfile(rc); err == nil || !strings.Contains(err.Error(), "profile service unavailable") {
		t.Fatalf("profile render guard error = %v", err)
	}
	rc.Cmd.Mode = "unknown"
	if _, err := executeProfile(rc); err == nil || !strings.Contains(err.Error(), "unsupported profile mode") {
		t.Fatalf("unsupported profile error = %v", err)
	}
	if _, _, err := renderProfileMessageForQuery(nil, userQueryParams{}, "jp", false); err == nil {
		t.Fatal("nil profile context unexpectedly rendered")
	}
	if _, _, err := renderProfileMessageForQuery(rc, userQueryParams{}, "jp", false); err == nil {
		t.Fatal("missing profile services unexpectedly rendered")
	}
	if isRequesterModularProfileEnabled(nil, userQueryParams{}) || isRequesterModularProfileEnabled(rc, userQueryParams{}) {
		t.Fatal("missing binding service reported modular profile")
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
		if _, err := executeCheckData(rc); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("executeCheckData(%q) error = %v", tt.mode, err)
		}
	}
	for _, mode := range []string{"mysekai", "suite"} {
		params, _ := json.Marshal(userQueryParams{Mode: "self", Platform: "qq", PlatformUserID: "1"})
		rc.Cmd.Mode = mode
		rc.Cmd.Params = params
		if _, err := executeCheckData(rc); err == nil {
			t.Errorf("executeCheckData(%q) unexpectedly resolved without bindings", mode)
		}
	}
}

func TestProfileDifficultyToggleValueSemantics(t *testing.T) {
	toggles, err := parseProfileDifficultyToggles("easy on normal off")
	if err != nil {
		t.Fatalf("parse toggles: %v", err)
	}
	want := []accountdata.ProfileDifficultyToggle{
		{Difficulty: sekaiapi.MusicDifficultyEasy, Enabled: true},
		{Difficulty: sekaiapi.MusicDifficultyNormal, Enabled: false},
	}
	if !reflect.DeepEqual(toggles, want) {
		t.Fatalf("toggles = %+v, want %+v", toggles, want)
	}
}
