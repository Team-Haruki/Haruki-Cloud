package subscription

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"haruki-cloud/config"
	pjskenttest "haruki-cloud/database/pjsk/enttest"
	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/identity"
	"haruki-cloud/internal/pjsk/accountdata"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"

	json "github.com/bytedance/sonic"
	_ "github.com/mattn/go-sqlite3"
)

func TestParseBirthdayMonitorCommandDefaultsToDiamond(t *testing.T) {
	cmd, err := ParseBirthdayMonitorCommand("/烤森生日监听")
	if err != nil {
		t.Fatalf("ParseBirthdayMonitorCommand returned error: %v", err)
	}
	if cmd.Cancel {
		t.Fatalf("expected monitor command")
	}
	if cmd.DurationMinutes != DefaultBirthdayMonitorMinutes {
		t.Fatalf("duration = %d, want %d", cmd.DurationMinutes, DefaultBirthdayMonitorMinutes)
	}
	if !slices.Equal(cmd.Materials, []string{"diamond"}) {
		t.Fatalf("materials = %+v, want [diamond]", cmd.Materials)
	}
}

func TestParseBirthdayMonitorCommandSupportsSelectorDurationAndMaterials(t *testing.T) {
	cmd, err := ParseBirthdayMonitorCommand("/ms生日监听 u2 120 夕桐 四叶草")
	if err != nil {
		t.Fatalf("ParseBirthdayMonitorCommand returned error: %v", err)
	}
	if cmd.Selector != "u2" {
		t.Fatalf("selector = %q, want u2", cmd.Selector)
	}
	if cmd.DurationMinutes != 120 {
		t.Fatalf("duration = %d, want 120", cmd.DurationMinutes)
	}
	if !slices.Equal(cmd.Materials, []string{"yuugiri", "clover"}) {
		t.Fatalf("materials = %+v, want [yuugiri clover]", cmd.Materials)
	}
}

func TestParseBirthdayMonitorCommandSupportsRegionPrefix(t *testing.T) {
	cmd, err := ParseBirthdayMonitorCommand("/jp烤森生日监听 u2 钻石 四叶草 10")
	if err != nil {
		t.Fatalf("ParseBirthdayMonitorCommand returned error: %v", err)
	}
	if !cmd.RegionExplicit || cmd.Region != "jp" {
		t.Fatalf("region = %q explicit=%t, want jp explicit", cmd.Region, cmd.RegionExplicit)
	}
	if cmd.Selector != "u2" {
		t.Fatalf("selector = %q, want u2", cmd.Selector)
	}
	if cmd.DurationMinutes != 10 {
		t.Fatalf("duration = %d, want 10", cmd.DurationMinutes)
	}
	if !slices.Equal(cmd.Materials, []string{"diamond", "clover"}) {
		t.Fatalf("materials = %+v, want [diamond clover]", cmd.Materials)
	}
}

func TestParseBirthdayMonitorCommandRejectsAllMaterialsDisabled(t *testing.T) {
	_, err := ParseBirthdayMonitorCommand("/烤森生日监听 钻石关闭")
	if err == nil || !strings.Contains(err.Error(), "至少需要开启一种监听材料") {
		t.Fatalf("error = %v, want all-disabled error", err)
	}
}

func TestParseBirthdayMonitorCommandRejectsDurationOverLimit(t *testing.T) {
	_, err := ParseBirthdayMonitorCommand("/烤森生日监听 121")
	if err == nil || !strings.Contains(err.Error(), "监听时长不能超过 120 分钟") {
		t.Fatalf("error = %v, want duration limit error", err)
	}
}

func TestParseBirthdayMonitorCancelCommand(t *testing.T) {
	cmd, err := ParseBirthdayMonitorCommand("/mysekai birthday unmonitor u1")
	if err != nil {
		t.Fatalf("ParseBirthdayMonitorCommand returned error: %v", err)
	}
	if !cmd.Cancel {
		t.Fatalf("expected cancel command")
	}
	if cmd.Selector != "u1" {
		t.Fatalf("selector = %q, want u1", cmd.Selector)
	}
}

