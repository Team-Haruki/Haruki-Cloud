package pjsk

import (
	"bytes"
	"context"
	stdjson "encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"haruki-cloud/api"
	harukiConfig "haruki-cloud/config"
	pjskdb "haruki-cloud/database/pjsk"
	pjskenttest "haruki-cloud/database/pjsk/enttest"
	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/identity"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/internal/pjsk/subscription"
	"haruki-cloud/utils/imagecache"

	"github.com/gofiber/fiber/v3"
	_ "github.com/mattn/go-sqlite3"
	"github.com/shamaton/msgpack/v3"
)

func newAPIBirthdayDB(t *testing.T) *pjskdb.Client {
	t.Helper()
	client := pjskenttest.Open(t, "sqlite3", fmt.Sprintf("file:api_birthday_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func createAPIBirthdaySubscription(t *testing.T, client *pjskdb.Client) *pjskdb.MysekaiBirthdaySubscription {
	t.Helper()
	item, err := client.MysekaiBirthdaySubscription.Create().
		SetRegion("jp").
		SetUID("123456789012345678").
		SetPlatform("qq").
		SetPlatformUserID("user-1").
		SetPlatformGroupID("group-1").
		SetCloudBotID("bot-1").
		SetSelfID("self-1").
		SetMaterials([]string{"diamond", "clover"}).
		SetToken("version-1.secret").
		SetActive(true).
		SetExpiresAt(time.Now().Add(time.Hour)).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create birthday subscription: %v", err)
	}
	return item
}

func storeAPIBirthdayEvent(t *testing.T, client *pjskdb.Client, sub *pjskdb.MysekaiBirthdaySubscription, empty bool, payload []byte) *subscription.StoredBirthdayEvent {
	t.Helper()
	stored, err := subscription.NewService(client, nil).StoreEvent(context.Background(), subscription.BirthdayEventPayload{
		SubscriptionID:  fmt.Sprint(sub.ID),
		Region:          sub.Region,
		UID:             sub.UID,
		EmptyResult:     empty,
		FilteredPayload: payload,
	})
	if err != nil {
		t.Fatalf("store birthday event: %v", err)
	}
	return stored
}

func birthdayHandlerRequest(t *testing.T, app *fiber.App, method, target string, body any, contentType string) (*http.Response, []byte) {
	t.Helper()
	var raw []byte
	var err error
	if body != nil {
		if strings.Contains(contentType, "msgpack") {
			raw, err = msgpack.Marshal(body)
		} else {
			raw, err = stdjson.Marshal(body)
		}
		if err != nil {
			t.Fatalf("encode request: %v", err)
		}
	}
	req := httptest.NewRequest(method, target, bytes.NewReader(raw))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, target, err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return resp, responseBody
}

func TestBirthdayMonitorInternalHandlersLifecycle(t *testing.T) {
	client := newAPIBirthdayDB(t)
	sub := createAPIBirthdaySubscription(t, client)
	renderApp := &renderapp.App{PJSK: client}

	app := fiber.New()
	app.Get("/active", makeBirthdayMonitorActiveHandler(renderApp))
	app.Get("/validate", makeBirthdayMonitorTokenValidateHandler(renderApp))
	app.Post("/events", makeBirthdayMonitorEventWriteHandler(renderApp))

	resp, body := birthdayHandlerRequest(t, app, http.MethodGet, "/active?region=jp&uid="+sub.UID, nil, "")
	var active activeBirthdaySubscriptionResponse
	if resp.StatusCode != fiber.StatusOK || stdjson.Unmarshal(body, &active) != nil || !active.Active || active.SubscriptionID != fmt.Sprint(sub.ID) || !reflect.DeepEqual(active.MaterialIDs, []int{12, 20}) {
		t.Fatalf("active response status=%d body=%s decoded=%+v", resp.StatusCode, body, active)
	}
	_, body = birthdayHandlerRequest(t, app, http.MethodGet, "/active?region=jp&uid=missing", nil, "")
	if err := stdjson.Unmarshal(body, &active); err != nil || active.Active {
		t.Fatalf("inactive response body=%s decoded=%+v err=%v", body, active, err)
	}

	eventReq := birthdayEventWriteRequest{
		SubscriptionID: fmt.Sprint(sub.ID), Region: "JP", UID: " " + sub.UID + " ",
		UploadTime: time.Now().UnixMilli(), MatchedMaterialIDs: []int{20, 12, 20},
		FilteredPayload: map[string]any{"materials": []any{12, 20}},
	}
	resp, body = birthdayHandlerRequest(t, app, http.MethodPost, "/events", eventReq, fiber.MIMEApplicationJSON)
	var stored birthdayEventWriteResponse
	if resp.StatusCode != fiber.StatusOK || stdjson.Unmarshal(body, &stored) != nil || stored.EventID == "" || stored.SubscriptionID != fmt.Sprint(sub.ID) {
		t.Fatalf("event response status=%d body=%s decoded=%+v", resp.StatusCode, body, stored)
	}
	resp, _ = birthdayHandlerRequest(t, app, http.MethodPost, "/events", birthdayEventWriteRequest{SubscriptionID: fmt.Sprint(sub.ID), Region: "en", UID: sub.UID}, fiber.MIMEApplicationJSON)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("mismatched event status = %d", resp.StatusCode)
	}
	badReq := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader("{"))
	badReq.Header.Set("Content-Type", fiber.MIMEApplicationJSON)
	badResp, err := app.Test(badReq)
	if err != nil || badResp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("malformed event response = %+v, %v", badResp, err)
	}
	if badResp != nil {
		_ = badResp.Body.Close()
	}

	resp, body = birthdayHandlerRequest(t, app, http.MethodGet, "/validate?subscription_id="+fmt.Sprint(sub.ID)+"&subscription_version=version-1&token=version-1.secret", nil, "")
	var validation birthdayTokenValidationResponse
	if resp.StatusCode != fiber.StatusOK || stdjson.Unmarshal(body, &validation) != nil || !validation.Valid || validation.SubscriptionID != fmt.Sprint(sub.ID) || len(validation.PendingEvents) != 1 {
		t.Fatalf("validation response status=%d body=%s decoded=%+v", resp.StatusCode, body, validation)
	}
	_, body = birthdayHandlerRequest(t, app, http.MethodGet, "/validate?subscription_id="+fmt.Sprint(sub.ID)+"&token=wrong", nil, "")
	if err := stdjson.Unmarshal(body, &validation); err != nil || validation.Valid {
		t.Fatalf("invalid-token response body=%s decoded=%+v err=%v", body, validation, err)
	}

	broken := fiber.New()
	broken.Get("/active", makeBirthdayMonitorActiveHandler(&renderapp.App{}))
	broken.Get("/validate", makeBirthdayMonitorTokenValidateHandler(&renderapp.App{}))
	broken.Post("/events", makeBirthdayMonitorEventWriteHandler(&renderapp.App{}))
	for _, check := range []struct{ method, target string }{
		{http.MethodGet, "/active?region=jp&uid=x"},
		{http.MethodGet, "/validate?subscription_id=1&token=x"},
	} {
		resp, _ := birthdayHandlerRequest(t, broken, check.method, check.target, nil, "")
		if resp.StatusCode != fiber.StatusInternalServerError {
			t.Fatalf("broken %s status = %d", check.target, resp.StatusCode)
		}
	}
	resp, _ = birthdayHandlerRequest(t, broken, http.MethodPost, "/events", eventReq, fiber.MIMEApplicationJSON)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("broken event status = %d", resp.StatusCode)
	}
}

