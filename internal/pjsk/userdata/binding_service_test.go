package userdata_test

import (
	"context"
	"testing"

	pjskenttest "haruki-cloud/database/pjsk/enttest"
	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/identity"
	"haruki-cloud/internal/pjsk/userdata"
	sekaiapi "haruki-cloud/utils/sekai"

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

func TestBindingServiceBindListAndDefaultSwitch(t *testing.T) {
	pjskClient := pjskenttest.Open(t, "sqlite3", "file:pjsk_bind_test?mode=memory&cache=shared&_fk=1")
	defer pjskClient.Close()
	usersClient := usersenttest.Open(t, "sqlite3", "file:users_bind_test?mode=memory&cache=shared&_fk=1")
	defer usersClient.Close()

	service := userdata.NewBindingService(
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
	if items[0].UserID != "1000" || items[1].UserID != "2000" {
		t.Fatalf("expected bindings sorted by uid asc, got %+v", items)
	}
	if items[0].IsGlobalDefault {
		t.Fatalf("expected global default to remain on first bound account, got %+v", items)
	}
	if !items[1].IsGlobalDefault || !items[1].IsServerDefault {
		t.Fatalf("expected JP binding to keep defaults, got %+v", items[1])
	}

	result, err := service.SetDefault(ctx, "qq", "42", "u1", "cn", "")
	if err != nil {
		t.Fatalf("set global default: %v", err)
	}
	if result.Scope != userdata.DefaultScopeGlobal || result.Binding.UserID != "1000" {
		t.Fatalf("unexpected set default result: %+v", result)
	}

	items, err = service.List(ctx, "qq", "42")
	if err != nil {
		t.Fatalf("list after set default: %v", err)
	}
	if !items[0].IsGlobalDefault || items[1].IsGlobalDefault {
		t.Fatalf("expected CN binding to become global default, got %+v", items)
	}
}

func TestBindingServiceUnbindReassignsDefaults(t *testing.T) {
	pjskClient := pjskenttest.Open(t, "sqlite3", "file:pjsk_unbind_test?mode=memory&cache=shared&_fk=1")
	defer pjskClient.Close()
	usersClient := usersenttest.Open(t, "sqlite3", "file:users_unbind_test?mode=memory&cache=shared&_fk=1")
	defer usersClient.Close()

	service := userdata.NewBindingService(
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
}
