package handler

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	harukiConfig "haruki-cloud/config"
	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	renderapp "haruki-cloud/internal/pjsk/render/app"

	_ "github.com/mattn/go-sqlite3"
)

func rejectionText(t *testing.T, msg onebot11.Message) string {
	t.Helper()
	if len(msg) != 1 {
		t.Fatalf("message = %+v", msg)
	}
	data, ok := msg[0].Data.(onebot11.TextData)
	if !ok {
		t.Fatalf("segment = %+v", msg[0])
	}
	return data.Text
}

func TestRejectCNMySekaiWithoutTrackingStillNotifies(t *testing.T) {
	msg, err := rejectCNMySekai(nil)
	if err != nil || rejectionText(t, msg) != cnMySekaiNeverOpensNotice {
		t.Fatalf("nil rc = %+v, %v", msg, err)
	}
	rc := &RequestContext{Ctx: context.Background(), App: &renderapp.App{}, Cmd: &CommandRequest{RequesterPlatform: "qq", RequesterUserID: "1"}}
	msg, err = rejectCNMySekai(rc)
	if err != nil || rejectionText(t, msg) != cnMySekaiNeverOpensNotice {
		t.Fatalf("nil ban checker = %+v, %v", msg, err)
	}
}

func TestRejectCNMySekaiCountsAndBansOnThirdAttempt(t *testing.T) {
	original := harukiConfig.Cfg.PJSK.CNMySekaiBanDuration
	harukiConfig.Cfg.PJSK.CNMySekaiBanDuration = 2 * time.Hour
	t.Cleanup(func() { harukiConfig.Cfg.PJSK.CNMySekaiBanDuration = original })

	client := usersenttest.Open(t, "sqlite3", "file:cn_mysekai_reject?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = client.Close() })
	rc := &RequestContext{
		Ctx: context.Background(),
		App: &renderapp.App{BanChecker: accountdata.NewBanService(client)},
		Cmd: &CommandRequest{RequesterPlatform: "qq", RequesterUserID: "20001"},
	}

	for i := 1; i < accountdata.CNMySekaiAttemptThreshold; i++ {
		msg, err := rejectCNMySekai(rc)
		if err != nil {
			t.Fatal(err)
		}
		text := rejectionText(t, msg)
		if !strings.HasPrefix(text, cnMySekaiNeverOpensNotice) || !strings.HasSuffix(text, fmt.Sprintf("（%d/3）", i)) {
			t.Fatalf("attempt %d text = %q", i, text)
		}
	}
	msg, err := rejectCNMySekai(rc)
	if err != nil {
		t.Fatal(err)
	}
	text := rejectionText(t, msg)
	if !strings.HasPrefix(text, cnMySekaiNeverOpensNotice) || !strings.Contains(text, "（3/3）\n已被短暂封禁至 ") {
		t.Fatalf("third attempt text = %q", text)
	}
	row, err := client.User.Query().Only(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !row.BanState || row.BanExpiresAt == nil || time.Until(*row.BanExpiresAt) < 119*time.Minute {
		t.Fatalf("row = ban %v expires %v", row.BanState, row.BanExpiresAt)
	}
}
