package pjsk

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"haruki-cloud/api"
	botauth "haruki-cloud/api/bot/auth"
	harukiConfig "haruki-cloud/config"
	botDB "haruki-cloud/database/bot"
	"haruki-cloud/database/bot/commandmanifest"
	botuser "haruki-cloud/database/bot/user"
	"haruki-cloud/internal/cluster"
	"haruki-cloud/internal/core/crypto"
	commandregistry "haruki-cloud/internal/handler"
	"haruki-cloud/internal/middleware/secure"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	commandhandler "haruki-cloud/internal/pjsk/handler"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/utils/logger"
	"haruki-cloud/version"

	"entgo.io/ent/dialect/sql"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"github.com/shamaton/msgpack/v3"
	json "haruki-cloud/internal/jsonutil"
)

const botRouteBase = "/api/v2/bot"

// BotRouteDispatchers owns request-path background work registered with the
// PJSK routes. Close is idempotent and flushes the Redis guard cleanup before
// command analytics, so dedup leases are not left behind during shutdown.
type BotRouteDispatchers struct {
	guard     *RequestGuard
	election  *ResponseElectionCoordinator
	telemetry *botauth.CommandTelemetryDispatcher
}

func (d *BotRouteDispatchers) Close() {
	if d == nil {
		return
	}
	if d.election != nil {
		d.election.Close()
	}
	if d.guard != nil {
		d.guard.Close()
	}
	if d.telemetry != nil {
		d.telemetry.Close()
	}
}

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
// registered command manifest routes on startup and the manifest endpoint returns
// live data from the database.
// Pass nil to keep the placeholder response (e.g. in unit tests).
func RegisterPJSKBotRoutes(app *fiber.App, renderApp *renderapp.App, redisClient *redis.Client, botDBClient *botDB.Client, noiseKeyPair *crypto.KeyPair) *BotRouteDispatchers {
	return RegisterPJSKBotRoutesWithContext(context.Background(), app, renderApp, redisClient, botDBClient, noiseKeyPair)
}

func RegisterPJSKBotRoutesWithContext(initCtx context.Context, app *fiber.App, renderApp *renderapp.App, redisClient *redis.Client, botDBClient *botDB.Client, noiseKeyPair *crypto.KeyPair) *BotRouteDispatchers {
	if renderApp == nil {
		return nil
	}
	if initCtx == nil {
		initCtx = context.Background()
	}

	commandhandler.EnsureCommandHandlersRegistered()

	if botDBClient != nil && !cluster.IsReadOnly() {
		if err := SeedCommandManifests(initCtx, botDBClient); err != nil {
			// Non-fatal: manifest table seed failure should not block startup.
			logger.Warn("bot manifest seed failed", "error_type", fmt.Sprintf("%T", err))
		}
	}

	var sessionMiddleware fiber.Handler
	if redisClient != nil {
		sessionMiddleware = api.VerifyBotSession(redisClient)
	} else {
		sessionMiddleware = api.VerifyBotSessionTestBypass()
	}
	botMiddleware := []any{commandTraceMiddleware, sessionMiddleware}
	if botDBClient != nil && renderApp.BanChecker != nil {
		botMiddleware = append(botMiddleware, verifyBotOwnerNotBanned(botDBClient, renderApp.BanChecker))
	}
	bot := app.Group(botRouteBase+"/:botId", botMiddleware...)

	preview3DEnabled := renderApp.Config.Preview3D.Enabled
	bot.Get("/command/manifests", buildManifestHandler(botDBClient, preview3DEnabled))

	guard := NewRequestGuard(redisClient)
	replay := newReplayGuard(
		redisClient,
		harukiConfig.Cfg.HarukiBotDB.RequestNonceWindow,
		harukiConfig.Cfg.HarukiBotDB.RequireRequestNonce,
	)
	election := NewResponseElectionCoordinator(
		logger.DetachedContext(initCtx),
		redisClient,
		harukiConfig.Cfg.HarukiBotDB.ResponseElectionWindow,
	)
	if harukiConfig.Cfg.HarukiBotDB.ResponseElectionRoster {
		election = election.WithRoster(newResponseElectionRoster(redisClient))
	}
	var commandElection commandResponseElection
	if election != nil {
		commandElection = election
	} else if guard != nil {
		commandElection = &requestGuardResponseElection{guard: guard}
	}
	telemetry := botauth.NewCommandTelemetryDispatcher(botDBClient)

	pjsk := bot.Group("/pjsk")
	if noiseKeyPair != nil {
		pjsk.Use(secure.New(secure.Config{ServerPrivateKey: noiseKeyPair}))
	}
	registerBirthdayMonitorRoutes(pjsk, app, renderApp, guard)
	routes := commandregistry.ListBotRoutes()
	hasMysekaiBlueprintRoute := false
	for _, route := range routes {
		if !botRouteEnabled(route.Path, preview3DEnabled) {
			continue
		}
		h := makeBotHandler(renderApp, commandElection, telemetry, replay, route.Path, route.Commands)
		path := "/" + route.Path
		pjsk.Post(path, h)
		if route.Path == "mysekai/blueprint" {
			hasMysekaiBlueprintRoute = true
		}
	}
	if !hasMysekaiBlueprintRoute {
		// Keep the retired endpoint alive so older manifests can continue posting
		// /msb until they refresh to the canonical mysekai/talk-list path.
		pjsk.Post("/mysekai/blueprint", makeBotHandler(renderApp, commandElection, telemetry, replay, "mysekai/blueprint", nil))
	}
	return &BotRouteDispatchers{guard: guard, election: election, telemetry: telemetry}
}

