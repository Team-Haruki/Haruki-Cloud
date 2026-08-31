package pjsk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"haruki-cloud/api"
	botauth "haruki-cloud/api/bot/auth"
	usersenttest "haruki-cloud/database/users/enttest"
	commandregistry "haruki-cloud/internal/handler"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	commandhandler "haruki-cloud/internal/pjsk/handler"
	renderapp "haruki-cloud/internal/pjsk/render/app"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"github.com/shamaton/msgpack/v3"
)

func TestHandlerEncodingAndPureHelperBranches(t *testing.T) {
	testSharedCommandEncodingAndValidation(t)
	testExplicitProfileRegionSynchronization(t)
	testBotCommandTextAndCompatibilityHelpers(t)
}

func testSharedCommandEncodingAndValidation(t *testing.T) {
	t.Helper()
	metadata := sharedCommandMetadata{Command: "/coverage", Outcome: "ok"}
	result := encodeSharedCommandResult(
		context.Background(),
		newBotResponseEnvelope(fiber.StatusOK, api.ResponseOK, make(chan int)),
		metadata,
		true,
	)
	if result.Response.HTTPStatus != fiber.StatusInternalServerError {
		t.Fatalf("fallback response status = %d", result.Response.HTTPStatus)
	}
	if result.Metadata.Outcome != "error" || result.Metadata.ErrorType == "" || !result.ForceExecutor {
		t.Fatalf("fallback result = %+v", result)
	}

	validationErr := &botValidationError{msg: "wrong route", actualPath: "profile/other"}
	envelope := commandErrorEnvelope(validationErr, "profile/want", "/want", false)
	if envelope.HTTPStatus != fiber.StatusBadRequest || envelope.Message != "指令与当前接口不匹配" {
		t.Fatalf("validation envelope = %+v", envelope)
	}
	wire, ok := envelope.Data.(BotCommandErrorResponse)
	if !ok || wire.Error != "wrong route" || wire.ExpectedPath != "profile/want" || wire.MatchedCommand != "/want" {
		t.Fatalf("validation envelope data = %#v", envelope.Data)
	}
	if validationErr.Error() != "wrong route" || !isExpectedCommandError(validationErr) {
		t.Fatalf("validation error not classified: %v", validationErr)
	}
	if isExpectedCommandError(nil) || isExpectedCommandError(errors.New("unexpected")) {
		t.Fatal("nil and generic errors must not be expected command errors")
	}
}

