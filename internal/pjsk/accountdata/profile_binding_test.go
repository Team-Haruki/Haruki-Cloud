package accountdata_test

import (
	"context"
	"encoding/json"
	"haruki-cloud/internal/i18n"
	"strings"
	"testing"

	pjskenttest "haruki-cloud/database/pjsk/enttest"
	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/identity"
	"haruki-cloud/internal/pjsk/accountdata"

	_ "github.com/mattn/go-sqlite3"
)

func newProfileBindingTestService(t *testing.T, profiles map[string]map[string]string) *accountdata.BindingService {
	t.Helper()

	pjskClient := pjskenttest.Open(t, "sqlite3", "file:pjsk_profile_binding_test?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = pjskClient.Close() })

	usersClient := usersenttest.Open(t, "sqlite3", "file:users_profile_binding_test?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = usersClient.Close() })

	return accountdata.NewBindingService(
		pjskClient,
		identity.NewResolver(usersClient),
		&fakeProfileValidator{profiles: profiles},
	)
}

func TestDecodeProfileBindingParams(t *testing.T) {
	params, err := accountdata.DecodeProfileBindingParams(json.RawMessage(`{
		"platform": " qq ",
		"platform_user_id": " 42 ",
		"selector": " u1 ",
		"selector_other": " u2 ",
		"server": " jp ",
		"scope": " jp "
	}`))
	if err != nil {
		t.Fatalf("decode params: %v", err)
	}

	if params.Platform != "qq" || params.PlatformUserID != "42" || params.Selector != "u1" || params.SelectorOther != "u2" || params.Server != "jp" || params.Scope != "jp" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestExecuteProfileBindingCommandBindAndList(t *testing.T) {
	service := newProfileBindingTestService(t, map[string]map[string]string{
		"jp": {"2000": "JP User"},
	})

	ctx := context.Background()

	bindText, err := accountdata.ExecuteProfileBindingCommand(ctx, service, accountdata.ProfileModeBind, accountdata.ProfileBindingCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Selector:       "2000",
	})
	if err != nil {
		t.Fatalf("execute bind: %v", err)
	}

	expectedBind := i18n.T("account.bind.done", i18n.Data{"Account": i18n.AccountLabel("jp", "2000", false), "Name": "JP User"}) + "\n" +
		i18n.T("account.bind.note_global_default") + "\n" +
		i18n.T("account.bind.note_region_default", i18n.Data{"Region": i18n.RegionLabel("jp")})
	if string(bindText) != expectedBind {
		t.Fatalf("unexpected bind text:\n%s", string(bindText))
	}

	listText, err := accountdata.ExecuteProfileBindingCommand(ctx, service, accountdata.ProfileModeBindList, accountdata.ProfileBindingCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
	})
	if err != nil {
		t.Fatalf("execute bind list: %v", err)
	}

	expectedList := i18n.T("account.list.header") + "\n" + listItem(1, "jp", "2000", true, "jp")
	if string(listText) != expectedList {
		t.Fatalf("unexpected list text:\n%s", string(listText))
	}
}

func TestExecuteProfileBindingCommandBindListFiltersByServer(t *testing.T) {
	service := newProfileBindingTestService(t, map[string]map[string]string{
		"jp": {"2000": "JP User"},
		"cn": {"3000": "CN User"},
	})

	ctx := context.Background()

	if _, err := accountdata.ExecuteProfileBindingCommand(ctx, service, accountdata.ProfileModeBind, accountdata.ProfileBindingCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Selector:       "2000",
	}); err != nil {
		t.Fatalf("bind jp: %v", err)
	}
	if _, err := accountdata.ExecuteProfileBindingCommand(ctx, service, accountdata.ProfileModeBind, accountdata.ProfileBindingCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Selector:       "3000",
	}); err != nil {
		t.Fatalf("bind cn: %v", err)
	}

	listText, err := accountdata.ExecuteProfileBindingCommand(ctx, service, accountdata.ProfileModeBindList, accountdata.ProfileBindingCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Server:         "cn",
	})
	if err != nil {
		t.Fatalf("execute bind list with server filter: %v", err)
	}

	expectedList := i18n.T("account.list.header_region", i18n.Data{"Region": i18n.RegionLabel("cn")}) + "\n" + listItem(1, "cn", "3000", false, "cn")
	if string(listText) != expectedList {
		t.Fatalf("unexpected filtered list text:\n%s", string(listText))
	}
}

