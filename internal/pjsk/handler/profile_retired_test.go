package handler

import (
	"context"
	"strings"
	"testing"

	"haruki-cloud/ent/pjsk/schema"
	corehandler "haruki-cloud/internal/handler"
	json "haruki-cloud/internal/jsonutil"
)

func TestModularProfileCommandsAreNotRegistered(t *testing.T) {
	EnsureCommandHandlersRegistered()
	for _, route := range corehandler.ListBotRoutes() {
		if strings.HasPrefix(route.Path, "profile/modular/") {
			t.Fatalf("retired route registered: %s", route.Path)
		}
	}
	for _, command := range []string{"/开启模块个人信息", "/开启模块化个人信息", "/开启模块化资料", "/pjsk modular profile on", "/关闭模块个人信息", "/关闭模块化个人信息", "/关闭模块化资料", "/pjsk modular profile off", "/jp开启模块化个人信息"} {
		request, err := dispatchForTest(context.Background(), helpExampleEvent(command))
		if err == nil && request != nil {
			t.Errorf("retired command still dispatched: %q", command)
		}
	}
}

func TestLegacyModularPreferenceIsIgnored(t *testing.T) {
	var settings schema.UserSettings
	if err := json.Unmarshal([]byte(`{"modular_profile_enabled":true,"time_zone":"UTC","chart_style":"white"}`), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.TimeZone != "UTC" || settings.ChartStyle != "white" {
		t.Fatalf("unrelated preferences lost: %+v", settings)
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "modular_profile") {
		t.Fatalf("retired preference persisted: %s", encoded)
	}
}