func testExplicitProfileRegionSynchronization(t *testing.T) {
	t.Helper()
	syncExplicitRegionToProfileParams(nil, "jp")
	invalidRegion := &commandhandler.CommandRequest{Mode: accountdata.ProfileModeBindList, Params: []byte(`{"platform":"qq","platform_user_id":"1"}`)}
	syncExplicitRegionToProfileParams(invalidRegion, "invalid")
	unknownMode := &commandhandler.CommandRequest{Mode: "other", Params: []byte(`{"unchanged":true}`)}
	syncExplicitRegionToProfileParams(unknownMode, "jp")
	if string(unknownMode.Params) != `{"unchanged":true}` {
		t.Fatalf("unknown mode params changed: %s", unknownMode.Params)
	}

	badBinding := &commandhandler.CommandRequest{Mode: accountdata.ProfileModeBindList, Params: []byte(`{`)}
	syncExplicitRegionToProfileParams(badBinding, "en")
	if string(badBinding.Params) != "{" {
		t.Fatalf("invalid binding params changed: %s", badBinding.Params)
	}
	bindingRaw, err := json.Marshal(accountdata.ProfileBindingCommandParams{
		Platform: "qq", PlatformUserID: "1", Server: "jp",
	})
	if err != nil {
		t.Fatalf("marshal binding params: %v", err)
	}
	binding := &commandhandler.CommandRequest{Mode: accountdata.ProfileModeDefaultSet, Params: bindingRaw}
	syncExplicitRegionToProfileParams(binding, "en")
	decodedBinding, err := accountdata.DecodeProfileBindingParams(binding.Params)
	if err != nil {
		t.Fatalf("decode synchronized binding params: %v", err)
	}
	if decodedBinding.Server != "en" || decodedBinding.Scope != "en" {
		t.Fatalf("synchronized binding params = %+v", decodedBinding)
	}

	badSettings := &commandhandler.CommandRequest{Mode: accountdata.ProfileModeHideID, Params: []byte(`{`)}
	syncExplicitRegionToProfileParams(badSettings, "tw")
	if string(badSettings.Params) != "{" {
		t.Fatalf("invalid settings params changed: %s", badSettings.Params)
	}
	settingsRaw, err := json.Marshal(accountdata.ProfileSettingsCommandParams{
		Platform: "qq", PlatformUserID: "1", Server: "jp",
	})
	if err != nil {
		t.Fatalf("marshal settings params: %v", err)
	}
	settings := &commandhandler.CommandRequest{Mode: accountdata.ProfileModeShowID, Params: settingsRaw}
	syncExplicitRegionToProfileParams(settings, "cn")
	decodedSettings, err := accountdata.DecodeProfileSettingsParams(settings.Params)
	if err != nil {
		t.Fatalf("decode synchronized settings params: %v", err)
	}
	if decodedSettings.Server != "cn" || !decodedSettings.RegionExplicit {
		t.Fatalf("synchronized settings params = %+v", decodedSettings)
	}
}

func testBotCommandTextAndCompatibilityHelpers(t *testing.T) {
	t.Helper()
	text := extractBotCommandText(onebot11.Message{
		{Type: onebot11.TypeImage, Data: onebot11.ImageData{File: "ignored"}},
		{Type: onebot11.TypeText, Data: map[string]any{onebot11.KeyText: "map-any "}},
		{Type: onebot11.TypeText, Data: map[string]string{onebot11.KeyText: "map-string"}},
	})
	if text != "map-any map-string" {
		t.Fatalf("extracted text = %q", text)
	}
	if _, ok := explicitRegionFromCommandText(""); ok {
		t.Fatal("empty command unexpectedly had a region")
	}

	compatTests := []struct {
		expected string
		actual   string
		want     bool
	}{
		{expected: "", actual: "profile/a"},
		{expected: "profile/a", actual: "sk/a"},
		{expected: "profile/a", actual: "profile/b", want: true},
		{expected: "event/a", actual: "event/b"},
	}
	for _, tt := range compatTests {
		if got := canRetryBotPathCompat(tt.expected, tt.actual); got != tt.want {
			t.Errorf("canRetryBotPathCompat(%q, %q) = %v, want %v", tt.expected, tt.actual, got, tt.want)
		}
	}
	if got := botPathFamily(" profile "); got != "profile" {
		t.Fatalf("botPathFamily without slash = %q", got)
	}
}

