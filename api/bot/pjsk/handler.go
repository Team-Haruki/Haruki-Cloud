package pjsk

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"haruki-cloud/api"
	botDB "haruki-cloud/database/bot"
	"haruki-cloud/database/bot/commandmanifest"
	commandhandler "haruki-cloud/internal/pjsk/handler"
	sekaihandler "haruki-cloud/internal/pjsk/handler/sekai"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"

	"entgo.io/ent/dialect/sql"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
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
// When redisClient is non-nil, the api.VerifyBotSession middleware is applied to
// authenticate requests via X-Haruki-Bot-Id and X-Haruki-Bot-Session-Token headers.
// Pass nil for redisClient in unit tests (auth is skipped).
//
// When botDBClient is non-nil, the manifest table is synchronized from the
// registered handler routes on startup and the manifest endpoint returns live
// data from the database.
// Pass nil to keep the placeholder response (e.g. in unit tests).
func RegisterPJSKBotRoutes(app *fiber.App, renderApp *renderapp.App, redisClient *redis.Client, botDBClient *botDB.Client) {
	if renderApp == nil {
		return
	}

	sekaihandler.EnsureCommandHandlersRegistered(nil)

	if botDBClient != nil {
		if err := SeedCommandManifests(context.Background(), botDBClient); err != nil {
			// Non-fatal: manifest table seed failure should not block startup.
			_ = err
		}
	}

	bot := app.Group(botRouteBase+"/:botId", api.VerifyBotSession(redisClient))

	bot.Get("/command/manifests", buildManifestHandler(botDBClient))

	pjsk := bot.Group("/pjsk")
	for _, route := range commandhandler.ListBotRoutes() {
		h := makeBotHandler(renderApp, route.Path)
		path := "/" + route.Path
		pjsk.Get(path, h)
		pjsk.Post(path, h)
	}
}

// makeBotHandler returns a fiber.Handler that validates matched_command belongs
// to the current endpoint path, then lets the registered handler parse the
// original text and produce a resolved render command.
func makeBotHandler(renderApp *renderapp.App, expectedPath string) fiber.Handler {
	return func(c fiber.Ctx) error {
		req, err := parseBotRequest(c)
		if err != nil {
			return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
		}
		if req.Command == "" {
			return api.JSONResponse(c, fiber.StatusBadRequest, "command is required")
		}
		if req.MatchedCommand == "" {
			return api.JSONResponse(c, fiber.StatusBadRequest, "matched_command is required")
		}

		commandText, err := decodeCommand(req.Command)
		if err != nil {
			return api.JSONResponse(c, fiber.StatusBadRequest, "failed to decode command", BotCommandErrorResponse{
				Error: err.Error(),
			})
		}

		resolved, err := resolveBotCommand(commandText, req.MatchedCommand, expectedPath)
		if err != nil {
			return api.JSONResponse(c, fiber.StatusBadRequest, "command does not match this endpoint",
				BotCommandErrorResponse{
					Error:          err.Error(),
					ExpectedPath:   expectedPath,
					MatchedCommand: req.MatchedCommand,
				})
		}

		if req.Server != "" {
			resolved.Region = req.Server
		}

		pngBytes, err := commandhandler.Execute(context.Background(), resolved, renderApp)
		if err != nil {
			return api.JSONResponse(c, fiber.StatusInternalServerError, "render failed", BotCommandErrorResponse{
				Error: err.Error(),
				Mode:  resolved.Mode,
			})
		}

		c.Set("Content-Type", "image/png")
		return c.Send(pngBytes)
	}
}

// parseBotRequest reads BotCommandRequest from either GET query params or POST JSON body.
func parseBotRequest(c fiber.Ctx) (BotCommandRequest, error) {
	if c.Method() == fiber.MethodGet {
		return BotCommandRequest{
			IMPlatform:     c.Query("im_platform"),
			IMUserID:       c.Query("im_user_id"),
			Command:        c.Query("command"),
			MatchedCommand: c.Query("matched_command"),
			Server:         c.Query("server"),
		}, nil
	}
	var req BotCommandRequest
	if err := c.Bind().Body(&req); err != nil {
		return BotCommandRequest{}, err
	}
	return req, nil
}

