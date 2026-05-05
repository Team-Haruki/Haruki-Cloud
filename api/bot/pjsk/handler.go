package pjsk

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"haruki-cloud/api"
	botauth "haruki-cloud/api/bot/auth"
	harukiConfig "haruki-cloud/config"
	botDB "haruki-cloud/database/bot"
	"haruki-cloud/database/bot/commandmanifest"
	"haruki-cloud/internal/core/crypto"
	commandregistry "haruki-cloud/internal/handler"
	"haruki-cloud/internal/middleware/secure"
	"haruki-cloud/internal/onebot11"
	commandhandler "haruki-cloud/internal/pjsk/handler"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/utils/logger"
	"haruki-cloud/version"

	"entgo.io/ent/dialect/sql"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"github.com/shamaton/msgpack/v3"
)

const botRouteBase = "/api/v2/bot"

const slowCommandTraceThreshold = 1500 * time.Millisecond

// RegisterPJSKBotRoutes registers per-feature bot endpoints under
//
//	/api/v2/bot/:botId/pjsk/<path>
//
// and the command manifest endpoint at
//
//	GET /api/v2/bot/:botId/command/manifests
//
// The canonical PJSK bot protocol is POST + JSON body:
//
//	POST /api/v2/bot/:botId/pjsk/<path>
//	Content-Type: application/json
//
//	{"platform":"qq","platform_user_id":"12345","server":"jp",
//	 "matched_command":"/cmd","message":[{"type":"text","data":{"text":"/cmd args"}}],
//	 "enableParamEcho":false}
//
// When noiseKeyPair is non-nil, the Noise IK transport encryption middleware is applied
// to the pjsk route group. Clients must then send Noise IK Message 1 containing a
// MsgPack-encoded BotCommandRequest as the HTTP body, and will receive Noise IK Message 2
// containing a MsgPack-encoded response. The manifest endpoint is NOT behind Noise.
//
// When redisClient is non-nil, the api.VerifyBotSession middleware is applied to
// authenticate requests via X-Haruki-Bot-Id and X-Haruki-Bot-Session-Token headers.
// Pass nil for redisClient in unit tests (auth is skipped).
//
// When botDBClient is non-nil, the manifest table is synchronized from the
// registered handler routes on startup and the manifest endpoint returns live
// data from the database.
// Pass nil to keep the placeholder response (e.g. in unit tests).
func RegisterPJSKBotRoutes(app *fiber.App, renderApp *renderapp.App, redisClient *redis.Client, botDBClient *botDB.Client, noiseKeyPair *crypto.KeyPair) {
	RegisterPJSKBotRoutesWithContext(context.Background(), app, renderApp, redisClient, botDBClient, noiseKeyPair)
}

func RegisterPJSKBotRoutesWithContext(initCtx context.Context, app *fiber.App, renderApp *renderapp.App, redisClient *redis.Client, botDBClient *botDB.Client, noiseKeyPair *crypto.KeyPair) {
	if renderApp == nil {
		return
	}
	if initCtx == nil {
		initCtx = context.Background()
	}

	commandhandler.EnsureCommandHandlersRegistered()

	if botDBClient != nil {
		if err := SeedCommandManifests(initCtx, botDBClient); err != nil {
			// Non-fatal: manifest table seed failure should not block startup.
			logger.Warnf("bot manifest seed failed: %v", err)
		}
	}

	var sessionMiddleware fiber.Handler
	if redisClient != nil {
		sessionMiddleware = api.VerifyBotSession(redisClient)
	} else {
		sessionMiddleware = api.VerifyBotSessionTestBypass()
	}
	bot := app.Group(botRouteBase+"/:botId", sessionMiddleware)

	bot.Get("/command/manifests", buildManifestHandler(botDBClient))

	guard := NewRequestGuard(redisClient)

	pjsk := bot.Group("/pjsk")
	if noiseKeyPair != nil {
		pjsk.Use(secure.New(secure.Config{ServerPrivateKey: noiseKeyPair}))
	}
	registerBirthdayMonitorRoutes(pjsk, app, renderApp)
	routes := commandregistry.ListBotRoutes()
	hasMysekaiBlueprintRoute := false
	for _, route := range routes {
		h := makeBotHandler(renderApp, guard, botDBClient, route.Path, route.Commands)
		path := "/" + route.Path
		pjsk.Post(path, h)
		if route.Path == "mysekai/blueprint" {
			hasMysekaiBlueprintRoute = true
		}
	}
	if !hasMysekaiBlueprintRoute {
		// Keep the retired endpoint alive so older manifests can continue posting
		// /msb until they refresh to the canonical mysekai/talk-list path.
		pjsk.Post("/mysekai/blueprint", makeBotHandler(renderApp, guard, botDBClient, "mysekai/blueprint", nil))
	}
}

