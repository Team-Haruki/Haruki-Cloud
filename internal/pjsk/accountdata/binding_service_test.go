package accountdata_test

import (
	"context"
	"strings"
	"testing"

	pjskenttest "haruki-cloud/database/pjsk/enttest"
	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/identity"
	"haruki-cloud/internal/pjsk/accountdata"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"

	_ "github.com/mattn/go-sqlite3"
)

type fakeProfileValidator struct {
	profiles map[string]map[string]string
}

func (f *fakeProfileValidator) GetUserProfile(server, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
	users := f.profiles[server]
	if users == nil {
		return nil, sekaiapi.ErrUserNotFound
	}
	name, ok := users[userID]
	if !ok {
		return nil, sekaiapi.ErrUserNotFound
	}
	return &sekaiapi.GetAnotherProfileResponse{
		User: sekaiapi.AnotherUser{
			UserID: 1,
			Name:   name,
		},
	}, nil
}

func TestBindingServiceBannedGameAccountAttemptsBanAfterThirdWarning(t *testing.T) {
	pjskClient := pjskenttest.Open(t, "sqlite3", "file:pjsk_banned_account_bind_test?mode=memory&cache=shared&_fk=1")
	defer pjskClient.Close()
	usersClient := usersenttest.Open(t, "sqlite3", "file:users_banned_account_bind_test?mode=memory&cache=shared&_fk=1")
	defer usersClient.Close()

	service := accountdata.NewBindingService(
		pjskClient,
		identity.NewResolver(usersClient),
		&fakeProfileValidator{
			profiles: map[string]map[string]string{
				"jp": {"9000": "Banned User"},
			},
		},
	)
	service.SetUsersDB(usersClient)

	ctx := context.Background()
	if _, err := pjskClient.GameAccount.Create().
		SetServer("jp").
		SetUserID("9000").
		SetIsBanned(true).
		Save(ctx); err != nil {
		t.Fatalf("create banned game account: %v", err)
	}

	const warning = "你正在尝试绑定已被封禁用户，请不要再次尝试"
	for attempt := 1; attempt <= 3; attempt++ {
		_, err := service.Bind(ctx, "qq", "42", "9000")
		if err == nil {
			t.Fatalf("attempt %d unexpectedly succeeded", attempt)
		}
		switch {
		case attempt < 3 && err.Error() != warning:
			t.Fatalf("attempt %d expected warning, got %q", attempt, err.Error())
		case attempt == 3 && !strings.Contains(err.Error(), "您已被禁止使用PJSK 功能，原因：多次尝试绑定被封禁游戏账号"):
			t.Fatalf("attempt %d expected ban message, got %q", attempt, err.Error())
		}
	}

	u, err := usersClient.User.Query().Only(ctx)
	if err != nil {
		t.Fatalf("query user: %v", err)
	}
	if u.PjskBannedGameAccountBindAttempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", u.PjskBannedGameAccountBindAttempts)
	}
	if !u.PjskBanState || u.PjskBanReason != "多次尝试绑定被封禁游戏账号" {
		t.Fatalf("expected pjsk ban state and reason, got state=%v reason=%q", u.PjskBanState, u.PjskBanReason)
	}

	count, err := pjskClient.UserBinding.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count bindings: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no binding to be created, got %d", count)
	}
}

func TestBindingServiceBannedGameAccountIgnoresExistingBinding(t *testing.T) {
	pjskClient := pjskenttest.Open(t, "sqlite3", "file:pjsk_existing_banned_account_bind_test?mode=memory&cache=shared&_fk=1")
	defer pjskClient.Close()
	usersClient := usersenttest.Open(t, "sqlite3", "file:users_existing_banned_account_bind_test?mode=memory&cache=shared&_fk=1")
	defer usersClient.Close()

	service := accountdata.NewBindingService(
		pjskClient,
		identity.NewResolver(usersClient),
		&fakeProfileValidator{
			profiles: map[string]map[string]string{
				"jp": {"9001": "Existing User"},
			},
		},
	)
	service.SetUsersDB(usersClient)

	ctx := context.Background()
	if _, err := service.Bind(ctx, "qq", "42", "9001"); err != nil {
		t.Fatalf("initial bind: %v", err)
	}
	if _, err := pjskClient.GameAccount.Update().
		SetIsBanned(true).
		Save(ctx); err != nil {
		t.Fatalf("mark account banned: %v", err)
	}

	result, err := service.Bind(ctx, "qq", "42", "9001")
	if err != nil {
		t.Fatalf("rebind existing banned account: %v", err)
	}
	if !result.AlreadyBound {
		t.Fatalf("expected existing binding result, got %+v", result)
	}

	u, err := usersClient.User.Query().Only(ctx)
	if err != nil {
		t.Fatalf("query user: %v", err)
	}
	if u.PjskBannedGameAccountBindAttempts != 0 || u.PjskBanState {
		t.Fatalf("expected existing binding to be ignored, got attempts=%d banned=%v", u.PjskBannedGameAccountBindAttempts, u.PjskBanState)
	}
}

