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

func TestRecordCNMySekaiAttemptBansOnThirdAttempt(t *testing.T) {
	client := usersenttest.Open(t, "sqlite3", "file:cn_mysekai_attempts?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = client.Close() })
	service := accountdata.NewBanService(client)
	ctx := context.Background()

	for want := 1; want < accountdata.CNMySekaiAttemptThreshold; want++ {
		attempt, err := service.RecordCNMySekaiAttempt(ctx, "qq", "10001", 0)
		if err != nil {
			t.Fatalf("attempt %d: %v", want, err)
		}
		if attempt.Banned || attempt.Attempts != want || attempt.Threshold != accountdata.CNMySekaiAttemptThreshold {
			t.Fatalf("attempt %d = %+v", want, attempt)
		}
		if err := service.CheckBan(ctx, "qq", "10001", parser.ModuleMysekai); err != nil {
			t.Fatalf("attempt %d must not ban yet: %v", want, err)
		}
	}

	before := time.Now()
	attempt, err := service.RecordCNMySekaiAttempt(ctx, "qq", "10001", 30*time.Minute)
	if err != nil {
		t.Fatalf("third attempt: %v", err)
	}
	if !attempt.Banned || attempt.Attempts != accountdata.CNMySekaiAttemptThreshold {
		t.Fatalf("third attempt = %+v", attempt)
	}
	if got := attempt.ExpiresAt.Sub(before); got < 29*time.Minute || got > 31*time.Minute {
		t.Fatalf("ban length = %v, want ~30m", got)
	}
	err = service.CheckBan(ctx, "qq", "10001", parser.ModuleMusic)
	if err == nil || !strings.Contains(err.Error(), "MySekai") || !strings.Contains(err.Error(), "封禁至") {
		t.Fatalf("CheckBan after ban = %v", err)
	}

	row, err := client.User.Query().Only(ctx)
	if err != nil {
		t.Fatalf("query user: %v", err)
	}
	if row.PjskCnMysekaiAttempts != 0 || !row.BanState || row.BanExpiresAt == nil {
		t.Fatalf("row after ban = attempts %d ban %v expires %v", row.PjskCnMysekaiAttempts, row.BanState, row.BanExpiresAt)
	}
}

func TestRecordCNMySekaiAttemptDefaultsToTenMinutes(t *testing.T) {
	client := usersenttest.Open(t, "sqlite3", "file:cn_mysekai_default_hour?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = client.Close() })
	service := accountdata.NewBanService(client)
	ctx := context.Background()

	var attempt accountdata.CNMySekaiAttempt
	for range accountdata.CNMySekaiAttemptThreshold {
		var err error
		if attempt, err = service.RecordCNMySekaiAttempt(ctx, "qq", "10002", 0); err != nil {
			t.Fatal(err)
		}
	}
	if got := time.Until(attempt.ExpiresAt); got < 9*time.Minute || got > 11*time.Minute {
		t.Fatalf("default ban length = %v, want ~10m", got)
	}
}

func TestRecordCNMySekaiAttemptIsNilAndReadOnlySafe(t *testing.T) {
	var nilService *accountdata.BanService
	attempt, err := nilService.RecordCNMySekaiAttempt(context.Background(), "qq", "1", 0)
	if err != nil || attempt.Attempts != 0 || attempt.Banned {
		t.Fatalf("nil service = %+v, %v", attempt, err)
	}

	client := usersenttest.Open(t, "sqlite3", "file:cn_mysekai_readonly?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = client.Close() })
	service := accountdata.NewBanService(client)
	if _, err := service.RecordCNMySekaiAttempt(context.Background(), "", "1", 0); err != nil {
		t.Fatalf("blank platform must be ignored: %v", err)
	}
	service.SetReadOnly(true)
	if _, err := service.RecordCNMySekaiAttempt(context.Background(), "qq", "1", 0); err == nil {
		t.Fatal("read-only node must refuse to record")
	}
}
