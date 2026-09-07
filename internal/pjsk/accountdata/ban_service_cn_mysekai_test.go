package accountdata_test

import (
	"context"
	"sync"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	usersdb "haruki-cloud/database/users"
	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/parser"

	_ "github.com/mattn/go-sqlite3"
)

func TestRecordCNMySekaiAttemptWarnsThreeTimesThenStaysSilent(t *testing.T) {
	client := usersenttest.Open(t, "sqlite3", "file:cn_mysekai_attempts?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = client.Close() })
	service := accountdata.NewBanService(client)
	ctx := context.Background()
	for want := 1; want <= accountdata.CNMySekaiAttemptThreshold; want++ {
		attempt, err := service.RecordCNMySekaiAttempt(ctx, "qq", "10001")
		if err != nil || attempt.Silenced || attempt.Attempts != want || attempt.Threshold != 3 {
			t.Fatalf("attempt %d = %+v, %v", want, attempt, err)
		}
	}
	// A fresh service still observes the persisted limit.
	service = accountdata.NewBanService(client)
	for range 5 {
		attempt, err := service.RecordCNMySekaiAttempt(ctx, "qq", "10001")
		if err != nil || !attempt.Silenced || attempt.Attempts != 3 {
			t.Fatalf("later attempt = %+v, %v", attempt, err)
		}
	}
	row, err := client.User.Query().Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if row.PjskCnMysekaiAttempts != 3 || row.BanState || row.BanExpiresAt != nil {
		t.Fatalf("unexpected warning/ban state: %+v", row)
	}
	for _, module := range []parser.TargetModule{parser.ModuleMusic, parser.ModuleMysekai} {
		if err := service.CheckBan(ctx, "qq", "10001", module); err != nil {
			t.Fatalf("warning limit must not ban other regions or features: %v", err)
		}
	}
	service.SetReadOnly(true)
	attempt, err := service.RecordCNMySekaiAttempt(ctx, "qq", "10001")
	if err != nil || !attempt.Silenced {
		t.Fatalf("read-only existing silence = %+v, %v", attempt, err)
	}
	service.SetReadOnly(false)
	for _, identity := range [][2]string{{"qq", "10002"}, {"telegram", "10001"}} {
		attempt, err := service.RecordCNMySekaiAttempt(ctx, identity[0], identity[1])
		if err != nil || attempt.Silenced || attempt.Attempts != 1 {
			t.Fatalf("independent identity = %+v, %v", attempt, err)
		}
	}
}

func TestRecordCNMySekaiAttemptConcurrentRequestsOnlyWarnThreeTimes(t *testing.T) {
	driver, err := entsql.Open(dialect.SQLite, "file:cn_mysekai_concurrent?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	driver.DB().SetMaxOpenConns(1)
	client := usersdb.NewClient(usersdb.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.User.Create().SetID(100001).SetPlatform("qq").SetUserID("10003").Save(ctx); err != nil {
		t.Fatal(err)
	}
	service := accountdata.NewBanService(client)
	results := make(chan accountdata.CNMySekaiAttempt, 20)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			attempt, err := service.RecordCNMySekaiAttempt(ctx, "qq", "10003")
			if err != nil {
				t.Errorf("record: %v", err)
				return
			}
			results <- attempt
		})
	}
	wg.Wait()
	close(results)
	warnings := map[int]int{}
	silenced := 0
	for attempt := range results {
		if attempt.Silenced {
			silenced++
		} else {
			warnings[attempt.Attempts]++
		}
	}
	if warnings[1] != 1 || warnings[2] != 1 || warnings[3] != 1 || silenced != 17 {
		t.Fatalf("warnings = %v, silenced = %d", warnings, silenced)
	}
}

func TestRecordCNMySekaiAttemptIsNilAndReadOnlySafe(t *testing.T) {
	var nilService *accountdata.BanService
	attempt, err := nilService.RecordCNMySekaiAttempt(context.Background(), "qq", "1")
	if err != nil || attempt.Attempts != 0 || attempt.Silenced {
		t.Fatalf("nil service = %+v, %v", attempt, err)
	}
	client := usersenttest.Open(t, "sqlite3", "file:cn_mysekai_readonly?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = client.Close() })
	service := accountdata.NewBanService(client)
	if _, err := service.RecordCNMySekaiAttempt(context.Background(), "", "1"); err != nil {
		t.Fatalf("blank platform must be ignored: %v", err)
	}
	service.SetReadOnly(true)
	if _, err := service.RecordCNMySekaiAttempt(context.Background(), "qq", "1"); err == nil {
		t.Fatal("read-only node must refuse to record")
	}
}