func TestBirthdayMonitorRenderAndAckHandlerBranches(t *testing.T) {
	client := newAPIBirthdayDB(t)
	sub := createAPIBirthdaySubscription(t, client)
	empty := storeAPIBirthdayEvent(t, client, sub, true, nil)
	missingPayload := storeAPIBirthdayEvent(t, client, sub, false, nil)
	nonEmpty := storeAPIBirthdayEvent(t, client, sub, false, []byte(`{"materials":[12]}`))

	renderApp := &renderapp.App{PJSK: client}
	app := fiber.New()
	app.Post("/bots/:botId/render", makeBirthdayMonitorRenderHandler(renderApp))
	app.Post("/bots/:botId/ack", makeBirthdayMonitorAckHandler(renderApp))
	requestFor := func(eventID string) birthdayRenderRequest {
		return birthdayRenderRequest{
			Platform: "qq", PlatformUserID: sub.PlatformUserID, PlatformGroupID: sub.PlatformGroupID,
			SelfID: sub.SelfID, SubscriptionID: fmt.Sprint(sub.ID), Token: sub.Token, EventID: eventID,
		}
	}

	resp, body := birthdayHandlerRequest(t, app, http.MethodPost, "/bots/bot-1/render", requestFor(empty.EventID), fiber.MIMEApplicationJSON)
	if resp.StatusCode != fiber.StatusOK || !strings.Contains(string(body), subscription.EmptyBirthdayMonitorMessage) || !strings.Contains(string(body), sub.PlatformUserID) {
		t.Fatalf("empty render status=%d body=%s", resp.StatusCode, body)
	}
	_, body = birthdayHandlerRequest(t, app, http.MethodPost, "/bots/bot-1/render", requestFor(missingPayload.EventID), fiber.MIMEApplicationJSON)
	if !strings.Contains(string(body), "缺少可绘制数据") {
		t.Fatalf("missing-payload render body=%s", body)
	}
	_, body = birthdayHandlerRequest(t, app, http.MethodPost, "/bots/bot-1/render", requestFor(nonEmpty.EventID), fiber.MIMEApplicationJSON)
	if !strings.Contains(string(body), "服务未就绪") {
		t.Fatalf("unready render body=%s", body)
	}
	badToken := requestFor(nonEmpty.EventID)
	badToken.Token = "wrong"
	_, body = birthdayHandlerRequest(t, app, http.MethodPost, "/bots/bot-1/render", badToken, fiber.MIMEApplicationJSON)
	if !strings.Contains(string(body), "请求处理失败") {
		t.Fatalf("invalid-token render body=%s", body)
	}

	mysekaiController := rendermysekai.NewController(nil, nil, renderregion.JP, assets.NewAssetHelper("", nil), rendermysekai.MasterdataOptions{})
	renderApp.MySekai = mysekaiController
	_, body = birthdayHandlerRequest(t, app, http.MethodPost, "/bots/bot-1/render", requestFor(nonEmpty.EventID), fiber.MIMEApplicationJSON)
	if !strings.Contains(string(body), "服务未就绪") {
		t.Fatalf("nil-cache render body=%s", body)
	}
	renderApp.ImageCache = imagecache.New("https://cache.invalid", t.TempDir())
	_, body = birthdayHandlerRequest(t, app, http.MethodPost, "/bots/bot-1/render", requestFor(nonEmpty.EventID), fiber.MIMEApplicationJSON)
	if !strings.Contains(string(body), "渲染服务未就绪") {
		t.Fatalf("render failure body=%s", body)
	}

	invalidReq := httptest.NewRequest(http.MethodPost, "/bots/bot-1/render", strings.NewReader("{"))
	invalidReq.Header.Set("Content-Type", fiber.MIMEApplicationJSON)
	invalidResp, err := app.Test(invalidReq)
	if err != nil || invalidResp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("invalid render response = %+v, %v", invalidResp, err)
	}
	if invalidResp != nil {
		_ = invalidResp.Body.Close()
	}

	resp, body = birthdayHandlerRequest(t, app, http.MethodPost, "/bots/bot-1/ack", requestFor(nonEmpty.EventID), "application/msgpack")
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("ack status=%d body=%s", resp.StatusCode, body)
	}
	var ackEnvelope renderEnvelope
	if err := stdjson.Unmarshal(body, &ackEnvelope); err != nil {
		t.Fatalf("decode ack envelope: %v body=%s", err, body)
	}
	badAck := requestFor(empty.EventID)
	badAck.Token = "wrong"
	_, body = birthdayHandlerRequest(t, app, http.MethodPost, "/bots/bot-1/ack", badAck, fiber.MIMEApplicationJSON)
	if !strings.Contains(string(body), "请求处理失败") {
		t.Fatalf("invalid ack body=%s", body)
	}
	invalidAck := httptest.NewRequest(http.MethodPost, "/bots/bot-1/ack", strings.NewReader("{"))
	invalidAck.Header.Set("Content-Type", fiber.MIMEApplicationJSON)
	invalidAckResp, err := app.Test(invalidAck)
	if err != nil || invalidAckResp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("invalid ack response = %+v, %v", invalidAckResp, err)
	}
	if invalidAckResp != nil {
		_ = invalidAckResp.Body.Close()
	}
}