// makeBotHandler returns a POST-only fiber.Handler that validates the matched
// command field belongs to the current endpoint path, then lets the registered
// handler parse the OneBot message segments and produce a resolved render command.
func makeBotHandler(renderApp *renderapp.App, guard *RequestGuard, botDBClient *botDB.Client, expectedPath string, commands []string) fiber.Handler {
	return func(c fiber.Ctx) error {
		req, err := parseBotRequest(c)
		if err != nil {
			return botResponse(c, fiber.StatusBadRequest, "请求格式错误")
		}
		requestCtx, commandTrace := commandhandler.WithCommandTrace(c.Context())
		if len(req.Message) == 0 {
			return botResponse(c, fiber.StatusBadRequest, "缺少 message")
		}
		if req.MatchedCommand == "" {
			return botResponse(c, fiber.StatusBadRequest, "缺少 matched_command")
		}

		// Dedup + rate limit: acquire guard before doing any work.
		if !guard.Acquire(requestCtx, req) {
			return botResponse(c, fiber.StatusOK, api.ResponseOK, make(onebot11.Message, 0))
		}
		allowCompatReroute := allowBotCompatReroute(expectedPath)
		if !slices.Contains(commands, req.MatchedCommand) && !allowCompatReroute {
			return botResponse(c, fiber.StatusBadRequest, "当前接口不允许使用该 matched_command")
		}
		if botDBClient != nil {
			botID := fiber.Params[int](c, "botId", 0)
			entry := botauth.CommandLogEntry{
				Platform: req.Platform,
				PID:      strings.TrimSpace(c.Params("botId")),
				GID:      req.PlatformGroupID,
				UID:      req.PlatformUserID,
				Command:  req.MatchedCommand,
			}
			defer func() {
				if err := botauth.RecordCommandTelemetry(requestCtx, botDBClient, botID, entry); err != nil {
					logger.Warnf("bot command telemetry failed: bot_id=%d path=%s matched_command=%s err=%v", botID, expectedPath, req.MatchedCommand, err)
				}
			}()
		}

		tStart := time.Now()
		botID := strings.TrimSpace(c.Params("botId"))
		resolved, err := resolveBotCommand(requestCtx, req.Message, expectedPath, req, botID)
		if err != nil && allowCompatReroute {
			if validationErr, ok := errors.AsType[*botValidationError](err); ok && canRetryBotPathCompat(expectedPath, validationErr.actualPath) {
				logger.Warnf("bot command compat reroute: matched_command=%s expected_path=%s actual_path=%s", req.MatchedCommand, expectedPath, validationErr.actualPath)
				resolved, err = resolveBotCommand(requestCtx, req.Message, validationErr.actualPath, req, botID)
			}
		}
		tResolved := time.Now()
		if err != nil {
			logger.Warnf("bot command resolve failed: path=%s matched_command=%s parse=%dms err=%v", expectedPath, req.MatchedCommand, tResolved.Sub(tStart).Milliseconds(), err)
			return errorResponse(c, fiber.StatusOK, err, expectedPath, req.MatchedCommand, req.EnableParamEcho)
		}

		if server := strings.TrimSpace(req.Server); server != "" && !resolved.RegionExplicit {
			if normalized := renderregion.Normalize(server); !normalized.IsZero() {
				// Treat the transport-level server as authoritative so the final
				// command executor does not overwrite it with the user's global
				// default binding.
				resolved.Region = normalized.String()
				resolved.RegionExplicit = true
			} else {
				resolved.Region = server
			}
		}

		responseData, err := commandhandler.ExecuteCommandRequest(requestCtx, resolved, renderApp)
		tDone := time.Now()
		logSlowCommandTrace(commandTrace, req.MatchedCommand, expectedPath, tStart, tDone)
		if err != nil {
			logger.Errorf("bot command render failed: matched_command=%s parse=%dms exec=%dms total=%dms err=%v",
				req.MatchedCommand, tResolved.Sub(tStart).Milliseconds(), tDone.Sub(tResolved).Milliseconds(), tDone.Sub(tStart).Milliseconds(), err)
			guard.MarkComplete(requestCtx, req)
			return errorResponse(c, fiber.StatusOK, err, expectedPath, req.MatchedCommand, req.EnableParamEcho)
		}
		logger.Infof("bot command completed: matched_command=%s parse=%dms exec=%dms total=%dms",
			req.MatchedCommand, tResolved.Sub(tStart).Milliseconds(), tDone.Sub(tResolved).Milliseconds(), tDone.Sub(tStart).Milliseconds())
		guard.MarkComplete(requestCtx, req)
		return botResponse(c, fiber.StatusOK, api.ResponseOK, responseData)
	}
}

