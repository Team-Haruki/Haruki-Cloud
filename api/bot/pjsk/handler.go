package pjsk

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"haruki-cloud/api"
	botauth "haruki-cloud/api/bot/auth"
	harukiConfig "haruki-cloud/config"
	botDB "haruki-cloud/database/bot"
	"haruki-cloud/database/bot/commandmanifest"
	botuser "haruki-cloud/database/bot/user"
	"haruki-cloud/internal/cluster"
	"haruki-cloud/internal/core/crypto"
	"haruki-cloud/internal/core/secevent"
	"haruki-cloud/internal/core/trustsign"
	commandregistry "haruki-cloud/internal/handler"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/middleware/secure"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	commandhandler "haruki-cloud/internal/pjsk/handler"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/utils/logger"
	"haruki-cloud/utils/usererror"
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

// BotRouteOptions carries the optional security dependencies of the bot routes.
type BotRouteOptions struct {
	// NoiseKeys enables the Noise NK transport middleware on the /pjsk group.
	// Every key in the ring is accepted so keys can rotate.
	NoiseKeys *crypto.KeyRing
	// ManifestSigner, when set, wraps the command manifest in a signed
	// trustsign.Envelope (domain haruki-cloud/manifest/v1, JSON payload).
	ManifestSigner *trustsign.Signer
	// SessionPolicy re-checks live sessions against build-policy revocations.
	SessionPolicy api.SessionPolicy
	// Security receives replay and revocation events.
	Security secevent.Reporter
}

// RegisterPJSKBotRoutes registers per-feature bot endpoints under
//
//	/api/v2/bot/:botId/pjsk/<path>
//
// and the command manifest endpoint at
//
//	GET /api/v2/bot/:botId/command/manifests
//
// The canonical PJSK bot protocol is POST + body:
//
//	POST /api/v2/bot/:botId/pjsk/<path>
//
//	{"platform":"qq","platform_user_id":"12345","server":"jp",
//	 "matched_command":"/cmd","message":[{"type":"text","data":{"text":"/cmd args"}}],
//	 "session_token":"<jwt>","timestamp":1700000000,"nonce":"<hex>"}
//
// When NoiseKeys is set, the Noise NK transport middleware is applied to the
// pjsk route group: clients send Noise NK Message 1 containing a MsgPack-encoded
// body and receive Noise NK Message 2 containing a MsgPack-encoded response.
//
// Session authentication differs per route family:
//   - /pjsk POST routes read session_token from the decrypted body
//     (verifyBotSessionFromPayload), so the token never leaves the ciphertext.
//   - GET /command/manifests has no body and is not behind Noise; it keeps the
//     X-Haruki-Bot-Id / X-Haruki-Bot-Session-Token headers (api.VerifyBotSession).
//
// Pass nil for redisClient in unit tests (session auth is skipped).
//
// When botDBClient is non-nil, the manifest table is synchronized from the
// registered command manifest routes on startup and the manifest endpoint returns
// live data from the database.
// Pass nil to keep the placeholder response (e.g. in unit tests).
func RegisterPJSKBotRoutes(app *fiber.App, renderApp *renderapp.App, redisClient *redis.Client, botDBClient *botDB.Client, noiseKeys *crypto.KeyRing) *BotRouteDispatchers {
	return RegisterPJSKBotRoutesWithOptions(context.Background(), app, renderApp, redisClient, botDBClient, BotRouteOptions{NoiseKeys: noiseKeys})
}

func RegisterPJSKBotRoutesWithContext(initCtx context.Context, app *fiber.App, renderApp *renderapp.App, redisClient *redis.Client, botDBClient *botDB.Client, noiseKeys *crypto.KeyRing) *BotRouteDispatchers {
	return RegisterPJSKBotRoutesWithOptions(initCtx, app, renderApp, redisClient, botDBClient, BotRouteOptions{NoiseKeys: noiseKeys})
}