func verifyBotOwnerNotBanned(botDBClient *botDB.Client, checker *accountdata.BanService) fiber.Handler {
	return func(c fiber.Ctx) error {
		botID, err := strconv.Atoi(strings.TrimSpace(c.Params("botId")))
		if err != nil {
			return api.JSONResponse(c, fiber.StatusUnauthorized, "Bot 会话无效")
		}
		owner, err := botDBClient.User.Query().
			Where(botuser.BotIDEQ(botID)).
			Only(c.Context())
		if err != nil {
			if botDB.IsNotFound(err) {
				return api.JSONResponse(c, fiber.StatusUnauthorized, "Bot 会话无效")
			}
			return api.InternalError(c)
		}
		banned, err := checker.IsGloballyBanned(c.Context(), "qq", strconv.FormatInt(owner.OwnerUserID, 10))
		if err != nil {
			return api.InternalError(c)
		}
		if banned {
			return api.JSONResponse(c, fiber.StatusForbidden, botauth.ErrOwnerBanned)
		}
		return c.Next()
	}
}

func botRouteEnabled(path string, preview3DEnabled bool) bool {
	return preview3DEnabled || !strings.HasPrefix(path, "costume/")
}

// makeBotHandler returns a POST-only fiber.Handler that validates the matched
// command field belongs to the current endpoint path, then lets the registered
// handler parse the OneBot message segments and produce a resolved render command.
func makeBotHandler(renderApp *renderapp.App, election commandResponseElection, telemetry *botauth.CommandTelemetryDispatcher, replay *replayGuard, expectedPath string, commands []string) fiber.Handler {
	return func(c fiber.Ctx) error {
		setCommandTraceMetadata(c, "", expectedPath)
		req, err := parseBotRequest(c)
		if err != nil {
			setCommandTraceOutcome(c, "rejected", err)
			return botResponse(c, fiber.StatusBadRequest, "请求格式错误")
		}
		requestCtx := c.Context()
		traceCommand := allowedCommandTraceLabel(req.MatchedCommand, commands)
		setCommandTraceMetadata(c, traceCommand, expectedPath)
		finishValidation := commandtrace.MeasurePhase(requestCtx, "request_validate")
		if len(req.Message) == 0 {
			finishValidation()
			setCommandTraceOutcome(c, "rejected", nil)
			return botResponse(c, fiber.StatusBadRequest, "缺少 message")
		}
		if req.MatchedCommand == "" {
			finishValidation()
			setCommandTraceOutcome(c, "rejected", nil)
			return botResponse(c, fiber.StatusBadRequest, "缺少 matched_command")
		}
		allowCompatReroute := allowBotCompatReroute(expectedPath)
		if !slices.Contains(commands, req.MatchedCommand) && !allowCompatReroute {
			finishValidation()
			setCommandTraceOutcome(c, "rejected", nil)
			return botResponse(c, fiber.StatusBadRequest, "当前接口不允许使用该 matched_command")
		}
		finishValidation()
		if !replay.allow(requestCtx, req) {
			// A stale or replayed request is dropped exactly like a dedup drop:
			// empty OK, indistinguishable to the sender.
			setCommandTraceOutcome(c, "replayed", nil)
			return botResponse(c, fiber.StatusOK, api.ResponseOK, make(onebot11.Message, 0))
		}
		botID := strings.Clone(strings.TrimSpace(c.Params("botId")))
		electionRequest := responseElectionRequest{Request: req, BotID: botID}
		finishElection := commandtrace.MeasurePhase(requestCtx, "response_election")
		decision := coordinateCommandResponse(requestCtx, election, electionRequest, func(executionCtx context.Context) sharedCommandResult {
			return executeSharedBotCommand(executionCtx, renderApp, expectedPath, traceCommand, allowCompatReroute, req, botID)
		})
		finishElection()
		logResponseElectionDecision(requestCtx, electionRequest, decision)
		mergeSharedCommandOperations(requestCtx, decision.result.Operations)
		if !decision.visible {
			if decision.reason == "publish_unknown" {
				setCommandTraceOutcome(c, "error", nil)
				c.Locals(traceErrorTypeKey, "response_election_publish_unknown")
				commandtrace.SetErrorType(requestCtx, "response_election_publish_unknown")
			} else {
				setCommandTraceOutcome(c, "deduplicated", nil)
			}
			return botResponse(c, fiber.StatusOK, api.ResponseOK, make(onebot11.Message, 0))
		}

		metadata := decision.result.Metadata
		setCommandTraceMetadata(c, metadata.Command, metadata.CommandPath)
		setResolvedCommandTraceMetadata(c, metadata.Module, metadata.Mode, metadata.Region)
		setCommandTraceOutcome(c, metadata.Outcome, nil)
		if metadata.ErrorType != "" {
			c.Locals(traceErrorTypeKey, metadata.ErrorType)
			commandtrace.SetErrorType(requestCtx, metadata.ErrorType)
		}

		if telemetry != nil {
			finishTelemetry := commandtrace.MeasurePhase(requestCtx, "telemetry_enqueue")
			entry := botauth.CommandLogEntry{
				Platform: req.Platform,
				PID:      botID,
				GID:      req.PlatformGroupID,
				UID:      req.PlatformUserID,
				Command:  metadata.Command,
			}
			if !telemetry.Enqueue(requestCtx, fiber.Params[int](c, "botId", 0), entry) {
				logger.WarnContext(requestCtx, "bot command telemetry queue unavailable",
					"command_path", metadata.CommandPath,
					"command", metadata.Command,
				)
			}
			finishTelemetry()
		}
		return writeEncodedBotResponse(c, decision.result.Response)
	}
}

