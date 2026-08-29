package accountdata_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	pjskenttest "haruki-cloud/database/pjsk/enttest"
	"haruki-cloud/ent/pjsk/schema"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/pjsk/accountdata"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"

	_ "github.com/mattn/go-sqlite3"
)

func TestDecodeProfileSettingsParamsValidationAndNormalization(t *testing.T) {
	for _, raw := range []json.RawMessage{
		nil,
		json.RawMessage(`{`),
		json.RawMessage(`{"platform":"","platform_user_id":"42","server":"jp"}`),
		json.RawMessage(`{"platform":"qq","platform_user_id":"42","server":""}`),
	} {
		if _, err := accountdata.DecodeProfileSettingsParams(raw); err == nil {
			t.Fatalf("invalid params %s were accepted", raw)
		}
	}

	params, err := accountdata.DecodeProfileSettingsParams(json.RawMessage(`{
		"platform":" qq ",
		"platform_user_id":" 42 ",
		"server":" JP ",
		"time_zone":" Asia/Shanghai ",
		"chart_style":" BLACK ",
		"image_url":" https://example.com/bg.png ",
		"difficulty_toggles":[{"difficulty":" MASTER ","enabled":true},{"difficulty":"unknown","enabled":false}]
	}`))
	if err != nil {
		t.Fatalf("decode normalized params: %v", err)
	}
	if params.Platform != "qq" || params.PlatformUserID != "42" || params.Server != "jp" || params.TimeZone != "Asia/Shanghai" || params.ChartStyle != "black" || params.ImageURL != "https://example.com/bg.png" {
		t.Fatalf("normalized params = %#v", params)
	}
	if params.DifficultyToggles[0].Difficulty != sekaiapi.MusicDifficultyMaster || params.DifficultyToggles[1].Difficulty != "" {
		t.Fatalf("normalized difficulties = %#v", params.DifficultyToggles)
	}
}

func TestExecuteProfileSettingsVisibilityAndModularModes(t *testing.T) {
	service := newProfileBindingTestService(t, map[string]map[string]string{
		"jp": {"12345678901234": "JP User"},
	})
	ctx := context.Background()
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	params := accountdata.ProfileSettingsCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Server:         "jp",
		RegionExplicit: true,
	}

	for _, mode := range []string{
		accountdata.ProfileModeHideID,
		accountdata.ProfileModeShowID,
		accountdata.ProfileModeHideSuite,
		accountdata.ProfileModeShowSuite,
		accountdata.ProfileModeHideMySekai,
		accountdata.ProfileModeShowMySekai,
		accountdata.ProfileModeEnableModular,
		accountdata.ProfileModeDisableModular,
	} {
		text, err := accountdata.ExecuteProfileSettingsCommand(ctx, service, mode, params)
		if err != nil || len(text) == 0 {
			t.Fatalf("execute %s = %q, %v", mode, text, err)
		}
	}

	settings, _, err := service.GetUserSettings(ctx, "qq", "42")
	if err != nil {
		t.Fatalf("get modular settings: %v", err)
	}
	if settings.ModularProfileEnabled {
		t.Fatal("modular profile remained enabled after disable command")
	}

	service.SetReadOnly(true)
	if _, err := accountdata.ExecuteProfileSettingsCommand(ctx, service, accountdata.ProfileModeHideID, params); err == nil {
		t.Fatal("read-only visibility mutation succeeded")
	}
	service.SetReadOnly(false)
	if _, err := accountdata.ExecuteProfileSettingsCommand(ctx, nil, accountdata.ProfileModeHideID, params); !errors.Is(err, accountdata.ErrBindingServiceUnavailable) {
		t.Fatalf("nil service error = %v", err)
	}
	if _, err := accountdata.ExecuteProfileSettingsCommand(ctx, service, "unknown-mode", params); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported mode error = %v", err)
	}
}

