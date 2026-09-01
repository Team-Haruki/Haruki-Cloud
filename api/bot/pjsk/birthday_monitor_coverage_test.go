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
	"haruki-cloud/internal/testutil"
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
	testutil.Require(t, !(err != nil), "create birthday subscription: %v", err)

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
	testutil.Require(t, !(err != nil), "store birthday event: %v", err)

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
		testutil.Require(t, !(err != nil), "encode request: %v", err)

	}
	req := httptest.NewRequest(method, target, bytes.NewReader(raw))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := app.Test(req)
	testutil.Require(t, !(err != nil), "request %s %s: %v", method, target, err)

	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	testutil.Require(t, !(err != nil), "read response: %v", err)

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
	{
		testutil.Require(t, !(resp.StatusCode != fiber.StatusOK), "active response status=%d body=%s decoded=%+v", resp.StatusCode, body, active)
		testutil.Require(t, !(stdjson.Unmarshal(body, &active) != nil), "active response status=%d body=%s decoded=%+v", resp.StatusCode, body, active)
		testutil.Require(t, active.Active, "active response status=%d body=%s decoded=%+v", resp.StatusCode, body, active)
		testutil.Require(t, !(active.SubscriptionID != fmt.Sprint(sub.ID)), "active response status=%d body=%s decoded=%+v", resp.StatusCode, body, active)
		testutil.Require(t, reflect.DeepEqual(active.MaterialIDs, []int{12, 20}), "active response status=%d body=%s decoded=%+v", resp.StatusCode, body, active)
	}

	_, body = birthdayHandlerRequest(t, app, http.MethodGet, "/active?region=jp&uid=missing", nil, "")
	{
		err := stdjson.Unmarshal(body, &active)
		{
			testutil.Require(t, !(err != nil), "inactive response body=%s decoded=%+v err=%v", body, active, err)
			testutil.Require(t, !(active.Active), "inactive response body=%s decoded=%+v err=%v", body, active, err)
		}
	}

	eventReq := birthdayEventWriteRequest{
		SubscriptionID: fmt.Sprint(sub.ID), Region: "JP", UID: " " + sub.UID + " ",
		UploadTime: time.Now().UnixMilli(), MatchedMaterialIDs: []int{20, 12, 20},
		FilteredPayload: map[string]any{"materials": []any{12, 20}},
	}
	resp, body = birthdayHandlerRequest(t, app, http.MethodPost, "/events", eventReq, fiber.MIMEApplicationJSON)
	var stored birthdayEventWriteResponse
	{
		testutil.Require(t, !(resp.StatusCode != fiber.StatusOK), "event response status=%d body=%s decoded=%+v", resp.StatusCode, body, stored)
		testutil.Require(t, !(stdjson.Unmarshal(body, &stored) != nil), "event response status=%d body=%s decoded=%+v", resp.StatusCode, body, stored)
		testutil.Require(t, !(stored.EventID == ""), "event response status=%d body=%s decoded=%+v", resp.StatusCode, body, stored)
		testutil.Require(t, !(stored.SubscriptionID != fmt.Sprint(sub.ID)), "event response status=%d body=%s decoded=%+v", resp.StatusCode, body, stored)
	}

	resp, _ = birthdayHandlerRequest(t, app, http.MethodPost, "/events", birthdayEventWriteRequest{SubscriptionID: fmt.Sprint(sub.ID), Region: "en", UID: sub.UID}, fiber.MIMEApplicationJSON)
	testutil.Require(t, !(resp.StatusCode != fiber.StatusBadRequest), "mismatched event status = %d", resp.StatusCode)

	badReq := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader("{"))
	badReq.Header.Set("Content-Type", fiber.MIMEApplicationJSON)
	badResp, err := app.Test(badReq)
	{
		testutil.Require(t, !(err != nil), "malformed event response = %+v, %v", badResp, err)
		testutil.Require(t, !(badResp.StatusCode != fiber.StatusBadRequest), "malformed event response = %+v, %v", badResp, err)
	}

	if badResp != nil {
		_ = badResp.Body.Close()
	}

	resp, body = birthdayHandlerRequest(t, app, http.MethodGet, "/validate?subscription_id="+fmt.Sprint(sub.ID)+"&subscription_version=version-1&token=version-1.secret", nil, "")
	var validation birthdayTokenValidationResponse
	{
		testutil.Require(t, !(resp.StatusCode != fiber.StatusOK), "validation response status=%d body=%s decoded=%+v", resp.StatusCode, body, validation)
		testutil.Require(t, !(stdjson.Unmarshal(body, &validation) != nil), "validation response status=%d body=%s decoded=%+v", resp.StatusCode, body, validation)
		testutil.Require(t, validation.Valid, "validation response status=%d body=%s decoded=%+v", resp.StatusCode, body, validation)
		testutil.Require(t, !(validation.SubscriptionID != fmt.Sprint(sub.ID)), "validation response status=%d body=%s decoded=%+v", resp.StatusCode, body, validation)
		testutil.Require(t, !(len(validation.PendingEvents) != 1), "validation response status=%d body=%s decoded=%+v", resp.StatusCode, body, validation)
	}

	_, body = birthdayHandlerRequest(t, app, http.MethodGet, "/validate?subscription_id="+fmt.Sprint(sub.ID)+"&token=wrong", nil, "")
	{
		err := stdjson.Unmarshal(body, &validation)
		{
			testutil.Require(t, !(err != nil), "invalid-token response body=%s decoded=%+v err=%v", body, validation, err)
			testutil.Require(t, !(validation.Valid), "invalid-token response body=%s decoded=%+v err=%v", body, validation, err)
		}
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
		testutil.Require(t, !(resp.StatusCode != fiber.StatusInternalServerError), "broken %s status = %d", check.target, resp.StatusCode)

	}
	resp, _ = birthdayHandlerRequest(t, broken, http.MethodPost, "/events", eventReq, fiber.MIMEApplicationJSON)
	testutil.Require(t, !(resp.StatusCode != fiber.StatusBadRequest), "broken event status = %d", resp.StatusCode)

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
	{
		testutil.Require(t, !(resp.StatusCode != fiber.StatusOK), "empty render status=%d body=%s", resp.StatusCode, body)
		testutil.Require(t, strings.Contains(string(body), subscription.EmptyBirthdayMonitorMessage), "empty render status=%d body=%s", resp.StatusCode, body)
		testutil.Require(t, strings.Contains(string(body), sub.PlatformUserID), "empty render status=%d body=%s", resp.StatusCode, body)
	}

	_, body = birthdayHandlerRequest(t, app, http.MethodPost, "/bots/bot-1/render", requestFor(missingPayload.EventID), fiber.MIMEApplicationJSON)
	testutil.Require(t, strings.Contains(string(body), "缺少可绘制数据"), "missing-payload render body=%s", body)

	_, body = birthdayHandlerRequest(t, app, http.MethodPost, "/bots/bot-1/render", requestFor(nonEmpty.EventID), fiber.MIMEApplicationJSON)
	testutil.Require(t, strings.Contains(string(body), "服务未就绪"), "unready render body=%s", body)

	badToken := requestFor(nonEmpty.EventID)
	badToken.Token = "wrong"
	_, body = birthdayHandlerRequest(t, app, http.MethodPost, "/bots/bot-1/render", badToken, fiber.MIMEApplicationJSON)
	testutil.Require(t, strings.Contains(string(body), "请求处理失败"), "invalid-token render body=%s", body)

	mysekaiController := rendermysekai.NewController(nil, nil, renderregion.JP, assets.NewAssetHelper("", nil), rendermysekai.MasterdataOptions{})
	renderApp.MySekai = mysekaiController
	_, body = birthdayHandlerRequest(t, app, http.MethodPost, "/bots/bot-1/render", requestFor(nonEmpty.EventID), fiber.MIMEApplicationJSON)
	testutil.Require(t, strings.Contains(string(body), "服务未就绪"), "nil-cache render body=%s", body)

	renderApp.ImageCache = imagecache.New("https://cache.invalid", t.TempDir())
	_, body = birthdayHandlerRequest(t, app, http.MethodPost, "/bots/bot-1/render", requestFor(nonEmpty.EventID), fiber.MIMEApplicationJSON)
	testutil.Require(t, strings.Contains(string(body), "渲染服务未就绪"), "render failure body=%s", body)

	invalidReq := httptest.NewRequest(http.MethodPost, "/bots/bot-1/render", strings.NewReader("{"))
	invalidReq.Header.Set("Content-Type", fiber.MIMEApplicationJSON)
	invalidResp, err := app.Test(invalidReq)
	{
		testutil.Require(t, !(err != nil), "invalid render response = %+v, %v", invalidResp, err)
		testutil.Require(t, !(invalidResp.StatusCode != fiber.StatusBadRequest), "invalid render response = %+v, %v", invalidResp, err)
	}

	if invalidResp != nil {
		_ = invalidResp.Body.Close()
	}

	resp, body = birthdayHandlerRequest(t, app, http.MethodPost, "/bots/bot-1/ack", requestFor(nonEmpty.EventID), "application/msgpack")
	testutil.Require(t, !(resp.StatusCode != fiber.StatusOK), "ack status=%d body=%s", resp.StatusCode, body)

	var ackEnvelope renderEnvelope
	{
		err := stdjson.Unmarshal(body, &ackEnvelope)
		testutil.Require(t, !(err != nil), "decode ack envelope: %v body=%s", err, body)
	}

	badAck := requestFor(empty.EventID)
	badAck.Token = "wrong"
	_, body = birthdayHandlerRequest(t, app, http.MethodPost, "/bots/bot-1/ack", badAck, fiber.MIMEApplicationJSON)
	testutil.Require(t, strings.Contains(string(body), "请求处理失败"), "invalid ack body=%s", body)

	invalidAck := httptest.NewRequest(http.MethodPost, "/bots/bot-1/ack", strings.NewReader("{"))
	invalidAck.Header.Set("Content-Type", fiber.MIMEApplicationJSON)
	invalidAckResp, err := app.Test(invalidAck)
	{
		testutil.Require(t, !(err != nil), "invalid ack response = %+v, %v", invalidAckResp, err)
		testutil.Require(t, !(invalidAckResp.StatusCode != fiber.StatusBadRequest), "invalid ack response = %+v, %v", invalidAckResp, err)
	}

	if invalidAckResp != nil {
		_ = invalidAckResp.Body.Close()
	}
}