func TestBindingServiceBannedGameAccountCountsAfterUnbind(t *testing.T) {
	pjskClient := pjskenttest.Open(t, "sqlite3", "file:pjsk_unbound_banned_account_bind_test?mode=memory&cache=shared&_fk=1")
	defer pjskClient.Close()
	usersClient := usersenttest.Open(t, "sqlite3", "file:users_unbound_banned_account_bind_test?mode=memory&cache=shared&_fk=1")
	defer usersClient.Close()

	service := accountdata.NewBindingService(
		pjskClient,
		identity.NewResolver(usersClient),
		&fakeProfileValidator{
			profiles: map[string]map[string]string{
				"jp": {"9002": "Unbound User"},
			},
		},
	)
	service.SetUsersDB(usersClient)

	ctx := context.Background()
	if _, err := service.Bind(ctx, "qq", "42", "9002"); err != nil {
		t.Fatalf("initial bind: %v", err)
	}
	if _, err := service.Unbind(ctx, "qq", "42", "9002", ""); err != nil {
		t.Fatalf("unbind: %v", err)
	}
	if _, err := pjskClient.GameAccount.Update().
		SetIsBanned(true).
		Save(ctx); err != nil {
		t.Fatalf("mark account banned: %v", err)
	}

	_, err := service.Bind(ctx, "qq", "42", "9002")
	if err == nil {
		t.Fatalf("rebind unbound banned account unexpectedly succeeded")
	}
	const warning = "你正在尝试绑定已被封禁用户，请不要再次尝试"
	if err.Error() != warning {
		t.Fatalf("expected warning, got %q", err.Error())
	}

	u, err := usersClient.User.Query().Only(ctx)
	if err != nil {
		t.Fatalf("query user: %v", err)
	}
	if u.PjskBannedGameAccountBindAttempts != 1 || u.PjskBanState {
		t.Fatalf("expected one warning attempt without ban, got attempts=%d banned=%v", u.PjskBannedGameAccountBindAttempts, u.PjskBanState)
	}
}

func TestBindingServiceBindListAndDefaultSwitch(t *testing.T) {
	pjskClient := pjskenttest.Open(t, "sqlite3", "file:pjsk_bind_test?mode=memory&cache=shared&_fk=1")
	defer pjskClient.Close()
	usersClient := usersenttest.Open(t, "sqlite3", "file:users_bind_test?mode=memory&cache=shared&_fk=1")
	defer usersClient.Close()

	service := accountdata.NewBindingService(
		pjskClient,
		identity.NewResolver(usersClient),
		&fakeProfileValidator{
			profiles: map[string]map[string]string{
				"jp": {"2000": "JP User"},
				"cn": {"1000": "CN User"},
			},
		},
	)

	ctx := context.Background()

	first, err := service.Bind(ctx, "qq", "42", "2000")
	if err != nil {
		t.Fatalf("bind first: %v", err)
	}
	if !first.SetGlobalDefault || !first.SetServerDefault {
		t.Fatalf("expected first bind to set both defaults, got %+v", first)
	}

	second, err := service.Bind(ctx, "qq", "42", "1000")
	if err != nil {
		t.Fatalf("bind second: %v", err)
	}
	if second.Server != "cn" {
		t.Fatalf("expected second bind on cn, got %+v", second)
	}

	items, err := service.List(ctx, "qq", "42")
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(items))
	}
	if items[0].Visible || items[1].Visible {
		t.Fatalf("expected new bindings to hide uid by default, got %+v", items)
	}
	if items[0].UserID != "2000" || items[1].UserID != "1000" {
		t.Fatalf("expected bindings sorted by bind order, got %+v", items)
	}
	if !items[0].IsGlobalDefault || !items[0].IsServerDefault {
		t.Fatalf("expected JP binding to keep defaults, got %+v", items[0])
	}

	result, err := service.SetDefault(ctx, "qq", "42", "u1", "cn", "")
	if err != nil {
		t.Fatalf("set global default: %v", err)
	}
	if result.Scope != accountdata.DefaultScopeGlobal || result.Binding.UserID != "1000" {
		t.Fatalf("unexpected set default result: %+v", result)
	}

	items, err = service.List(ctx, "qq", "42")
	if err != nil {
		t.Fatalf("list after set default: %v", err)
	}
	if items[0].IsGlobalDefault || !items[1].IsGlobalDefault {
		t.Fatalf("expected CN binding to become global default, got %+v", items)
	}
}

