package accountdata_test

import (
	"context"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

type fakeFastVerifier struct {
	bindings []sekaiapi.UserGameBinding
	err      error
}

func (f fakeFastVerifier) GetToolboxUserFastVerificationGameAccountBindings(platform, platformUserID string) ([]sekaiapi.UserGameBinding, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]sekaiapi.UserGameBinding(nil), f.bindings...), nil
}

type fakeProfileBGStore struct {
	saved        []string
	savedServers []string
	savedUserIDs []string
	deleted      []string
}

func (s *fakeProfileBGStore) SaveProfileBackground(ctx context.Context, server string, userID string, imageURL string) (*drawing.ProfileBgSettings, error) {
	s.saved = append(s.saved, imageURL)
	s.savedServers = append(s.savedServers, server)
	s.savedUserIDs = append(s.savedUserIDs, userID)
	path := accountdata.DefaultProfileBGRelativeDir + "/" + server + "/uid_" + userID + ".jpg"
	return &drawing.ProfileBgSettings{
		ImgPath:  &path,
		Blur:     4,
		Alpha:    80,
		Vertical: false,
	}, nil
}

func (s *fakeProfileBGStore) DeleteProfileBackground(ctx context.Context, settings *drawing.ProfileBgSettings) error {
	if settings != nil && settings.ImgPath != nil {
		s.deleted = append(s.deleted, *settings.ImgPath)
	}
	return nil
}

func TestBindingServiceProfileSettingsLifecycle(t *testing.T) {
	service := newProfileBindingTestService(t, map[string]map[string]string{
		"jp": {"12345678901234": "JP User"},
	})
	service.SetFastVerificationProvider(fakeFastVerifier{
		bindings: []sekaiapi.UserGameBinding{{Server: "jp", GameUserID: "12345678901234"}},
	})
	bgStore := &fakeProfileBGStore{}
	service.SetProfileBGStorage(bgStore)

	ctx := context.Background()
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	item, err := service.SetBindingVisible(ctx, "qq", "42", "jp", false)
	if err != nil {
		t.Fatalf("hide id: %v", err)
	}
	if item.Visible {
		t.Fatalf("expected visible=false, got %+v", item)
	}

	item, err = service.SetBindingSuiteVisible(ctx, "qq", "42", "jp", false)
	if err != nil {
		t.Fatalf("hide suite: %v", err)
	}
	if item.SuiteVisible {
		t.Fatalf("expected suite_visible=false, got %+v", item)
	}

	item, err = service.SetBindingMySekaiVisible(ctx, "qq", "42", "jp", false)
	if err != nil {
		t.Fatalf("hide mysekai: %v", err)
	}
	if item.MySekaiVisible {
		t.Fatalf("expected mysekai_visible=false, got %+v", item)
	}

	item, alreadyVerified, err := service.VerifyCurrentBinding(ctx, "qq", "42", "jp")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if alreadyVerified {
		t.Fatalf("expected first verify call to perform verification")
	}
	if !item.Verified {
		t.Fatalf("expected verified=true, got %+v", item)
	}

	_, alreadyVerified, err = service.VerifyCurrentBinding(ctx, "qq", "42", "jp")
	if err != nil {
		t.Fatalf("verify second call: %v", err)
	}
	if !alreadyVerified {
		t.Fatalf("expected second verify call to report already verified")
	}

	item, err = service.SetCurrentBindingProfileBG(ctx, "qq", "42", "jp", "https://example.com/bg.png")
	if err != nil {
		t.Fatalf("set bg: %v", err)
	}
	if item.Bg == nil || item.Bg.ImgPath == nil || *item.Bg.ImgPath == "" {
		t.Fatalf("expected bg settings to be stored, got %+v", item)
	}
	if len(bgStore.saved) != 1 || bgStore.saved[0] != "https://example.com/bg.png" {
		t.Fatalf("unexpected saved backgrounds: %+v", bgStore.saved)
	}

	blur := 7
	alpha := 66
	vertical := true
	item, err = service.AdjustCurrentBindingProfileBG(ctx, "qq", "42", "jp", &blur, &alpha, &vertical)
	if err != nil {
		t.Fatalf("adjust bg: %v", err)
	}
	if item.Bg == nil || item.Bg.Blur != 7 || item.Bg.Alpha != 66 || !item.Bg.Vertical {
		t.Fatalf("unexpected adjusted bg: %+v", item.Bg)
	}

	item, err = service.ClearCurrentBindingProfileBG(ctx, "qq", "42", "jp")
	if err != nil {
		t.Fatalf("clear bg: %v", err)
	}
	if item.Bg != nil {
		t.Fatalf("expected bg to be cleared, got %+v", item.Bg)
	}
	if len(bgStore.deleted) != 1 {
		t.Fatalf("expected one deleted background, got %+v", bgStore.deleted)
	}
}

