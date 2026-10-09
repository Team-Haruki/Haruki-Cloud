package accountdata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

// TestAccountRepliesGolden locks the zh-CN text of the account replies
// (account.toml) as users see them, so a wording change shows up as a
// reviewable diff in testdata/replies.zh-CN.golden. Regenerate with
// HARUKI_UPDATE_GOLDEN=1 go test ./internal/pjsk/accountdata/.
func TestAccountRepliesGolden(t *testing.T) {
	bgPath := "bg.jpg"
	jpDefault := BindingListItem{Index: 1, Server: "jp", UserID: "7487590788965145370", Verified: true, IsGlobalDefault: true, IsServerDefault: true, SuiteVisible: true, MySekaiVisible: true,
		Bg: &drawing.ProfileBgSettings{ImgPath: &bgPath, Blur: 5, Alpha: 70, Vertical: true}}
	cnVisible := BindingListItem{Index: 1, Server: "cn", UserID: "123456789", Visibility: UniformVisibility(true), IsServerDefault: true, SuiteVisible: true, MySekaiVisible: true}
	twPlain := BindingListItem{Index: 1, Server: "tw", UserID: "987654321"}
	items := []BindingListItem{jpDefault, cnVisible, twPlain}
	candidates := make([]string, 22)
	for i := range candidates {
		candidates[i] = "Etc/Zone" + strings.Repeat("x", i%3)
	}
	rows := []struct{ name, got string }{
		{"bind list (none)", formatBindingListText(nil, "")},
		{"bind list (none in region)", formatBindingListText(nil, "kr")},
		{"bind list", formatBindingListText(items, "")},
		{"bind list (region)", formatBindingListText([]BindingListItem{jpDefault}, "jp")},
		{"bind", formatBindResultText(&BindResult{Server: "jp", UserID: "7487590788965145370", UserName: "ほのか", AlreadyBound: true, SetGlobalDefault: true, SetServerDefault: true, MultipleServerMatch: true})},
		{"unbind", formatUnbindResultText(&UnbindResult{Removed: twPlain, ReassignedGlobal: &cnVisible, ReassignedServer: &jpDefault})},
		{"default set (global)", formatDefaultBindingSetText(&DefaultBindingResult{Scope: DefaultScopeGlobal, Binding: cnVisible})},
		{"default set (region)", formatDefaultBindingSetText(&DefaultBindingResult{Scope: DefaultScopeServer, Server: "jp", Binding: jpDefault})},
		{"default cleared (global)", formatDefaultBindingClearedText(&DefaultBindingResult{Scope: DefaultScopeGlobal, Binding: cnVisible})},
		{"default cleared (region)", formatDefaultBindingClearedText(&DefaultBindingResult{Scope: DefaultScopeServer, Server: "jp", Binding: jpDefault})},
		{"swap (region)", formatBindingSwapResultText("u1", "u2", "jp", []BindingListItem{jpDefault})},
		{"verify list", formatVerifyListText(items, "")},
		{"verify list (region)", formatVerifyListText([]BindingListItem{jpDefault}, "jp")},
		{"hide uid", profileVisibilityResultText(ProfileModeHideID, jpDefault)},
		{"show uid", profileVisibilityResultText(ProfileModeShowID, cnVisible)},
		{"hide suite", profileVisibilityResultText(ProfileModeHideSuite, withSuite(jpDefault, false, true))},
		{"show suite", profileVisibilityResultText(ProfileModeShowSuite, jpDefault)},
		{"hide mysekai", profileVisibilityResultText(ProfileModeHideMySekai, withSuite(jpDefault, true, false))},
		{"show mysekai", profileVisibilityResultText(ProfileModeShowMySekai, jpDefault)},
		{"hide sk", profileVisibilityResultText(ProfileModeHideSK, cnVisible.withVisibility(cnVisible.Visibility.With(ExposureSK, false)))},
		{"show sk", profileVisibilityResultText(ProfileModeShowSK, jpDefault.withVisibility(jpDefault.Visibility.With(ExposureSK, true)))},
		{"hide profile", profileVisibilityResultText(ProfileModeHideInfo, cnVisible.withVisibility(cnVisible.Visibility.With(ExposureProfile, false)))},
		{"show profile", profileVisibilityResultText(ProfileModeShowInfo, jpDefault.withVisibility(jpDefault.Visibility.With(ExposureProfile, true)))},
		{"hide arrest", profileVisibilityResultText(ProfileModeHideArrest, cnVisible.withVisibility(cnVisible.Visibility.With(ExposureArrest, false)))},
		{"show arrest", profileVisibilityResultText(ProfileModeShowArrest, jpDefault.withVisibility(jpDefault.Visibility.With(ExposureArrest, true)))},
		{"hide all", profileVisibilityResultText(ProfileModeHideAll, jpDefault)},
		{"show all", profileVisibilityResultText(ProfileModeShowAll, cnVisible)},
		{"privacy settings", profileVisibilityResultText(ProfileModeVisibility, BindingListItem{Index: 2, Server: "jp", UserID: "123456789", Visibility: Visibility{UID: false, SK: true, Profile: true, Arrest: false}, SuiteVisible: true, MySekaiVisible: false})},
		{"arrest difficulty", formatProfileDifficultySummary([]sekaiapi.MusicDifficultyType{sekaiapi.MusicDifficultyExpert, sekaiapi.MusicDifficultyMaster})},
		{"arrest difficulty (none)", formatProfileDifficultySummary(nil)},
		{"bg settings", formatProfileBGSettingsText(jpDefault)},
		{"bg settings (none)", formatProfileBGSettingsText(twPlain)},
		{"timezone candidates", formatTimeZoneCandidatesText(candidates)},
	}
	var b strings.Builder
	for _, row := range rows {
		b.WriteString("== " + row.name + "\n" + row.got + "\n")
	}
	compareRepliesGolden(t, b.String())
}

func compareRepliesGolden(t *testing.T, got string) {
	t.Helper()
	path := filepath.Join("testdata", "replies.zh-CN.golden")
	if os.Getenv("HARUKI_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (regenerate with HARUKI_UPDATE_GOLDEN=1)", path, err)
	}
	if string(want) != got {
		t.Fatalf("%s is out of date; review the diff and regenerate with HARUKI_UPDATE_GOLDEN=1\n--- got ---\n%s", path, got)
	}
}

func (item BindingListItem) withVisibility(v Visibility) BindingListItem {
	item.Visibility = v
	return item
}

func withSuite(item BindingListItem, suite, mySekai bool) BindingListItem {
	item.SuiteVisible, item.MySekaiVisible = suite, mySekai
	return item
}