func logSlowCommandTrace(trace *commandhandler.CommandTrace, matchedCommand, expectedPath string, startedAt, finishedAt time.Time) {
	if trace == nil || finishedAt.Sub(startedAt) < slowCommandTraceThreshold {
		return
	}
	summary := trace.Summary()
	if summary == "" {
		return
	}
	logger.Infof("bot command trace: matched_command=%s path=%s total=%dms %s",
		matchedCommand, expectedPath, finishedAt.Sub(startedAt).Milliseconds(), summary)
}

// parseBotRequest binds BotCommandRequest from the POST body.
// When the request arrived through the Noise IK middleware, the Content-Type is
// application/msgpack and the body is decoded with MsgPack.
// Otherwise the standard JSON binding is used.
func parseBotRequest(c fiber.Ctx) (BotCommandRequest, error) {
	var req BotCommandRequest
	ct := string(c.Request().Header.ContentType())
	if strings.Contains(ct, "msgpack") {
		if err := msgpack.Unmarshal(c.Body(), &req); err != nil {
			return BotCommandRequest{}, err
		}
	} else if err := c.Bind().Body(&req); err != nil {
		return BotCommandRequest{}, err
	}
	logger.Debugf("before parse: %+v", req.Message)
	req.Message = onebot11.ParseMessage(req.Message)
	logger.Debugf("after parse: %+v", req.Message)
	return req, nil
}

// botResponse sends a response using MsgPack when the request came through the
// Noise IK transport layer, and JSON otherwise.
func botResponse(c fiber.Ctx, status int, message string, data ...any) error {
	if c.Locals("secure_noise") != nil {
		return api.MsgPackResponse(c, status, message, data...)
	}
	return api.JSONResponse(c, status, message, data...)
}

func errorResponse(c fiber.Ctx, status int, err error, expectedPath, matchedCommand string, enableParamEcho bool) error {
	if _, ok := errors.AsType[*botValidationError](err); ok {
		return botResponse(c, fiber.StatusBadRequest, "指令与当前接口不匹配",
			BotCommandErrorResponse{
				Error:          err.Error(),
				ExpectedPath:   expectedPath,
				MatchedCommand: matchedCommand,
			})
	}
	if replyErr, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return botResponse(c, fiber.StatusOK, api.ResponseOK,
			[]onebot11.Segment{onebot11.Text(clientErrorText(string(replyErr), enableParamEcho))},
		)
	}
	return botResponse(c, fiber.StatusOK, api.ResponseOK,
		[]onebot11.Segment{onebot11.Text(clientErrorText(err.Error(), enableParamEcho))},
	)
}

