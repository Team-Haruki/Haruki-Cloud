package handler

import (
	"context"
	"fmt"
	"testing"

	harukiConfig "haruki-cloud/config"
	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderinventory "haruki-cloud/internal/pjsk/render/inventory"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
)

func rejectionText(t *testing.T, message onebot11.Message) string {
	t.Helper()
	if len(message) != 1 {
		t.Fatalf("expected one warning: %+v", message)
	}
	data, ok := message[0].Data.(onebot11.TextData)
	if !ok {
		t.Fatalf("unexpected warning: %+v", message)
	}
	return data.Text
}

func TestRejectCNMySekaiWithoutTrackingStillNotifies(t *testing.T) {
	for _, rc := range []*RequestContext{nil, {Ctx: context.Background(), App: &renderapp.App{}, Cmd: &CommandRequest{RequesterPlatform: "qq", RequesterUserID: "1"}}} {
		msg, err := rejectCNMySekai(rc)
		if err != nil || rejectionText(t, msg) != cnMySekaiNotice() {
			t.Fatalf("message = %+v, %v", msg, err)
		}
	}
}

func TestCNMySekaiWarningsAreSharedAcrossCommandsAndGroups(t *testing.T) {
	original := harukiConfig.Cfg.PJSK.AllowCNMySekai
	harukiConfig.Cfg.PJSK.AllowCNMySekai = nil
	t.Cleanup(func() { harukiConfig.Cfg.PJSK.AllowCNMySekai = original })
	client := usersenttest.Open(t, "sqlite3", "file:cn_mysekai_reject?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = client.Close() })
	app := &renderapp.App{
		BanChecker: accountdata.NewBanService(client),
		MySekai:    rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{}),
		Inventory:  renderinventory.NewController(nil, nil, nil, renderregion.JP, renderinventory.MasterdataOptions{}),
	}
	requests := []struct {
		module       parser.TargetModule
		mode, params string
		execute      func(*RequestContext) (onebot11.Message, error)
	}{
		{parser.ModuleMysekai, "mysekai-resource", "{}", executeMysekai},
		{parser.ModuleCheckData, "mysekai", `{"mode":"self","platform":"qq","platform_user_id":"20001"}`, executeCheckData},
		{parser.ModuleMisc, "inventory-list", `{"filter":"mysekai"}`, executeInventory},
		{parser.ModuleMysekai, "mysekai-map", "{}", executeMysekai},
		{parser.ModuleMysekai, "mysekai-photo", "{}", executeMysekai},
	}
	for i := range 10 {
		request := requests[i%len(requests)]
		cmd := &CommandRequest{Module: request.module, Mode: request.mode, Region: "cn", RegionExplicit: true, RequesterPlatform: "qq", RequesterUserID: "20001", RequesterGroupID: fmt.Sprint(i), Params: []byte(request.params), executor: wrapRequestExecutor(request.execute)}
		msg, err := ExecuteCommandRequest(context.Background(), cmd, app)
		if err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
		if i < 3 {
			text := rejectionText(t, msg)
			id := "mysekai.cn_gate.notice_counted"
			if i == 2 {
				id = "mysekai.cn_gate.notice_last"
			}
			want := i18n.T(id, i18n.Data{"Region": i18n.RegionLabel("cn"), "Attempts": i + 1, "Threshold": 3})
			if text != want {
				t.Fatalf("warning %d: %q, want %q", i+1, text, want)
			}
		} else {
			assertEmptyMySekaiMessage(t, msg)
		}
	}
}