func RegisterPJSKBotRoutesWithOptions(initCtx context.Context, app *fiber.App, renderApp *renderapp.App, redisClient *redis.Client, botDBClient *botDB.Client, opts BotRouteOptions) *BotRouteDispatchers {
	if renderApp == nil {
		return nil
	}
	if initCtx == nil {
		initCtx = context.Background()
	}

	commandhandler.EnsureCommandHandlersRegistered()
	seedBotCommandManifests(initCtx, botDBClient)
	bot := app.Group(botRouteBase+"/:botId", commandTraceMiddleware)
	ownerGuard := botOwnerGuardMiddleware(renderApp, botDBClient)

	preview3DEnabled := renderApp.Config.Preview3D.Enabled
	manifestChain := append(ownerGuard, buildManifestHandler(botDBClient, preview3DEnabled, opts.ManifestSigner))
	bot.Get("/command/manifests", headerSessionMiddleware(redisClient, opts.SessionPolicy, opts.Security), manifestChain...)

	guard := NewRequestGuard(redisClient)
	replay := newReplayGuard(
		redisClient,
		harukiConfig.Cfg.HarukiBotDB.RequestNonceWindow,
		!harukiConfig.Cfg.HarukiBotDB.AllowRequestsWithoutNonce,
		opts.Security,
	)
	election, commandElection := newBotCommandElection(initCtx, redisClient, guard)
	telemetry := botauth.NewCommandTelemetryDispatcher(botDBClient)

	pjsk := bot.Group("/pjsk")
	if opts.NoiseKeys != nil {
		pjsk.Use(secure.New(secure.Config{KeyRing: opts.NoiseKeys}))
	}
	// Session and owner checks run after Noise so the token is read from the
	// decrypted body and rejections are encrypted on the way out.
	pjsk.Use(verifyBotSessionFromPayload(redisClient, opts.SessionPolicy, opts.Security))
	for _, guard := range ownerGuard {
		pjsk.Use(guard)
	}
	registerBirthdayMonitorRoutes(pjsk, app, renderApp, guard)
	if !registerBotCommandRoutes(pjsk, renderApp, commandElection, telemetry, replay, preview3DEnabled) {
		// Keep the retired endpoint alive so older manifests can continue posting
		// /msb until they refresh to the canonical mysekai/talk-list path.
		pjsk.Post("/mysekai/blueprint", makeBotHandler(renderApp, commandElection, telemetry, replay, "mysekai/blueprint", nil))
	}
	return &BotRouteDispatchers{guard: guard, election: election, telemetry: telemetry}
}

func seedBotCommandManifests(ctx context.Context, client *botDB.Client) {
	if client == nil || cluster.IsReadOnly() {
		return
	}
	if err := SeedCommandManifests(ctx, client); err != nil {
		// Non-fatal: manifest table seed failure should not block startup.
		logger.Warn("bot manifest seed failed", "error_type", fmt.Sprintf("%T", err))
	}
}

// headerSessionMiddleware authenticates body-less routes (the manifest GET)
// from the X-Haruki-Bot-Id / X-Haruki-Bot-Session-Token headers.
func headerSessionMiddleware(redisClient *redis.Client, policy api.SessionPolicy, reporter secevent.Reporter) fiber.Handler {
	if redisClient == nil {
		return api.VerifyBotSessionTestBypass()
	}
	return api.VerifyBotSessionWithPolicy(redisClient, policy, reporter)
}

// botOwnerGuardMiddleware rejects bots whose owner is globally banned. It is
// empty when the bot database or ban checker is unavailable (unit tests).
func botOwnerGuardMiddleware(renderApp *renderapp.App, botDBClient *botDB.Client) []any {
	if botDBClient == nil || renderApp.BanChecker == nil {
		return nil
	}
	return []any{verifyBotOwnerNotBanned(botDBClient, renderApp.BanChecker)}
}

func newBotCommandElection(initCtx context.Context, redisClient *redis.Client, guard *RequestGuard) (*ResponseElectionCoordinator, commandResponseElection) {
	election := NewResponseElectionCoordinator(
		logger.DetachedContext(initCtx),
		redisClient,
		harukiConfig.Cfg.HarukiBotDB.ResponseElectionWindow,
	)
	if harukiConfig.Cfg.HarukiBotDB.ResponseElectionRoster {
		election = election.WithRoster(newResponseElectionRoster(redisClient))
	}
	if election != nil {
		return election, election
	}
	if guard != nil {
		return nil, &requestGuardResponseElection{guard: guard}
	}
	return nil, nil
}

func registerBotCommandRoutes(
	router fiber.Router,
	renderApp *renderapp.App,
	election commandResponseElection,
	telemetry *botauth.CommandTelemetryDispatcher,
	replay *replayGuard,
	preview3DEnabled bool,
) bool {
	hasMysekaiBlueprintRoute := false
	for _, route := range commandregistry.ListBotRoutes() {
		if !botRouteEnabled(route.Path, preview3DEnabled) {
			continue
		}
		router.Post("/"+route.Path, makeBotHandler(renderApp, election, telemetry, replay, route.Path, route.Commands))
		hasMysekaiBlueprintRoute = hasMysekaiBlueprintRoute || route.Path == "mysekai/blueprint"
	}
	return hasMysekaiBlueprintRoute
}

