package accountdata

import (
	"context"
	"errors"
	"strings"
	"testing"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/internal/testutil"
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

func (s accountCoverageBGStorage) SaveProfileBackgroundTracked(ctx context.Context, server, userID, imageURL string, beforePut func(*drawing.ProfileBgSettings) error) (*drawing.ProfileBgSettings, error) {
	settings, err := s.SaveProfileBackground(ctx, server, userID, imageURL)
	if err == nil && beforePut != nil {
		err = beforePut(settings)
	}
	return settings, err
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
	second := i18n.AccountLabel("jp", "7002", false)
	if text, err := ExecuteProfileBindingCommand(ctx, service, ProfileModeDefaultSet, set); err != nil || string(text) != i18n.T("account.default.set_global", i18n.Data{"Account": second}) {
		t.Fatalf("default set command = %q, %v", text, err)
	}
	if text, err := ExecuteProfileBindingCommand(ctx, service, ProfileModeDefaultClear, set); err != nil || string(text) != i18n.T("account.default.cleared_global", i18n.Data{"Account": second}) {
		t.Fatalf("default clear command = %q, %v", text, err)
	}
	unbind := base
	unbind.Selector = "u2"
	if text, err := ExecuteProfileBindingCommand(ctx, service, ProfileModeUnbind, unbind); err != nil || !strings.HasPrefix(string(text), i18n.T("account.unbind.done", i18n.Data{"Account": second})) {
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
	visible := BindingListItem{Index: 2, BindingID: 2, Server: "jp", UserID: "123456789", Visibility: UniformVisibility(true), IsGlobalDefault: true, IsServerDefault: true}
	hidden := BindingListItem{Index: 1, BindingID: 1, Server: "jp", UserID: "123456789"}
	testProfileBindingListFormatting(t, visible, hidden)
	testProfileBindingResultFormatting(t, visible, hidden)
}

func testProfileBindingListFormatting(t *testing.T, visible, hidden BindingListItem) {
	t.Helper()
	jp := i18n.RegionLabel("jp")
	if formatBindingListText(nil, "") != i18n.T("binding.none") || formatBindingListText(nil, "jp") != i18n.T("binding.none_in_region", i18n.Data{"Region": jp}) {
		t.Fatal("empty binding list formatting mismatch")
	}
	marks := i18n.T("account.mark.global_default") + "、" + i18n.T("account.mark.region_default", i18n.Data{"Region": jp})
	want := i18n.T("account.list.header_region", i18n.Data{"Region": jp}) + "\n" +
		i18n.T("account.list.item_marked", i18n.Data{"Index": 2, "Account": i18n.AccountLabel("jp", visible.UserID, true), "Marks": marks})
	if text := formatBindingListText([]BindingListItem{visible}, "jp"); text != want {
		t.Fatalf("server binding list text = %q, want %q", text, want)
	}
	want = i18n.T("account.list.header") + "\n" + i18n.T("account.list.item", i18n.Data{"Index": 1, "Account": i18n.AccountLabel("jp", hidden.UserID, false)})
	if text := formatBindingListText([]BindingListItem{hidden}, ""); text != want || strings.Contains(text, hidden.UserID) {
		t.Fatalf("global hidden binding list text = %q, want %q", text, want)
	}
}

func testProfileBindingResultFormatting(t *testing.T, visible, hidden BindingListItem) {
	t.Helper()
	jp := i18n.RegionLabel("jp")
	if formatBindResultText(nil) != i18n.T("account.bind.done_plain") {
		t.Fatal("nil bind result formatting mismatch")
	}
	bindText := formatBindResultText(&BindResult{Server: "jp", UserID: "123456789", UserName: "name", AlreadyBound: true, SetGlobalDefault: true, SetServerDefault: true, MultipleServerMatch: true})
	wantBind := strings.Join([]string{
		i18n.T("account.bind.done", i18n.Data{"Account": i18n.AccountLabel("jp", "123456789", false), "Name": "name"}),
		i18n.T("account.bind.note_already_bound"),
		i18n.T("account.bind.note_global_default"),
		i18n.T("account.bind.note_region_default", i18n.Data{"Region": jp}),
		i18n.T("account.bind.note_multiple_regions"),
	}, "\n")
	if bindText != wantBind || strings.Contains(bindText, "123456789") {
		t.Fatalf("bind result text = %q, want %q", bindText, wantBind)
	}
	if formatUnbindResultText(nil) != i18n.T("account.unbind.done_plain") {
		t.Fatal("nil unbind result formatting mismatch")
	}
	unbindText := formatUnbindResultText(&UnbindResult{Removed: hidden, ReassignedGlobal: &visible, ReassignedServer: &visible})
	visibleLabel := i18n.AccountLabel("jp", visible.UserID, true)
	wantUnbind := strings.Join([]string{
		i18n.T("account.unbind.done", i18n.Data{"Account": i18n.AccountLabel("jp", hidden.UserID, false)}),
		i18n.T("account.unbind.reassigned_global", i18n.Data{"Account": visibleLabel}),
		i18n.T("account.unbind.reassigned_region", i18n.Data{"Region": jp, "Account": visibleLabel}),
	}, "\n")
	if unbindText != wantUnbind {
		t.Fatalf("unbind result text = %q, want %q", unbindText, wantUnbind)
	}
	if formatDefaultBindingSetText(nil) != i18n.T("account.default.set_plain") || formatDefaultBindingClearedText(nil) != i18n.T("account.default.cleared_plain") {
		t.Fatal("nil default binding formatting mismatch")
	}
	serverResult := &DefaultBindingResult{Scope: DefaultScopeServer, Server: "jp", Binding: visible}
	if text := formatDefaultBindingSetText(serverResult); text != i18n.T("account.default.set_region", i18n.Data{"Region": jp, "Account": visibleLabel}) {
		t.Fatalf("server default binding text = %q", text)
	}
	globalResult := &DefaultBindingResult{Scope: DefaultScopeGlobal, Binding: visible}
	if text := formatDefaultBindingClearedText(globalResult); text != i18n.T("account.default.cleared_global", i18n.Data{"Account": visibleLabel}) {
		t.Fatalf("global default clear text = %q", text)
	}
	if text := formatDefaultBindingClearedText(serverResult); text != i18n.T("account.default.cleared_region", i18n.Data{"Region": jp, "Account": visibleLabel}) {
		t.Fatalf("server default clear text = %q", text)
	}
	if text := formatDefaultBindingSetText(globalResult); text != i18n.T("account.default.set_global", i18n.Data{"Account": visibleLabel}) {
		t.Fatalf("global default binding text = %q", text)
	}
	wantSwap := i18n.T("account.swap.done_region", i18n.Data{"Region": jp, "Left": "u1", "Right": "u2", "List": formatBindingListText([]BindingListItem{visible}, "jp")})
	if text := formatBindingSwapResultText(" u1 ", " u2 ", "jp", []BindingListItem{visible}); text != wantSwap {
		t.Fatalf("server swap text = %q", text)
	}
	if text := formatBindingSwapResultText("u1", "u2", "", nil); text != i18n.T("account.swap.done", i18n.Data{"Left": "u1", "Right": "u2", "List": i18n.T("binding.none")}) {
		t.Fatalf("global swap text = %q", text)
	}
	if i18n.MaskUID("123", false) != "123" || i18n.MaskUID("123456789", false) != "123***789" {
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
	if text, err := ExecuteProfileSettingsCommand(ctx, service, ProfileModeVerifyList, params); err != nil || string(text) != i18n.T("binding.none_in_region", i18n.Data{"Region": i18n.RegionLabel("jp")}) {
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
	jp := i18n.RegionLabel("jp")
	if formatVerifyListText(nil, "") != i18n.T("binding.none") || formatVerifyListText(nil, "jp") != i18n.T("binding.none_in_region", i18n.Data{"Region": jp}) {
		t.Fatal("empty verify list formatting mismatch")
	}
	visiblePath := "bg.jpg"
	verified := BindingListItem{Index: 2, Server: "jp", UserID: "123456789", Verified: true, Visibility: UniformVisibility(true), IsGlobalDefault: true, IsServerDefault: true, Bg: &drawing.ProfileBgSettings{ImgPath: &visiblePath, Blur: 5, Alpha: 70, Vertical: true}}
	unverified := BindingListItem{Index: 1, Server: "tw", UserID: "987654321"}
	verifyText := formatVerifyListText([]BindingListItem{verified, unverified}, "")
	marks := i18n.T("account.mark.global_default") + "、" + i18n.T("account.mark.region_default", i18n.Data{"Region": jp})
	wantVerify := strings.Join([]string{
		i18n.T("account.verify_list.header"),
		i18n.T("account.verify_list.item_marked", i18n.Data{"Index": 1, "Account": i18n.AccountLabel("jp", verified.UserID, true), "Status": i18n.M("account.verify_list.verified"), "Marks": marks}),
		i18n.T("account.verify_list.item", i18n.Data{"Index": 2, "Account": i18n.AccountLabel("tw", unverified.UserID, false), "Status": i18n.M("account.verify_list.unverified")}),
	}, "\n")
	if verifyText != wantVerify {
		t.Fatalf("verify list text = %q, want %q", verifyText, wantVerify)
	}
	if text := formatVerifyListText([]BindingListItem{verified}, "jp"); !strings.HasPrefix(text, i18n.T("account.verify_list.header_region", i18n.Data{"Region": jp})+"\nu2 ") {
		t.Fatalf("regional verify list text = %q", text)
	}
	if text := formatProfileBGSettingsText(BindingListItem{Server: "jp"}); text != i18n.T("account.bg.none", i18n.Data{"Region": jp}) {
		t.Fatalf("empty profile background text = %q", text)
	}
	bgData := i18n.Data{"Region": jp, "Account": i18n.AccountLabel("jp", verified.UserID, true), "Orientation": i18n.M("account.bg.vertical"), "Blur": 5, "Alpha": 70}
	if text := formatProfileBGSettingsText(verified); text != i18n.T("account.bg.settings", bgData) {
		t.Fatalf("profile background settings text = %q", text)
	}
	verified.Bg.Vertical = false
	bgData["Orientation"] = i18n.M("account.bg.horizontal")
	if text := formatProfileBGSettingsText(verified); text != i18n.T("account.bg.settings", bgData) {
		t.Fatalf("horizontal profile background text = %q", text)
	}
	candidates := make([]string, 25)
	for i := range candidates {
		candidates[i] = strings.Repeat("x", i+1)
	}
	text := formatTimeZoneCandidatesText(candidates)
	if !strings.HasSuffix(text, "\n"+i18n.T("account.settings.timezone_more", i18n.Data{"Count": 5})) || strings.Count(text, "\n") != strings.Count(i18n.T("account.settings.timezone_candidates", i18n.Data{"Candidates": ""}), "\n")+20 {
		t.Fatalf("timezone candidates text = %q", text)
	}
	if text := formatTimeZoneCandidatesText([]string{"UTC"}); text != i18n.T("account.settings.timezone_candidates", i18n.Data{"Candidates": "UTC"}) {
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
	wantSummary := i18n.T("account.settings.arrest_difficulty_set", i18n.Data{"Enabled": "EASY", "Disabled": "NORMAL、HARD、EXPERT、MASTER、APPEND"})
	if summary := formatProfileDifficultySummary([]sekaiapi.MusicDifficultyType{"bad", sekaiapi.MusicDifficultyEasy}); summary != wantSummary {
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
	if _, err := service.setBindingProfileBG(ctx, "qq", "42", binding, "url"); err == nil || testutil.MessageID(err) != "profile.bg.unverified" {
		t.Fatalf("unverified profile background set = %v", err)
	}
	if _, err := service.clearBindingProfileBG(ctx, "qq", "42", nil); err == nil {
		t.Fatal("nil binding profile background clear should fail")
	}
	if _, err := service.clearBindingProfileBG(ctx, "qq", "42", binding); err == nil || testutil.MessageID(err) != "profile.bg.unverified" {
		t.Fatalf("unverified profile background clear = %v", err)
	}
	if _, err := service.adjustBindingProfileBG(ctx, "qq", "42", nil, nil, nil, nil); err == nil {
		t.Fatal("nil binding profile background adjust should fail")
	}
	if _, err := service.adjustBindingProfileBG(ctx, "qq", "42", binding, nil, nil, nil); err == nil || testutil.MessageID(err) != "profile.bg.unverified" {
		t.Fatalf("unverified profile background adjust = %v", err)
	}
}

func testVerifiedProfileBackgroundDefenses(t *testing.T, ctx context.Context, service *BindingService, client *pjskdb.Client, binding *pjskdb.UserBinding) {
	t.Helper()
	if _, err := client.UserBinding.UpdateOneID(binding.ID).SetVerified(true).Save(ctx); err != nil {
		t.Fatalf("verify property binding in DB: %v", err)
	}
	binding.Verified = true
	if _, err := service.adjustBindingProfileBG(ctx, "qq", "42", binding, nil, nil, nil); testutil.MessageID(err) != "profile.bg.none" {
		t.Fatalf("adjust without a background = %v", err)
	}
	service.bgStorage = nil
	if item, err := service.clearBindingProfileBG(ctx, "qq", "42", binding); err != nil || item == nil || item.Bg != nil {
		t.Fatalf("clear absent background = %+v, %v", item, err)
	}
	service.bgStorage = accountCoverageBGStorage{deleteErr: errors.New("delete failed")}
	if _, err := service.clearBindingProfileBG(ctx, "qq", "42", binding); err != nil {
		t.Fatalf("background clear must commit independently of deletion: %v", err)
	}
}

func testBindingVerificationBranches(t *testing.T, ctx context.Context, service *BindingService, binding *pjskdb.UserBinding) {
	t.Helper()
	if _, _, err := (*BindingService)(nil).VerifyCurrentBinding(ctx, "qq", "42", "jp"); err == nil {
		t.Fatal("nil fast verification service should fail")
	}
	service.fastVerifier = accountCoverageFastVerifier{err: errors.New("verification failed")}
	binding.Verified = false
	if _, _, err := service.verifyBindingEntity(ctx, "qq", "42", binding); err == nil || !strings.Contains(testutil.ErrorDetail(err), "verification failed") {
		t.Fatalf("verification provider failure = %v", err)
	}
	service.fastVerifier = accountCoverageFastVerifier{records: []sekaiapi.UserGameBinding{{Server: "jp", GameUserID: "other"}}}
	if _, _, err := service.verifyBindingEntity(ctx, "qq", "42", binding); err == nil || testutil.MessageID(err) != "binding.verify.not_listed" {
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