func executeSharedBotCommand(
	ctx context.Context,
	renderApp *renderapp.App,
	expectedPath string,
	traceCommand string,
	allowCompatReroute bool,
	req BotCommandRequest,
	executorBotID string,
) sharedCommandResult {
	metadata := sharedCommandMetadata{
		Command:       traceCommand,
		CommandPath:   expectedPath,
		Outcome:       "error",
		ExecutorBotID: executorBotID,
	}
	finishResolve := commandtrace.MeasurePhase(ctx, "command_resolve")
	resolved, err := resolveBotCommand(ctx, req.Message, expectedPath, req, executorBotID)
	if err != nil && allowCompatReroute {
		if validationErr, ok := errors.AsType[*botValidationError](err); ok && canRetryBotPathCompat(expectedPath, validationErr.actualPath) {
			logger.WarnContext(ctx, "bot command compatibility reroute",
				"command", traceCommand,
				"expected_command_path", expectedPath,
				"actual_command_path", validationErr.actualPath,
			)
			resolved, err = resolveBotCommand(ctx, req.Message, validationErr.actualPath, req, executorBotID)
		}
	}
	finishResolve()
	if err != nil {
		if isExpectedCommandError(err) {
			metadata.Outcome = "rejected"
		} else {
			logger.ErrorContext(ctx, "bot command resolve failed",
				"command_path", expectedPath,
				"command", traceCommand,
				"error_type", fmt.Sprintf("%T", err),
			)
		}
		metadata.ErrorType = fmt.Sprintf("%T", err)
		return encodeSharedCommandResult(ctx, commandErrorEnvelope(err, expectedPath, req.MatchedCommand, req.EnableParamEcho), metadata, false)
	}

	metadata.Command = resolved.TriggerCommand
	metadata.CommandPath = resolved.CommandPath
	metadata.Module = fmt.Sprint(resolved.Module)
	metadata.Mode = resolved.Mode
	metadata.Region = resolved.Region
	if region, ok := explicitRegionFromBotRequest(req); ok {
		resolved.Region = region
		resolved.RegionExplicit = true
		syncExplicitRegionToProfileParams(resolved, region)
	} else if server := strings.TrimSpace(req.Server); server != "" && !resolved.RegionExplicit {
		if normalized := renderregion.Normalize(server); !normalized.IsZero() {
			resolved.Region = normalized.String()
			resolved.RegionExplicit = true
			syncExplicitRegionToProfileParams(resolved, normalized.String())
		} else {
			resolved.Region = server
		}
	}

	responseData, err := commandhandler.ExecuteCommandRequest(ctx, resolved, renderApp)
	metadata.Region = resolved.Region
	forceExecutor := commandResponseDependsOnExecutor(resolved)
	if err != nil {
		if isExpectedCommandError(err) {
			metadata.Outcome = "rejected"
		} else {
			logger.ErrorContext(ctx, "bot command execution failed",
				"command_path", resolved.CommandPath,
				"command", resolved.TriggerCommand,
				"error_type", fmt.Sprintf("%T", err),
			)
		}
		metadata.ErrorType = fmt.Sprintf("%T", err)
		return encodeSharedCommandResult(ctx, commandErrorEnvelope(err, resolved.CommandPath, resolved.TriggerCommand, req.EnableParamEcho), metadata, forceExecutor)
	}
	metadata.Outcome = "ok"
	return encodeSharedCommandResult(ctx, newBotResponseEnvelope(fiber.StatusOK, api.ResponseOK, responseData), metadata, forceExecutor)
}

