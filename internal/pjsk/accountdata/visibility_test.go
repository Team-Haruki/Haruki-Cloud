package accountdata

import (
	"context"
	"strings"
	"testing"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/internal/i18n"
)

func createLegacyBinding(t *testing.T, ctx context.Context, client *pjskdb.Client, harukiUserID int, uid string, visible bool) *pjskdb.UserBinding {
	t.Helper()
	account, err := client.GameAccount.Create().SetServer("jp").SetUserID(uid).Save(ctx)
	if err != nil {
		t.Fatalf("create account %s: %v", uid, err)
	}
	// A row as the binary before the split wrote it: only visible is set.
	binding, err := client.UserBinding.Create().SetHarukiUserID(harukiUserID).SetGameAccountID(account.ID).SetVisible(visible).Save(ctx)
	if err != nil {
		t.Fatalf("create binding %s: %v", uid, err)
	}
	return binding
}

func reloadVisibility(t *testing.T, ctx context.Context, client *pjskdb.Client, id int) (*pjskdb.UserBinding, Visibility) {
	t.Helper()
	row, err := client.UserBinding.Get(ctx, id)
	if err != nil {
		t.Fatalf("reload binding %d: %v", id, err)
	}
	return row, bindingVisibility(row)
}

// TestBackfillBindingVisibilityCopiesLegacyFlag: a binding that was hidden
// with the single flag ends up hidden for every exposure, a shown one shown
// for every exposure, a flag already set is kept, and a second run changes
// nothing.
func TestBackfillBindingVisibilityCopiesLegacyFlag(t *testing.T) {
	ctx := context.Background()
	_, client := openAccountCoverageService(t, "visibility_backfill", accountCoverageValidator{})
	hidden := createLegacyBinding(t, ctx, client, 1, "1001", false)
	shown := createLegacyBinding(t, ctx, client, 2, "1002", true)
	partial := createLegacyBinding(t, ctx, client, 3, "1003", false)
	if _, err := client.UserBinding.UpdateOneID(partial.ID).SetUIDVisible(true).Save(ctx); err != nil {
		t.Fatalf("set partial flag: %v", err)
	}

	// Before the backfill, NULL flags read as the legacy flag.
	if _, v := reloadVisibility(t, ctx, client, hidden.ID); v != UniformVisibility(false) {
		t.Fatalf("hidden before backfill = %+v", v)
	}
	if _, v := reloadVisibility(t, ctx, client, shown.ID); v != UniformVisibility(true) {
		t.Fatalf("shown before backfill = %+v", v)
	}

	updated, err := BackfillBindingVisibility(ctx, client)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if want := 4 + 4 + 3; updated != want {
		t.Fatalf("backfill updated %d column values, want %d", updated, want)
	}
	for _, tc := range []struct {
		id   int
		want Visibility
	}{
		{hidden.ID, UniformVisibility(false)},
		{shown.ID, UniformVisibility(true)},
		{partial.ID, Visibility{UID: true}},
	} {
		row, v := reloadVisibility(t, ctx, client, tc.id)
		if v != tc.want {
			t.Fatalf("binding %d after backfill = %+v, want %+v", tc.id, v, tc.want)
		}
		if row.UIDVisible == nil || row.SkVisible == nil || row.ProfileVisible == nil || row.ArrestVisible == nil {
			t.Fatalf("binding %d still has NULL flags: %+v", tc.id, row)
		}
	}
	if again, err := BackfillBindingVisibility(ctx, client); err != nil || again != 0 {
		t.Fatalf("second backfill = %d, %v", again, err)
	}
	if n, err := BackfillBindingVisibility(ctx, nil); err != nil || n != 0 {
		t.Fatalf("nil client backfill = %d, %v", n, err)
	}
}

