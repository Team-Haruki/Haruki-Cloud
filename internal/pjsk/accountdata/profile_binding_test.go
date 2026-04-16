package accountdata_test

import (
	"context"
	"encoding/json"
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
		"server": " jp ",
		"scope": " jp "
	}`))
	if err != nil {
		t.Fatalf("decode params: %v", err)
	}

	if params.Platform != "qq" || params.PlatformUserID != "42" || params.Selector != "u1" || params.Server != "jp" || params.Scope != "jp" {
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

	expectedBind := "JP服绑定成功: JP User (2000)\n已自动设为你的全局默认绑定\n已自动设为你的JP服默认绑定"
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

	expectedList := "已绑定账号列表（u序号全局编号）:\nu1 [JP] 2000 (全局默认 / JP服默认)"
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

	expectedList := "已绑定CN服账号列表（u序号按该区服编号）:\nu1 [CN] 3000 (CN服默认)"
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

	expectedList := "你还没有绑定任何CN服PJSK账号"
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

	expectedList := "已绑定账号列表（u序号全局编号）:\nu1 [JP] 123********234 (全局默认 / JP服默认)"
	if string(listText) != expectedList {
		t.Fatalf("unexpected masked list text:\n%s", string(listText))
	}
}