func TestHandlerRequestTransportAndDetachBranches(t *testing.T) {
	req := detachBotCommandRequest(BotCommandRequest{Message: onebot11.Message{
		{Type: onebot11.TypeImage, Data: onebot11.ImageData{File: "file", Url: "url"}},
		{Type: onebot11.TypeAt, Data: onebot11.AtData{QQ: "123"}},
		{Type: "custom", Data: map[string]any{"value": "kept"}},
	}})
	if image, ok := req.Message[0].Data.(onebot11.ImageData); !ok || image.File != "file" || image.Url != "url" {
		t.Fatalf("detached image = %#v", req.Message[0].Data)
	}
	if at, ok := req.Message[1].Data.(onebot11.AtData); !ok || at.QQ != "123" {
		t.Fatalf("detached at = %#v", req.Message[1].Data)
	}
	if _, ok := req.Message[2].Data.(map[string]any); !ok {
		t.Fatalf("detached custom data = %#v", req.Message[2].Data)
	}

	app := fiber.New()
	app.Post("/parse", func(c fiber.Ctx) error {
		if _, err := parseBotRequest(c); err != nil {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	app.Get("/secure", func(c fiber.Ctx) error {
		c.Locals("secure_noise", true)
		return botResponse(c, fiber.StatusAccepted, "accepted", map[string]string{"value": "ok"})
	})

	invalid := httptest.NewRequest(http.MethodPost, "/parse", bytes.NewReader([]byte{0x81}))
	invalid.Header.Set(fiber.HeaderContentType, api.ContentTypeMsgPack)
	response, err := app.Test(invalid)
	if err != nil {
		t.Fatalf("invalid msgpack request: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("invalid msgpack status = %d", response.StatusCode)
	}

	encodedRequest, err := msgpack.Marshal(BotCommandRequest{
		Platform:       "qq",
		PlatformUserID: "1",
		MatchedCommand: "/ok",
		Message:        onebot11.Message{onebot11.Text("/ok")},
	})
	if err != nil {
		t.Fatalf("encode msgpack request: %v", err)
	}
	valid := httptest.NewRequest(http.MethodPost, "/parse", bytes.NewReader(encodedRequest))
	valid.Header.Set(fiber.HeaderContentType, api.ContentTypeMsgPack)
	response, err = app.Test(valid)
	if err != nil {
		t.Fatalf("valid msgpack request: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != fiber.StatusNoContent {
		t.Fatalf("valid msgpack status = %d", response.StatusCode)
	}

	response, err = app.Test(httptest.NewRequest(http.MethodGet, "/secure", nil))
	if err != nil {
		t.Fatalf("secure response request: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != fiber.StatusAccepted || response.Header.Get(fiber.HeaderContentType) != api.ContentTypeMsgPack {
		t.Fatalf("secure response status/type = %d/%q", response.StatusCode, response.Header.Get(fiber.HeaderContentType))
	}
}

func TestVerifyBotOwnerNotBannedFailureAndPassThroughBranches(t *testing.T) {
	ctx := context.Background()
	botClient := newBotCommandTestClient(t, "owner_ban_coverage")
	const ownerID int64 = 99887766
	const botID = 7654321
	if _, err := botClient.User.Create().
		SetOwnerUserID(ownerID).
		SetBotID(botID).
		SetCredential("coverage").
		Save(ctx); err != nil {
		t.Fatalf("create bot owner: %v", err)
	}
	usersClient := usersenttest.Open(t, "sqlite3", fmt.Sprintf("file:bot_owner_ban_coverage_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))

	app := fiber.New()
	app.Get("/bot/:botId", verifyBotOwnerNotBanned(botClient, accountdata.NewBanService(usersClient)), func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})
	for _, tt := range []struct {
		path string
		want int
	}{
		{path: "/bot/not-a-number", want: fiber.StatusUnauthorized},
		{path: "/bot/123", want: fiber.StatusUnauthorized},
		{path: fmt.Sprintf("/bot/%d", botID), want: fiber.StatusNoContent},
	} {
		response, err := app.Test(httptest.NewRequest(http.MethodGet, tt.path, nil))
		if err != nil {
			t.Fatalf("GET %s: %v", tt.path, err)
		}
		response.Body.Close()
		if response.StatusCode != tt.want {
			t.Errorf("GET %s status = %d, want %d", tt.path, response.StatusCode, tt.want)
		}
	}

	if err := usersClient.Close(); err != nil {
		t.Fatalf("close users client: %v", err)
	}
	response, err := app.Test(httptest.NewRequest(http.MethodGet, fmt.Sprintf("/bot/%d", botID), nil))
	if err != nil {
		t.Fatalf("closed users DB request: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("closed users DB status = %d", response.StatusCode)
	}

	closedBotClient := newBotCommandTestClient(t, "owner_ban_closed_bot")
	if err := closedBotClient.Close(); err != nil {
		t.Fatalf("close bot client: %v", err)
	}
	closedApp := fiber.New()
	closedApp.Get("/bot/:botId", verifyBotOwnerNotBanned(closedBotClient, accountdata.NewBanService(usersClient)))
	response, err = closedApp.Test(httptest.NewRequest(http.MethodGet, "/bot/1", nil))
	if err != nil {
		t.Fatalf("closed bot DB request: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("closed bot DB status = %d", response.StatusCode)
	}
}

func TestMakeBotHandlerValidationElectionAndTelemetryBranches(t *testing.T) {
	request := BotCommandRequest{
		Platform:       "qq",
		PlatformUserID: "123",
		MatchedCommand: "/ok",
		Message:        onebot11.Message{onebot11.Text("/ok")},
	}
	requestBody, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("encode handler request: %v", err)
	}

	t.Run("decode error", func(t *testing.T) {
		testBotHandlerDecodeError(t)
	})
	t.Run("command not allowed", func(t *testing.T) {
		testBotHandlerCommandNotAllowed(t, request)
	})
	t.Run("replay rejected", func(t *testing.T) {
		testBotHandlerReplayRejected(t, requestBody)
	})
	for _, reason := range []string{"publish_unknown", "not_selected"} {
		t.Run(reason, func(t *testing.T) {
			testBotHandlerInvisibleDecision(t, requestBody, reason)
		})
	}

	t.Run("closed telemetry", func(t *testing.T) {
		testBotHandlerClosedTelemetry(t, requestBody)
	})
}

func postBotHandlerStatus(t *testing.T, handler fiber.Handler, body []byte) int {
	t.Helper()
	app := fiber.New()
	app.Post("/bot/:botId", handler)
	req := httptest.NewRequest(http.MethodPost, "/bot/42", bytes.NewReader(body))
	req.Header.Set(fiber.HeaderContentType, api.ContentTypeJSON)
	response, err := app.Test(req)
	if err != nil {
		t.Fatalf("handler request: %v", err)
	}
	response.Body.Close()
	return response.StatusCode
}

func testBotHandlerDecodeError(t *testing.T) {
	t.Helper()
	handler := makeBotHandler(&renderapp.App{}, nil, nil, nil, "event/detail", []string{"/ok"})
	if got := postBotHandlerStatus(t, handler, []byte(`{`)); got != fiber.StatusBadRequest {
		t.Fatalf("decode error status = %d", got)
	}
}

func testBotHandlerCommandNotAllowed(t *testing.T, request BotCommandRequest) {
	t.Helper()
	request.MatchedCommand = "/bad"
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal bad command: %v", err)
	}
	handler := makeBotHandler(&renderapp.App{}, nil, nil, nil, "event/detail", []string{"/ok"})
	if got := postBotHandlerStatus(t, handler, body); got != fiber.StatusBadRequest {
		t.Fatalf("disallowed command status = %d", got)
	}
}

func testBotHandlerReplayRejected(t *testing.T, requestBody []byte) {
	t.Helper()
	election := &responseElectionCoverageStub{}
	replay := newTestReplayGuard(&fakeNonceStore{}, true, time.Now())
	handler := makeBotHandler(&renderapp.App{}, election, nil, replay, "event/detail", []string{"/ok"})
	if got := postBotHandlerStatus(t, handler, requestBody); got != fiber.StatusOK || election.called != 0 {
		t.Fatalf("replay status/election calls = %d/%d", got, election.called)
	}
}

func testBotHandlerInvisibleDecision(t *testing.T, requestBody []byte, reason string) {
	t.Helper()
	election := &responseElectionCoverageStub{decision: responseElectionDecision{reason: reason}}
	handler := makeBotHandler(&renderapp.App{}, election, nil, nil, "event/detail", []string{"/ok"})
	if got := postBotHandlerStatus(t, handler, requestBody); got != fiber.StatusOK || election.called != 1 {
		t.Fatalf("invisible status/election calls = %d/%d", got, election.called)
	}
}

func testBotHandlerClosedTelemetry(t *testing.T, requestBody []byte) {
	t.Helper()
	botClient := newBotCommandTestClient(t, "handler_closed_telemetry")
	telemetry := botauth.NewCommandTelemetryDispatcher(botClient)
	telemetry.Close()
	encoded, err := encodeBotResponseEnvelope(newBotResponseEnvelope(fiber.StatusOK, api.ResponseOK))
	if err != nil {
		t.Fatalf("encode visible response: %v", err)
	}
	election := &responseElectionCoverageStub{decision: responseElectionDecision{
		visible: true,
		result: sharedCommandResult{
			Response: encoded,
			Metadata: sharedCommandMetadata{Command: "/ok", CommandPath: "event/detail", Outcome: "ok"},
		},
	}}
	handler := makeBotHandler(&renderapp.App{}, election, telemetry, nil, "event/detail", []string{"/ok"})
	if got := postBotHandlerStatus(t, handler, requestBody); got != fiber.StatusOK {
		t.Fatalf("visible response status = %d", got)
	}
}

func TestBotRouteDispatchersCloseAllComponents(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	botClient := newBotCommandTestClient(t, "dispatcher_close_coverage")
	dispatchers := &BotRouteDispatchers{
		guard:     NewRequestGuard(client),
		election:  NewResponseElectionCoordinator(context.Background(), client, time.Millisecond),
		telemetry: botauth.NewCommandTelemetryDispatcher(botClient),
	}
	dispatchers.Close()
	dispatchers.Close()
}

func TestBotCommandMatchFallbackBranches(t *testing.T) {
	openHandler := &commandregistry.CommandHandlerBase{Path: "profile/open"}
	closedHandler := &commandregistry.CommandHandlerBase{}
	disabledHandler := &commandregistry.CommandHandlerBase{Disabled: true, Path: "profile/disabled"}
	matched := commandregistry.MatchedHandler{Command: "/open", Handler: openHandler}

	if !botCommandMatchRegistered(matched, true) {
		t.Fatal("enabled registered command should be recognized")
	}
	if botCommandMatchRegistered(matched, false) {
		t.Fatal("failed lookup must not be recognized as registered")
	}
	if botCommandMatchRegistered(commandregistry.MatchedHandler{}, true) {
		t.Fatal("nil handler must not be recognized as registered")
	}
	if botCommandMatchRegistered(commandregistry.MatchedHandler{Handler: disabledHandler}, true) {
		t.Fatal("disabled handler must not be recognized as registered")
	}

	actual := commandregistry.MatchedHandler{Command: "/actual", Handler: openHandler}
	got, err := fallbackBotCommandMatch(commandregistry.MatchedHandler{}, actual, false, "/missing")
	if err != nil || got.Command != actual.Command || got.Handler != actual.Handler {
		t.Fatalf("actual command fallback = %+v, %v", got, err)
	}

	_, err = fallbackBotCommandMatch(
		commandregistry.MatchedHandler{Command: "/closed", Handler: closedHandler},
		commandregistry.MatchedHandler{},
		true,
		"/closed",
	)
	var validationErr *botValidationError
	if !errors.As(err, &validationErr) || validationErr.msg != "matched_command 未开放给 Bot API: /closed" {
		t.Fatalf("closed command error = %v", err)
	}

	_, err = fallbackBotCommandMatch(
		commandregistry.MatchedHandler{},
		commandregistry.MatchedHandler{Handler: disabledHandler},
		false,
		"/missing",
	)
	if !errors.As(err, &validationErr) || validationErr.msg != "matched_command 未注册: /missing" {
		t.Fatalf("missing command error = %v", err)
	}
}
