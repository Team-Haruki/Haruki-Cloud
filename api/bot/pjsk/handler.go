package pjsk

import (
	"context"
	"errors"
	"fmt"
	"haruki-cloud/api"
	onebot11 "haruki-cloud/api/bot/onebot11"
	botDB "haruki-cloud/database/bot"
	"haruki-cloud/database/bot/commandmanifest"
	"haruki-cloud/internal/core/crypto"
	"haruki-cloud/internal/middleware/secure"
	commandhandler "haruki-cloud/internal/pjsk/handler"
	sekaihandler "haruki-cloud/internal/pjsk/handler/sekai"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/utils/logger"
	"regexp"
	"slices"
	"strings"

	"entgo.io/ent/dialect/sql"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"github.com/shamaton/msgpack/v3"
)

const botRouteBase = "/api/v2/bot"

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
//	 "matched_command":"/cmd","message":[{"type":"text","data":{"text":"/cmd args"}}]}
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

	sekaihandler.EnsureCommandHandlersRegistered(nil)

	if botDBClient != nil {
		if err := SeedCommandManifests(initCtx, botDBClient); err != nil {
			// Non-fatal: manifest table seed failure should not block startup.
			logger.Warnf("bot manifest seed failed: %v", err)
		}
	}

	bot := app.Group(botRouteBase+"/:botId", api.VerifyBotSession(redisClient))

	bot.Get("/command/manifests", buildManifestHandler(botDBClient))

	pjsk := bot.Group("/pjsk")
	if noiseKeyPair != nil {
		pjsk.Use(secure.New(secure.Config{ServerPrivateKey: noiseKeyPair}))
	}
	for _, route := range commandhandler.ListBotRoutes() {
		h := makeBotHandler(renderApp, route.Path, route.Commands)
		path := "/" + route.Path
		pjsk.Post(path, h)
	}
}

// makeBotHandler returns a POST-only fiber.Handler that validates the matched
// command field belongs to the current endpoint path, then lets the registered
// handler parse the OneBot message segments and produce a resolved render command.
func makeBotHandler(renderApp *renderapp.App, expectedPath string, commands []string) fiber.Handler {
	return func(c fiber.Ctx) error {
		req, err := parseBotRequest(c)
		if err != nil {
			return botResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
		}
		if len(req.Message) == 0 {
			return botResponse(c, fiber.StatusBadRequest, "message is required")
		}
		if req.MatchedCommand == "" {
			return botResponse(c, fiber.StatusBadRequest, "matched_command is required")
		}
		// Backward compatibility: /skp moved from sk/rank-trace to sk/predict.
		// Some clients may still post /skp to the old endpoint until manifests refresh.
		legacySKPredictCompat := expectedPath == "sk/rank-trace"
		if !slices.Contains(commands, req.MatchedCommand) && !legacySKPredictCompat {
			return botResponse(c, fiber.StatusBadRequest, "matched command is not allowed for this endpoint")
		}

		resolved, err := resolveBotCommand(c.Context(), req.Message, expectedPath, req)
		if err != nil && legacySKPredictCompat {
			var validationErr *botValidationError
			if errors.As(err, &validationErr) && strings.Contains(validationErr.Error(), "belongs to path sk/predict") {
				resolved, err = resolveBotCommand(c.Context(), req.Message, "sk/predict", req)
			}
		}
		if err != nil {
			var validationErr *botValidationError
			if errors.As(err, &validationErr) {
				return botResponse(c, fiber.StatusBadRequest, "command does not match this endpoint",
					BotCommandErrorResponse{
						Error:          err.Error(),
						ExpectedPath:   expectedPath,
						MatchedCommand: req.MatchedCommand,
					})
			}
			var replyErr onebot11.ReplayError
			if errors.As(err, &replyErr) {
				return botResponse(c, fiber.StatusOK, "ok",
					[]onebot11.Segment{onebot11.Text(string(replyErr))},
				)
			}
			logger.Warnf("bot command resolve failed: path=%s matched_command=%s err=%v", expectedPath, req.MatchedCommand, err)
			return botResponse(c, fiber.StatusOK, "ok",
				[]onebot11.Segment{onebot11.Text(err.Error())},
			)
		}

		if req.Server != "" && !resolved.RegionExplicit {
			resolved.Region = req.Server
		}

		responseData, err := commandhandler.Execute(c.Context(), resolved, renderApp)
		if err != nil {
			var replyErr onebot11.ReplayError
			if errors.As(err, &replyErr) {
				return botResponse(c, fiber.StatusOK, "ok",
					[]onebot11.Segment{onebot11.Text(string(replyErr))},
				)
			}
			logger.Errorf("bot command render failed: mode=%s matched_command=%s err=%v", resolved.Mode, req.MatchedCommand, err)
			return botResponse(c, fiber.StatusOK, "ok",
				[]onebot11.Segment{onebot11.Text(err.Error())},
			)
		}
		return botResponse(c, fiber.StatusOK, "ok", responseData)
	}
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
	logger.Infof("before parse: %+v", req.Message)
	req.Message = parseOnebot11Message(req.Message)
	logger.Infof("after parse: %+v", req.Message)
	return req, nil
}

