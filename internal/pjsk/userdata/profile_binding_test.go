package userdata_test

import (
	"context"
	"encoding/json"
	"testing"

	pjskenttest "haruki-cloud/database/pjsk/enttest"
	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/identity"
	"haruki-cloud/internal/pjsk/userdata"

	_ "github.com/mattn/go-sqlite3"
)

func newProfileBindingTestService(t *testing.T, profiles map[string]map[string]string) *userdata.BindingService {
	t.Helper()

	pjskClient := pjskenttest.Open(t, "sqlite3", "file:pjsk_profile_binding_test?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = pjskClient.Close() })

	usersClient := usersenttest.Open(t, "sqlite3", "file:users_profile_binding_test?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = usersClient.Close() })

	return userdata.NewBindingService(
		pjskClient,
		identity.NewResolver(usersClient),
		&fakeProfileValidator{profiles: profiles},
	)
}

func TestDecodeProfileBindingParams(t *testing.T) {
	params, err := userdata.DecodeProfileBindingParams(json.RawMessage(`{
		"platform": " qq ",
		"platform_user_id": " 42 ",
		"selector": " u1 ",
		"scope": " jp "
	}`))
	if err != nil {
		t.Fatalf("decode params: %v", err)
	}

	if params.Platform != "qq" || params.PlatformUserID != "42" || params.Selector != "u1" || params.Scope != "jp" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestExecuteProfileBindingCommandBindAndList(t *testing.T) {
	service := newProfileBindingTestService(t, map[string]map[string]string{
		"jp": {"2000": "JP User"},
	})

	ctx := context.Background()

	bindText, err := userdata.ExecuteProfileBindingCommand(ctx, service, userdata.ProfileModeBind, userdata.ProfileBindingCommandParams{
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

	listText, err := userdata.ExecuteProfileBindingCommand(ctx, service, userdata.ProfileModeBindList, userdata.ProfileBindingCommandParams{
		Platform:       "qq",
		PlatformUserID: "42",
	})
	if err != nil {
		t.Fatalf("execute bind list: %v", err)
	}

	expectedList := "已绑定账号列表（按账号ID升序）:\nu1 [JP] 2000 (全局默认 / JP服默认)"
	if string(listText) != expectedList {
		t.Fatalf("unexpected list text:\n%s", string(listText))
	}
}
