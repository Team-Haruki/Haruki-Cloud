package accountdata

import (
	"context"
	"errors"
	"strings"
	"testing"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

type accountCoverageFastVerifier struct {
	records []sekaiapi.UserGameBinding
	err     error
}

func (f accountCoverageFastVerifier) GetToolboxUserFastVerificationGameAccountBindings(string, string) ([]sekaiapi.UserGameBinding, error) {
	return f.records, f.err
}

type accountCoverageContextFastVerifier struct {
	accountCoverageFastVerifier
	called bool
}

func (f *accountCoverageContextFastVerifier) GetToolboxUserFastVerificationGameAccountBindingsContext(ctx context.Context, platform, platformUserID string) ([]sekaiapi.UserGameBinding, error) {
	f.called = true
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.records, f.err
}

type accountCoverageBGStorage struct {
	saveErr   error
	deleteErr error
}

func (s accountCoverageBGStorage) SaveProfileBackground(context.Context, string, string, string) (*drawing.ProfileBgSettings, error) {
	if s.saveErr != nil {
		return nil, s.saveErr
	}
	path := DefaultProfileBGRelativeDir + "/jp/coverage.jpg"
	return &drawing.ProfileBgSettings{ImgPath: &path, Blur: 1, Alpha: 50}, nil
}

func (s accountCoverageBGStorage) DeleteProfileBackground(context.Context, *drawing.ProfileBgSettings) error {
	return s.deleteErr
}

func TestProfileBindingCommandWrappersAndFormatBranches(t *testing.T) {
	testProfileBindingParamDecoding(t)
	testProfileBindingCommandExecution(t)
	testProfileBindingFormatting(t)
}

func testProfileBindingParamDecoding(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := DecodeProfileBindingParams(nil); err == nil {
		t.Fatal("missing binding params should fail")
	}
	if _, err := DecodeProfileBindingParams([]byte(`{`)); err == nil {
		t.Fatal("malformed binding params should fail")
	}
	if _, err := DecodeProfileBindingParams([]byte(`{"platform":" ","platform_user_id":"42"}`)); err == nil {
		t.Fatal("missing binding identity should fail")
	}
	params, err := DecodeProfileBindingParams([]byte(`{"platform":" qq ","platform_user_id":" 42 ","selector":" u1 ","selector_other":" u2 ","server":" JP ","scope":" default "}`))
	if err != nil || params.Platform != "qq" || params.PlatformUserID != "42" || params.Server != "jp" || params.SelectorOther != "u2" {
		t.Fatalf("decoded binding params = %+v, %v", params, err)
	}
	if _, err := ExecuteProfileBindingCommand(ctx, nil, ProfileModeBindList, params); !errors.Is(err, ErrBindingServiceUnavailable) {
		t.Fatalf("nil binding command service = %v", err)
	}
}

func testProfileBindingCommandExecution(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	service, _ := openAccountCoverageService(t, "binding_commands", accountCoverageValidator{profiles: map[string]string{"jp": "First"}})
	if _, err := service.Bind(ctx, "qq", "42", "7001"); err != nil {
		t.Fatalf("bind first command account: %v", err)
	}
	service.validator = accountCoverageValidator{profiles: map[string]string{"jp": "Second"}}
	if _, err := service.Bind(ctx, "qq", "42", "7002"); err != nil {
		t.Fatalf("bind second command account: %v", err)
	}
	base := ProfileBindingCommandParams{Platform: "qq", PlatformUserID: "42", Server: "jp"}
	set := base
	set.Selector = "u2"
	set.Scope = "default"
	if text, err := ExecuteProfileBindingCommand(ctx, service, ProfileModeDefaultSet, set); err != nil || !strings.Contains(string(text), "已设置") {
		t.Fatalf("default set command = %q, %v", text, err)
	}
	if text, err := ExecuteProfileBindingCommand(ctx, service, ProfileModeDefaultClear, set); err != nil || !strings.Contains(string(text), "已取消") {
		t.Fatalf("default clear command = %q, %v", text, err)
	}
	unbind := base
	unbind.Selector = "u2"
	if text, err := ExecuteProfileBindingCommand(ctx, service, ProfileModeUnbind, unbind); err != nil || !strings.Contains(string(text), "已解绑") {
		t.Fatalf("unbind command = %q, %v", text, err)
	}
	if _, err := ExecuteProfileBindingCommand(ctx, service, "unsupported", base); err == nil {
		t.Fatal("unsupported binding command should fail")
	}
	bad := base
	bad.Selector = "u99"
	for _, mode := range []string{ProfileModeUnbind, ProfileModeDefaultSet, ProfileModeDefaultClear, ProfileModeQueryUID, ProfileModeBindSwap} {
		if _, err := ExecuteProfileBindingCommand(ctx, service, mode, bad); err == nil {
			t.Fatalf("binding command %q should propagate selector errors", mode)
		}
	}
}

func testProfileBindingFormatting(t *testing.T) {
	t.Helper()
	visible := BindingListItem{Index: 2, BindingID: 2, Server: "jp", UserID: "123456789", Visible: true, IsGlobalDefault: true, IsServerDefault: true}
	hidden := BindingListItem{Index: 1, BindingID: 1, Server: "jp", UserID: "123456789"}
	testProfileBindingListFormatting(t, visible, hidden)
	testProfileBindingResultFormatting(t, visible, hidden)
}

func testProfileBindingListFormatting(t *testing.T, visible, hidden BindingListItem) {
	t.Helper()
	if formatBindingListText(nil, "") != "你还没有绑定任何PJSK账号" || !strings.Contains(formatBindingListText(nil, "jp"), "JP服") {
		t.Fatal("empty binding list formatting mismatch")
	}
	if text := formatBindingListText([]BindingListItem{visible}, "jp"); !strings.Contains(text, "u2") || !strings.Contains(text, "全局默认") || !strings.Contains(text, "JP服默认") || !strings.Contains(text, visible.UserID) {
		t.Fatalf("server binding list text = %q", text)
	}
	if text := formatBindingListText([]BindingListItem{hidden}, ""); !strings.Contains(text, "u1") || strings.Contains(text, hidden.UserID) {
		t.Fatalf("global hidden binding list text = %q", text)
	}
}

func testProfileBindingResultFormatting(t *testing.T, visible, hidden BindingListItem) {
	t.Helper()
	if formatBindResultText(nil) != "绑定成功" {
		t.Fatal("nil bind result formatting mismatch")
	}
	bindText := formatBindResultText(&BindResult{Server: "jp", UserID: "123456789", UserName: "name", AlreadyBound: true, SetGlobalDefault: true, SetServerDefault: true, MultipleServerMatch: true})
	for _, phrase := range []string{"此前已绑定", "全局默认", "JP服默认", "多个服务器"} {
		if !strings.Contains(bindText, phrase) {
			t.Fatalf("bind result missing %q: %q", phrase, bindText)
		}
	}
	if formatUnbindResultText(nil) != "解绑成功" {
		t.Fatal("nil unbind result formatting mismatch")
	}
	unbindText := formatUnbindResultText(&UnbindResult{Removed: hidden, ReassignedGlobal: &visible, ReassignedServer: &visible})
	if !strings.Contains(unbindText, "全局默认") || !strings.Contains(unbindText, "JP服默认") {
		t.Fatalf("unbind result text = %q", unbindText)
	}
	if formatDefaultBindingResultText("已设置", nil) != "已设置默认绑定" {
		t.Fatal("nil default binding formatting mismatch")
	}
	if text := formatDefaultBindingResultText("已设置", &DefaultBindingResult{Scope: DefaultScopeServer, Server: "jp", Binding: visible}); !strings.Contains(text, "JP服默认绑定") {
		t.Fatalf("server default binding text = %q", text)
	}
	if text := formatBindingSwapResultText(" u1 ", " u2 ", "jp", []BindingListItem{visible}); !strings.Contains(text, "JP服") {
		t.Fatalf("server swap text = %q", text)
	}
	if text := formatBindingSwapResultText("u1", "u2", "", nil); !strings.Contains(text, "已交换") {
		t.Fatalf("global swap text = %q", text)
	}
	if formatBindingUID(visible) != visible.UserID || formatBindingUID(hidden) == hidden.UserID || maskUID("123") != "123" || maskUID("123456789") != "123***789" {
		t.Fatal("binding UID formatting mismatch")
	}
}

func TestProfileSettingsPureFormattingAndStableErrorBranches(t *testing.T) {
	testProfileSettingsStableErrors(t)
	testProfileSettingsMutationClassification(t)
	testProfileSettingsFormatting(t)
	testProfileDifficultyHelpers(t)
}

func testProfileSettingsStableErrors(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	service, _ := openAccountCoverageService(t, "settings_errors", accountCoverageValidator{})
	params := ProfileSettingsCommandParams{Platform: "qq", PlatformUserID: "42", Server: "jp", RegionExplicit: true}
	if _, err := ExecuteProfileSettingsCommand(ctx, nil, ProfileModeVerifyList, params); !errors.Is(err, ErrBindingServiceUnavailable) {
		t.Fatalf("nil profile settings service = %v", err)
	}
	if _, err := ExecuteProfileSettingsCommand(ctx, service, "unsupported", params); err == nil {
		t.Fatal("unsupported profile settings mode should fail")
	}
	for _, mode := range []string{ProfileModeHideID, ProfileModeShowID, ProfileModeHideSuite, ProfileModeShowSuite, ProfileModeHideMySekai, ProfileModeShowMySekai, ProfileModeBGUpload, ProfileModeBGClear} {
		if _, err := ExecuteProfileSettingsCommand(ctx, service, mode, params); err == nil {
			t.Fatalf("profile settings mode %q should fail without a binding", mode)
		}
	}
	if _, err := ExecuteProfileSettingsCommand(ctx, service, ProfileModeVerify, params); err == nil {
		t.Fatal("verify without a fast verifier should fail")
	}
	service.fastVerifier = accountCoverageFastVerifier{}
	if _, err := ExecuteProfileSettingsCommand(ctx, service, ProfileModeVerify, params); err == nil {
		t.Fatal("verify without a binding should fail")
	}
	if text, err := ExecuteProfileSettingsCommand(ctx, service, ProfileModeVerifyList, params); err != nil || !strings.Contains(string(text), "还没有绑定") {
		t.Fatalf("empty verify list = %q, %v", text, err)
	}
	if _, err := ExecuteProfileSettingsCommand(ctx, service, ProfileModeSetTimeZone, params); err == nil {
		t.Fatal("blank timezone should fail")
	}
	if _, err := ExecuteProfileSettingsCommand(ctx, service, ProfileModeSetArrestDiff, params); err == nil {
		t.Fatal("empty arrest difficulty toggles should fail")
	}
	invalidToggle := params
	invalidToggle.DifficultyToggles = []ProfileDifficultyToggle{{Difficulty: "invalid", Enabled: true}}
	if _, err := ExecuteProfileSettingsCommand(ctx, service, ProfileModeSetArrestDiff, invalidToggle); err == nil {
		t.Fatal("invalid arrest difficulty should fail")
	}
	if _, err := ExecuteProfileSettingsCommand(ctx, service, ProfileModeSetChartStyle, params); err == nil {
		t.Fatal("blank chart style should fail")
	}
	if _, err := ExecuteProfileSettingsCommand(ctx, service, ProfileModeBGAdjust, params); err == nil {
		t.Fatal("background query without a binding should fail")
	}
	service.SetReadOnly(true)
	if _, err := ExecuteProfileSettingsCommand(ctx, service, ProfileModeShowID, params); err == nil {
		t.Fatal("read-only mutating settings mode should fail")
	}
	service.SetReadOnly(false)
}

func testProfileSettingsMutationClassification(t *testing.T) {
	t.Helper()
	params := ProfileSettingsCommandParams{}
	for _, mode := range []string{ProfileModeHideID, ProfileModeShowID, ProfileModeHideSuite, ProfileModeShowSuite, ProfileModeHideMySekai, ProfileModeShowMySekai, ProfileModeSetTimeZone, ProfileModeSetArrestDiff, ProfileModeSetChartStyle, ProfileModeEnableModular, ProfileModeDisableModular, ProfileModeBGUpload, ProfileModeBGClear} {
		if !profileSettingsModeMutates(mode, params) {
			t.Fatalf("mode %q should mutate", mode)
		}
	}
	if profileSettingsModeMutates(ProfileModeVerifyList, params) || profileSettingsModeMutates(ProfileModeBGAdjust, params) {
		t.Fatal("read-only settings modes should not mutate")
	}
	params.Blur = new(2)
	if !profileSettingsModeMutates(ProfileModeBGAdjust, params) {
		t.Fatal("background adjustment with a value should mutate")
	}
}

func testProfileSettingsFormatting(t *testing.T) {
	t.Helper()
	if formatVerifyListText(nil, "") != "你还没有绑定任何PJSK账号" || !strings.Contains(formatVerifyListText(nil, "jp"), "JP服") {
		t.Fatal("empty verify list formatting mismatch")
	}
	visiblePath := "bg.jpg"
	verified := BindingListItem{Index: 2, Server: "jp", UserID: "123456789", Verified: true, Visible: true, IsGlobalDefault: true, IsServerDefault: true, Bg: &drawing.ProfileBgSettings{ImgPath: &visiblePath, Blur: 5, Alpha: 70, Vertical: true}}
	unverified := BindingListItem{Index: 1, Server: "tw", UserID: "987654321"}
	verifyText := formatVerifyListText([]BindingListItem{verified, unverified}, "")
	if !strings.Contains(verifyText, "✅") || !strings.Contains(verifyText, "❌") || !strings.Contains(verifyText, "全局默认") {
		t.Fatalf("verify list text = %q", verifyText)
	}
	if text := formatVerifyListText([]BindingListItem{verified}, "jp"); !strings.Contains(text, "u2") {
		t.Fatalf("regional verify list text = %q", text)
	}
	if !strings.Contains(formatProfileBGSettingsText(BindingListItem{Server: "jp"}), "还没有") || !strings.Contains(formatProfileBGSettingsText(verified), "竖屏") {
		t.Fatal("profile background settings formatting mismatch")
	}
	verified.Bg.Vertical = false
	if !strings.Contains(formatProfileBGSettingsText(verified), "横屏") {
		t.Fatal("horizontal profile background formatting mismatch")
	}
	candidates := make([]string, 25)
	for i := range candidates {
		candidates[i] = strings.Repeat("x", i+1)
	}
	if text := formatTimeZoneCandidatesText(" +08 ", candidates); !strings.Contains(text, "另外 5 个候选") || strings.Count(text, "\n") != 21 {
		t.Fatalf("timezone candidates text = %q", text)
	}
	if text := formatTimeZoneCandidatesText("UTC", []string{"UTC"}); !strings.Contains(text, "UTC") {
		t.Fatalf("short timezone candidates text = %q", text)
	}
}

func testProfileDifficultyHelpers(t *testing.T) {
	t.Helper()
	for _, diff := range []sekaiapi.MusicDifficultyType{sekaiapi.MusicDifficultyEasy, sekaiapi.MusicDifficultyNormal, sekaiapi.MusicDifficultyHard, sekaiapi.MusicDifficultyExpert, sekaiapi.MusicDifficultyMaster, sekaiapi.MusicDifficultyAppend} {
		if normalizeProfileDifficulty(sekaiapi.MusicDifficultyType(" "+strings.ToUpper(string(diff))+" ")) != diff {
			t.Fatalf("difficulty %q did not normalize", diff)
		}
	}
	if normalizeProfileDifficulty("invalid") != "" {
		t.Fatal("invalid difficulty should normalize to empty")
	}
	updated, err := applyProfileDifficultyToggles([]sekaiapi.MusicDifficultyType{"invalid", sekaiapi.MusicDifficultyExpert}, []ProfileDifficultyToggle{{Difficulty: sekaiapi.MusicDifficultyMaster, Enabled: true}, {Difficulty: sekaiapi.MusicDifficultyExpert, Enabled: false}})
	if err != nil || len(updated) != 1 || updated[0] != sekaiapi.MusicDifficultyMaster {
		t.Fatalf("difficulty toggles = %+v, %v", updated, err)
	}
	if _, err := applyProfileDifficultyToggles(nil, []ProfileDifficultyToggle{{Difficulty: "bad", Enabled: true}}); err == nil {
		t.Fatal("unsupported difficulty toggle should fail")
	}
	if summary := formatProfileDifficultySummary([]sekaiapi.MusicDifficultyType{"bad", sekaiapi.MusicDifficultyEasy}); !strings.Contains(summary, "easy开启") || !strings.Contains(summary, "master关闭") {
		t.Fatalf("difficulty summary = %q", summary)
	}
	defaults := newDefaultUserSettings()
	if len(defaults.PJSKEnabledDifficulties) != 2 {
		t.Fatalf("default user settings = %+v", defaults)
	}
}

func TestBindingPropertyDefensiveVerificationAndClearBranches(t *testing.T) {
	ctx := context.Background()
	service, client := openAccountCoverageService(t, "property_edges", accountCoverageValidator{profiles: map[string]string{"jp": "Player"}})
	if _, err := service.Bind(ctx, "qq", "42", "8001"); err != nil {
		t.Fatalf("bind property account: %v", err)
	}
	binding, err := service.currentBindingEntity(ctx, "qq", "42", "jp")
	if err != nil {
		t.Fatalf("current property binding: %v", err)
	}
	testBindingProfileBackgroundDefenses(t, ctx, service, client, binding)
	testBindingVerificationBranches(t, ctx, service, binding)
	testBindingPropertyMutationErrors(t, ctx, service, binding)
}

func testBindingProfileBackgroundDefenses(t *testing.T, ctx context.Context, service *BindingService, client *pjskdb.Client, binding *pjskdb.UserBinding) {
	t.Helper()
	testUnverifiedProfileBackgroundDefenses(t, ctx, service, binding)
	testVerifiedProfileBackgroundDefenses(t, ctx, service, client, binding)
}

func testUnverifiedProfileBackgroundDefenses(t *testing.T, ctx context.Context, service *BindingService, binding *pjskdb.UserBinding) {
	t.Helper()
	if _, err := (*BindingService)(nil).setBindingProfileBG(ctx, "qq", "42", binding, "url"); err == nil {
		t.Fatal("nil profile background service should fail")
	}
	if _, err := service.setBindingProfileBG(ctx, "qq", "42", binding, "url"); err == nil {
		t.Fatal("missing profile background storage should fail")
	}
	service.bgStorage = accountCoverageBGStorage{}
	if _, err := service.setBindingProfileBG(ctx, "qq", "42", nil, "url"); err == nil {
		t.Fatal("nil binding profile background set should fail")
	}
	if _, err := service.setBindingProfileBG(ctx, "qq", "42", binding, "url"); err == nil || !strings.Contains(err.Error(), "尚未验证") {
		t.Fatalf("unverified profile background set = %v", err)
	}
	if _, err := service.clearBindingProfileBG(ctx, "qq", "42", nil); err == nil {
		t.Fatal("nil binding profile background clear should fail")
	}
	if _, err := service.clearBindingProfileBG(ctx, "qq", "42", binding); err == nil || !strings.Contains(err.Error(), "尚未验证") {
		t.Fatalf("unverified profile background clear = %v", err)
	}
	if _, err := service.adjustBindingProfileBG(ctx, "qq", "42", nil, nil, nil, nil); err == nil {
		t.Fatal("nil binding profile background adjust should fail")
	}
	if _, err := service.adjustBindingProfileBG(ctx, "qq", "42", binding, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "尚未验证") {
		t.Fatalf("unverified profile background adjust = %v", err)
	}
}

func testVerifiedProfileBackgroundDefenses(t *testing.T, ctx context.Context, service *BindingService, client *pjskdb.Client, binding *pjskdb.UserBinding) {
	t.Helper()
	if _, err := client.UserBinding.UpdateOneID(binding.ID).SetVerified(true).Save(ctx); err != nil {
		t.Fatalf("verify property binding in DB: %v", err)
	}
	binding.Verified = true
	if _, err := service.adjustBindingProfileBG(ctx, "qq", "42", binding, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "还没有") {
		t.Fatalf("adjust without a background = %v", err)
	}
	service.bgStorage = nil
	if item, err := service.clearBindingProfileBG(ctx, "qq", "42", binding); err != nil || item == nil || item.Bg != nil {
		t.Fatalf("clear absent background = %+v, %v", item, err)
	}
	service.bgStorage = accountCoverageBGStorage{deleteErr: errors.New("delete failed")}
	if _, err := service.clearBindingProfileBG(ctx, "qq", "42", binding); err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("background delete failure = %v", err)
	}
}