func verifyBotOwnerNotBanned(botDBClient *botDB.Client, checker *accountdata.BanService) fiber.Handler {
	return func(c fiber.Ctx) error {
		finish := commandtrace.MeasureOperation(c.Context(), "request.owner_check")
		defer finish()
		botID, err := strconv.Atoi(strings.TrimSpace(c.Params("botId")))
		if err != nil {
			return botResponse(c, fiber.StatusUnauthorized, i18n.T("account.api.bot_session_invalid"))
		}
		owner, err := botDBClient.User.Query().
			Where(botuser.BotIDEQ(botID)).
			Only(c.Context())
		if err != nil {
			if botDB.IsNotFound(err) {
				return botResponse(c, fiber.StatusUnauthorized, i18n.T("account.api.bot_session_invalid"))
			}
			return api.InternalError(c)
		}
		banned, err := checker.IsGloballyBanned(c.Context(), "qq", strconv.FormatInt(owner.OwnerUserID, 10))
		if err != nil {
			return api.InternalError(c)
		}
		if banned {
			return botResponse(c, fiber.StatusForbidden, botauth.ErrOwnerBanned)
		}
		finish()
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
		req, traceCommand, allowCompatReroute, rejection := validateBotHandlerRequest(c, expectedPath, commands)
		if rejection != nil {
			setCommandTraceOutcome(c, "rejected", rejection.cause)
			return botResponse(c, fiber.StatusBadRequest, rejection.message)
		}
		requestCtx := c.Context()
		if !replay.allow(requestCtx, c.Params("botId"), req) {
			// A stale or replayed request is dropped exactly like a dedup drop:
			// empty OK, indistinguishable to the sender.
			setCommandTraceOutcome(c, "replayed", nil)
			return botResponse(c, fiber.StatusOK, api.ResponseOK, make(onebot11.Message, 0))
		}
		botID := strings.Clone(strings.TrimSpace(c.Params("botId")))
		decision := runBotResponseElection(requestCtx, renderApp, election, expectedPath, traceCommand, allowCompatReroute, req, botID)
		if !decision.visible {
			return writeInvisibleBotDecision(c, decision)
		}

		metadata := decision.result.Metadata
		applySharedBotCommandMetadata(c, metadata)
		enqueueBotCommandTelemetry(c, telemetry, req, botID, metadata)
		return writeEncodedBotResponse(c, decision.result.responseFor(req))
	}
}

// responseFor is the reply for req: the echo variant only when its client
// enabled parameter echo and the result has one.
func (r sharedCommandResult) responseFor(req BotCommandRequest) encodedBotResponse {
	if req.EnableParamEcho && len(r.EchoResponse.JSONBody) > 0 {
		return r.EchoResponse
	}
	return r.Response
}

type botHandlerRejection struct {
	message string
	cause   error
}

func validateBotHandlerRequest(c fiber.Ctx, expectedPath string, commands []string) (BotCommandRequest, string, bool, *botHandlerRejection) {
	setCommandTraceMetadata(c, "", expectedPath)
	req, err := parseBotRequest(c)
	if err != nil {
		return BotCommandRequest{}, "", false, &botHandlerRejection{message: i18n.T("account.api.request_invalid"), cause: err}
	}
	traceCommand := allowedCommandTraceLabel(req.MatchedCommand, commands)
	setCommandTraceMetadata(c, traceCommand, expectedPath)
	finishValidation := commandtrace.MeasurePhase(c.Context(), "request_validate")
	defer finishValidation()
	if len(req.Message) == 0 {
		return req, traceCommand, false, &botHandlerRejection{message: i18n.T("account.api.message_missing")}
	}
	if req.MatchedCommand == "" {
		return req, traceCommand, false, &botHandlerRejection{message: i18n.T("account.api.matched_command_missing")}
	}
	allowCompatReroute := allowBotCompatReroute(expectedPath)
	if !slices.Contains(commands, req.MatchedCommand) && !allowCompatReroute {
		return req, traceCommand, false, &botHandlerRejection{message: i18n.T("account.api.matched_command_not_allowed")}
	}
	return req, traceCommand, allowCompatReroute, nil
}