func encodeSharedCommandResult(ctx context.Context, envelope botResponseEnvelope, metadata sharedCommandMetadata, forceExecutor bool) sharedCommandResult {
	finishEncode := commandtrace.MeasureOperation(ctx, "response_payload_encode")
	defer finishEncode()
	encoded, err := encodeBotResponseEnvelope(envelope)
	if err == nil {
		return sharedCommandResult{Response: encoded, Metadata: metadata, ForceExecutor: forceExecutor}
	}
	logger.Error("bot response encoding failed", "error_type", fmt.Sprintf("%T", err))
	fallback, fallbackErr := encodeBotResponseEnvelope(newBotResponseEnvelope(fiber.StatusInternalServerError, api.ErrInternalServer))
	if fallbackErr != nil {
		return sharedCommandResult{Metadata: sharedCommandMetadata{Outcome: "error", ErrorType: fmt.Sprintf("%T", fallbackErr)}}
	}
	metadata.Outcome = "error"
	metadata.ErrorType = fmt.Sprintf("%T", err)
	return sharedCommandResult{Response: fallback, Metadata: metadata, ForceExecutor: forceExecutor}
}

func commandErrorEnvelope(err error, expectedPath, matchedCommand string, enableParamEcho bool) botResponseEnvelope {
	if _, ok := errors.AsType[*botValidationError](err); ok {
		return newBotResponseEnvelope(fiber.StatusBadRequest, "指令与当前接口不匹配",
			BotCommandErrorResponse{
				Error:          err.Error(),
				ExpectedPath:   expectedPath,
				MatchedCommand: matchedCommand,
			})
	}
	if replyErr, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return newBotResponseEnvelope(fiber.StatusOK, api.ResponseOK,
			[]onebot11.Segment{onebot11.Text(clientErrorTextForCommand(string(replyErr), enableParamEcho, matchedCommand, expectedPath))})
	}
	return newBotResponseEnvelope(fiber.StatusOK, api.ResponseOK,
		[]onebot11.Segment{onebot11.Text(clientErrorTextForCommand(err.Error(), enableParamEcho, matchedCommand, expectedPath))})
}