func TestProfileBackgroundRequiresVerifiedBinding(t *testing.T) {
	service := newProfileBindingTestService(t, map[string]map[string]string{
		"jp": {"12345678901234": "JP User"},
	})
	service.SetProfileBGStorage(&fakeProfileBGStore{})

	ctx := context.Background()
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	_, err := service.SetCurrentBindingProfileBG(ctx, "qq", "42", "jp", "https://example.com/bg.png")
	if err == nil {
		t.Fatal("expected unverified binding to reject bg upload")
	}
	if !strings.Contains(err.Error(), "尚未验证") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteProfileSettingsCommandVerifyListMasksUID(t *testing.T) {
	service := newProfileBindingTestService(t, map[string]map[string]string{
		"jp": {"12345678901234": "JP User"},
	})
	service.SetFastVerificationProvider(fakeFastVerifier{
		bindings: []sekaiapi.UserGameBinding{{Server: "jp", GameUserID: "12345678901234"}},
	})

	ctx := context.Background()
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if _, err := service.SetBindingVisible(ctx, "qq", "42", "jp", false); err != nil {
		t.Fatalf("hide id: %v", err)
	}
	if _, _, err := service.VerifyCurrentBinding(ctx, "qq", "42", "jp"); err != nil {
		t.Fatalf("verify: %v", err)
	}

	text, err := accountdata.ExecuteProfileSettingsCommand(ctx, service, accountdata.ProfileModeVerifyList, accountdata.ProfileSettingsCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Server:         "jp",
	})
	if err != nil {
		t.Fatalf("verify list: %v", err)
	}

	expected := "已绑定账号验证状态（u序号按区服分别编号）:\nu1 [JP] 123********234 ✅ (全局默认/JP服默认)"
	if string(text) != expected {
		t.Fatalf("unexpected verify list text:\n%s", text)
	}
}

func TestExecuteProfileSettingsCommandVerifyReturnsVerificationText(t *testing.T) {
	service := newProfileBindingTestService(t, map[string]map[string]string{
		"jp": {"12345678901234": "JP User"},
	})
	service.SetFastVerificationProvider(fakeFastVerifier{
		bindings: []sekaiapi.UserGameBinding{{Server: "jp", GameUserID: "12345678901234"}},
	})

	ctx := context.Background()
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	text, err := accountdata.ExecuteProfileSettingsCommand(ctx, service, accountdata.ProfileModeVerify, accountdata.ProfileSettingsCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Server:         "jp",
	})
	if err != nil {
		t.Fatalf("verify command: %v", err)
	}
	if got := string(text); got != "已验证JP服账号 123********234" {
		t.Fatalf("unexpected verify text:\n%s", got)
	}
}

