package handler

import (
	"context"
	"testing"

	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/pjsk/accountdata"
	renderapp "haruki-cloud/internal/pjsk/render/app"

	_ "github.com/mattn/go-sqlite3"
)

func TestRejectCNMySekaiSilentlyDropsWithoutTracking(t *testing.T) {
	msg, err := rejectCNMySekai(nil)
	if err != nil || len(msg) != 0 {
		t.Fatalf("nil rc = %+v, %v", msg, err)
	}
	rc := &RequestContext{Ctx: context.Background(), App: &renderapp.App{}, Cmd: &CommandRequest{RequesterPlatform: "qq", RequesterUserID: "1"}}
	msg, err = rejectCNMySekai(rc)
	if err != nil || len(msg) != 0 {
		t.Fatalf("nil ban checker = %+v, %v", msg, err)
	}
}

func TestRejectCNMySekaiDoesNotRecordOrBan(t *testing.T) {
	ctx := context.Background()
	client := usersenttest.Open(t, "sqlite3", "file:cn_mysekai_reject?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = client.Close() })
	rc := &RequestContext{
		Ctx: ctx,
		App: &renderapp.App{BanChecker: accountdata.NewBanService(client)},
		Cmd: &CommandRequest{RequesterPlatform: "qq", RequesterUserID: "20001"},
	}

	for range accountdata.CNMySekaiAttemptThreshold {
		msg, err := rejectCNMySekai(rc)
		if err != nil || len(msg) != 0 {
			t.Fatalf("message = %+v, %v", msg, err)
		}
	}
	count, err := client.User.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("unexpected user records: %d", count)
	}
}