func resolveBotCommand(commandText, matchedCommand, expectedPath string) (*parser.ResolvedCommand, error) {
	matched := commandhandler.MatchCommandHandler(matchedCommand)
	if matched.Handler == nil || matched.Handler.IsDisabled() {
		return nil, fmt.Errorf("matched_command is not registered: %s", matchedCommand)
	}

	if remaining, ok := commandhandler.ExtractCommandArgs(matchedCommand, matched.Command); !ok || remaining != "" {
		return nil, fmt.Errorf("matched_command is not an exact registered command: %s", matchedCommand)
	}

	if matched.Handler.GetPath() == "" {
		return nil, fmt.Errorf("matched_command is not exposed by the bot api: %s", matchedCommand)
	}

	if matched.Handler.GetPath() != expectedPath {
		return nil, fmt.Errorf("matched_command belongs to path %s", matched.Handler.GetPath())
	}

	args, ok := commandhandler.ExtractCommandArgs(commandText, matchedCommand)
	if !ok {
		return nil, fmt.Errorf("original command does not start with matched_command")
	}

	ctx := &commandhandler.HandlerContext{
		Context:     context.Background(),
		TriggerCmd:  matchedCommand,
		ArgText:     args,
		MessageType: commandhandler.MessageTypePrivate,
		Message:     commandText,
		Event: commandhandler.Event{
			MessageType: commandhandler.MessageTypePrivate,
			Message:     commandText,
		},
	}

	result, err := matched.Handler.Handle(ctx)
	if err != nil {
		return nil, err
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		return nil, fmt.Errorf("handler returned %T instead of *parser.ResolvedCommand", result)
	}
	return resolved, nil
}

// ---------------------------------------------------------------------------
// Base64 + OneBot JSON payload decoding
// ---------------------------------------------------------------------------

// decodeCommand decodes the command parameter.
// Expected format: Base64-encoded OneBot JSON payload → extract raw text command.
// Falls back to treating the input as a plain text command if decoding fails.
func decodeCommand(raw string) (string, error) {
	decoded, ok := tryBase64Decode(raw)
	if !ok {
		// Not valid Base64 — treat raw input as plain text
		return raw, nil
	}

	// Try to extract command text from OneBot JSON
	text, err := extractOneBotText(decoded)
	if err == nil {
		return text, nil
	}

	// Base64 decoded successfully but not valid OneBot JSON — treat as plain text
	return raw, nil
}

// tryBase64Decode attempts multiple Base64 variants (std/URL-safe, with/without padding).
func tryBase64Decode(s string) ([]byte, bool) {
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		if decoded, err := enc.DecodeString(s); err == nil {
			return decoded, true
		}
	}
	return nil, false
}

// extractOneBotText parses a OneBot v11 JSON payload and returns the text command.
// Tries raw_message (string) first, then message (array of segments), then message (string).
func extractOneBotText(data []byte) (string, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", err
	}

	// Prefer raw_message (OneBot v11 — plain text of the entire message)
	if rawMsg, ok := payload["raw_message"]; ok {
		var text string
		if err := json.Unmarshal(rawMsg, &text); err == nil && text != "" {
			return text, nil
		}
	}

	// Try message as array of segments (OneBot v11 rich message)
	if msgRaw, ok := payload["message"]; ok {
		var segments []oneBotSegment
		if err := json.Unmarshal(msgRaw, &segments); err == nil && len(segments) > 0 {
			var sb strings.Builder
			for _, seg := range segments {
				if seg.Type == "text" {
					sb.WriteString(seg.Data.Text)
				}
			}
			if sb.Len() > 0 {
				return sb.String(), nil
			}
		}

		// message as plain string (some OneBot implementations)
		var msgStr string
		if err := json.Unmarshal(msgRaw, &msgStr); err == nil && msgStr != "" {
			return msgStr, nil
		}
	}

	return "", fmt.Errorf("no command text found in OneBot payload")
}

// oneBotSegment represents a single message segment in OneBot v11 format.
type oneBotSegment struct {
	Type string            `json:"type"`
	Data oneBotSegmentData `json:"data"`
}

type oneBotSegmentData struct {
	Text string `json:"text"`
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