func parseOnebot11Message(msg onebot11.Message) onebot11.Message {
	segs := make([]onebot11.Segment, 0, len(msg))
	for _, s := range msg {
		data := make(map[string]string)
		switch d := s.Data.(type) {
		case map[string]interface{}:
			for k, v := range d {
				data[k] = fmt.Sprint(v)
			}
		case map[interface{}]interface{}:
			for k, v := range d {
				ks, _ := k.(string)
				vs := fmt.Sprint(v)
				if ks != "" {
					data[ks] = vs
				}
			}
		case map[string]string:
			data = d
		}
		switch s.Type {
		case onebot11.TYPE_TEXT:
			segs = append(segs, onebot11.Text(stripInlineCQTags(data[onebot11.KEY_TEXT])))
		case onebot11.TYPE_IMAGE:
			segs = append(segs, onebot11.Image(data[onebot11.KEY_FILE], data[onebot11.KEY_URL]))
		case onebot11.TYPE_AT:
			segs = append(segs, onebot11.At(data[onebot11.KEY_QQ]))
		}
	}
	return segs
}

var inlineCQPattern = regexp.MustCompile(`(?i)\[cq:[^\]]+\]`)

func stripInlineCQTags(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	return inlineCQPattern.ReplaceAllString(text, " ")
}

// botResponse sends a response using MsgPack when the request came through the
// Noise IK transport layer, and JSON otherwise.
func botResponse(c fiber.Ctx, status int, message string, data ...interface{}) error {
	if c.Locals("secure_noise") != nil {
		return api.MsgPackResponse(c, status, message, data...)
	}
	return api.JSONResponse(c, status, message, data...)
}

type botValidationError struct {
	msg string
}

func (e *botValidationError) Error() string {
	return e.msg
}

func resolveBotCommand(requestCtx context.Context, message onebot11.Message, expectedPath string, req BotCommandRequest) (*parser.ResolvedCommand, error) {

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
		return nil, fmt.Errorf("failed to build handler context: %w", err)
	}
	matched := commandhandler.MatchCommandHandler(ctx.GetArgs())
	if matched.Handler == nil || matched.Handler.IsDisabled() {
		return nil, &botValidationError{msg: fmt.Sprintf("matched_command is not registered: %s", matchedCommand)}
	}
	if matched.Handler.GetPath() == "" {
		return nil, &botValidationError{msg: fmt.Sprintf("matched_command is not exposed by the bot api: %s", matchedCommand)}
	}

	if matched.Handler.GetPath() != expectedPath {
		return nil, &botValidationError{msg: fmt.Sprintf("matched_command belongs to path %s", matched.Handler.GetPath())}
	}

	ctx.TriggerCmd = matched.Command
	ctx.ArgText = strings.TrimSpace(string(matched.ArgText))
	ctx.MessageType = messageType
	result, err := matched.Handler.Handle(ctx)
	if err != nil {
		return nil, err
	}
	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		return nil, fmt.Errorf("handler returned %T", result)
	}
	resolved.RequesterPlatform = req.Platform
	resolved.RequesterUserID = req.PlatformUserID
	resolved.RequesterGroupID = req.PlatformGroupID
	return resolved, nil
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
				"command manifests not available — bot DB not configured", nil)
		}

		rows, err := botDBClient.CommandManifest.Query().
			Order(commandmanifest.ByCommandPriority(sql.OrderDesc())).
			All(c.Context())
		if err != nil {
			return api.JSONResponse(c, fiber.StatusInternalServerError, "failed to load manifests", nil)
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
		return api.JSONResponse(c, fiber.StatusOK, "ok", ManifestResponse{Entries: entries})
	}
}