func commandResponseDependsOnExecutor(resolved *commandhandler.CommandRequest) bool {
	if resolved == nil || !strings.EqualFold(strings.TrimSpace(resolved.Region), "cn") {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(resolved.Mode))
	return mode == "mysekai" ||
		strings.HasPrefix(mode, "mysekai-") && mode != "mysekai-housing-sk"
}

func allowedCommandTraceLabel(candidate string, allowed []string) string {
	candidate = strings.TrimSpace(candidate)
	for _, command := range allowed {
		if candidate == command {
			return command
		}
	}
	return "<invalid>"
}

func isExpectedCommandError(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := errors.AsType[*botValidationError](err); ok {
		return true
	}
	_, ok := errors.AsType[onebot11.ReplayError](err)
	return ok
}

func syncExplicitRegionToProfileParams(resolved *commandhandler.CommandRequest, region string) {
	if resolved == nil {
		return
	}
	normalized := renderregion.Normalize(region)
	if normalized.IsZero() {
		return
	}
	switch resolved.Mode {
	case accountdata.ProfileModeBindList,
		accountdata.ProfileModeBindSwap,
		accountdata.ProfileModeUnbind,
		accountdata.ProfileModeDefaultSet,
		accountdata.ProfileModeDefaultClear,
		accountdata.ProfileModeQueryUID:
		syncExplicitRegionToProfileBindingParams(resolved, normalized)
	case accountdata.ProfileModeHideID,
		accountdata.ProfileModeShowID,
		accountdata.ProfileModeHideSuite,
		accountdata.ProfileModeShowSuite,
		accountdata.ProfileModeHideMySekai,
		accountdata.ProfileModeShowMySekai,
		accountdata.ProfileModeVerify,
		accountdata.ProfileModeVerifyList,
		accountdata.ProfileModeEnableModular,
		accountdata.ProfileModeDisableModular,
		accountdata.ProfileModeBGUpload,
		accountdata.ProfileModeBGClear,
		accountdata.ProfileModeBGAdjust:
		syncExplicitRegionToProfileSettingsParams(resolved, normalized)
	default:
		return
	}
}

func syncExplicitRegionToProfileBindingParams(resolved *commandhandler.CommandRequest, normalized renderregion.Value) {
	params, err := accountdata.DecodeProfileBindingParams(resolved.Params)
	if err != nil {
		return
	}
	params.Server = normalized.String()
	switch resolved.Mode {
	case accountdata.ProfileModeDefaultSet, accountdata.ProfileModeDefaultClear:
		params.Scope = normalized.String()
	}
	if data, err := json.Marshal(params); err == nil {
		resolved.Params = data
	}
}

func syncExplicitRegionToProfileSettingsParams(resolved *commandhandler.CommandRequest, normalized renderregion.Value) {
	params, err := accountdata.DecodeProfileSettingsParams(resolved.Params)
	if err != nil {
		return
	}
	params.Server = normalized.String()
	params.RegionExplicit = true
	if data, err := json.Marshal(params); err == nil {
		resolved.Params = data
	}
}

// parseBotRequest binds BotCommandRequest from the POST body.
// When the request arrived through the Noise IK middleware, the Content-Type is
// application/msgpack and the body is decoded with MsgPack.
// Otherwise the standard JSON binding is used.
func parseBotRequest(c fiber.Ctx) (BotCommandRequest, error) {
	finish := commandtrace.MeasurePhase(c.Context(), "request_decode")
	defer finish()
	var req BotCommandRequest
	ct := string(c.Request().Header.ContentType())
	if strings.Contains(ct, "msgpack") {
		if err := msgpack.Unmarshal(c.Body(), &req); err != nil {
			return BotCommandRequest{}, err
		}
	} else if err := c.Bind().Body(&req); err != nil {
		return BotCommandRequest{}, err
	}
	req.Message = onebot11.ParseMessage(req.Message)
	return detachBotCommandRequest(req), nil
}