// TestVisibilityCommandsChangeOnlyTheirExposure runs the visibility commands
// on one binding and checks each changes its own exposure, that the legacy
// visible column stays "all shown", and that the status command writes
// nothing.
func TestVisibilityCommandsChangeOnlyTheirExposure(t *testing.T) {
	ctx := context.Background()
	service, client := openAccountCoverageService(t, "visibility_modes", accountCoverageValidator{profiles: map[string]string{"jp": "JP Player"}})
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	items, err := service.List(ctx, "qq", "42")
	if err != nil || len(items) != 1 {
		t.Fatalf("list = %+v, %v", items, err)
	}
	bindingID := items[0].BindingID
	if items[0].Visibility != NewBindingVisibility {
		t.Fatalf("new binding visibility = %+v, want everything hidden", items[0].Visibility)
	}
	params := ProfileSettingsCommandParams{Platform: "qq", PlatformUserID: "42", Server: "jp", RegionExplicit: true}
	run := func(mode string) string {
		t.Helper()
		reply, err := ExecuteProfileSettingsCommand(ctx, service, mode, params)
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		return string(reply)
	}

	steps := []struct {
		mode string
		want Visibility
	}{
		{ProfileModeShowAll, UniformVisibility(true)},
		{ProfileModeHideID, Visibility{UID: false, SK: true, Profile: true, Arrest: true}},
		{ProfileModeHideSK, Visibility{UID: false, SK: false, Profile: true, Arrest: true}},
		{ProfileModeShowID, Visibility{UID: true, SK: false, Profile: true, Arrest: true}},
		{ProfileModeHideInfo, Visibility{UID: true, SK: false, Profile: false, Arrest: true}},
		{ProfileModeHideArrest, Visibility{UID: true, SK: false, Profile: false, Arrest: false}},
		{ProfileModeShowSK, Visibility{UID: true, SK: true, Profile: false, Arrest: false}},
		{ProfileModeShowInfo, Visibility{UID: true, SK: true, Profile: true, Arrest: false}},
		{ProfileModeShowArrest, UniformVisibility(true)},
		{ProfileModeHideAll, UniformVisibility(false)},
	}
	for _, step := range steps {
		reply := run(step.mode)
		row, got := reloadVisibility(t, ctx, client, bindingID)
		if got != step.want {
			t.Fatalf("after %s visibility = %+v, want %+v", step.mode, got, step.want)
		}
		if row.Visible != step.want.All() {
			t.Fatalf("after %s legacy visible = %v, want %v", step.mode, row.Visible, step.want.All())
		}
		if !strings.Contains(reply, i18n.T("account.visibility.state.hidden")) && step.want != UniformVisibility(true) {
			t.Fatalf("after %s reply lists no settings: %q", step.mode, reply)
		}
	}

	// Suite and MySekai keep their own toggles; /隐藏全部 does not touch them.
	if row, _ := reloadVisibility(t, ctx, client, bindingID); !row.SuiteVisible || !row.MysekaiVisible {
		t.Fatalf("hide all changed suite/mysekai: %+v", row)
	}

	before, _ := reloadVisibility(t, ctx, client, bindingID)
	status := run(ProfileModeVisibility)
	after, _ := reloadVisibility(t, ctx, client, bindingID)
	if before.Visible != after.Visible || *before.SkVisible != *after.SkVisible {
		t.Fatalf("status command changed the binding: %+v -> %+v", before, after)
	}
	for _, label := range []string{"游戏 UID", "活动排名", "个人信息", "逮捕信息", "抓包数据", "烤森数据"} {
		if !strings.Contains(status, label) {
			t.Fatalf("status reply misses %s: %q", label, status)
		}
	}
	if profileSettingsModeMutates(ProfileModeVisibility, params) {
		t.Fatal("the status command must not need a writable node")
	}
}

func TestVisibilityHelpers(t *testing.T) {
	v := UniformVisibility(true)
	for _, exposure := range Exposures {
		if !v.Allows(exposure) || v.With(exposure, false).Allows(exposure) || !v.With(exposure, false).With(exposure, true).All() {
			t.Fatalf("exposure %s", exposure)
		}
		if v.With(exposure, false).All() {
			t.Fatalf("All with %s hidden", exposure)
		}
	}
	if v.Allows("unknown") || v.With("unknown", false) != v {
		t.Fatal("unknown exposure")
	}
	if bindingVisibility(nil) != (Visibility{}) {
		t.Fatal("nil binding visibility")
	}
}