func TestExecuteProfileSettingsCommandBGUploadUsesGlobalDefaultBindingWhenRegionImplicit(t *testing.T) {
	service := newProfileBindingTestService(t, map[string]map[string]string{
		"tw": {"11111111111111": "TW User"},
		"jp": {"22222222222222": "JP User"},
	})
	service.SetFastVerificationProvider(fakeFastVerifier{
		bindings: []sekaiapi.UserGameBinding{
			{Server: "tw", GameUserID: "11111111111111"},
			{Server: "jp", GameUserID: "22222222222222"},
		},
	})
	bgStore := &fakeProfileBGStore{}
	service.SetProfileBGStorage(bgStore)

	ctx := context.Background()
	if _, err := service.Bind(ctx, "qq", "42", "11111111111111"); err != nil {
		t.Fatalf("bind tw: %v", err)
	}
	if _, err := service.Bind(ctx, "qq", "42", "22222222222222"); err != nil {
		t.Fatalf("bind jp: %v", err)
	}
	if _, _, err := service.VerifyCurrentBinding(ctx, "qq", "42", "tw"); err != nil {
		t.Fatalf("verify tw: %v", err)
	}
	if _, _, err := service.VerifyCurrentBinding(ctx, "qq", "42", "jp"); err != nil {
		t.Fatalf("verify jp: %v", err)
	}

	text, err := accountdata.ExecuteProfileSettingsCommand(ctx, service, accountdata.ProfileModeBGUpload, accountdata.ProfileSettingsCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Server:         "jp",
		RegionExplicit: false,
		ImageURL:       "https://example.com/bg-tw.png",
	})
	if err != nil {
		t.Fatalf("bg upload command: %v", err)
	}
	if got := string(text); got != "已更新TW服个人信息背景" {
		t.Fatalf("unexpected bg upload text:\n%s", got)
	}
	if len(bgStore.saved) != 1 || bgStore.saved[0] != "https://example.com/bg-tw.png" {
		t.Fatalf("unexpected saved backgrounds: %+v", bgStore.saved)
	}
	if len(bgStore.savedServers) != 1 || bgStore.savedServers[0] != "tw" {
		t.Fatalf("unexpected saved servers: %+v", bgStore.savedServers)
	}
	if len(bgStore.savedUserIDs) != 1 || bgStore.savedUserIDs[0] != "11111111111111" {
		t.Fatalf("unexpected saved user ids: %+v", bgStore.savedUserIDs)
	}

	items, err := service.List(ctx, "qq", "42")
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	var twHasBG, jpHasBG bool
	for _, item := range items {
		switch item.Server {
		case "tw":
			twHasBG = item.Bg != nil && item.Bg.ImgPath != nil && *item.Bg.ImgPath != ""
		case "jp":
			jpHasBG = item.Bg != nil && item.Bg.ImgPath != nil && *item.Bg.ImgPath != ""
		}
	}
	if !twHasBG {
		t.Fatalf("expected TW binding to receive custom bg")
	}
	if jpHasBG {
		t.Fatalf("expected JP binding to remain unchanged")
	}
}

func TestExecuteProfileSettingsCommandBGUploadSelectorUsesGlobalIndicesWhenRegionImplicit(t *testing.T) {
	service := newProfileBindingTestService(t, map[string]map[string]string{
		"tw": {"11111111111111": "TW User"},
		"jp": {"22222222222222": "JP User"},
	})
	service.SetFastVerificationProvider(fakeFastVerifier{
		bindings: []sekaiapi.UserGameBinding{
			{Server: "tw", GameUserID: "11111111111111"},
			{Server: "jp", GameUserID: "22222222222222"},
		},
	})
	bgStore := &fakeProfileBGStore{}
	service.SetProfileBGStorage(bgStore)

	ctx := context.Background()
	if _, err := service.Bind(ctx, "qq", "42", "11111111111111"); err != nil {
		t.Fatalf("bind tw: %v", err)
	}
	if _, err := service.Bind(ctx, "qq", "42", "22222222222222"); err != nil {
		t.Fatalf("bind jp: %v", err)
	}
	if _, _, err := service.VerifyCurrentBinding(ctx, "qq", "42", "tw"); err != nil {
		t.Fatalf("verify tw: %v", err)
	}
	if _, _, err := service.VerifyCurrentBinding(ctx, "qq", "42", "jp"); err != nil {
		t.Fatalf("verify jp: %v", err)
	}

	text, err := accountdata.ExecuteProfileSettingsCommand(ctx, service, accountdata.ProfileModeBGUpload, accountdata.ProfileSettingsCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Server:         "jp",
		RegionExplicit: false,
		Selector:       "u1",
		ImageURL:       "https://example.com/bg-global-u1.png",
	})
	if err != nil {
		t.Fatalf("bg upload command with implicit-region selector: %v", err)
	}
	if got := string(text); got != "已更新TW服个人信息背景" {
		t.Fatalf("unexpected bg upload text:\n%s", got)
	}
	if len(bgStore.savedServers) != 1 || bgStore.savedServers[0] != "tw" {
		t.Fatalf("unexpected saved servers: %+v", bgStore.savedServers)
	}
	if len(bgStore.savedUserIDs) != 1 || bgStore.savedUserIDs[0] != "11111111111111" {
		t.Fatalf("unexpected saved user ids: %+v", bgStore.savedUserIDs)
	}
}

