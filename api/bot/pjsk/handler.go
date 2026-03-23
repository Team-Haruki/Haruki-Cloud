package pjsk

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"haruki-cloud/api"
	pjskHandler "haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

const botRouteBase = "/api/v2/bot"

// RegisterPJSKBotRoutes registers per-feature bot endpoints under
//
//	/api/v2/bot/:botId/pjsk/<module>/<mode>
//
// and a placeholder manifest at
//
//	GET /api/v2/bot/:botId/command/manifests
//
// When redisClient is non-nil, the api.VerifyBotSession middleware is applied to
// authenticate requests via X-Haruki-Bot-Id and X-Haruki-Bot-Session-Token headers.
// Pass nil for redisClient in unit tests (auth is skipped).
func RegisterPJSKBotRoutes(app *fiber.App, resolver *parser.GlobalCommandResolver, renderApp *renderapp.App, redisClient *redis.Client) {
	if resolver == nil || renderApp == nil {
		return
	}

	bot := app.Group(botRouteBase+"/:botId", api.VerifyBotSession(redisClient))

	// TODO: implement full command manifest — placeholder for now
	bot.Get("/command/manifests", buildManifestHandler())

	pjsk := bot.Group("/pjsk")
	for _, entry := range botModeTable {
		h := makeBotHandler(resolver, renderApp, entry.module, entry.mode)
		path := "/" + entry.path
		pjsk.Get(path, h)
		pjsk.Post(path, h)
	}
}

// makeBotHandler returns a fiber.Handler that decodes the command from
// a Base64-encoded OneBot JSON payload, validates the resolved module+mode
// matches the endpoint, then executes via the bridge.
func makeBotHandler(
	resolver *parser.GlobalCommandResolver,
	renderApp *renderapp.App,
	expectedModule parser.TargetModule,
	expectedMode string,
) fiber.Handler {
	return func(c fiber.Ctx) error {
		req, err := parseBotRequest(c)
		if err != nil {
			return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
		}
		if req.Command == "" {
			return api.JSONResponse(c, fiber.StatusBadRequest, "command is required")
		}

		commandText, err := decodeCommand(req.Command)
		if err != nil {
			return api.JSONResponse(c, fiber.StatusBadRequest, "failed to decode command", BotCommandErrorResponse{
				Error: err.Error(),
			})
		}

		resolved, err := resolver.Resolve(commandText)
		if err != nil {
			return api.JSONResponse(c, fiber.StatusBadRequest, "unrecognized command", BotCommandErrorResponse{
				Error: err.Error(),
			})
		}

		if resolved.Module != expectedModule || resolved.Mode != expectedMode {
			return api.JSONResponse(c, fiber.StatusBadRequest, "command does not match this endpoint",
				BotCommandErrorResponse{
					Error: fmt.Sprintf(
						"got module=%s mode=%s, endpoint expects module=%s mode=%s",
						moduleNameStr(resolved.Module), resolved.Mode,
						moduleNameStr(expectedModule), expectedMode,
					),
					ExpectedModule: moduleNameStr(expectedModule),
					ExpectedMode:   expectedMode,
				})
		}

		if req.Server != "" {
			resolved.Region = req.Server
		}

		pngBytes, err := pjskHandler.Execute(context.Background(), resolved, renderApp)
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
			IMPlatform: c.Query("im_platform"),
			IMUserID:   c.Query("im_user_id"),
			Command:    c.Query("command"),
			Server:     c.Query("server"),
		}, nil
	}
	var req BotCommandRequest
	if err := c.Bind().Body(&req); err != nil {
		return BotCommandRequest{}, err
	}
	return req, nil
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
// Manifest (TODO — placeholder)
// ---------------------------------------------------------------------------

// buildManifestHandler returns a placeholder handler for the command manifest endpoint.
// TODO: Implement full command manifests with Bot-side pre-matching support.
func buildManifestHandler() fiber.Handler {
	return func(c fiber.Ctx) error {
		return api.JSONResponse(c, fiber.StatusOK,
			"command manifests not yet implemented — placeholder", nil)
	}
}