func TestBirthdayMonitorPureHelpersAndResponseEncoding(t *testing.T) {
	if newBirthdayMonitorService(nil) != nil || newBirthdayMonitorDBService(nil) != nil {
		t.Fatal("nil render app produced a birthday service")
	}
	client := newAPIBirthdayDB(t)
	renderApp := &renderapp.App{PJSK: client, Config: renderapp.Config{ReadOnly: true}}
	if service := newBirthdayMonitorService(renderApp); service == nil {
		t.Fatal("birthday monitor service is nil")
	}
	if service := newBirthdayMonitorDBService(renderApp); service == nil {
		t.Fatal("birthday DB service is nil")
	}

	originalConfig := harukiConfig.Cfg
	t.Cleanup(func() { harukiConfig.Cfg = originalConfig })
	harukiConfig.Cfg.HMES.PublicBaseURL = " https://hmes.example/ "
	if birthdayMonitorActions(nil) != nil || birthdayMonitorActions(&subscription.BirthdayMonitorResult{}) != nil {
		t.Fatal("incomplete birthday result produced actions")
	}
	sub := createAPIBirthdaySubscription(t, client)
	result := &subscription.BirthdayMonitorResult{Subscription: sub, SubscriptionVersion: "v1", Token: "token"}
	actions := birthdayMonitorActions(result)
	if len(actions) != 1 || actions[0].Endpoint != "https://hmes.example/sse" || actions[0].SubscriptionID != fmt.Sprint(sub.ID) || actions[0].ExpiresAt != sub.ExpiresAt.Unix() {
		t.Fatalf("birthday actions = %+v", actions)
	}
	harukiConfig.Cfg.HMES.PublicBaseURL = " "
	if birthdayMonitorActions(result) != nil {
		t.Fatal("blank public URL produced actions")
	}

	message := BotCommandRequest{Message: onebot11.Message{
		onebot11.Text(" first "),
		{Type: onebot11.TypeText, Data: map[string]string{onebot11.KeyText: "second"}},
		{Type: onebot11.TypeText, Data: map[string]any{onebot11.KeyText: "third"}},
		{Type: onebot11.TypeText, Data: map[string]any{onebot11.KeyText: 4}},
		onebot11.Image("ignored", ""),
	}}
	if got := requestMessageText(message); got != "first secondthird" {
		t.Fatalf("request message text = %q", got)
	}
	if got := birthdayMonitorCommandText(BotCommandRequest{Message: onebot11.Message{onebot11.Text("/烤森生日监听 钻石")}}); got != "/烤森生日监听 钻石" {
		t.Fatalf("complete command text = %q", got)
	}
	if got := birthdayMonitorCommandText(BotCommandRequest{MatchedCommand: " /烤森生日监听 ", Message: onebot11.Message{onebot11.Text("bad")}}); got != "/烤森生日监听 bad" {
		t.Fatalf("fallback command text = %q", got)
	}
	if got := birthdayMonitorCommandText(BotCommandRequest{MatchedCommand: "/烤森生日监听"}); got != "/烤森生日监听" {
		t.Fatalf("matched-only command text = %q", got)
	}
	if got := birthdayMonitorCommandText(BotCommandRequest{Message: onebot11.Message{onebot11.Text("bad")}}); got != "bad" {
		t.Fatalf("message-only command text = %q", got)
	}

	prefixes := buildBirthdayMonitorManifestCommandPrefixes([]string{"", " /test ", "/test"})
	if len(prefixes) != 16 || !slices.Contains(prefixes, "/test") || !slices.Contains(prefixes, "/jptest") || !slices.Contains(prefixes, "/jp /test") {
		t.Fatalf("manifest prefixes = %v", prefixes)
	}
	if !isCancelBirthdayMonitorText("/烤森生日取消监听") || isCancelBirthdayMonitorText("/烤森生日监听") || isCancelBirthdayMonitorText("bad") {
		t.Fatal("birthday cancel recognition mismatch")
	}

	finished := 0
	finishPhaseOnPanic(func() { finished++ })
	if finished != 0 {
		t.Fatal("finish callback ran without a panic")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("finishPhaseOnPanic swallowed panic")
			}
		}()
		defer finishPhaseOnPanic(func() { finished++ })
		panic("boom")
	}()
	if finished != 1 {
		t.Fatalf("panic finish count = %d", finished)
	}

	app := fiber.New()
	app.Post("/json", func(c fiber.Ctx) error {
		return botResponseWithActions(c, fiber.StatusCreated, api.ResponseOK, onebot11.Message{onebot11.Text("ok")}, actions)
	})
	app.Post("/msgpack", func(c fiber.Ctx) error {
		c.Locals("secure_noise", true)
		return botResponseWithActions(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{}, actions)
	})
	app.Post("/msgpack-error", func(c fiber.Ctx) error {
		c.Locals("secure_noise", true)
		return botResponseWithActions(c, fiber.StatusOK, api.ResponseOK, make(chan int), actions)
	})
	resp, body := birthdayHandlerRequest(t, app, http.MethodPost, "/json", nil, "")
	if resp.StatusCode != fiber.StatusCreated || !strings.Contains(string(body), "client_actions") {
		t.Fatalf("JSON action response status=%d body=%s", resp.StatusCode, body)
	}
	resp, body = birthdayHandlerRequest(t, app, http.MethodPost, "/msgpack", nil, "")
	var decoded map[string]any
	if resp.StatusCode != fiber.StatusOK || msgpack.Unmarshal(body, &decoded) != nil || decoded["client_actions"] == nil {
		t.Fatalf("MsgPack action response status=%d body=%x decoded=%v", resp.StatusCode, body, decoded)
	}
	resp, _ = birthdayHandlerRequest(t, app, http.MethodPost, "/msgpack-error", nil, "")
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("MsgPack encode error status = %d", resp.StatusCode)
	}

	parser := fiber.New()
	parser.Post("/parse", func(c fiber.Ctx) error {
		var req birthdayRenderRequest
		if err := parseRequestBody(c, &req); err != nil {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		return c.SendString(req.EventID)
	})
	for _, contentType := range []string{fiber.MIMEApplicationJSON, "application/msgpack"} {
		resp, body := birthdayHandlerRequest(t, parser, http.MethodPost, "/parse", birthdayRenderRequest{EventID: "event"}, contentType)
		if resp.StatusCode != fiber.StatusOK || string(body) != "event" {
			t.Fatalf("parse %s status=%d body=%s", contentType, resp.StatusCode, body)
		}
	}

}