func TestExecuteProfileSettingsCommandBGUploadSelectorUsesRegionalIndicesWhenRegionExplicit(t *testing.T) {
	service := newProfileBindingTestService(t, map[string]map[string]string{
		"tw": {"11111111111111": "TW User"},
		"jp": {"22222222222222": "JP User"},
	})
	service.SetFastVerificationProvider(fakeFastVerifier{
		bindings: []sekaiapi.UserGameBinding{
			{Server: "tw", GameUserID: "11111111111111"},
			{Server: "jp", GameUserID: "22222222222222"},
		},
	})
	bgStore := &fakeProfileBGStore{}
	service.SetProfileBGStorage(bgStore)

	ctx := context.Background()
	if _, err := service.Bind(ctx, "qq", "42", "11111111111111"); err != nil {
		t.Fatalf("bind tw: %v", err)
	}
	if _, err := service.Bind(ctx, "qq", "42", "22222222222222"); err != nil {
		t.Fatalf("bind jp: %v", err)
	}
	if _, _, err := service.VerifyCurrentBinding(ctx, "qq", "42", "tw"); err != nil {
		t.Fatalf("verify tw: %v", err)
	}
	if _, _, err := service.VerifyCurrentBinding(ctx, "qq", "42", "jp"); err != nil {
		t.Fatalf("verify jp: %v", err)
	}

	text, err := accountdata.ExecuteProfileSettingsCommand(ctx, service, accountdata.ProfileModeBGUpload, accountdata.ProfileSettingsCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Server:         "jp",
		RegionExplicit: true,
		Selector:       "u1",
		ImageURL:       "https://example.com/bg-jp-u1.png",
	})
	if err != nil {
		t.Fatalf("bg upload command with explicit-region selector: %v", err)
	}
	if got := string(text); got != "已更新JP服个人信息背景" {
		t.Fatalf("unexpected bg upload text:\n%s", got)
	}
	if len(bgStore.savedServers) != 1 || bgStore.savedServers[0] != "jp" {
		t.Fatalf("unexpected saved servers: %+v", bgStore.savedServers)
	}
	if len(bgStore.savedUserIDs) != 1 || bgStore.savedUserIDs[0] != "22222222222222" {
		t.Fatalf("unexpected saved user ids: %+v", bgStore.savedUserIDs)
	}
}

func TestProfileBackgroundPersistsAcrossUnbindAndRebind(t *testing.T) {
	service := newProfileBindingTestService(t, map[string]map[string]string{
		"jp": {"12345678901234": "JP User"},
	})
	service.SetFastVerificationProvider(fakeFastVerifier{
		bindings: []sekaiapi.UserGameBinding{{Server: "jp", GameUserID: "12345678901234"}},
	})
	bgStore := &fakeProfileBGStore{}
	service.SetProfileBGStorage(bgStore)

	ctx := context.Background()
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if _, _, err := service.VerifyCurrentBinding(ctx, "qq", "42", "jp"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if _, err := service.SetCurrentBindingProfileBG(ctx, "qq", "42", "jp", "https://example.com/bg.png"); err != nil {
		t.Fatalf("set bg: %v", err)
	}

	if _, err := service.Unbind(ctx, "qq", "42", "12345678901234", ""); err != nil {
		t.Fatalf("unbind: %v", err)
	}
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("rebind: %v", err)
	}

	items, err := service.List(ctx, "qq", "42")
	if err != nil {
		t.Fatalf("list after rebind: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one binding after rebind, got %d", len(items))
	}
	if items[0].Bg == nil || items[0].Bg.ImgPath == nil || *items[0].Bg.ImgPath == "" {
		t.Fatalf("expected background to persist after rebind, got %+v", items[0].Bg)
	}
}
