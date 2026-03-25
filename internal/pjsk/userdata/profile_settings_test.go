package userdata_test

import (
	"context"
	"testing"

	"haruki-cloud/internal/pjsk/userdata"
	"haruki-cloud/utils/drawing"
	sekaiapi "haruki-cloud/utils/sekai"
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
	saved   []string
	deleted []string
}

func (s *fakeProfileBGStore) SaveProfileBackground(ctx context.Context, server string, bindingID int, imageURL string) (*drawing.ProfileBgSettings, error) {
	s.saved = append(s.saved, imageURL)
	path := userdata.DefaultProfileBGRelativeDir + "/" + server + "/binding_" + "1" + ".jpg"
	if bindingID != 1 {
		path = userdata.DefaultProfileBGRelativeDir + "/" + server + "/binding_" + "2" + ".jpg"
	}
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

	text, err := userdata.ExecuteProfileSettingsCommand(ctx, service, userdata.ProfileModeVerifyList, userdata.ProfileSettingsCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Server:         "jp",
	})
	if err != nil {
		t.Fatalf("verify list: %v", err)
	}

	expected := "你验证过的JP服游戏ID:\nu1 123********234"
	if string(text) != expected {
		t.Fatalf("unexpected verify list text:\n%s", text)
	}
}