type botValidationError struct {
	msg        string
	actualPath string
}

func (e *botValidationError) Error() string {
	return e.msg
}

var botCompatPathFamilies = map[string]struct{}{
	"mysekai": {},
	"profile": {},
	"sk":      {},
}

func allowBotCompatReroute(path string) bool {
	_, ok := botCompatPathFamilies[botPathFamily(path)]
	return ok
}

func canRetryBotPathCompat(expectedPath, actualPath string) bool {
	if expectedPath == "" || actualPath == "" || expectedPath == actualPath {
		return false
	}
	expectedFamily := botPathFamily(expectedPath)
	if expectedFamily == "" || expectedFamily != botPathFamily(actualPath) {
		return false
	}
	_, ok := botCompatPathFamilies[expectedFamily]
	return ok
}

func botPathFamily(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if idx := strings.IndexByte(path, '/'); idx >= 0 {
		return path[:idx]
	}
	return path
}

func resolveBotCommand(requestCtx context.Context, message onebot11.Message, expectedPath string, req BotCommandRequest, botID string) (*commandhandler.CommandRequest, error) {

	matchedCommand := req.MatchedCommand
	messageType := commandhandler.MessageTypePrivate
	if req.PlatformGroupID != "" {
		messageType = commandhandler.MessageTypeGroup
	}

	event := commandhandler.Event{
		Platform:    req.Platform,
		MessageType: messageType,
		Message:     message,
		UserId:      req.PlatformUserID,
		GroupId:     req.PlatformGroupID,
	}

	ctx, err := commandhandler.BuildContext(requestCtx, event)
	if err != nil {
		return nil, fmt.Errorf("构建指令上下文失败: %w", err)
	}
	actualMatched := commandregistry.MatchCommandHandler(ctx.GetArgs())
	matched, ok := commandregistry.LookupCommandHandler(matchedCommand)
	if shouldPreferMoreSpecificMessageMatch(ctx.GetArgs(), matched, ok, actualMatched) {
		logger.Warnf("bot command corrected matched_command from %s to %s", matchedCommand, actualMatched.Command)
		matched = actualMatched
		ok = true
		expectedPath = actualMatched.Handler.GetPath()
	}
	if !ok || matched.Handler == nil || matched.Handler.IsDisabled() || matched.Handler.GetPath() == "" {
		if actualMatched.Handler == nil || actualMatched.Handler.IsDisabled() {
			if !ok || matched.Handler == nil || matched.Handler.IsDisabled() {
				return nil, &botValidationError{msg: fmt.Sprintf("matched_command 未注册: %s", matchedCommand)}
			}
			return nil, &botValidationError{msg: fmt.Sprintf("matched_command 未开放给 Bot API: %s", matchedCommand)}
		}
		matched = actualMatched
	}

	if matched.Handler.GetPath() != expectedPath {
		return nil, &botValidationError{
			msg:        fmt.Sprintf("matched_command 属于接口路径 %s", matched.Handler.GetPath()),
			actualPath: matched.Handler.GetPath(),
		}
	}

	args, ok := commandregistry.ExtractCommandArgs(ctx.GetArgs(), matched.Command)
	triggerCmd := matched.Command
	if !ok {
		actualMatched := commandregistry.MatchCommandHandler(ctx.GetArgs())
		if actualMatched.Handler == nil || actualMatched.Handler.IsDisabled() {
			return nil, &botValidationError{msg: fmt.Sprintf("message 与 matched_command 不匹配: %s", matchedCommand)}
		}
		if actualMatched.Handler.GetPath() != matched.Handler.GetPath() {
			return nil, &botValidationError{msg: fmt.Sprintf("message 与 matched_command 不匹配: %s", matchedCommand)}
		}
		args = strings.TrimSpace(string(actualMatched.ArgText))
		triggerCmd = actualMatched.Command
	}

	ctx.TriggerCmd = triggerCmd
	ctx.ArgText = args
	ctx.MessageType = messageType
	executable, ok := matched.Handler.(commandhandler.CommandHandler)
	if !ok {
		return nil, fmt.Errorf("注册的指令处理器未实现 PJSK 指令接口: %T", matched.Handler)
	}
	resolved, err := executable.Handle(ctx)
	if err != nil {
		return nil, err
	}
	if resolved == nil {
		return nil, fmt.Errorf("指令处理器返回空结果")
	}
	resolved.RequesterPlatform = req.Platform
	resolved.RequesterUserID = req.PlatformUserID
	resolved.RequesterGroupID = req.PlatformGroupID
	resolved.RequesterBotID = strings.TrimSpace(botID)
	return resolved, nil
}