func TestBindingServiceUnbindReassignsDefaults(t *testing.T) {
	pjskClient := pjskenttest.Open(t, "sqlite3", "file:pjsk_unbind_test?mode=memory&cache=shared&_fk=1")
	defer pjskClient.Close()
	usersClient := usersenttest.Open(t, "sqlite3", "file:users_unbind_test?mode=memory&cache=shared&_fk=1")
	defer usersClient.Close()

	service := accountdata.NewBindingService(
		pjskClient,
		identity.NewResolver(usersClient),
		&fakeProfileValidator{
			profiles: map[string]map[string]string{
				"jp": {
					"2000": "JP A",
					"3000": "JP B",
				},
			},
		},
	)

	ctx := context.Background()
	if _, err := service.Bind(ctx, "qq", "99", "2000"); err != nil {
		t.Fatalf("bind first: %v", err)
	}
	if _, err := service.Bind(ctx, "qq", "99", "3000"); err != nil {
		t.Fatalf("bind second: %v", err)
	}

	result, err := service.Unbind(ctx, "qq", "99", "2000", "")
	if err != nil {
		t.Fatalf("unbind: %v", err)
	}
	if result.ReassignedGlobal == nil || result.ReassignedGlobal.UserID != "3000" {
		t.Fatalf("expected global default reassigned to 3000, got %+v", result)
	}
	if result.ReassignedServer == nil || result.ReassignedServer.UserID != "3000" {
		t.Fatalf("expected server default reassigned to 3000, got %+v", result)
	}

	items, err := service.List(ctx, "qq", "99")
	if err != nil {
		t.Fatalf("list after unbind: %v", err)
	}
	if len(items) != 1 || items[0].UserID != "3000" || !items[0].IsGlobalDefault || !items[0].IsServerDefault {
		t.Fatalf("unexpected bindings after unbind: %+v", items)
	}
	if items[0].Visible {
		t.Fatalf("expected remaining binding to stay hidden by default, got %+v", items[0])
	}
}

func TestBindingServiceSwapUsesPersistentDisplayOrder(t *testing.T) {
	pjskClient := pjskenttest.Open(t, "sqlite3", "file:pjsk_swap_test?mode=memory&cache=shared&_fk=1")
	defer pjskClient.Close()
	usersClient := usersenttest.Open(t, "sqlite3", "file:users_swap_test?mode=memory&cache=shared&_fk=1")
	defer usersClient.Close()

	service := accountdata.NewBindingService(
		pjskClient,
		identity.NewResolver(usersClient),
		&fakeProfileValidator{
			profiles: map[string]map[string]string{
				"jp": {"2000": "JP A", "3000": "JP B"},
				"cn": {"1000": "CN A"},
			},
		},
	)

	ctx := context.Background()
	if _, err := service.Bind(ctx, "qq", "77", "2000"); err != nil {
		t.Fatalf("bind jp first: %v", err)
	}
	if _, err := service.Bind(ctx, "qq", "77", "1000"); err != nil {
		t.Fatalf("bind cn: %v", err)
	}
	if _, err := service.Bind(ctx, "qq", "77", "3000"); err != nil {
		t.Fatalf("bind jp second: %v", err)
	}

	items, err := service.Swap(ctx, "qq", "77", "u1", "u3", "")
	if err != nil {
		t.Fatalf("swap bindings: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 bindings after swap, got %d", len(items))
	}
	if items[0].UserID != "3000" || items[1].UserID != "1000" || items[2].UserID != "2000" {
		t.Fatalf("unexpected swapped order: %+v", items)
	}

	items, err = service.List(ctx, "qq", "77")
	if err != nil {
		t.Fatalf("list after swap: %v", err)
	}
	if items[0].UserID != "3000" || items[1].UserID != "1000" || items[2].UserID != "2000" {
		t.Fatalf("expected swapped order to persist, got %+v", items)
	}
}