func TestExecuteProfileBackgroundCommandModes(t *testing.T) {
	service := newProfileBindingTestService(t, map[string]map[string]string{
		"jp": {"12345678901234": "JP User"},
	})
	service.SetFastVerificationProvider(fakeFastVerifier{
		bindings: []sekaiapi.UserGameBinding{{Server: "jp", GameUserID: "12345678901234"}},
	})
	store := &fakeProfileBGStore{}
	service.SetProfileBGStorage(store)
	ctx := context.Background()
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if _, _, err := service.VerifyCurrentBinding(ctx, "qq", "42", "jp"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	params := accountdata.ProfileSettingsCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Server:         "jp",
		RegionExplicit: true,
		ImageURL:       "https://example.com/bg.png",
	}

	if text, err := accountdata.ExecuteProfileSettingsCommand(ctx, service, accountdata.ProfileModeBGUpload, params); err != nil || !strings.Contains(string(text), "已更新JP服") {
		t.Fatalf("upload command = %q, %v", text, err)
	}
	params.ImageURL = ""
	if text, err := accountdata.ExecuteProfileSettingsCommand(ctx, service, accountdata.ProfileModeBGAdjust, params); err != nil || !strings.Contains(string(text), "模糊度") {
		t.Fatalf("inspect command = %q, %v", text, err)
	}
	blur, alpha, vertical := 8, 60, true
	params.Blur, params.Alpha, params.Vertical = &blur, &alpha, &vertical
	if text, err := accountdata.ExecuteProfileSettingsCommand(ctx, service, accountdata.ProfileModeBGAdjust, params); err != nil || !strings.Contains(string(text), "已更新JP服") {
		t.Fatalf("adjust command = %q, %v", text, err)
	}
	if text, err := accountdata.ExecuteProfileSettingsCommand(ctx, service, accountdata.ProfileModeBGClear, params); err != nil || !strings.Contains(string(text), "已清空JP服") {
		t.Fatalf("clear command = %q, %v", text, err)
	}
	params.Blur, params.Alpha, params.Vertical = nil, nil, nil
	if text, err := accountdata.ExecuteProfileSettingsCommand(ctx, service, accountdata.ProfileModeBGAdjust, params); err != nil || !strings.Contains(string(text), "还没有自定义") {
		t.Fatalf("empty inspect command = %q, %v", text, err)
	}
}

func TestUserSettingsStorageAndCounters(t *testing.T) {
	ctx := context.Background()
	service := newProfileBindingTestService(t, map[string]map[string]string{})
	db := pjskenttest.Open(t, "sqlite3", fmt.Sprintf("file:user_settings_additional_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = db.Close() })

	if _, err := accountdata.GetUserSettings(ctx, nil, 1); err == nil {
		t.Fatal("nil database GetUserSettings succeeded")
	}
	if _, err := accountdata.GetUserSettings(ctx, db, 0); err == nil {
		t.Fatal("invalid user id GetUserSettings succeeded")
	}
	if err := accountdata.UpsertUserSettings(ctx, nil, 1, &schema.UserSettings{}); err == nil {
		t.Fatal("nil database UpsertUserSettings succeeded")
	}
	if err := accountdata.UpsertUserSettings(ctx, db, 0, &schema.UserSettings{}); err == nil {
		t.Fatal("invalid user id UpsertUserSettings succeeded")
	}
	if err := accountdata.UpsertUserSettings(ctx, db, 1, nil); err != nil {
		t.Fatalf("nil settings upsert: %v", err)
	}

	settings := &schema.UserSettings{ChartStyle: "black", NoncompliantBGCount: 2}
	userID, err := service.UpsertUserSettings(ctx, "qq", "settings-user", settings)
	if err != nil {
		t.Fatalf("create user settings: %v", err)
	}
	got, gotUserID, err := service.GetUserSettings(ctx, "qq", "settings-user")
	if err != nil || gotUserID != userID || got.ChartStyle != "black" || got.NoncompliantBGCount != 2 {
		t.Fatalf("get user settings = %#v, user=%d, err=%v", got, gotUserID, err)
	}
	settings.ChartStyle = "white"
	if _, err := service.UpsertUserSettings(ctx, "qq", "settings-user", settings); err != nil {
		t.Fatalf("update user settings: %v", err)
	}
	if err := accountdata.UpsertUserSettings(ctx, db, userID, &schema.UserSettings{NoncompliantBGCount: 2}); err != nil {
		t.Fatalf("seed direct user settings: %v", err)
	}
	count, err := accountdata.IncrNoncompliantBGCount(ctx, db, userID)
	if err != nil || count != 3 {
		t.Fatalf("increment noncompliant count = %d, %v", count, err)
	}
	if _, err := accountdata.IncrNoncompliantBGCount(ctx, nil, userID); err == nil {
		t.Fatal("nil database counter increment succeeded")
	}
	if _, err := accountdata.IncrNoncompliantBGCount(ctx, db, 0); err == nil {
		t.Fatal("invalid user counter increment succeeded")
	}
}