func shouldPreferMoreSpecificMessageMatch(message string, matched commandregistry.MatchedHandler, lookupOK bool, actual commandregistry.MatchedHandler) bool {
	if !lookupOK || matched.Handler == nil || matched.Handler.IsDisabled() {
		return false
	}
	if actual.Handler == nil || actual.Handler.IsDisabled() || actual.Command == "" || matched.Command == "" {
		return false
	}
	if actual.Command == matched.Command {
		return false
	}

	providedPrefixLength, ok := commandregistry.MatchCommandPrefix(message, matched.Command)
	if !ok {
		_, related := commandregistry.MatchCommandPrefix(matched.Command, actual.Command)
		return related
	}
	if actual.PrefixLength <= providedPrefixLength {
		return false
	}

	actualCommandPrefixLength, ok := commandregistry.MatchCommandPrefix(actual.Command, matched.Command)
	if !ok {
		return false
	}
	return actualCommandPrefixLength < len([]rune(actual.Command))
}

// ---------------------------------------------------------------------------
// Manifest
// ---------------------------------------------------------------------------

// buildManifestHandler returns a handler for GET /api/v2/bot/:botId/command/manifests.
// When botDBClient is non-nil it queries the command_manifests table and returns
// the full manifest ordered by priority descending.
// When botDBClient is nil it returns a 501 Not Implemented response (test / no-DB mode).
func buildManifestHandler(botDBClient *botDB.Client) fiber.Handler {
	return func(c fiber.Ctx) error {
		if botDBClient == nil {
			return api.JSONResponse(c, fiber.StatusNotImplemented,
				"指令清单不可用：bot 数据库未配置", nil)
		}

		rows, err := botDBClient.CommandManifest.Query().
			Order(commandmanifest.ByCommandPriority(sql.OrderDesc())).
			All(c.Context())
		if err != nil {
			return api.JSONResponse(c, fiber.StatusInternalServerError, "加载指令清单失败", nil)
		}

		entries := make([]ManifestEntry, 0, len(rows))
		for _, r := range rows {
			entries = append(entries, ManifestEntry{
				CommandPrefixes:         r.CommandPrefixes,
				CommandPriority:         r.CommandPriority,
				CommandMode:             r.CommandMode,
				CommandModule:           r.CommandModule,
				CommandPath:             r.CommandPath,
				CommandAdditionalParams: r.CommandAdditionalParams,
			})
		}
		return api.JSONResponse(c, fiber.StatusOK, api.ResponseOK, ManifestResponse{
			Entries:                   entries,
			CurrentHarukiCloudVersion: version.Get(),
			LatestHarukiClientVersion: harukiConfig.Cfg.Backend.LatestHarukiClientVersion,
			Profile:                   string(harukiConfig.Cfg.Profile),
		})
	}
}