func runBotResponseElection(
	ctx context.Context,
	renderApp *renderapp.App,
	election commandResponseElection,
	expectedPath, traceCommand string,
	allowCompatReroute bool,
	req BotCommandRequest,
	botID string,
) responseElectionDecision {
	electionRequest := responseElectionRequest{Request: req, BotID: botID}
	finishElection := commandtrace.MeasurePhase(ctx, "response_election")
	decision := coordinateCommandResponse(ctx, election, electionRequest, func(executionCtx context.Context) sharedCommandResult {
		return executeSharedBotCommand(executionCtx, renderApp, expectedPath, traceCommand, allowCompatReroute, req, botID)
	})
	finishElection()
	logResponseElectionDecision(ctx, electionRequest, decision)
	mergeSharedCommandOperations(ctx, decision.result.Operations)
	return decision
}

func writeInvisibleBotDecision(c fiber.Ctx, decision responseElectionDecision) error {
	if decision.reason == "publish_unknown" {
		setCommandTraceOutcome(c, "error", nil)
		c.Locals(traceErrorTypeKey, "response_election_publish_unknown")
		commandtrace.SetErrorType(c.Context(), "response_election_publish_unknown")
	} else {
		setCommandTraceOutcome(c, "deduplicated", nil)
	}
	return botResponse(c, fiber.StatusOK, api.ResponseOK, make(onebot11.Message, 0))
}

func applySharedBotCommandMetadata(c fiber.Ctx, metadata sharedCommandMetadata) {
	setCommandTraceMetadata(c, metadata.Command, metadata.CommandPath)
	setResolvedCommandTraceMetadata(c, metadata.Module, metadata.Mode, metadata.Region)
	setCommandTraceOutcome(c, metadata.Outcome, nil)
	if metadata.ErrorType != "" {
		c.Locals(traceErrorTypeKey, metadata.ErrorType)
		commandtrace.SetErrorType(c.Context(), metadata.ErrorType)
	}
	if metadata.ErrorMessage != "" {
		c.Locals(traceErrorMessageKey, metadata.ErrorMessage)
	}
}

// maxErrorChainDepth bounds error_chain; wrap chains in this code base are a
// few levels deep.
const maxErrorChainDepth = 8

// errorTypeChain lists the dynamic types along err's Unwrap chain (the first
// branch of a joined error), outermost first, e.g.
// "*fmt.wrapError > *pgconn.PgError".
func errorTypeChain(err error) string {
	types := make([]string, 0, 4)
	for depth := 0; err != nil && depth < maxErrorChainDepth; depth++ {
		types = append(types, fmt.Sprintf("%T", err))
		switch wrapped := err.(type) {
		case interface{ Unwrap() error }:
			err = wrapped.Unwrap()
		case interface{ Unwrap() []error }:
			if errs := wrapped.Unwrap(); len(errs) > 0 {
				err = errs[0]
			} else {
				err = nil
			}
		default:
			err = nil
		}
	}
	return strings.Join(types, " > ")
}