func TestExecuteProfileBindingCommandBindListFiltersByServerWhenEmpty(t *testing.T) {
	service := newProfileBindingTestService(t, map[string]map[string]string{
		"jp": {"2000": "JP User"},
	})

	ctx := context.Background()

	if _, err := accountdata.ExecuteProfileBindingCommand(ctx, service, accountdata.ProfileModeBind, accountdata.ProfileBindingCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Selector:       "2000",
	}); err != nil {
		t.Fatalf("bind jp: %v", err)
	}

	listText, err := accountdata.ExecuteProfileBindingCommand(ctx, service, accountdata.ProfileModeBindList, accountdata.ProfileBindingCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Server:         "cn",
	})
	if err != nil {
		t.Fatalf("execute bind list with empty server filter: %v", err)
	}

	expectedList := i18n.T("binding.none_in_region", i18n.Data{"Region": i18n.RegionLabel("cn")})
	if string(listText) != expectedList {
		t.Fatalf("unexpected empty filtered list text:\n%s", string(listText))
	}
}

func TestExecuteProfileBindingCommandBindListMasksUIDByDefault(t *testing.T) {
	service := newProfileBindingTestService(t, map[string]map[string]string{
		"jp": {"12345678901234": "JP User"},
	})

	ctx := context.Background()

	if _, err := accountdata.ExecuteProfileBindingCommand(ctx, service, accountdata.ProfileModeBind, accountdata.ProfileBindingCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Selector:       "12345678901234",
	}); err != nil {
		t.Fatalf("execute bind: %v", err)
	}

	listText, err := accountdata.ExecuteProfileBindingCommand(ctx, service, accountdata.ProfileModeBindList, accountdata.ProfileBindingCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
	})
	if err != nil {
		t.Fatalf("execute bind list: %v", err)
	}

	expectedList := i18n.T("account.list.header") + "\n" + listItem(1, "jp", "123********234", true, "jp")
	if string(listText) != expectedList {
		t.Fatalf("unexpected masked list text:\n%s", string(listText))
	}
}

func TestExecuteProfileBindingCommandQueryUIDIgnoresHiddenID(t *testing.T) {
	service := newProfileBindingTestService(t, map[string]map[string]string{
		"jp": {"12345678901234": "JP User"},
	})

	ctx := context.Background()

	if _, err := accountdata.ExecuteProfileBindingCommand(ctx, service, accountdata.ProfileModeBind, accountdata.ProfileBindingCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Selector:       "12345678901234",
	}); err != nil {
		t.Fatalf("execute bind: %v", err)
	}

	uidText, err := accountdata.ExecuteProfileBindingCommand(ctx, service, accountdata.ProfileModeQueryUID, accountdata.ProfileBindingCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
	})
	if err != nil {
		t.Fatalf("execute query uid: %v", err)
	}

	if string(uidText) != "12345678901234" {
		t.Fatalf("unexpected uid text: %q", string(uidText))
	}
}

