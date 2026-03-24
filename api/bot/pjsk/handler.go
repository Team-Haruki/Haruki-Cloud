package pjsk

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"haruki-cloud/api"
	botDB "haruki-cloud/database/bot"
	"haruki-cloud/database/bot/commandmanifest"
	commandhandler "haruki-cloud/internal/pjsk/handler"
	sekaihandler "haruki-cloud/internal/pjsk/handler/sekai"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"strings"

	"entgo.io/ent/dialect/sql"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	zeromessage "github.com/wdvxdr1123/ZeroBot/message"

	"haruki-cloud/api/bot/onebot11"
)

const botRouteBase = "/api/v2/bot"

const (
	botQueryCommandPayload   = "command_payload"
	botHeaderPlatform        = "X-Haruki-Bot-Platform"
	botHeaderPlatformUserID  = "X-Haruki-Bot-Platform-User-Id"
	botHeaderPlatformGroupID = "X-Haruki-Bot-Platform-Group-Id"
	botHeaderPJSKServer      = "X-Haruki-Bot-Pjsk-Server"
	botHeaderMatchedCommand  = "X-Haruki-Bot-Matched-Command"
)

// RegisterPJSKBotRoutes registers per-feature bot endpoints under
//
//	/api/v2/bot/:botId/pjsk/<path>
//
// and the command manifest endpoint at
//
//	GET /api/v2/bot/:botId/command/manifests
//
// The canonical PJSK bot protocol is:
//
//	GET /api/v2/bot/:botId/pjsk/<path>?command_payload=<base64(onebot-v11-payload)>
//
// with metadata carried in headers such as X-Haruki-Bot-Matched-Command and
// X-Haruki-Bot-Pjsk-Server.
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
	}
}

// makeBotHandler returns a GET-only fiber.Handler that validates the matched
// command header belongs to the current endpoint path, then lets the registered
// handler parse the original OneBot payload and produce a resolved render command.
func makeBotHandler(renderApp *renderapp.App, expectedPath string) fiber.Handler {
	return func(c fiber.Ctx) error {
		req, err := parseBotRequest(c)
		if err != nil {
			return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
		}
		if req.CommandPayload == "" {
			return api.JSONResponse(c, fiber.StatusBadRequest, "command_payload is required")
		}
		if req.MatchedCommand == "" {
			return api.JSONResponse(c, fiber.StatusBadRequest, "X-Haruki-Bot-Matched-Command is required")
		}

		message, err := decodeCommand(req.CommandPayload)
		if err != nil {
			return api.JSONResponse(c, fiber.StatusBadRequest, "failed to decode command", BotCommandErrorResponse{
				Error: err.Error(),
			})
		}

		resolved, err := resolveBotCommand(message, expectedPath, req)
		if err != nil {
			var validationErr *botValidationError
			if errors.As(err, &validationErr) {
				return api.JSONResponse(c, fiber.StatusBadRequest, "command does not match this endpoint",
					BotCommandErrorResponse{
						Error:          err.Error(),
						ExpectedPath:   expectedPath,
						MatchedCommand: req.MatchedCommand,
					})
			}
			return api.JSONResponse(c, fiber.StatusBadRequest, err.Error(), BotCommandErrorResponse{
				Error:          err.Error(),
				ExpectedPath:   expectedPath,
				MatchedCommand: req.MatchedCommand,
			})
		}

		if req.Server != "" {
			resolved.Region = req.Server
		}

		responseData, dataType, err := commandhandler.Execute(context.Background(), resolved, renderApp)
		if err != nil {
			return api.JSONResponse(c, fiber.StatusInternalServerError, "render failed", BotCommandErrorResponse{
				Error: err.Error(),
				Mode:  resolved.Mode,
			})
		}

		switch dataType {
		case commandhandler.CommandResultDataTypeImagePNG:
			c.Set("Content-Type", string(dataType))
			return c.Send(responseData)
		case commandhandler.CommandResultDataTypeImageURL:
			return api.JSONResponse(c, fiber.StatusOK, "ok", onebot11.Image(string(responseData)))
		case commandhandler.CommandResultDataTypeText:
			return api.JSONResponse(c, fiber.StatusOK, "ok", onebot11.Text(string(responseData)))
		default:
			return api.JSONResponse(c, fiber.StatusInternalServerError, "unsupported command result", BotCommandErrorResponse{
				Error: fmt.Sprintf("execute returned unsupported data type %q", dataType),
			})
		}
	}
}

// parseBotRequest reads BotCommandRequest from the canonical GET query + header protocol.
func parseBotRequest(c fiber.Ctx) (BotCommandRequest, error) {
	return BotCommandRequest{
		Platform:        strings.TrimSpace(c.Get(botHeaderPlatform)),
		PlatformUserID:  strings.TrimSpace(c.Get(botHeaderPlatformUserID)),
		PlatformGroupID: strings.TrimSpace(c.Get(botHeaderPlatformGroupID)),
		CommandPayload:  c.Query(botQueryCommandPayload),
		MatchedCommand:  strings.TrimSpace(c.Get(botHeaderMatchedCommand)),
		Server:          strings.TrimSpace(c.Get(botHeaderPJSKServer)),
	}, nil
}