func enqueueBotCommandTelemetry(c fiber.Ctx, telemetry *botauth.CommandTelemetryDispatcher, req BotCommandRequest, botID string, metadata sharedCommandMetadata) {
	if telemetry == nil {
		return
	}
	ctx := c.Context()
	finishTelemetry := commandtrace.MeasurePhase(ctx, "telemetry_enqueue")
	defer finishTelemetry()
	entry := botauth.CommandLogEntry{
		Platform: req.Platform,
		PID:      botID,
		GID:      req.PlatformGroupID,
		UID:      req.PlatformUserID,
		Command:  metadata.Command,
	}
	if !telemetry.Enqueue(ctx, fiber.Params[int](c, "botId", 0), entry) {
		logger.WarnContext(ctx, "bot command telemetry queue unavailable",
			"command_path", metadata.CommandPath,
			"command", metadata.Command,
		)
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
	resolved, err := resolveBotCommandWithCompat(ctx, expectedPath, traceCommand, allowCompatReroute, req, executorBotID)
	finishResolve()
	if err != nil {
		return failedSharedBotCommand(ctx, err, expectedPath, traceCommand, req.MatchedCommand, metadata, false, "resolve")
	}

	metadata.Command = resolved.TriggerCommand
	metadata.CommandPath = resolved.CommandPath
	metadata.Module = fmt.Sprint(resolved.Module)
	metadata.Mode = resolved.Mode
	metadata.Region = resolved.Region
	applyBotRequestRegion(resolved, req)

	responseData, err := commandhandler.ExecuteCommandRequest(ctx, resolved, renderApp)
	metadata.Region = resolved.Region
	forceExecutor := commandResponseDependsOnExecutor(resolved)
	if err != nil {
		return failedSharedBotCommand(ctx, err, resolved.CommandPath, resolved.TriggerCommand, resolved.TriggerCommand, metadata, forceExecutor, "execution")
	}
	metadata.Outcome = "ok"
	return succeededSharedBotCommand(ctx, responseData, metadata, forceExecutor)
}

// succeededSharedBotCommand encodes a command's reply. Text built with
// onebot11.LocalizedText (a success reply that repeats unreviewed user
// input) is rendered twice: without echo for Response and with echo for
// EchoResponse, so the delivering bot can pick by its own request.
func succeededSharedBotCommand(ctx context.Context, responseData onebot11.Message, metadata sharedCommandMetadata, forceExecutor bool) sharedCommandResult {
	locale := i18n.LocaleFromContext(ctx)
	result := encodeSharedCommandResult(ctx, newBotResponseEnvelope(fiber.StatusOK, api.ResponseOK, responseData.Render(i18n.RenderOptions{Locale: locale, NoEcho: true})), metadata, forceExecutor)
	if !responseData.HasLocalizedText() || result.Metadata.Outcome != "ok" {
		return result
	}
	return withEchoVariant(ctx, result, newBotResponseEnvelope(fiber.StatusOK, api.ResponseOK, responseData.Render(i18n.RenderOptions{Locale: locale})))
}

// withEchoVariant adds echoEnvelope to result as the reply for clients that
// enabled parameter echo, when it differs from result's own reply (which is
// always the one without echo). A shared result keeps both because the bot
// that delivers it may not be the one that executed it.
func withEchoVariant(ctx context.Context, result sharedCommandResult, echoEnvelope botResponseEnvelope) sharedCommandResult {
	if len(result.Response.JSONBody) == 0 {
		return result
	}
	echo, err := encodeBotResponseEnvelopeContext(ctx, echoEnvelope)
	if err != nil || bytes.Equal(echo.JSONBody, result.Response.JSONBody) {
		return result
	}
	result.EchoResponse = echo
	return result
}

func resolveBotCommandWithCompat(
	ctx context.Context,
	expectedPath, traceCommand string,
	allowCompatReroute bool,
	req BotCommandRequest,
	executorBotID string,
) (*commandhandler.CommandRequest, error) {
	resolved, err := resolveBotCommand(ctx, req.Message, expectedPath, req, executorBotID)
	if err == nil || !allowCompatReroute {
		return resolved, err
	}
	validationErr, ok := errors.AsType[*botValidationError](err)
	if !ok || !canRetryBotPathCompat(expectedPath, validationErr.actualPath) {
		return resolved, err
	}
	logger.WarnContext(ctx, "bot command compatibility reroute",
		"command", traceCommand,
		"expected_command_path", expectedPath,
		"actual_command_path", validationErr.actualPath,
	)
	return resolveBotCommand(ctx, req.Message, validationErr.actualPath, req, executorBotID)
}

func applyBotRequestRegion(resolved *commandhandler.CommandRequest, req BotCommandRequest) {
	if region, ok := explicitRegionFromBotRequest(req); ok {
		resolved.Region = region
		resolved.RegionExplicit = true
		syncExplicitRegionToProfileParams(resolved, region)
		return
	}
	server := strings.TrimSpace(req.Server)
	if server == "" || resolved.RegionExplicit {
		return
	}
	normalized := renderregion.Normalize(server)
	if normalized.IsZero() {
		resolved.Region = server
		return
	}
	resolved.Region = normalized.String()
	resolved.RegionExplicit = true
	syncExplicitRegionToProfileParams(resolved, normalized.String())
}

func failedSharedBotCommand(
	ctx context.Context,
	err error,
	commandPath, command, matchedCommand string,
	metadata sharedCommandMetadata,
	forceExecutor bool,
	stage string,
) sharedCommandResult {
	if isExpectedCommandError(err) {
		metadata.Outcome = "rejected"
	} else {
		metadata.ErrorMessage = usererror.RedactForLog(usererror.LogText(err), usererror.DefaultLogMessageLimit)
		logger.ErrorContext(ctx, "bot command "+stage+" failed",
			"command_path", commandPath,
			"command", command,
			"region", metadata.Region,
			"error_type", fmt.Sprintf("%T", err),
			"error_chain", errorTypeChain(err),
			"error_message", metadata.ErrorMessage,
		)
	}
	metadata.ErrorType = fmt.Sprintf("%T", err)
	envelope := commandErrorEnvelope(i18n.WithParamEcho(ctx, false), err, commandPath, matchedCommand)
	echoEnvelope := commandErrorEnvelope(i18n.WithParamEcho(ctx, true), err, commandPath, matchedCommand)
	return withEchoVariant(ctx, encodeSharedCommandResult(ctx, envelope, metadata, forceExecutor), echoEnvelope)
}

func encodeSharedCommandResult(ctx context.Context, envelope botResponseEnvelope, metadata sharedCommandMetadata, forceExecutor bool) sharedCommandResult {
	finishEncode := commandtrace.MeasureOperation(ctx, "response_payload_encode")
	defer finishEncode()
	encoded, err := encodeBotResponseEnvelopeContext(ctx, envelope)
	if err == nil {
		return sharedCommandResult{Response: encoded, Metadata: metadata, ForceExecutor: forceExecutor}
	}
	logger.Error("bot response encoding failed", "error_type", fmt.Sprintf("%T", err))
	fallback, fallbackErr := encodeBotResponseEnvelopeContext(ctx, newBotResponseEnvelope(fiber.StatusInternalServerError, api.ErrInternalServer))
	if fallbackErr != nil {
		return sharedCommandResult{Metadata: sharedCommandMetadata{Outcome: "error", ErrorType: fmt.Sprintf("%T", fallbackErr)}}
	}
	metadata.Outcome = "error"
	metadata.ErrorType = fmt.Sprintf("%T", err)
	return sharedCommandResult{Response: fallback, Metadata: metadata, ForceExecutor: forceExecutor}
}

func commandErrorEnvelope(ctx context.Context, err error, expectedPath, matchedCommand string) botResponseEnvelope {
	if _, ok := errors.AsType[*botValidationError](err); ok {
		return newBotResponseEnvelope(fiber.StatusBadRequest, i18n.T("account.api.command_path_mismatch"),
			BotCommandErrorResponse{
				Error:          err.Error(),
				ExpectedPath:   expectedPath,
				MatchedCommand: matchedCommand,
			})
	}
	return newBotResponseEnvelope(fiber.StatusOK, api.ResponseOK,
		[]onebot11.Segment{onebot11.Text(commandErrorText(ctx, err, expectedPath, matchedCommand))})
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
	// Unparsable queries, unknown names and out-of-range indexes are the
	// user's input, not a failure of Cloud or its upstreams; typed errors
	// decide by their code.
	return usererror.IsExpected(err)
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
	finishDecode := commandtrace.MeasureOperation(c.Context(), "request.body_decode")
	defer finishDecode()
	ct := string(c.Request().Header.ContentType())
	if strings.Contains(ct, "msgpack") {
		if err := msgpack.Unmarshal(c.Body(), &req); err != nil {
			return BotCommandRequest{}, err
		}
	} else if err := c.Bind().Body(&req); err != nil {
		return BotCommandRequest{}, err
	}
	finishDecode()
	finishMessage := commandtrace.MeasureOperation(c.Context(), "request.message_parse")
	req.Message = onebot11.ParseMessage(req.Message)
	finishMessage()
	finishDetach := commandtrace.MeasureOperation(c.Context(), "request.detach")
	defer finishDetach()
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
	messageType := botCommandMessageType(req.PlatformGroupID)
	event := commandhandler.Event{
		Platform:    req.Platform,
		MessageType: messageType,
		Message:     message,
		UserId:      req.PlatformUserID,
		GroupId:     req.PlatformGroupID,
	}

	finishContext := commandtrace.MeasureOperation(requestCtx, "command.context_build")
	ctx, err := commandhandler.BuildContext(requestCtx, event)
	finishContext()
	if err != nil {
		return nil, fmt.Errorf("build command context: %w", err)
	}
	finishMatch := commandtrace.MeasureOperation(requestCtx, "command.match")
	defer finishMatch()
	matched, expectedPath, err := selectBotCommandMatch(requestCtx, ctx.GetArgs(), expectedPath, req.MatchedCommand)
	if err != nil {
		return nil, err
	}
	if matched.Handler.GetPath() != expectedPath {
		return nil, &botValidationError{
			msg:        fmt.Sprintf("matched_command belongs to path %s", matched.Handler.GetPath()),
			actualPath: matched.Handler.GetPath(),
		}
	}
	args, triggerCmd, err := resolveBotCommandArgs(ctx.GetArgs(), matched, req.MatchedCommand)
	if err != nil {
		return nil, err
	}

	finishMatch()
	ctx.TriggerCmd = triggerCmd
	ctx.ArgText = args
	ctx.MessageType = messageType
	executable, ok := matched.Handler.(commandhandler.CommandHandler)
	if !ok {
		return nil, fmt.Errorf("registered handler %T does not implement the PJSK command interface", matched.Handler)
	}
	finishParse := commandtrace.MeasureOperation(requestCtx, "command.parse")
	resolved, err := executable.Handle(ctx)
	finishParse()
	if err != nil {
		return nil, err
	}
	if resolved == nil {
		return nil, fmt.Errorf("command handler returned no command")
	}
	resolved.RequesterPlatform = req.Platform
	resolved.RequesterUserID = req.PlatformUserID
	resolved.RequesterGroupID = req.PlatformGroupID
	resolved.RequesterBotID = strings.TrimSpace(botID)
	return resolved, nil
}

func botCommandMessageType(groupID string) commandhandler.MessageType {
	if groupID != "" {
		return commandhandler.MessageTypeGroup
	}
	return commandhandler.MessageTypePrivate
}

func selectBotCommandMatch(requestCtx context.Context, message, expectedPath, matchedCommand string) (commandregistry.MatchedHandler, string, error) {
	actual := commandregistry.MatchCommandHandler(message)
	matched, ok := commandregistry.LookupCommandHandler(matchedCommand)
	if shouldPreferMoreSpecificMessageMatch(message, matched, ok, actual) {
		logger.WarnContext(requestCtx, "bot command matched route corrected", "corrected_command", actual.Command)
		return actual, actual.Handler.GetPath(), nil
	}
	if botCommandMatchOpen(matched, ok) {
		return matched, expectedPath, nil
	}
	fallback, err := fallbackBotCommandMatch(matched, actual, ok, matchedCommand)
	return fallback, expectedPath, err
}

func botCommandMatchOpen(matched commandregistry.MatchedHandler, lookupOK bool) bool {
	return lookupOK && matched.Handler != nil && !matched.Handler.IsDisabled() && matched.Handler.GetPath() != ""
}

func botCommandMatchRegistered(matched commandregistry.MatchedHandler, lookupOK bool) bool {
	return lookupOK && matched.Handler != nil && !matched.Handler.IsDisabled()
}

func fallbackBotCommandMatch(matched, actual commandregistry.MatchedHandler, lookupOK bool, matchedCommand string) (commandregistry.MatchedHandler, error) {
	if actual.Handler != nil && !actual.Handler.IsDisabled() {
		return actual, nil
	}
	if !botCommandMatchRegistered(matched, lookupOK) {
		return commandregistry.MatchedHandler{}, &botValidationError{msg: fmt.Sprintf("matched_command is not registered: %s", matchedCommand)}
	}
	return commandregistry.MatchedHandler{}, &botValidationError{msg: fmt.Sprintf("matched_command is not open to the bot API: %s", matchedCommand)}
}

func resolveBotCommandArgs(message string, matched commandregistry.MatchedHandler, matchedCommand string) (string, string, error) {
	args, ok := commandregistry.ExtractCommandArgs(message, matched.Command)
	if ok {
		return args, matched.Command, nil
	}
	actual := commandregistry.MatchCommandHandler(message)
	if actual.Handler == nil || actual.Handler.IsDisabled() || actual.Handler.GetPath() != matched.Handler.GetPath() {
		return "", "", &botValidationError{msg: fmt.Sprintf("message does not match matched_command: %s", matchedCommand)}
	}
	return strings.TrimSpace(string(actual.ArgText)), actual.Command, nil
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
//
// When signer is non-nil the manifest is returned as a trustsign.Envelope in
// the data field: the JSON bytes of ManifestResponse are the payload, signed
// under DomainManifest. Clients verify the signature over the raw payload bytes
// before decoding them.
func buildManifestHandler(botDBClient *botDB.Client, preview3DEnabled bool, signer *trustsign.Signer) fiber.Handler {
	cache := &manifestCache{}
	return func(c fiber.Ctx) error {
		if botDBClient == nil {
			return api.JSONResponse(c, fiber.StatusNotImplemented,
				"command manifest unavailable", nil)
		}
		entry, failure := cache.get(time.Now(), func() ([]byte, string) {
			return buildManifestPayload(c.Context(), botDBClient, preview3DEnabled)
		}, func(payload []byte) ([]byte, string) {
			return encodeManifestResponse(payload, signer)
		})
		if failure != "" {
			return api.JSONResponse(c, fiber.StatusInternalServerError, failure, nil)
		}
		c.Set(fiber.HeaderETag, entry.etag)
		c.Set(fiber.HeaderCacheControl, "no-cache")
		if manifestETagMatches(c.Get(fiber.HeaderIfNoneMatch), entry.etag) {
			return c.SendStatus(fiber.StatusNotModified)
		}
		c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSONCharsetUTF8)
		return c.Status(fiber.StatusOK).Send(entry.body)
	}
}

// manifestRefreshInterval bounds how stale a served manifest can be after
// the command_manifests table changes (a node seeding a new release).
const manifestRefreshInterval = 30 * time.Second

// manifestCache keeps the encoded (and, with a signer, signed) manifest
// response. Every manifestRefreshInterval the rows are re-read and the JSON
// payload rebuilt; signing and response encoding run again only when the
// payload bytes changed. Ed25519 signatures are deterministic, so a reused
// envelope is byte-identical to a freshly signed one.
type manifestCache struct {
	mu          sync.Mutex
	checkedAt   time.Time
	payloadHash [sha256.Size]byte
	body        []byte
	etag        string
}

type manifestCacheEntry struct {
	body []byte
	etag string
}

func (m *manifestCache) get(now time.Time, build func() ([]byte, string), encode func([]byte) ([]byte, string)) (manifestCacheEntry, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.body != nil && now.Sub(m.checkedAt) < manifestRefreshInterval {
		return manifestCacheEntry{body: m.body, etag: m.etag}, ""
	}
	payload, failure := build()
	if failure != "" {
		if m.body != nil {
			// Keep serving the last good manifest; the next request retries.
			return manifestCacheEntry{body: m.body, etag: m.etag}, ""
		}
		return manifestCacheEntry{}, failure
	}
	hash := sha256.Sum256(payload)
	if m.body != nil && hash == m.payloadHash {
		m.checkedAt = now
		return manifestCacheEntry{body: m.body, etag: m.etag}, ""
	}
	body, failure := encode(payload)
	if failure != "" {
		return manifestCacheEntry{}, failure
	}
	bodyHash := sha256.Sum256(body)
	m.body, m.payloadHash, m.checkedAt = body, hash, now
	m.etag = `"` + hex.EncodeToString(bodyHash[:16]) + `"`
	return manifestCacheEntry{body: m.body, etag: m.etag}, ""
}

// buildManifestPayload returns the manifest's JSON payload, or a failure
// message for the 500 response.
func buildManifestPayload(ctx context.Context, botDBClient *botDB.Client, preview3DEnabled bool) ([]byte, string) {
	rows, err := botDBClient.CommandManifest.Query().
		Order(commandmanifest.ByCommandPriority(sql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, "failed to load command manifest"
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
	manifest := ManifestResponse{
		Entries:                   entries,
		CurrentHarukiCloudVersion: version.Get(),
		LatestHarukiClientVersion: harukiConfig.Cfg.Backend.LatestHarukiClientVersion,
		Profile:                   string(harukiConfig.Cfg.Profile),
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		return nil, "failed to encode command manifest"
	}
	return payload, ""
}

// encodeManifestResponse wraps payload in the standard response envelope:
// as the manifest object itself, or as a trustsign.Envelope over the exact
// payload bytes when a signer is configured.
func encodeManifestResponse(payload []byte, signer *trustsign.Signer) ([]byte, string) {
	var data any = json.RawMessage(payload)
	if signer != nil {
		envelope, err := signer.Sign(trustsign.DomainManifest, trustsign.EncodingJSON, payload)
		if err != nil {
			return nil, "failed to sign command manifest"
		}
		data = envelope
	}
	body, err := json.Marshal(api.BuildResponseMap(fiber.StatusOK, api.ResponseOK, data))
	if err != nil {
		return nil, "failed to encode command manifest"
	}
	return body, ""
}

// manifestETagMatches implements If-None-Match for the manifest's strong
// ETag: "*" or any listed tag, weak or strong, matches.
func manifestETagMatches(header, etag string) bool {
	header = strings.TrimSpace(header)
	if header == "" || etag == "" {
		return false
	}
	if header == "*" {
		return true
	}
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if candidate == etag {
			return true
		}
	}
	return false
}