func TestExecuteProfileBindingCommandQueryUIDUsesSelector(t *testing.T) {
	service := newProfileBindingTestService(t, map[string]map[string]string{
		"jp": {"11111111111111": "JP User"},
		"en": {"22222222222222": "EN User"},
	})

	ctx := context.Background()

	for _, uid := range []string{"11111111111111", "22222222222222"} {
		if _, err := accountdata.ExecuteProfileBindingCommand(ctx, service, accountdata.ProfileModeBind, accountdata.ProfileBindingCommandParams{
			Platform:       "qq",
			PlatformUserID: "42",
			Selector:       uid,
		}); err != nil {
			t.Fatalf("execute bind %s: %v", uid, err)
		}
	}

	uidText, err := accountdata.ExecuteProfileBindingCommand(ctx, service, accountdata.ProfileModeQueryUID, accountdata.ProfileBindingCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Selector:       "u2",
	})
	if err != nil {
		t.Fatalf("execute query uid: %v", err)
	}

	if string(uidText) != "22222222222222" {
		t.Fatalf("unexpected uid text: %q", string(uidText))
	}
}

func TestExecuteProfileBindingCommandSwap(t *testing.T) {
	service := newProfileBindingTestService(t, map[string]map[string]string{
		"jp": {"2000": "JP User"},
		"cn": {"3000": "CN User"},
	})

	ctx := context.Background()
	if _, err := accountdata.ExecuteProfileBindingCommand(ctx, service, accountdata.ProfileModeBind, accountdata.ProfileBindingCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Selector:       "2000",
	}); err != nil {
		t.Fatalf("bind jp: %v", err)
	}
	if _, err := accountdata.ExecuteProfileBindingCommand(ctx, service, accountdata.ProfileModeBind, accountdata.ProfileBindingCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Selector:       "3000",
	}); err != nil {
		t.Fatalf("bind cn: %v", err)
	}

	swapText, err := accountdata.ExecuteProfileBindingCommand(ctx, service, accountdata.ProfileModeBindSwap, accountdata.ProfileBindingCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
		Selector:       "u1",
		SelectorOther:  "u2",
	})
	if err != nil {
		t.Fatalf("execute swap: %v", err)
	}

	expected := i18n.T("account.swap.done", i18n.Data{"Left": "u1", "Right": "u2", "List": i18n.T("account.list.header") + "\n" +
		listItem(1, "cn", "3000", false, "cn") + "\n" + listItem(2, "jp", "2000", true, "jp")})
	if string(swapText) != expected {
		t.Fatalf("unexpected swap text:\n%s", string(swapText))
	}
}

// listItem renders one binding or verification list line: index, the
// account label with uid shown as given, and the default-binding marks.
func listItem(index int, region, uid string, globalDefault bool, defaultRegion string) string {
	account := i18n.AccountLabel(region, uid, true)
	var marks []string
	if globalDefault {
		marks = append(marks, i18n.T("account.mark.global_default"))
	}
	if defaultRegion != "" {
		marks = append(marks, i18n.T("account.mark.region_default", i18n.Data{"Region": i18n.RegionLabel(defaultRegion)}))
	}
	if len(marks) == 0 {
		return i18n.T("account.list.item", i18n.Data{"Index": index, "Account": account})
	}
	return i18n.T("account.list.item_marked", i18n.Data{"Index": index, "Account": account, "Marks": strings.Join(marks, "、")})
}

// verifyItem renders one verification list line like listItem, with the
// verification status.
func verifyItem(index int, region, uid string, verified, globalDefault bool, defaultRegion string) string {
	status := i18n.M("account.verify_list.unverified")
	if verified {
		status = i18n.M("account.verify_list.verified")
	}
	account := i18n.AccountLabel(region, uid, true)
	var marks []string
	if globalDefault {
		marks = append(marks, i18n.T("account.mark.global_default"))
	}
	if defaultRegion != "" {
		marks = append(marks, i18n.T("account.mark.region_default", i18n.Data{"Region": i18n.RegionLabel(defaultRegion)}))
	}
	if len(marks) == 0 {
		return i18n.T("account.verify_list.item", i18n.Data{"Index": index, "Account": account, "Status": status})
	}
	return i18n.T("account.verify_list.item_marked", i18n.Data{"Index": index, "Account": account, "Status": status, "Marks": strings.Join(marks, "、")})
}