func detachBotCommandRequest(req BotCommandRequest) BotCommandRequest {
	req.Platform = strings.Clone(req.Platform)
	req.PlatformUserID = strings.Clone(req.PlatformUserID)
	req.PlatformGroupID = strings.Clone(req.PlatformGroupID)
	req.SelfID = strings.Clone(req.SelfID)
	req.Server = strings.Clone(req.Server)
	req.MatchedCommand = strings.Clone(req.MatchedCommand)
	message := make(onebot11.Message, len(req.Message))
	for index, segment := range req.Message {
		message[index].Type = strings.Clone(segment.Type)
		switch data := segment.Data.(type) {
		case onebot11.TextData:
			message[index].Data = onebot11.TextData{Text: strings.Clone(data.Text)}
		case onebot11.ImageData:
			message[index].Data = onebot11.ImageData{
				File: strings.Clone(data.File),
				Url:  strings.Clone(data.Url),
			}
		case onebot11.AtData:
			message[index].Data = onebot11.AtData{QQ: strings.Clone(data.QQ)}
		default:
			message[index].Data = data
		}
	}
	req.Message = message
	return req
}

// botResponse sends a response using MsgPack when the request came through the
// Noise IK transport layer, and JSON otherwise.
func botResponse(c fiber.Ctx, status int, message string, data ...any) error {
	if c.Locals("secure_noise") != nil {
		return api.MsgPackResponse(c, status, message, data...)
	}
	return api.JSONResponse(c, status, message, data...)
}

func explicitRegionFromBotRequest(req BotCommandRequest) (string, bool) {
	candidates := []string{
		strings.TrimSpace(req.MatchedCommand),
		extractBotCommandText(req.Message),
	}
	for _, candidate := range candidates {
		if region, ok := explicitRegionFromCommandText(candidate); ok {
			return region, true
		}
	}
	return "", false
}

func explicitRegionFromCommandText(text string) (string, bool) {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return "", false
	}
	for _, region := range []renderregion.Value{
		renderregion.JP,
		renderregion.CN,
		renderregion.TW,
		renderregion.KR,
		renderregion.EN,
	} {
		prefix := "/" + region.String()
		if strings.HasPrefix(text, prefix) {
			return region.String(), true
		}
	}
	return "", false
}

func extractBotCommandText(message onebot11.Message) string {
	var builder strings.Builder
	for _, seg := range message {
		if seg.Type != onebot11.TypeText {
			continue
		}
		switch data := seg.Data.(type) {
		case onebot11.TextData:
			builder.WriteString(data.Text)
		case map[string]any:
			if raw, ok := data[onebot11.KeyText]; ok {
				builder.WriteString(fmt.Sprint(raw))
			}
		case map[string]string:
			builder.WriteString(data[onebot11.KeyText])
		}
	}
	return strings.TrimSpace(builder.String())
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
		logger.WarnContext(requestCtx, "bot command matched route corrected",
			"corrected_command", actualMatched.Command,
		)
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
func buildManifestHandler(botDBClient *botDB.Client, preview3DEnabled bool) fiber.Handler {
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

		clientPolicyScopes := commandManifestClientPolicyScopes()
		entries := make([]ManifestEntry, 0, len(rows))
		for _, r := range rows {
			if !botRouteEnabled(r.CommandPath, preview3DEnabled) {
				continue
			}
			entries = append(entries, ManifestEntry{
				CommandPrefixes:         r.CommandPrefixes,
				CommandPriority:         r.CommandPriority,
				CommandMode:             r.CommandMode,
				CommandModule:           r.CommandModule,
				CommandPath:             r.CommandPath,
				CommandAdditionalParams: r.CommandAdditionalParams,
				ClientPolicyScope:       clientPolicyScopes[manifestKey(r.CommandModule, r.CommandPath)],
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