type apiBirthdayProfileValidator struct{}

func (apiBirthdayProfileValidator) GetUserProfile(_ string, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
	return &sekaiapi.GetAnotherProfileResponse{User: sekaiapi.AnotherUser{UserID: 1234567890, Name: userID}}, nil
}

func TestBirthdayMonitorCommandHandlerCreateAndCancelSuccess(t *testing.T) {
	ctx := context.Background()
	pjskClient := newAPIBirthdayDB(t)
	usersClient := usersenttest.Open(t, "sqlite3", fmt.Sprintf("file:api_birthday_users_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = usersClient.Close() })
	bindings := accountdata.NewBindingService(pjskClient, identity.NewResolver(usersClient), apiBirthdayProfileValidator{})
	if _, err := bindings.Bind(ctx, "qq", "user-1", "123456789012345678"); err != nil {
		t.Fatalf("bind birthday account: %v", err)
	}
	if err := pjskClient.UserBinding.Update().SetVerified(true).Exec(ctx); err != nil {
		t.Fatalf("verify birthday binding: %v", err)
	}

	toolbox := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut && r.Method != http.MethodDelete {
			http.Error(w, "unexpected method", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer toolbox.Close()

	originalConfig := harukiConfig.Cfg
	t.Cleanup(func() { harukiConfig.Cfg = originalConfig })
	harukiConfig.Cfg.HMES.PublicBaseURL = "https://hmes.example"
	harukiConfig.Cfg.HMES.InternalBaseURL = ""
	renderApp := &renderapp.App{
		PJSK: pjskClient, Bindings: bindings,
		Toolbox: sekaiapi.NewToolboxClient(&harukiConfig.ToolboxConfig{BaseURL: toolbox.URL, APIToken: "test"}),
	}
	guard := &birthdayMonitorTestGuard{allow: true}
	app := fiber.New()
	app.Post("/bots/:botId/monitor", makeBirthdayMonitorHandler(renderApp, guard))

	request := BotCommandRequest{
		Platform: "qq", PlatformUserID: "user-1", PlatformGroupID: "group-1", SelfID: "self-1", Server: "jp",
		MatchedCommand: "/烤森生日监听", Message: onebot11.Message{onebot11.Text("/烤森生日监听 钻石 10")},
	}
	resp, body := birthdayHandlerRequest(t, app, http.MethodPost, "/bots/bot-1/monitor", request, fiber.MIMEApplicationJSON)
	if resp.StatusCode != fiber.StatusOK || !strings.Contains(string(body), "有效期 10 分钟") || !strings.Contains(string(body), "client_actions") || !strings.Contains(string(body), "hmes_sse") {
		t.Fatalf("create monitor status=%d body=%s", resp.StatusCode, body)
	}

	request.MatchedCommand = "/烤森生日取消监听"
	request.Message = onebot11.Message{onebot11.Text("/烤森生日取消监听")}
	resp, body = birthdayHandlerRequest(t, app, http.MethodPost, "/bots/bot-1/monitor", request, fiber.MIMEApplicationJSON)
	if resp.StatusCode != fiber.StatusOK || !strings.Contains(string(body), "监听已取消") {
		t.Fatalf("cancel monitor status=%d body=%s", resp.StatusCode, body)
	}
	if guard.completed != 2 {
		t.Fatalf("guard completion count = %d", guard.completed)
	}

	invalid := httptest.NewRequest(http.MethodPost, "/bots/bot-1/monitor", strings.NewReader("{"))
	invalid.Header.Set("Content-Type", fiber.MIMEApplicationJSON)
	invalidResp, err := app.Test(invalid)
	if err != nil || invalidResp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("invalid monitor response = %+v, %v", invalidResp, err)
	}
	if invalidResp != nil {
		_ = invalidResp.Body.Close()
	}
}