func TestBirthdayMonitorPureHelpersAndResponseEncoding(t *testing.T) {
	{
		testutil.RequireArgs(t, !(newBirthdayMonitorService(nil) != nil), "nil render app produced a birthday service")
		testutil.RequireArgs(t, !(newBirthdayMonitorDBService(nil) != nil), "nil render app produced a birthday service")
	}

	client := newAPIBirthdayDB(t)
	renderApp := &renderapp.App{PJSK: client, Config: renderapp.Config{ReadOnly: true}}
	{
		service := newBirthdayMonitorService(renderApp)
		testutil.RequireArgs(t, !(service == nil), "birthday monitor service is nil")
	}
	{

		service := newBirthdayMonitorDBService(renderApp)
		testutil.RequireArgs(t, !(service == nil), "birthday DB service is nil")
	}

	originalConfig := harukiConfig.Cfg
	t.Cleanup(func() { harukiConfig.Cfg = originalConfig })
	harukiConfig.Cfg.HMES.PublicBaseURL = " https://hmes.example/ "
	{
		testutil.RequireArgs(t, !(birthdayMonitorActions(nil) != nil), "incomplete birthday result produced actions")
		testutil.RequireArgs(t, !(birthdayMonitorActions(&subscription.BirthdayMonitorResult{}) != nil), "incomplete birthday result produced actions")
	}

	sub := createAPIBirthdaySubscription(t, client)
	result := &subscription.BirthdayMonitorResult{Subscription: sub, SubscriptionVersion: "v1", Token: "token"}
	actions := birthdayMonitorActions(result)
	{
		testutil.Require(t, !(len(actions) != 1), "birthday actions = %+v", actions)
		testutil.Require(t, !(actions[0].Endpoint != "https://hmes.example/sse"), "birthday actions = %+v", actions)
		testutil.Require(t, !(actions[0].SubscriptionID != fmt.Sprint(sub.ID)), "birthday actions = %+v", actions)
		testutil.Require(t, !(actions[0].ExpiresAt != sub.ExpiresAt.Unix()), "birthday actions = %+v", actions)
	}

	harukiConfig.Cfg.HMES.PublicBaseURL = " "
	testutil.RequireArgs(t, !(birthdayMonitorActions(result) != nil), "blank public URL produced actions")

	message := BotCommandRequest{Message: onebot11.Message{
		onebot11.Text(" first "),
		{Type: onebot11.TypeText, Data: map[string]string{onebot11.KeyText: "second"}},
		{Type: onebot11.TypeText, Data: map[string]any{onebot11.KeyText: "third"}},
		{Type: onebot11.TypeText, Data: map[string]any{onebot11.KeyText: 4}},
		onebot11.Image("ignored", ""),
	}}
	{
		got := requestMessageText(message)
		testutil.Require(t, !(got != "first secondthird"), "request message text = %q", got)
	}
	{

		got := birthdayMonitorCommandText(BotCommandRequest{Message: onebot11.Message{onebot11.Text("/烤森生日监听 钻石")}})
		testutil.Require(t, !(got != "/烤森生日监听 钻石"), "complete command text = %q", got)
	}
	{

		got := birthdayMonitorCommandText(BotCommandRequest{MatchedCommand: " /烤森生日监听 ", Message: onebot11.Message{onebot11.Text("bad")}})
		testutil.Require(t, !(got != "/烤森生日监听 bad"), "fallback command text = %q", got)
	}
	{

		got := birthdayMonitorCommandText(BotCommandRequest{MatchedCommand: "/烤森生日监听"})
		testutil.Require(t, !(got != "/烤森生日监听"), "matched-only command text = %q", got)
	}
	{

		got := birthdayMonitorCommandText(BotCommandRequest{Message: onebot11.Message{onebot11.Text("bad")}})
		testutil.Require(t, !(got != "bad"), "message-only command text = %q", got)
	}

	prefixes := buildBirthdayMonitorManifestCommandPrefixes([]string{"", " /test ", "/test"})
	{
		testutil.Require(t, !(len(prefixes) != 16), "manifest prefixes = %v", prefixes)
		testutil.Require(t, slices.Contains(prefixes, "/test"), "manifest prefixes = %v", prefixes)
		testutil.Require(t, slices.Contains(prefixes, "/jptest"), "manifest prefixes = %v", prefixes)
		testutil.Require(t, slices.Contains(prefixes, "/jp /test"), "manifest prefixes = %v", prefixes)
	}
	{
		testutil.RequireArgs(t, isCancelBirthdayMonitorText("/烤森生日取消监听"), "birthday cancel recognition mismatch")
		testutil.RequireArgs(t, !(isCancelBirthdayMonitorText("/烤森生日监听")), "birthday cancel recognition mismatch")
		testutil.RequireArgs(t, !(isCancelBirthdayMonitorText("bad")), "birthday cancel recognition mismatch")
	}

	finished := 0
	finishPhaseOnPanic(func() { finished++ })
	testutil.RequireArgs(t, !(finished != 0), "finish callback ran without a panic")

	func() {
		defer func() {
			testutil.RequireArgs(t, !(recover() == nil), "finishPhaseOnPanic swallowed panic")

		}()
		defer finishPhaseOnPanic(func() { finished++ })
		panic("boom")
	}()
	testutil.Require(t, !(finished != 1), "panic finish count = %d", finished)

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
	{
		testutil.Require(t, !(resp.StatusCode != fiber.StatusCreated), "JSON action response status=%d body=%s", resp.StatusCode, body)
		testutil.Require(t, strings.Contains(string(body), "client_actions"), "JSON action response status=%d body=%s", resp.StatusCode, body)
	}

	resp, body = birthdayHandlerRequest(t, app, http.MethodPost, "/msgpack", nil, "")
	var decoded map[string]any
	{
		testutil.Require(t, !(resp.StatusCode != fiber.StatusOK), "MsgPack action response status=%d body=%x decoded=%v", resp.StatusCode, body, decoded)
		testutil.Require(t, !(msgpack.Unmarshal(body, &decoded) != nil), "MsgPack action response status=%d body=%x decoded=%v", resp.StatusCode, body, decoded)
		testutil.Require(t, !(decoded["client_actions"] == nil), "MsgPack action response status=%d body=%x decoded=%v", resp.StatusCode, body, decoded)
	}

	resp, _ = birthdayHandlerRequest(t, app, http.MethodPost, "/msgpack-error", nil, "")
	testutil.Require(t, !(resp.StatusCode != fiber.StatusInternalServerError), "MsgPack encode error status = %d", resp.StatusCode)

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
		{
			testutil.Require(t, !(resp.StatusCode != fiber.StatusOK), "parse %s status=%d body=%s", contentType, resp.StatusCode, body)
			testutil.Require(t, !(string(body) != "event"), "parse %s status=%d body=%s", contentType, resp.StatusCode, body)
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
	{
		_, err := bindings.Bind(ctx, "qq", "user-1", "123456789012345678")
		testutil.Require(t, !(err != nil), "bind birthday account: %v", err)
	}
	{

		err := pjskClient.UserBinding.Update().SetVerified(true).Exec(ctx)
		testutil.Require(t, !(err != nil), "verify birthday binding: %v", err)
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
	{
		testutil.Require(t, !(resp.StatusCode != fiber.StatusOK), "create monitor status=%d body=%s", resp.StatusCode, body)
		testutil.Require(t, strings.Contains(string(body), "有效期 10 分钟"), "create monitor status=%d body=%s", resp.StatusCode, body)
		testutil.Require(t, strings.Contains(string(body), "client_actions"), "create monitor status=%d body=%s", resp.StatusCode, body)
		testutil.Require(t, strings.Contains(string(body), "hmes_sse"), "create monitor status=%d body=%s", resp.StatusCode, body)
	}

	request.MatchedCommand = "/烤森生日取消监听"
	request.Message = onebot11.Message{onebot11.Text("/烤森生日取消监听")}
	resp, body = birthdayHandlerRequest(t, app, http.MethodPost, "/bots/bot-1/monitor", request, fiber.MIMEApplicationJSON)
	{
		testutil.Require(t, !(resp.StatusCode != fiber.StatusOK), "cancel monitor status=%d body=%s", resp.StatusCode, body)
		testutil.Require(t, strings.Contains(string(body), "监听已取消"), "cancel monitor status=%d body=%s", resp.StatusCode, body)
	}
	testutil.Require(t, !(guard.completed != 2), "guard completion count = %d", guard.completed)

	invalid := httptest.NewRequest(http.MethodPost, "/bots/bot-1/monitor", strings.NewReader("{"))
	invalid.Header.Set("Content-Type", fiber.MIMEApplicationJSON)
	invalidResp, err := app.Test(invalid)
	{
		testutil.Require(t, !(err != nil), "invalid monitor response = %+v, %v", invalidResp, err)
		testutil.Require(t, !(invalidResp.StatusCode != fiber.StatusBadRequest), "invalid monitor response = %+v, %v", invalidResp, err)
	}

	if invalidResp != nil {
		_ = invalidResp.Body.Close()
	}
}