type botValidationError struct {
	msg string
}

func (e *botValidationError) Error() string {
	return e.msg
}

func resolveBotCommand(message []zeromessage.Segment, expectedPath string, req BotCommandRequest) (*parser.ResolvedCommand, error) {
	matchedCommand := req.MatchedCommand
	matched := commandhandler.MatchCommandHandler(matchedCommand)
	if matched.Handler == nil || matched.Handler.IsDisabled() {
		return nil, &botValidationError{msg: fmt.Sprintf("matched_command is not registered: %s", matchedCommand)}
	}

	if remaining, ok := commandhandler.ExtractCommandArgs(matchedCommand, matched.Command); !ok || remaining != "" {
		return nil, &botValidationError{msg: fmt.Sprintf("matched_command is not an exact registered command: %s", matchedCommand)}
	}

	if matched.Handler.GetPath() == "" {
		return nil, &botValidationError{msg: fmt.Sprintf("matched_command is not exposed by the bot api: %s", matchedCommand)}
	}

	if matched.Handler.GetPath() != expectedPath {
		return nil, &botValidationError{msg: fmt.Sprintf("matched_command belongs to path %s", matched.Handler.GetPath())}
	}

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

	ctx, err := commandhandler.BuildContext(context.Background(), event, matchedCommand)
	if err != nil {
		return nil, fmt.Errorf("failed to build handler context: %w", err)
	}
	args, ok := commandhandler.ExtractCommandArgs(ctx.ArgText, matchedCommand)
	if !ok {
		return nil, &botValidationError{msg: "original command does not start with matched_command"}
	}

	ctx.ArgText = args
	ctx.MessageType = messageType
	result, err := matched.Handler.Handle(ctx)
	if err != nil {
		return nil, err
	}
	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		return nil, fmt.Errorf("handler returned %T", result)
	}
	return resolved, nil
}

// ---------------------------------------------------------------------------
// Base64 + OneBot JSON payload decoding
// ---------------------------------------------------------------------------

// decodeCommand decodes the command payload parameter.
// Expected format: Base64-encoded OneBot JSON payload → extract message segments.
// Falls back to treating the input as a single plain text segment if decoding fails.
func decodeCommand(raw string) ([]zeromessage.Segment, error) {
	decoded, ok := tryBase64Decode(raw)
	if !ok {
		// Not valid Base64 — treat raw input as plain text
		return []zeromessage.Segment{{Type: "text", Data: map[string]string{"text": raw}}}, nil
	}

	// Try to extract command text from OneBot JSON
	text, err := extractOneBotMessage(decoded)
	if err == nil {
		return text, nil
	}

	// Base64 decoded successfully but not valid OneBot JSON — treat as plain text
	return []zeromessage.Segment{{Type: "text", Data: map[string]string{"text": raw}}}, nil
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

// extractOneBotText parses a OneBot v11 JSON payload and returns a canonical
// command string. It prefers message segment arrays so "at" segments can be
// normalized into "@qq", then falls back to raw_message or message strings.
func extractOneBotMessage(data []byte) ([]zeromessage.Segment, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}

	// Prefer message segment arrays because they retain structured mention data.
	if msgRaw, ok := payload["message"]; ok {
		var segments []zeromessage.Segment
		if err := json.Unmarshal(msgRaw, &segments); err == nil && len(segments) > 0 {
			return segments, nil
		}

		// message as plain string (some OneBot implementations)
		var msgStr string
		if err := json.Unmarshal(msgRaw, &msgStr); err == nil && msgStr != "" {
			return []zeromessage.Segment{{Type: "text", Data: map[string]string{"text": msgStr}}}, nil
		}
	}

	// Fallback to raw_message when segment arrays are unavailable.
	if rawMsg, ok := payload["raw_message"]; ok {
		var text string
		if err := json.Unmarshal(rawMsg, &text); err == nil && text != "" {
			return []zeromessage.Segment{{Type: "text", Data: map[string]string{"text": text}}}, nil
		}
	}

	return nil, fmt.Errorf("no command text found in OneBot payload")
}

func flattenOneBotSegments(segments []zeromessage.Segment) string {
	var sb strings.Builder
	for _, seg := range segments {
		switch seg.Type {
		case "text":
			sb.WriteString(seg.Data["text"])
		case "at":
			qq := strings.TrimSpace(seg.Data["qq"])
			if qq == "" {
				continue
			}
			sb.WriteByte('@')
			sb.WriteString(qq)
		}
	}
	return sb.String()
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