func testBindingVerificationBranches(t *testing.T, ctx context.Context, service *BindingService, binding *pjskdb.UserBinding) {
	t.Helper()
	if _, _, err := (*BindingService)(nil).VerifyCurrentBinding(ctx, "qq", "42", "jp"); err == nil {
		t.Fatal("nil fast verification service should fail")
	}
	service.fastVerifier = accountCoverageFastVerifier{err: errors.New("verification failed")}
	binding.Verified = false
	if _, _, err := service.verifyBindingEntity(ctx, "qq", "42", binding); err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("verification provider failure = %v", err)
	}
	service.fastVerifier = accountCoverageFastVerifier{records: []sekaiapi.UserGameBinding{{Server: "jp", GameUserID: "other"}}}
	if _, _, err := service.verifyBindingEntity(ctx, "qq", "42", binding); err == nil || !strings.Contains(err.Error(), "未出现在") {
		t.Fatalf("unmatched verification = %v", err)
	}
	contextual := &accountCoverageContextFastVerifier{accountCoverageFastVerifier: accountCoverageFastVerifier{records: []sekaiapi.UserGameBinding{{Server: " JP ", GameUserID: "8001"}}}}
	service.fastVerifier = contextual
	item, already, err := service.verifyBindingEntity(ctx, "qq", "42", binding)
	if err != nil || already || item == nil || !item.Verified || !contextual.called {
		t.Fatalf("contextual verification = %+v, %v, %v", item, already, err)
	}
	if item, already, err := service.verifyBindingEntity(ctx, "qq", "42", &pjskdb.UserBinding{ID: binding.ID, Verified: true}); err != nil || !already || item == nil {
		t.Fatalf("already-verified binding = %+v, %v, %v", item, already, err)
	}
	if _, err := service.ListVerifiedBindings(ctx, "qq", "42", " "); err == nil {
		t.Fatal("verified binding list without a server should fail")
	}
	if items, err := service.ListVerifiedBindings(ctx, "qq", "42", "jp"); err != nil || len(items) != 1 || !items[0].Verified {
		t.Fatalf("verified JP bindings = %+v, %v", items, err)
	}
	if items, err := service.ListVerifiedBindings(ctx, "qq", "42", "tw"); err != nil || len(items) != 0 {
		t.Fatalf("verified TW bindings = %+v, %v", items, err)
	}
}