func TestCreateOrUpdateUsesRegionPrefixedSelectorBinding(t *testing.T) {
	ctx := context.Background()
	pjskClient := pjskenttest.Open(t, "sqlite3", fmt.Sprintf("file:birthday_monitor_pjsk_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = pjskClient.Close() })
	usersClient := usersenttest.Open(t, "sqlite3", fmt.Sprintf("file:birthday_monitor_users_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = usersClient.Close() })

	bindings := accountdata.NewBindingService(
		pjskClient,
		identity.NewResolver(usersClient),
		birthdayMonitorProfileValidator{},
	)
	if _, err := bindings.Bind(ctx, "qq", "42", "11111111111111"); err != nil {
		t.Fatalf("bind first account: %v", err)
	}
	if _, err := bindings.Bind(ctx, "qq", "42", "22222222222222"); err != nil {
		t.Fatalf("bind second account: %v", err)
	}
	if err := pjskClient.UserBinding.Update().SetVerified(true).Exec(ctx); err != nil {
		t.Fatalf("mark bindings verified: %v", err)
	}

	var captured sekaiapi.MysekaiBirthdayMonitorUpsertRequest
	toolboxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.HasPrefix(r.URL.Path, "/internal/mysekai-birthday-monitors/") {
			http.NotFound(w, r)
			return
		}
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&captured); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer toolboxServer.Close()

	service := NewServiceWithToolbox(
		pjskClient,
		bindings,
		sekaiapi.NewToolboxClient(&config.ToolboxConfig{BaseURL: toolboxServer.URL, APIToken: "test"}),
	)
	result, err := service.CreateOrUpdate(ctx, "qq", "42", "100", "bot", "self", "", false, "/jp烤森生日监听 u2 钻石", false)
	if err != nil {
		t.Fatalf("CreateOrUpdate returned error: %v", err)
	}
	if result.Subscription.Region != "jp" || result.Subscription.UID != "22222222222222" {
		t.Fatalf("subscription target = %s/%s, want jp/22222222222222", result.Subscription.Region, result.Subscription.UID)
	}
	if captured.Region != "jp" || captured.UID != "22222222222222" {
		t.Fatalf("toolbox target = %s/%s, want jp/22222222222222", captured.Region, captured.UID)
	}
}

func TestCreateOrUpdateRejectsUnverifiedBindingWithGuidance(t *testing.T) {
	ctx := context.Background()
	pjskClient := pjskenttest.Open(t, "sqlite3", fmt.Sprintf("file:birthday_monitor_pjsk_unverified_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = pjskClient.Close() })
	usersClient := usersenttest.Open(t, "sqlite3", fmt.Sprintf("file:birthday_monitor_users_unverified_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = usersClient.Close() })

	bindings := accountdata.NewBindingService(
		pjskClient,
		identity.NewResolver(usersClient),
		birthdayMonitorProfileValidator{},
	)
	if _, err := bindings.Bind(ctx, "qq", "42", "11111111111111"); err != nil {
		t.Fatalf("bind account: %v", err)
	}

	service := NewService(pjskClient, bindings)
	_, err := service.CreateOrUpdate(ctx, "qq", "42", "100", "bot", "self", "", false, "/烤森生日监听", false)
	if err == nil {
		t.Fatal("expected unverified binding error")
	}
	want := "该账号尚未验证，请先在工具箱验证账号再发送\"/pjsk验证\"后才可使用"
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

type birthdayMonitorProfileValidator struct{}

func (birthdayMonitorProfileValidator) GetUserProfile(server, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
	if strings.EqualFold(server, "jp") {
		return &sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{
				UserID: 1234567890,
				Name:   userID,
			},
		}, nil
	}
	return nil, sekaiapi.ErrUserNotFound
}
