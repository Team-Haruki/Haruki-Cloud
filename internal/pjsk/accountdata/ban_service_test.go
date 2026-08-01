package accountdata_test

import (
	"context"
	"strings"
	"testing"
	"time"

	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/parser"

	_ "github.com/mattn/go-sqlite3"
)

func TestBanServiceKillAndBack(t *testing.T) {
	client := usersenttest.Open(t, "sqlite3", "file:global_ban_kill_back?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = client.Close() })
	service := accountdata.NewBanService(client)
	ctx := context.Background()
	expiresAt := time.Now().Add(48 * time.Hour).Round(time.Second)

	status, err := service.Kill(ctx, "123456789", "恶意滥用", &expiresAt)
	if err != nil {
		t.Fatalf("Kill() error = %v", err)
	}
	if !status.Active || status.ExpiresAt == nil || status.Reason != "恶意滥用" {
		t.Fatalf("unexpected kill status: %+v", status)
	}
	banned, err := service.IsGloballyBanned(ctx, "qq", "123456789")
	if err != nil || !banned {
		t.Fatalf("IsGloballyBanned() = %v, %v", banned, err)
	}
	if err := service.CheckBan(ctx, "qq", "123456789", parser.ModuleMusic); err == nil || !strings.Contains(err.Error(), "恶意滥用") || !strings.Contains(err.Error(), "封禁至") {
		t.Fatalf("unexpected CheckBan() error: %v", err)
	}

	if err := service.Back(ctx, "123456789"); err != nil {
		t.Fatalf("Back() error = %v", err)
	}
	row, err := client.User.Query().Only(ctx)
	if err != nil {
		t.Fatalf("query user: %v", err)
	}
	if row.BanState || row.BanReason != "" || row.BanExpiresAt != nil {
		t.Fatalf("ban metadata not cleared: %+v", row)
	}
}

func TestBanServiceExpiredBanIsInactive(t *testing.T) {
	client := usersenttest.Open(t, "sqlite3", "file:global_ban_expired?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	expiredAt := time.Now().Add(-time.Minute)
	_, err := client.User.Create().
		SetID(123456).
		SetPlatform("qq").
		SetUserID("987654321").
		SetBanState(true).
		SetBanReason("已过期").
		SetBanExpiresAt(expiredAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	service := accountdata.NewBanService(client)
	banned, err := service.IsGloballyBanned(ctx, "qq", "987654321")
	if err != nil || banned {
		t.Fatalf("expired ban should be inactive: banned=%v err=%v", banned, err)
	}
	if err := service.CheckBan(ctx, "qq", "987654321", parser.ModuleMusic); err != nil {
		t.Fatalf("expired ban blocked command: %v", err)
	}
}

func TestBanServicePermanentBanHasNoExpiry(t *testing.T) {
	client := usersenttest.Open(t, "sqlite3", "file:global_ban_permanent?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = client.Close() })
	service := accountdata.NewBanService(client)
	reason := strings.Repeat("中", 255)
	status, err := service.Kill(context.Background(), "555666777", reason, nil)
	if err != nil {
		t.Fatalf("Kill() error = %v", err)
	}
	if !status.Active || status.ExpiresAt != nil || status.Reason != reason {
		t.Fatalf("unexpected permanent status: %+v", status)
	}
}

func TestBanServiceGlobalAdminRosterIsExplicit(t *testing.T) {
	client := usersenttest.Open(t, "sqlite3", "file:global_ban_admins?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = client.Close() })
	service := accountdata.NewBanService(client)
	service.SetAdminQQIDs([]string{" 03164679932 ", "invalid"})
	if !service.IsAdmin("qq", "3164679932") {
		t.Fatal("configured QQ was not recognized as global admin")
	}
	if service.IsAdmin("qq", "9001") || service.IsAdmin("discord", "3164679932") {
		t.Fatal("unconfigured or non-QQ identity was authorized")
	}
}