func testBindingPropertyMutationErrors(t *testing.T, ctx context.Context, service *BindingService, binding *pjskdb.UserBinding) {
	t.Helper()
	service.SetReadOnly(true)
	for _, call := range []func() error{
		func() error { _, err := service.SetBindingVisible(ctx, "qq", "42", "jp", true); return err },
		func() error { _, err := service.SetBindingSuiteVisible(ctx, "qq", "42", "jp", true); return err },
		func() error { _, err := service.SetBindingMySekaiVisible(ctx, "qq", "42", "jp", true); return err },
		func() error { _, err := service.clearBindingProfileBG(ctx, "qq", "42", binding); return err },
		func() error {
			_, err := service.adjustBindingProfileBG(ctx, "qq", "42", binding, nil, nil, nil)
			return err
		},
	} {
		if err := call(); err == nil {
			t.Fatal("read-only binding property mutation should fail")
		}
	}
	service.SetReadOnly(false)
	for _, call := range []func() error{
		func() error { _, err := service.SetBindingVisible(ctx, "qq", "42", "en", true); return err },
		func() error { _, err := service.SetBindingSuiteVisible(ctx, "qq", "42", "en", true); return err },
		func() error { _, err := service.SetBindingMySekaiVisible(ctx, "qq", "42", "en", true); return err },
		func() error { _, err := service.SetCurrentBindingProfileBG(ctx, "qq", "42", "en", "url"); return err },
		func() error { _, err := service.ClearCurrentBindingProfileBG(ctx, "qq", "42", "en"); return err },
		func() error {
			_, err := service.AdjustCurrentBindingProfileBG(ctx, "qq", "42", "en", nil, nil, nil)
			return err
		},
	} {
		if err := call(); err == nil {
			t.Fatal("missing binding property operation should fail")
		}
	}
}
